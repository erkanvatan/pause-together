package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/erkanvatan/pause-together/internal/store"
)

var (
	ErrBadType   = errors.New("unknown library type")
	ErrBadPath   = errors.New("path is not inside the media folder")
	ErrNotFolder = errors.New("not a folder in the media folder")
	ErrOverlap   = errors.New("folder overlaps another library")
	ErrNotFound  = errors.New("no such library")
)

// Libraries adds, lists and removes libraries, and browses the media folder for the admin page.
type Libraries struct {
	DB   *sql.DB
	Root string // the media folder
}

// LibraryInfo is a library as the admin page lists it.
type LibraryInfo struct {
	Library
	Videos int `json:"videos"` // videos that aren't missing
}

// List returns the libraries that aren't removed, by path.
func (l *Libraries) List(ctx context.Context) ([]LibraryInfo, error) {
	rows, err := l.DB.QueryContext(ctx, `
		SELECT id, path, type, (SELECT count(*) FROM videos WHERE library_id = libraries.id AND NOT missing)
		FROM libraries WHERE NOT removed ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	libs := []LibraryInfo{}
	for rows.Next() {
		var li LibraryInfo
		if err := rows.Scan(&li.ID, &li.Path, &li.Type, &li.Videos); err != nil {
			return nil, err
		}
		libs = append(libs, li)
	}
	return libs, rows.Err()
}

// Get returns a library that isn't removed, or ErrNotFound.
func (l *Libraries) Get(ctx context.Context, id int64) (Library, error) {
	return getLibrary(ctx, l.DB, id)
}

// Add makes a library of a folder in the media folder, not the media folder itself. The folder must
// not be, contain or sit inside another library's folder, symlinks resolved: one file would get two
// identities. A removed library with the same folder comes back, so its videos keep their ids.
func (l *Libraries) Add(ctx context.Context, p string, typ Type) (Library, error) {
	switch typ {
	case Movies, TVShows, OtherVideos:
	default:
		return Library{}, ErrBadType
	}
	rel, err := cleanPath(p)
	if err != nil {
		return Library{}, err
	}
	resolved, err := l.resolve(rel)
	if err != nil {
		return Library{}, err
	}
	if resolved == "." {
		return Library{}, ErrBadPath
	}

	lib := Library{Path: rel, Type: typ}
	// The overlap check runs in the transaction, so two adds at once can't both pass it.
	err = store.InTx(ctx, l.DB, func(tx *sql.Tx) error {
		others, err := activePaths(ctx, tx)
		if err != nil {
			return err
		}
		for _, other := range others {
			otherPaths := []string{other}
			if r, err := l.resolve(other); err == nil {
				otherPaths = append(otherPaths, r) // a folder that is gone can only overlap by name
			}
			for _, a := range []string{rel, resolved} {
				for _, b := range otherPaths {
					if overlaps(a, b) {
						return ErrOverlap
					}
				}
			}
		}
		return tx.QueryRowContext(ctx, `
			INSERT INTO libraries (path, type) VALUES (?, ?)
			ON CONFLICT (path) DO UPDATE SET type = excluded.type, removed = 0
			RETURNING id`, rel, string(typ)).Scan(&lib.ID)
	})
	if err != nil {
		return Library{}, err
	}
	return lib, nil
}

func activePaths(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT path FROM libraries WHERE NOT removed")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// Remove marks a library removed. The row stays, since its videos point at it. Its videos turn
// missing and its skipped files are dropped. A scan still running on it stops at its next write.
func (l *Libraries) Remove(ctx context.Context, id int64) error {
	return store.InTx(ctx, l.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE libraries SET removed = 1 WHERE id = ? AND NOT removed", id)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, "UPDATE videos SET missing = 1 WHERE library_id = ?", id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM skipped_files WHERE library_id = ?", id)
		return err
	})
}

// Folders lists the folders inside a folder of the media folder, by name, for the admin's folder
// picker. It goes through os.Root, so neither ".." nor a symlink can lead outside the media folder.
// Hidden folders are left out, as the scanner skips them.
func (l *Libraries) Folders(p string) ([]string, error) {
	rel, err := cleanPath(p)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(l.Root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotFolder, err)
	}
	defer func() { _ = dir.Close() }()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotFolder, err)
	}

	folders := []string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			// Shown only if it leads to a folder inside the media folder.
			if fi, err := root.Stat(path.Join(rel, e.Name())); err != nil || !fi.IsDir() {
				continue
			}
		} else if !e.IsDir() {
			continue
		}
		folders = append(folders, e.Name())
	}
	slices.Sort(folders)
	return folders, nil
}

// cleanPath checks that p is a path inside the media folder and cleans it. "" and "." are the media
// folder itself.
func cleanPath(p string) (string, error) {
	if p == "" {
		p = "."
	}
	if !filepath.IsLocal(p) {
		return "", ErrBadPath
	}
	return filepath.ToSlash(filepath.Clean(p)), nil
}

// resolve checks that rel is a folder inside the media folder, and returns its path with every
// symlink resolved, relative to the media folder.
func (l *Libraries) resolve(rel string) (string, error) {
	root, err := os.OpenRoot(l.Root)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	if fi, err := root.Stat(rel); err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotFolder, err)
	} else if !fi.IsDir() {
		return "", ErrNotFolder
	}
	_, resolved, err := realPath(l.Root, rel)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotFolder, err)
	}
	return resolved, nil
}

// realPath resolves every symlink in rel, a path inside the media folder mediaRoot. It returns the
// real absolute path, and that path relative to the real media folder. It fails with ErrBadPath if
// the real path is outside the media folder.
func realPath(mediaRoot, rel string) (abs, realRel string, err error) {
	realRoot, err := filepath.EvalSymlinks(mediaRoot)
	if err != nil {
		return "", "", err
	}
	abs, err = filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", "", err
	}
	r, err := filepath.Rel(realRoot, abs)
	if err != nil || !filepath.IsLocal(r) {
		return "", "", ErrBadPath
	}
	return abs, filepath.ToSlash(r), nil
}

// overlaps reports whether two folders, relative to the media folder, are the same or one is inside
// the other. "." is the media folder itself, so it overlaps everything.
func overlaps(a, b string) bool {
	return a == b || a == "." || b == "." || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Problem is a file the admin page lists: one a scan skipped, a video no browser can play, or a video
// only Apple devices show right.
type Problem struct {
	LibraryID  int64  `json:"libraryId"`
	Path       string `json:"path"`             // relative to the library folder
	Reason     string `json:"reason,omitempty"` // a Reason for skipped files, a media.Unplayable for videos
	Codec      string `json:"codec,omitempty"`  // unplayable videos: ffprobe's codec name
	ProbeError string `json:"probeError,omitempty"`
}

// Problems is every Problem in libraries that aren't removed. Missing videos are left out.
type Problems struct {
	Skipped    []Problem `json:"skipped"`
	Unplayable []Problem `json:"unplayable"`
	AppleOnly  []Problem `json:"appleOnly"`
}

// Problems lists the files the admin page shows as unusable or Apple-only, by library and path.
func (l *Libraries) Problems(ctx context.Context) (Problems, error) {
	p := Problems{Skipped: []Problem{}, Unplayable: []Problem{}, AppleOnly: []Problem{}}
	queries := []struct {
		list *[]Problem
		sql  string
	}{
		{&p.Skipped, `
			SELECT s.library_id, s.path, s.reason, '', ''
			FROM skipped_files s JOIN libraries l ON l.id = s.library_id
			WHERE NOT l.removed ORDER BY s.library_id, s.path`},
		{&p.Unplayable, `
			SELECT v.library_id, v.path, v.unplayable, v.video_codec, COALESCE(v.probe_error, '')
			FROM videos v JOIN libraries l ON l.id = v.library_id
			WHERE NOT l.removed AND NOT v.missing AND v.unplayable IS NOT NULL ORDER BY v.library_id, v.path`},
		{&p.AppleOnly, `
			SELECT v.library_id, v.path, '', '', ''
			FROM videos v JOIN libraries l ON l.id = v.library_id
			WHERE NOT l.removed AND NOT v.missing AND v.apple_only ORDER BY v.library_id, v.path`},
	}
	for _, q := range queries {
		if err := l.problems(ctx, q.list, q.sql); err != nil {
			return Problems{}, err
		}
	}
	return p, nil
}

func (l *Libraries) problems(ctx context.Context, list *[]Problem, query string) error {
	rows, err := l.DB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pr Problem
		if err := rows.Scan(&pr.LibraryID, &pr.Path, &pr.Reason, &pr.Codec, &pr.ProbeError); err != nil {
			return err
		}
		*list = append(*list, pr)
	}
	return rows.Err()
}
