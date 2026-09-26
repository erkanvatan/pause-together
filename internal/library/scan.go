package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/erkanvatan/pause-together/internal/media"
)

// Library is a folder of videos under the media folder, plus its type.
type Library struct {
	ID   int64  `json:"id"`
	Path string `json:"path"` // relative to the media folder, "/" separators
	Type Type   `json:"type"`
}

// ErrRemoved stops a scan whose library was removed while it ran.
var ErrRemoved = errors.New("library removed")

// Prober reads a video file's technical facts. media.FFprobe is the real one.
type Prober interface {
	Probe(ctx context.Context, path string) (media.Info, error)
}

// Subtitler converts sidecar subtitles into WebVTT copies in the cache. media.Subtitles is the real one.
type Subtitler interface {
	// Sidecar makes key's copy from the file at src, unless it's there already. lang is the language
	// code from the file name.
	Sidecar(ctx context.Context, src, lang, key string) error
	Remove(key string) error
}

// Scanner fills the database from library folders.
type Scanner struct {
	DB        *sql.DB
	Root      string // the media folder; library paths are relative to it
	Prober    Prober
	Subtitles Subtitler
	// Watcher, if not nil, gets a watch on every folder a scan walks. Files it sees growing are left
	// alone until they stop.
	Watcher *Watcher
}

// fileStat is a file found in a library folder.
type fileStat struct {
	name  string
	size  int64
	mtime int64 // Unix nanoseconds
}

// ScanLibrary brings one library's rows up to date with its folder. New and changed videos are
// probed, and so are videos whose last probe failed. Videos no longer there are marked missing.
// Sidecar subtitles are converted to WebVTT when new or changed; rows of the ones gone are deleted.
// progress, if not nil, hears how many of the library's videos are done so far.
//
// A file still being written (see Watcher) is not probed or converted, and not marked missing; the
// watcher queues another scan once it stops growing.
//
// If the library folder itself is gone, can't be read, or leads outside the media folder, it returns
// an error and marks nothing missing. A sub-folder that can't be read is logged, and nothing under it
// is marked missing. If the library is removed meanwhile, it stops with ErrRemoved.
func (s *Scanner) ScanLibrary(ctx context.Context, lib Library, progress func(done, total int)) error {
	known, err := s.knownVideos(ctx, lib.ID)
	if err != nil {
		return err
	}
	// Resolved, so a library folder that is itself a symlink gets walked. Folder links inside it still
	// aren't. Checked on every scan: the link may point outside the media folder by now.
	root, _, err := realPath(s.Root, lib.Path)
	if err != nil {
		return err
	}
	// Watches of folders that are gone are dropped before the walk: a folder moved inside the library
	// keeps its old watch, and adding it again under its new name would find that old one.
	s.Watcher.prune(lib.ID)
	folders, unreadable, err := walk(root, func(dir string) { s.Watcher.watchDir(lib.ID, dir) })
	if err != nil {
		return err
	}

	// Every folder is parsed first, so the progress total is known before the slow probes start.
	type foundVideo struct {
		rel   string
		video Video
		st    fileStat
	}
	var found []foundVideo
	var subs []foundSidecar
	var skipped []skippedFile
	for _, dir := range slices.Sorted(maps.Keys(folders)) {
		stats := folders[dir]
		names := make([]string, len(stats))
		for i, st := range stats {
			names[i] = st.name
		}
		for i, f := range ParseDir(lib.Type, dir, names) {
			rel := path.Join(dir, f.Name)
			switch f.Kind {
			case KindSkipped:
				skipped = append(skipped, skippedFile{path: rel, reason: f.Reason})
			case KindVideo:
				found = append(found, foundVideo{rel: rel, video: f.Video, st: stats[i]})
			case KindSubtitle:
				subs = append(subs, foundSidecar{rel: rel, video: path.Join(dir, f.Subtitle.Video), sub: f.Subtitle,
					st: stats[i]})
			}
		}
	}
	if progress == nil {
		progress = func(int, int) {}
	}

	seen := make(map[string]bool)
	ids := make(map[string]int64) // video ids by path, for the sidecars
	var w scanWrites
	probed := 0
	for i, fv := range found {
		progress(i, len(found))
		seen[fv.rel] = true
		k, ok := known[fv.rel]
		if ok {
			ids[fv.rel] = k.id
		}
		abs := filepath.Join(root, filepath.FromSlash(fv.rel))
		if s.Watcher.growing(abs) {
			continue
		}
		if ok && k.size == fv.st.size && k.mtime == fv.st.mtime && !k.failed {
			// Unchanged file. Its row is written only if it was missing or its name parses differently now.
			if k.missing || k.video != fv.video {
				w.refresh = append(w.refresh, parsedVideo{id: k.id, video: fv.video})
			}
			continue
		}
		info, perr := s.Prober.Probe(ctx, abs)
		if perr != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		id, err := s.saveProbed(ctx, lib.ID, fv.rel, fv.video, fv.st, info, perr)
		if err != nil {
			return err
		}
		ids[fv.rel] = id
		probed++
	}

	for rel, k := range known {
		if !seen[rel] && !k.missing && !isUnder(rel, unreadable) {
			w.gone = append(w.gone, k.id)
		}
	}
	unusable, err := s.sidecars(ctx, lib.ID, root, subs, ids, unreadable, &w)
	if err != nil {
		return err
	}
	w.skipped = append(skipped, unusable...)
	if err := s.finishScan(ctx, lib.ID, w); err != nil {
		// The new copies' rows weren't written; nothing points at them.
		for _, sc := range w.sidecars {
			if sc.key != sc.oldKey {
				s.removeSidecar(sc.key)
			}
		}
		return err
	}
	// Only now that no row points at them.
	for _, sc := range w.sidecars {
		if sc.oldKey != "" && sc.oldKey != sc.key {
			s.removeSidecar(sc.oldKey)
		}
	}
	for _, k := range w.goneSidecars {
		s.removeSidecar(k.key)
	}
	progress(len(found), len(found))
	slog.Info("scanned library", "library", lib.Path, "videos", len(seen), "probed", probed,
		"skipped", len(w.skipped), "missing", len(w.gone))
	return nil
}

