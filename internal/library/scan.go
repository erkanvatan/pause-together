package library

import (
	"context"
	"database/sql"
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
	ID   int64
	Path string // relative to the media folder, "/" separators
	Type Type
}

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

// ScanAll scans every library, one after another. A library that fails is logged and the rest still
// scan; only a failure to list the libraries, or a cancel, is returned.
func (s *Scanner) ScanAll(ctx context.Context) error {
	libs, err := s.libraries(ctx)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		if err := s.ScanLibrary(ctx, lib); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("scan library", "library", lib.Path, "err", err)
		}
	}
	return nil
}

// fileStat is a file found in a library folder.
type fileStat struct {
	name  string
	size  int64
	mtime int64 // Unix nanoseconds
}

// ScanLibrary brings one library's rows up to date with its folder. New and changed videos are
// probed, and so are videos whose last probe failed. Videos no longer there are marked missing.
//
// If the library folder itself is gone or can't be read, it returns an error and marks nothing
// missing. A sub-folder that can't be read is logged, and nothing under it is marked missing.
func (s *Scanner) ScanLibrary(ctx context.Context, lib Library) error {
	known, err := s.knownVideos(ctx, lib.ID)
	if err != nil {
		return err
	}
	// Resolved, so a library folder that is itself a symlink gets walked. Folder links inside it still aren't.
	root, err := filepath.EvalSymlinks(filepath.Join(s.Root, filepath.FromSlash(lib.Path)))
	if err != nil {
		return err
	}
	folders, unreadable, err := walk(root)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	var refresh []parsedVideo
	var skipped []skippedFile
	probed := 0
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
				continue
			case KindVideo:
			default:
				continue // ignored; subtitles wait for their own slice
			}

			seen[rel] = true
			st := stats[i]
			if k, ok := known[rel]; ok && k.size == st.size && k.mtime == st.mtime && !k.failed {
				// Unchanged file. Its row is written only if it was missing or its name parses differently now.
				if k.missing || k.video != f.Video {
					refresh = append(refresh, parsedVideo{id: k.id, video: f.Video})
				}
				continue
			}
			info, perr := s.Prober.Probe(ctx, filepath.Join(root, filepath.FromSlash(rel)))
			if perr != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			if err := s.saveProbed(ctx, lib.ID, rel, f.Video, st, info, perr); err != nil {
				return err
			}
			probed++
		}
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
