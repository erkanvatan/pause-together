package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erkanvatan/pause-together/internal/media"
)

func TestBatcherDebounce(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBatcher(2*time.Second, 30*time.Second, 10*time.Second)

	if libs, wait := b.due(t0); libs != nil || wait != 0 {
		t.Errorf("empty: due = %v, %v; want nothing", libs, wait)
	}

	b.changed(1, t0)
	if libs, wait := b.due(t0.Add(time.Second)); libs != nil || wait != time.Second {
		t.Errorf("1 s after a change: due = %v, %v; want nothing, wait 1s", libs, wait)
	}
	b.changed(1, t0.Add(1500*time.Millisecond)) // restarts the quiet time
	if libs, wait := b.due(t0.Add(2 * time.Second)); libs != nil || wait != 1500*time.Millisecond {
		t.Errorf("after a second change: due = %v, %v; want nothing, wait 1.5s", libs, wait)
	}
	if libs, wait := b.due(t0.Add(3500 * time.Millisecond)); !reflect.DeepEqual(libs, []int64{1}) || wait != 0 {
		t.Errorf("quiet passed: due = %v, %v; want [1], nothing left", libs, wait)
	}
	if libs, _ := b.due(t0.Add(time.Minute)); libs != nil {
		t.Errorf("due twice: %v", libs)
	}
}

// A library that keeps changing is still scanned once it has waited maxWait since its first change.
func TestBatcherMaxWait(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBatcher(2*time.Second, 30*time.Second, 10*time.Second)
	for i := range 30 {
		now := t0.Add(time.Duration(i) * time.Second)
		b.changed(1, now)
		if libs, _ := b.due(now); libs != nil {
			t.Fatalf("due at %v: %v", now.Sub(t0), libs)
		}
	}
	if libs, _ := b.due(t0.Add(30 * time.Second)); !reflect.DeepEqual(libs, []int64{1}) {
		t.Errorf("after 30 s of changes: due = %v, want [1]", libs)
	}
	b.changed(1, t0.Add(31*time.Second)) // a new batch: its own maxWait
	if libs, wait := b.due(t0.Add(32 * time.Second)); libs != nil || wait != time.Second {
		t.Errorf("new batch: due = %v, %v; want nothing, wait 1s", libs, wait)
	}
}

func TestBatcherLibrariesApart(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBatcher(2*time.Second, 30*time.Second, 10*time.Second)
	b.changed(1, t0)
	b.changed(2, t0.Add(time.Second))
	if libs, wait := b.due(t0.Add(2 * time.Second)); !reflect.DeepEqual(libs, []int64{1}) || wait != time.Second {
		t.Errorf("due = %v, %v; want [1], wait 1s for library 2", libs, wait)
	}
	if libs, _ := b.due(t0.Add(3 * time.Second)); !reflect.DeepEqual(libs, []int64{2}) {
		t.Errorf("due = %v, want [2]", libs)
	}
}

// A file written in place is done once no write came for the settle time.
func TestBatcherStoppedGrowing(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBatcher(2*time.Second, 30*time.Second, 10*time.Second)
	const f = "/media/Movies/Heat (1995).mkv"

	for i := range 5 {
		b.wrote(1, f, t0.Add(time.Duration(i)*5*time.Second))
	}
	last := t0.Add(20 * time.Second)
	if !b.growing(f, last.Add(9*time.Second)) {
		t.Error("9 s after the last write: want growing")
	}
	if b.growing("/media/Movies/Ronin (1998).mkv", last) {
		t.Error("a file never written: want not growing")
	}
	if libs, wait := b.due(last.Add(9 * time.Second)); libs != nil || wait != time.Second {
		t.Errorf("still growing: due = %v, %v; want nothing, wait 1s", libs, wait)
	}
	if b.growing(f, last.Add(10*time.Second)) {
		t.Error("10 s after the last write: want not growing")
	}
	if libs, wait := b.due(last.Add(10 * time.Second)); !reflect.DeepEqual(libs, []int64{1}) || wait != 0 {
		t.Errorf("settled: due = %v, %v; want [1], nothing left", libs, wait)
	}
}

