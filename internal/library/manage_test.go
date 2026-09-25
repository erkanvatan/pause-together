package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/erkanvatan/pause-together/internal/store"
)

func TestOverlaps(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Movies", "Movies", true},
		{"Movies", "Movies/Action", true},
		{"Movies/Action", "Movies", true},
		{".", "Movies", true},
		{"Movies", ".", true},
		{"Movies", "TV", false},
		{"Movies", "Movies 4K", false},
		{"Movies/Action", "Movies/Drama", false},
		{"Mov", "Movies", false},
	}
	for _, tt := range tests {
		if got := overlaps(tt.a, tt.b); got != tt.want {
			t.Errorf("overlaps(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// newTestLibraries makes an empty media folder and database.
func newTestLibraries(t *testing.T) *Libraries {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Libraries{DB: db, Root: t.TempDir()}
}

// mkdirs makes folders under the media folder.
func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestFolders(t *testing.T) {
	l := newTestLibraries(t)
	mkdirs(t, l.Root, "Movies/Action", "Movies/Drama", "Movies/.hidden", "TV")
	if err := os.WriteFile(filepath.Join(l.Root, "Movies", "Heat (1995).mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	symlink(t, "../TV", filepath.Join(l.Root, "Movies", "Linked"))   // a folder inside: shown
	symlink(t, outside, filepath.Join(l.Root, "Movies", "Escape"))   // outside the media folder: hidden
	symlink(t, "nowhere", filepath.Join(l.Root, "Movies", "Broken")) // broken: hidden
	symlink(t, outside, filepath.Join(l.Root, "Out"))

	tests := []struct {
		path    string
		want    []string
		wantErr error
	}{
		{path: "", want: []string{"Movies", "TV"}},
		{path: ".", want: []string{"Movies", "TV"}},
		{path: "Movies", want: []string{"Action", "Drama", "Linked"}},
		{path: "Movies/Action", want: []string{}},
		{path: "Movies/Linked", want: []string{}},
		{path: "..", wantErr: ErrBadPath},
		{path: "Movies/../..", wantErr: ErrBadPath},
		{path: "/etc", wantErr: ErrBadPath},
		{path: "Out", wantErr: ErrNotFolder},
		{path: "Movies/Escape", wantErr: ErrNotFolder},
		{path: "Movies/Heat (1995).mkv", wantErr: ErrNotFolder},
		{path: "Nope", wantErr: ErrNotFolder},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := l.Folders(tt.path)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddLibrary(t *testing.T) {
	outside := t.TempDir()
	tests := []struct {
		name    string
		path    string
		typ     Type
		wantErr error
	}{
		{name: "sibling", path: "TV", typ: TVShows},
		{name: "sibling with a shared prefix", path: "Movies 4K", typ: Movies},
		{name: "cleaned", path: "Other/../TV", typ: TVShows},
		{name: "same folder", path: "Movies", typ: Movies, wantErr: ErrOverlap},
		{name: "same folder, other spelling", path: "Movies/", typ: Movies, wantErr: ErrOverlap},
		{name: "parent", path: "Shared", typ: Movies, wantErr: ErrOverlap},
		{name: "child", path: "Movies/Action", typ: Movies, wantErr: ErrOverlap},
		{name: "child, by its real path", path: "Shared/Movies/Action", typ: Movies, wantErr: ErrOverlap},
		{name: "the media folder", path: "", typ: Movies, wantErr: ErrBadPath},
		{name: "symlink to the media folder", path: "RootLink", typ: Movies, wantErr: ErrBadPath},
		{name: "symlink to another library", path: "MoviesLink", typ: Movies, wantErr: ErrOverlap},
		{name: "symlink into another library", path: "ActionLink", typ: Movies, wantErr: ErrOverlap},
		{name: "symlink to a parent", path: "SharedLink", typ: Movies, wantErr: ErrOverlap},
		{name: "a file", path: "file.mkv", typ: Movies, wantErr: ErrNotFolder},
		{name: "missing", path: "Nope", typ: Movies, wantErr: ErrNotFolder},
		{name: "dot dot", path: "../x", typ: Movies, wantErr: ErrBadPath},
		{name: "absolute", path: outside, typ: Movies, wantErr: ErrBadPath},
		{name: "symlink outside", path: "Escape", typ: Movies, wantErr: ErrNotFolder},
		{name: "bad type", path: "TV", typ: "music", wantErr: ErrBadType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestLibraries(t)
			mkdirs(t, l.Root, "Shared/Movies/Action", "TV", "Movies 4K")
			symlink(t, "Shared/Movies", filepath.Join(l.Root, "Movies"))
			symlink(t, "Movies", filepath.Join(l.Root, "MoviesLink"))
			symlink(t, "Shared/Movies/Action", filepath.Join(l.Root, "ActionLink"))
			symlink(t, "Shared", filepath.Join(l.Root, "SharedLink"))
			symlink(t, outside, filepath.Join(l.Root, "Escape"))
			symlink(t, ".", filepath.Join(l.Root, "RootLink"))
			if err := os.WriteFile(filepath.Join(l.Root, "file.mkv"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			// The library already there. Its folder is a symlink to Shared/Movies, so some cases
			// overlap only once symlinks are resolved.
			if _, err := l.Add(context.Background(), "Movies", Movies); err != nil {
				t.Fatal(err)
			}

			lib, err := l.Add(context.Background(), tt.path, tt.typ)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			libs, err := l.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			wantLibs := 1
			if tt.wantErr == nil {
				wantLibs = 2
				if lib.ID == 0 || lib.Type != tt.typ {
					t.Errorf("added %+v", lib)
				}
			}
			if len(libs) != wantLibs {
				t.Errorf("got %d libraries, want %d: %+v", len(libs), wantLibs, libs)
			}
		})
	}
}

// Another library whose folder is gone still blocks its old folder name.
func TestAddOverlapsGoneFolder(t *testing.T) {
	l := newTestLibraries(t)
	mkdirs(t, l.Root, "Movies")
	if _, err := l.Add(context.Background(), "Movies", Movies); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(l.Root, "Movies"), filepath.Join(l.Root, "Away")); err != nil {
		t.Fatal(err)
	}
	mkdirs(t, l.Root, "Movies/Action")
	if _, err := l.Add(context.Background(), "Movies/Action", Movies); !errors.Is(err, ErrOverlap) {
		t.Errorf("err = %v, want ErrOverlap", err)
	}
}

func TestRemoveAndReAdd(t *testing.T) {
	l := newTestLib(t, Movies)
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}
	ctx := context.Background()
	l.write("Heat (1995).mkv", "heat")
	l.write("Inception.2010.mkv", "no year")
	l.scan()
	before := l.videos()["Heat (1995).mkv"]

	if err := libs.Remove(ctx, l.lib.ID); err != nil {
		t.Fatal(err)
	}
	if v, ok := l.videos()["Heat (1995).mkv"]; !ok || !v.Missing || v.ID != before.ID || v.Title != "Heat" {
		t.Errorf("after remove: %+v (found %v), want the same row, missing, title kept", v, ok)
	}
	if got := l.skipped(); len(got) != 0 {
		t.Errorf("skipped after remove = %v, want none", got)
	}
	if got, err := libs.List(ctx); err != nil || len(got) != 0 {
		t.Errorf("List after remove = %+v, %v; want none", got, err)
	}
	if _, err := libs.Get(ctx, l.lib.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after remove: err = %v, want ErrNotFound", err)
	}
	if err := libs.Remove(ctx, l.lib.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove: err = %v, want ErrNotFound", err)
	}

	back, err := libs.Add(ctx, l.lib.Path, OtherVideos)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != l.lib.ID {
		t.Errorf("re-added library id = %d, want %d", back.ID, l.lib.ID)
	}
	l.lib = back
	l.scan()
	if v := l.videos()["Heat (1995).mkv"]; v.Missing || v.ID != before.ID || v.Title != "Heat (1995)" {
		t.Errorf("after re-add and scan: %+v, want the same row, not missing, named as Other Videos", v)
	}
}

func TestRemoveUnknown(t *testing.T) {
	l := newTestLibraries(t)
	if err := l.Remove(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListCountsPresentVideos(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()
	if err := os.Remove(filepath.Join(l.dir, "Ronin (1998).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	got, err := (&Libraries{DB: l.db, Root: l.scanner.Root}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []LibraryInfo{{Library: l.lib, Videos: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestProblems(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.write("Dune (2021).mkv", "dune")
	l.write("Gone (2000).mkv", "gone")
	l.write("Inception.2010.mkv", "no year")
	l.prober.fail["Ronin (1998).mkv"] = true
	l.scan()
	for _, q := range []string{
		"UPDATE videos SET unplayable = 'codec', video_codec = 'vp8' WHERE path = 'Heat (1995).mkv'",
		"UPDATE videos SET apple_only = 1 WHERE path = 'Dune (2021).mkv'",
		"UPDATE videos SET apple_only = 1, unplayable = 'codec', missing = 1 WHERE path = 'Gone (2000).mkv'",
	} {
		if _, err := l.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}
	got, err := libs.Problems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id := l.lib.ID
	want := Problems{
		Skipped: []Problem{{LibraryID: id, Path: "Inception.2010.mkv", Reason: string(ReasonMovieNoYear)}},
		Unplayable: []Problem{
			{LibraryID: id, Path: "Heat (1995).mkv", Reason: "codec", Codec: "vp8"},
			{LibraryID: id, Path: "Ronin (1998).mkv", Reason: "probe-failed", ProbeError: "broken file"},
		},
		AppleOnly: []Problem{{LibraryID: id, Path: "Dune (2021).mkv"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}

	// A removed library's files aren't problems any more.
	if err := libs.Remove(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got, err = libs.Problems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Skipped) + len(got.Unplayable) + len(got.AppleOnly); n != 0 {
		t.Errorf("after remove: %+v, want none", got)
	}
}

// A scan's transactions refuse to write into a removed library.
func TestScanRefusesRemovedLibrary(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	if _, err := l.db.Exec("UPDATE libraries SET removed = 1"); err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); !errors.Is(err, ErrRemoved) {
		t.Errorf("err = %v, want ErrRemoved", err)
	}
	if videos := l.videos(); len(videos) != 0 {
		t.Errorf("videos = %v, want none written", videos)
	}
}
