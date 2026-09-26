package library

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// watchQuiet is how long a library must go without a new file event before it is scanned.
	watchQuiet = 2 * time.Second
	// watchMaxWait caps the quiet wait, so a folder that keeps changing still gets scanned.
	watchMaxWait = 30 * time.Second
	// watchSettle is how long a file written in place must go without a write before it counts as
	// done and gets probed.
	watchSettle = 10 * time.Second
	// RescanInterval is how often every library is scanned anyway. Events get missed: the host was
	// off, a network drive, the inotify limits.
	RescanInterval = time.Hour
)

// batcher decides when a library is due for a scan after file events. It does no IO: every call
// gets the time.
type batcher struct {
	quiet   time.Duration
	maxWait time.Duration
	settle  time.Duration
	changes map[int64]change // library → its changes not yet scanned
	writes  map[string]write // file → its latest write
}

type change struct{ first, last time.Time }

type write struct {
	lib int64
	at  time.Time
}

func newBatcher(quiet, maxWait, settle time.Duration) *batcher {
	return &batcher{quiet: quiet, maxWait: maxWait, settle: settle, changes: map[int64]change{},
		writes: map[string]write{}}
}

// changed notes a file or folder that appeared, went, or was renamed in a library.
func (b *batcher) changed(lib int64, now time.Time) {
	c, ok := b.changes[lib]
	if !ok {
		c.first = now
	}
	c.last = now
	b.changes[lib] = c
}

// wrote notes a write to a file. The file is growing until no write came for the settle time.
func (b *batcher) wrote(lib int64, file string, now time.Time) { b.writes[file] = write{lib, now} }

// gone forgets the writes to a file that was removed or renamed.
func (b *batcher) gone(file string) { delete(b.writes, file) }

// growing reports whether a file was written less than the settle time ago.
func (b *batcher) growing(file string, now time.Time) bool {
	w, ok := b.writes[file]
	return ok && now.Sub(w.at) < b.settle
}

// due returns the libraries to scan now: those quiet since their latest change or waiting since
// their first for maxWait, and those with a file that stopped growing. wait is how long until the
// next one is due; 0 when nothing is pending.
func (b *batcher) due(now time.Time) (libs []int64, wait time.Duration) {
	ready := map[int64]bool{}
	check := func(end time.Time) bool {
		if !now.Before(end) {
			return true
		}
		if d := end.Sub(now); wait == 0 || d < wait {
			wait = d
		}
		return false
	}
	for f, w := range b.writes {
		if check(w.at.Add(b.settle)) {
			delete(b.writes, f)
			ready[w.lib] = true
		}
	}
	for lib, c := range b.changes {
		end := c.last.Add(b.quiet)
		if capped := c.first.Add(b.maxWait); capped.Before(end) {
			end = capped
		}
		if check(end) {
			delete(b.changes, lib)
			ready[lib] = true
		}
	}
	if len(ready) == 0 {
		return nil, wait
	}
	return slices.Sorted(maps.Keys(ready)), wait
}

// Watcher watches every folder of every library and queues a scan of a library when its files
// change. inotify watches one folder at a time, so each folder gets its own watch. Scans add the
// watches as they walk; new folders are added as they appear.
type Watcher struct {
	scans *Scans
	fsw   *fsnotify.Watcher

	mu       sync.Mutex
	dirs     map[string]int64 // watched folder (real path) → its library
	batch    *batcher
	loggedNo bool // the "out of inotify watches" warning was logged
}

// NewWatcher returns a watcher that queues scans on scans. Run handles its events.
func NewWatcher(scans *Scans) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{scans: scans, fsw: fsw, dirs: map[string]int64{},
		batch: newBatcher(watchQuiet, watchMaxWait, watchSettle)}, nil
}