// foundSidecar is a sidecar subtitle found in a library folder.
type foundSidecar struct {
	rel   string
	video string // its video's path, relative to the library folder
	sub   Subtitle
	st    fileStat
}

// sidecars converts the library's new and changed sidecar subtitles and adds the rows to write to w:
// sidecars to insert or update, and the rows of the ones gone. ids are the library's video ids by
// path. Unreadable files (media.ErrUnreadable) come back as skipped; their rows go too. Other
// conversion errors leave the row as it was.
func (s *Scanner) sidecars(ctx context.Context, libraryID int64, root string, subs []foundSidecar,
	ids map[string]int64, unreadable []string, w *scanWrites) ([]skippedFile, error) {
	known, err := s.knownSidecars(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	var skipped []skippedFile
	seen := make(map[string]bool)
	for _, f := range subs {
		seen[f.rel] = true
		abs := filepath.Join(root, filepath.FromSlash(f.rel))
		videoID, ok := ids[f.video]
		if !ok || s.Watcher.growing(abs) {
			continue // its video has no row yet (still being written), or it is being written itself
		}
		key := media.SidecarKey(libraryID, f.rel, f.st.size, f.st.mtime)
		switch err := s.Subtitles.Sidecar(ctx, abs, f.sub.Lang, key); {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, media.ErrUnreadable):
			slog.Warn("scan: can't convert subtitle", "path", abs, "err", err)
			skipped = append(skipped, skippedFile{path: f.rel, reason: ReasonSubUnreadable})
			delete(seen, f.rel) // its row goes
			continue
		case err != nil:
			// Not the file's fault (a disk hiccup, ffmpeg killed, a full cache disk): its row stays as it
			// was, and the next scan tries again.
			slog.Error("scan: convert subtitle", "path", abs, "err", err)
			continue
		}
		k := known[f.rel]
		if k.key != key || k.videoID != videoID {
			w.sidecars = append(w.sidecars, sidecarRow{id: k.id, videoID: videoID, name: path.Base(f.rel),
				sub: f.sub, key: key, oldKey: k.key})
		}
	}
	for rel, k := range known {
		if !seen[rel] && !isUnder(rel, unreadable) {
			w.goneSidecars = append(w.goneSidecars, k)
		}
	}
	return skipped, nil
}

// removeSidecar deletes a sidecar's copy. A failure only leaves a small file behind, so it's logged.
func (s *Scanner) removeSidecar(key string) {
	if err := s.Subtitles.Remove(key); err != nil {
		slog.Error("remove subtitle copy", "key", key, "err", err)
	}
}

// walk lists the files under root, grouped by folder relative to root ("." for root itself). Hidden
// folders are skipped. A directory symlink is never followed (it can loop); a file symlink counts as
// the file it points to. Folders that can't be read come back in unreadable.
//
// onDir gets each folder's absolute path before the folder is read, so a watch added there misses
// nothing the listing doesn't show.
func walk(root string, onDir func(dir string)) (folders map[string][]fileStat, unreadable []string, err error) {
	folders = make(map[string][]fileStat)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			slog.Warn("scan: can't read", "path", p, "err", err)
			unreadable = append(unreadable, filepath.ToSlash(rel))
			return nil
		}
		if p == root && !d.IsDir() {
			return fmt.Errorf("library %s is not a folder", root)
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			onDir(p)
			return nil
		}

		var info fs.FileInfo
		switch {
		case d.Type().IsRegular():
			info, err = d.Info()
		case d.Type()&fs.ModeSymlink != 0:
			info, err = os.Stat(p)
		default:
			return nil
		}
		if err != nil || !info.Mode().IsRegular() {
			return nil // gone since the folder was listed, a broken link, or a link to a folder
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(rel)
		folders[dir] = append(folders[dir], fileStat{name: d.Name(), size: info.Size(), mtime: info.ModTime().UnixNano()})
		return nil
	})
	return folders, unreadable, err
}

// isUnder reports whether rel is inside one of the folders in dirs.
func isUnder(rel string, dirs []string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}