func TestBatcherGone(t *testing.T) {
	t0 := time.Unix(1000, 0)
	b := newBatcher(2*time.Second, 30*time.Second, 10*time.Second)
	const f = "/media/Movies/Heat (1995).mkv"
	b.wrote(1, f, t0)
	b.gone(f)
	if b.growing(f, t0) {
		t.Error("removed file: want not growing")
	}
	if libs, wait := b.due(t0.Add(time.Hour)); libs != nil || wait != 0 {
		t.Errorf("due = %v, %v; want nothing", libs, wait)
	}
}

// safeProber is a fake prober the watch tests can read while scans run. It records the size of each
// file when it was probed.
type safeProber struct {
	mu    sync.Mutex
	sizes map[string][]int64 // file name → size at each probe
}

func (p *safeProber) Probe(_ context.Context, path string) (media.Info, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return media.Info{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sizes[filepath.Base(path)] = append(p.sizes[filepath.Base(path)], fi.Size())
	return fakeInfo, nil
}

func (p *safeProber) probes(name string) []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.sizes[name])
}

// watched lists the folders watched for a library.
func (w *Watcher) watched(lib int64) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var dirs []string
	for d, l := range w.dirs {
		if l == lib {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// watchLib is a test library with a scan queue and a watcher running, with short timings.
type watchLib struct {
	*testLib
	prober  *safeProber
	scans   *Scans
	watcher *Watcher
	outside string // a folder outside the media folder, to move files in from
}

func newWatchLib(t *testing.T, typ Type) *watchLib {
	t.Helper()
	l := newTestLib(t, typ)
	p := &safeProber{sizes: map[string][]int64{}}
	l.scanner.Prober = p
	q := NewScans(l.scanner)
	w, err := NewWatcher(q)
	if err != nil {
		t.Fatal(err)
	}
	w.batch = newBatcher(50*time.Millisecond, time.Second, 300*time.Millisecond)
	l.scanner.Watcher = w

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { q.Run(ctx) })
	wg.Go(func() { w.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	q.Request(l.lib.ID)
	waitIdle(t, q)
	return &watchLib{testLib: l, prober: p, scans: q, watcher: w, outside: t.TempDir()}
}

// moveIn writes a file outside the media folder, then renames it into the library, as most tools do
// when they finish.
func (l *watchLib) moveIn(rel, content string) {
	l.t.Helper()
	tmp := filepath.Join(l.outside, filepath.Base(rel))
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		l.t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(l.dir, rel)); err != nil {
		l.t.Fatal(err)
	}
}

// writeSlowly writes a file in place in the library, a chunk every 20 ms for 600 ms: longer than
// the settle time, with gaps far shorter. It returns the file's size.
func (l *watchLib) writeSlowly(rel string) int64 {
	l.t.Helper()
	f, err := os.Create(filepath.Join(l.dir, rel))
	if err != nil {
		l.t.Fatal(err)
	}
	chunk := make([]byte, 1024)
	for range 30 {
		if _, err := f.Write(chunk); err != nil {
			l.t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := f.Close(); err != nil {
		l.t.Fatal(err)
	}
	if got := l.prober.probes(filepath.Base(rel)); len(got) != 0 {
		l.t.Errorf("%s probed while growing: sizes %v", rel, got)
	}
	return 30 * 1024
}

// probedOnce checks that a file written in place got into the database, probed only once it was
// done.
func (l *watchLib) probedOnce(rel string, size int64) {
	l.t.Helper()
	l.eventually(rel, present(rel))
	waitIdle(l.t, l.scans)
	if got, want := l.prober.probes(filepath.Base(rel)), []int64{size}; !reflect.DeepEqual(got, want) {
		l.t.Errorf("%s: probes saw sizes %v, want %v", rel, got, want)
	}
}

// eventually waits until cond holds for the library's videos.
func (l *watchLib) eventually(what string, cond func(map[string]videoRow) bool) {
	l.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		v := l.videos()
		if cond(v) {
			return
		}
		if time.Now().After(deadline) {
			l.t.Fatalf("%s: videos = %+v", what, v)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func present(rel string) func(map[string]videoRow) bool {
	return func(v map[string]videoRow) bool { r, ok := v[rel]; return ok && !r.Missing }
}

func missing(rel string) func(map[string]videoRow) bool {
	return func(v map[string]videoRow) bool { r, ok := v[rel]; return ok && r.Missing }
}

func TestWatchAddRenameDelete(t *testing.T) {
	l := newWatchLib(t, Movies)
	l.write("notes.txt", "") // an emptied folder is a gone folder, not a missing video

	l.moveIn("Heat (1995).mkv", "heat")
	l.eventually("moved in", present("Heat (1995).mkv"))

	if err := os.Rename(filepath.Join(l.dir, "Heat (1995).mkv"), filepath.Join(l.dir, "Heat (1995) - 4K.mkv")); err != nil {
		t.Fatal(err)
	}
	l.eventually("renamed", func(v map[string]videoRow) bool {
		return missing("Heat (1995).mkv")(v) && present("Heat (1995) - 4K.mkv")(v)
	})

	if err := os.Remove(filepath.Join(l.dir, "Heat (1995) - 4K.mkv")); err != nil {
		t.Fatal(err)
	}
	l.eventually("deleted", missing("Heat (1995) - 4K.mkv"))
}

// A new folder is watched at once: a file written into it right away is held until it is done, and
// a file moved in later shows up.
func TestWatchNewFolder(t *testing.T) {
	l := newWatchLib(t, TVShows)
	season := filepath.Join(l.dir, "Show (2020)", "Season 01")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	const first = "Show (2020)/Season 01/Show (2020) - s01e01.mkv"
	l.probedOnce(first, l.writeSlowly(first))

	l.moveIn("Show (2020)/Season 01/Show (2020) - s01e02.mkv", "two")
	l.eventually("second episode", present("Show (2020)/Season 01/Show (2020) - s01e02.mkv"))
}

// A folder moved inside the library is watched under its new name.
func TestWatchMovedFolder(t *testing.T) {
	l := newWatchLib(t, TVShows)
	l.write("Show (2020)/Season 01/Show (2020) - s01e01.mkv", "one")
	l.scans.Request(l.lib.ID)
	waitIdle(t, l.scans)

	if err := os.Rename(filepath.Join(l.dir, "Show (2020)"), filepath.Join(l.dir, "Show (2021)")); err != nil {
		t.Fatal(err)
	}
	l.eventually("folder renamed", present("Show (2021)/Season 01/Show (2020) - s01e01.mkv"))
	waitIdle(t, l.scans)
	for _, d := range l.watcher.watched(l.lib.ID) {
		if strings.Contains(d, "Show (2020)") {
			t.Errorf("still watched under its old name: %s", d)
		}
	}

	const second = "Show (2021)/Season 01/Show (2021) - s01e02.mkv"
	l.probedOnce(second, l.writeSlowly(second))
}

// A file written in place is probed once, after it stopped growing.
func TestWatchWrittenInPlace(t *testing.T) {
	l := newWatchLib(t, Movies)
	const name = "Heat (1995).mkv"
	l.probedOnce(name, l.writeSlowly(name))
}

func TestWatchRemovedLibraryUnwatched(t *testing.T) {
	l := newWatchLib(t, Movies)
	l.write("Sub/Heat (1995).mkv", "heat")
	l.scans.Request(l.lib.ID)
	waitIdle(t, l.scans)
	if n := len(l.watcher.watched(l.lib.ID)); n != 2 {
		t.Fatalf("watched folders = %d, want 2", n)
	}
	l.scans.Removed(l.lib.ID)
	if got := l.watcher.watched(l.lib.ID); len(got) != 0 {
		t.Errorf("watched after Removed = %v, want none", got)
	}
}