// Run handles file events until ctx is cancelled, then closes the watcher.
func (w *Watcher) Run(ctx context.Context) {
	defer func() { _ = w.fsw.Close() }()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case ev := <-w.fsw.Events:
			w.handle(ev)
		case err := <-w.fsw.Errors:
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				slog.Warn("watch: events lost, rescanning every library")
				if err := w.scans.RequestAll(ctx); err != nil && ctx.Err() == nil {
					slog.Error("watch: rescan", "err", err)
				}
			} else {
				slog.Warn("watch", "err", err)
			}
		case <-timer.C:
		}

		w.mu.Lock()
		libs, wait := w.batch.due(time.Now())
		w.mu.Unlock()
		if len(libs) > 0 {
			w.scans.Request(libs...)
		}
		timer.Stop()
		if wait > 0 {
			timer.Reset(wait)
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	lib, ok := w.dirs[filepath.Dir(ev.Name)]
	if ok && strings.HasPrefix(filepath.Base(ev.Name), ".") {
		return // scans skip hidden files and folders
	}
	if !ok {
		// An event about a watched folder itself, or from a folder dropped meanwhile.
		if lib, ok = w.dirs[ev.Name]; !ok {
			return
		}
	}

	switch {
	case ev.Has(fsnotify.Create):
		// A file or folder created or moved in. A moved-in file is done. One being written sends
		// writes next, which hold it back until it stops growing. A new folder is watched at once, so
		// those writes aren't missed.
		if fi, err := os.Lstat(ev.Name); err == nil && fi.IsDir() {
			if _, _, err := walk(ev.Name, func(dir string) { w.add(lib, dir) }); err != nil {
				slog.Warn("watch: new folder", "path", ev.Name, "err", err)
			}
		}
		w.batch.changed(lib, now)
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		w.batch.gone(ev.Name)
		// A moved folder keeps its watches, under the old names: fsnotify reports their events with
		// the old path. Dropped here, so the new name can be watched afresh.
		w.unwatchTree(ev.Name)
		w.batch.changed(lib, now)
	case ev.Has(fsnotify.Write):
		w.batch.wrote(lib, ev.Name, now)
	}
}

// add watches one folder. The caller holds w.mu.
func (w *Watcher) add(lib int64, dir string) {
	if err := w.fsw.Add(dir); err != nil {
		switch {
		case errors.Is(err, fsnotify.ErrClosed):
			// Shutdown: Run returned while a scan was still walking.
		case errors.Is(err, syscall.ENOSPC):
			if !w.loggedNo {
				w.loggedNo = true
				slog.Warn("watch: out of inotify watches; raise fs.inotify.max_user_watches on the host. "+
					"Libraries are still rescanned on a timer", "path", dir, "every", RescanInterval)
			}
		default:
			slog.Warn("watch: can't watch", "path", dir, "err", err)
		}
		return
	}
	w.dirs[dir] = lib
}

// remove stops watching one folder. The caller holds w.mu.
func (w *Watcher) remove(dir string) {
	delete(w.dirs, dir)
	// Errors are expected: a deleted or moved folder's watch is often gone already, dropped by
	// fsnotify or by the kernel.
	_ = w.fsw.Remove(dir)
}

// unwatchTree stops watching a folder and every watched folder inside it. The caller holds w.mu.
func (w *Watcher) unwatchTree(root string) {
	for d := range w.dirs {
		if d == root || strings.HasPrefix(d, root+string(filepath.Separator)) {
			w.remove(d)
		}
	}
}

// The methods below are called by scans. They do nothing on a nil *Watcher, so a Scanner works
// without one.

// watchDir watches a folder of a library.
func (w *Watcher) watchDir(lib int64, dir string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.add(lib, dir)
}

// prune stops watching a library's folders that are gone or are no longer folders, when their events
// were missed.
func (w *Watcher) prune(lib int64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for d, l := range w.dirs {
		if l != lib {
			continue
		}
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
			w.remove(d)
		}
	}
}

// unwatch stops watching every folder of a library.
func (w *Watcher) unwatch(lib int64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for d, l := range w.dirs {
		if l == lib {
			w.remove(d)
		}
	}
}

// growing reports whether a file (real path) is still being written.
func (w *Watcher) growing(file string) bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.batch.growing(file, time.Now())
}
