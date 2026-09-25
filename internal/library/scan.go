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

// Scanner fills the database from library folders.
type Scanner struct {
	DB     *sql.DB
	Root   string // the media folder; library paths are relative to it
	Prober Prober
}

// fileStat is a file found in a library folder.
type fileStat struct {
	name  string
	size  int64
	mtime int64 // Unix nanoseconds
}

// ScanLibrary brings one library's rows up to date with its folder. New and changed videos are
// probed, and so are videos whose last probe failed. Videos no longer there are marked missing.
// progress, if not nil, hears how many of the library's videos are done so far.
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
	folders, unreadable, err := walk(root)
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
			}
			// Anything else is ignored; subtitles wait for their own slice.
		}
	}
	if progress == nil {
		progress = func(int, int) {}
	}

	seen := make(map[string]bool)
	var refresh []parsedVideo
	probed := 0
	for i, fv := range found {
		progress(i, len(found))
		seen[fv.rel] = true
		if k, ok := known[fv.rel]; ok && k.size == fv.st.size && k.mtime == fv.st.mtime && !k.failed {
			// Unchanged file. Its row is written only if it was missing or its name parses differently now.
			if k.missing || k.video != fv.video {
				refresh = append(refresh, parsedVideo{id: k.id, video: fv.video})
			}
			continue
		}
		info, perr := s.Prober.Probe(ctx, filepath.Join(root, filepath.FromSlash(fv.rel)))
		if perr != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.saveProbed(ctx, lib.ID, fv.rel, fv.video, fv.st, info, perr); err != nil {
			return err
		}
		probed++
	}

	var gone []int64
	for rel, k := range known {
		if !seen[rel] && !k.missing && !isUnder(rel, unreadable) {
			gone = append(gone, k.id)
		}
	}
	if err := s.finishScan(ctx, lib.ID, refresh, gone, skipped); err != nil {
		return err
	}
	progress(len(found), len(found))
	slog.Info("scanned library", "library", lib.Path, "videos", len(seen), "probed", probed,
		"skipped", len(skipped), "missing", len(gone))
	return nil
}

// walk lists the files under root, grouped by folder relative to root ("." for root itself). Hidden
// folders are skipped. A directory symlink is never followed (it can loop); a file symlink counts as
// the file it points to. Folders that can't be read come back in unreadable.
func walk(root string) (folders map[string][]fileStat, unreadable []string, err error) {
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
