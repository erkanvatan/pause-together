package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/store"
)

// fakeProber counts probes per file name and fails for the names in fail.
type fakeProber struct {
	calls map[string]int
	fail  map[string]bool
}

func newFakeProber() *fakeProber {
	return &fakeProber{calls: map[string]int{}, fail: map[string]bool{}}
}

var fakeInfo = media.Info{
	Duration:    90 * time.Minute,
	VideoCodec:  "h264",
	CodecString: "avc1.640028",
	Audio: []media.AudioTrack{
		{Stream: 1, Codec: "eac3", Channels: 6, Layout: "5.1(side)", Lang: "eng", Title: "Surround", Default: true},
		{Stream: 2, Codec: "aac", Channels: 2, Layout: "stereo", Lang: "tur"},
	},
	Subtitles: []media.SubtitleTrack{{Stream: 3, Codec: "subrip", Lang: "eng", Forced: true}},
}

func (f *fakeProber) Probe(_ context.Context, path string) (media.Info, error) {
	if !filepath.IsAbs(path) {
		return media.Info{}, errors.New("path not absolute")
	}
	name := filepath.Base(path)
	f.calls[name]++
	if f.fail[name] {
		return media.Info{}, errors.New("broken file")
	}
	return fakeInfo, nil
}

type proberFunc func(context.Context, string) (media.Info, error)

func (f proberFunc) Probe(ctx context.Context, path string) (media.Info, error) { return f(ctx, path) }

type testLib struct {
	t       *testing.T
	db      *sql.DB
	scanner *Scanner
	prober  *fakeProber
	lib     Library
	dir     string // the library folder
}

// newTestLib makes a media folder with one library folder of the given type, and a database.
func newTestLib(t *testing.T, typ Type) *testLib {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	root := t.TempDir()
	lib := Library{Path: "Library", Type: typ}
	if err := db.QueryRowContext(ctx,
		"INSERT INTO libraries (path, type) VALUES (?, ?) RETURNING id", lib.Path, string(typ),
	).Scan(&lib.ID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, lib.Path)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prober := newFakeProber()
	return &testLib{
		t: t, db: db, prober: prober, lib: lib, dir: dir,
		scanner: &Scanner{DB: db, Root: root, Prober: prober},
	}
}

// write makes a file (and its folders) in the library, with content as its bytes.
func (l *testLib) write(rel, content string) {
	l.t.Helper()
	p := filepath.Join(l.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		l.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		l.t.Fatal(err)
	}
}

func (l *testLib) scan() {
	l.t.Helper()
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); err != nil {
		l.t.Fatalf("scan: %v", err)
	}
}

type videoRow struct {
	ID         int64
	Title      string
	Year       int
	Missing    bool
	ProbeError string
	Codec      string
}

func (l *testLib) videos() map[string]videoRow {
	l.t.Helper()
	rows, err := l.db.Query(`SELECT id, path, title, year, missing, COALESCE(probe_error, ''), codec_string
		FROM videos WHERE library_id = ?`, l.lib.ID)
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]videoRow{}
	for rows.Next() {
		var path string
		var v videoRow
		if err := rows.Scan(&v.ID, &path, &v.Title, &v.Year, &v.Missing, &v.ProbeError, &v.Codec); err != nil {
			l.t.Fatal(err)
		}
		got[path] = v
	}
	if err := rows.Err(); err != nil {
		l.t.Fatal(err)
	}
	return got
}

func (l *testLib) skipped() map[string]string {
	l.t.Helper()
	rows, err := l.db.Query("SELECT path, reason FROM skipped_files WHERE library_id = ?", l.lib.ID)
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]string{}
	for rows.Next() {
		var path, reason string
		if err := rows.Scan(&path, &reason); err != nil {
			l.t.Fatal(err)
		}
		got[path] = reason
	}
	if err := rows.Err(); err != nil {
		l.t.Fatal(err)
	}
	return got
}

func TestScanMovies(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.write("Heat (1995)/Heat (1995).en.srt", "subtitle")
	l.write("Heat (1995)/Featurettes/Making Of.mkv", "extra")
	l.write("Heat (1995)/Heat (1995).nfo", "not a video")
	l.write("Inception.2010.1080p.mkv", "no year")
	l.write("Ronin (1998).en.srt", "subtitle without its video")
	l.write(".hidden/Secret (2000).mkv", "hidden folder")
	l.write("._Heat (1995).mkv", "macOS")
	// A directory symlink that loops back: never followed.
	if err := os.Symlink(".", filepath.Join(l.dir, "Loop (2001)")); err != nil {
		t.Fatal(err)
	}
	// A file symlink to a regular file is a video like any other.
	outside := filepath.Join(t.TempDir(), "target.mkv")
	if err := os.WriteFile(outside, []byte("linked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(l.dir, "Linked (2002).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()

	videos := l.videos()
	if len(videos) != 2 {
		t.Errorf("got videos %v, want Heat and Linked", videos)
	}
	heat := videos["Heat (1995)/Heat (1995).mkv"]
	if heat.Title != "Heat" || heat.Year != 1995 || heat.Missing || heat.ProbeError != "" || heat.Codec != "avc1.640028" {
		t.Errorf("Heat = %+v", heat)
	}
	if linked := videos["Linked (2002).mkv"]; linked.Title != "Linked" {
		t.Errorf("Linked = %+v", linked)
	}

	wantSkipped := map[string]string{
		"Inception.2010.1080p.mkv": string(ReasonMovieNoYear),
		"Ronin (1998).en.srt":      string(ReasonSubNoVideo),
	}
	if got := l.skipped(); !reflect.DeepEqual(got, wantSkipped) {
		t.Errorf("skipped = %v, want %v", got, wantSkipped)
	}
}

func TestScanStoresProbeResult(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "12345")
	l.scan()

	var size, mtime, durationMS int64
	var codec, codecString string
	var unplayable sql.NullString
	var appleOnly bool
	if err := l.db.QueryRow(`SELECT size, mtime, duration_ms, video_codec, codec_string, unplayable, apple_only
		FROM videos`).Scan(&size, &mtime, &durationMS, &codec, &codecString, &unplayable, &appleOnly); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(l.dir, "Heat (1995).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 || mtime != st.ModTime().UnixNano() || durationMS != fakeInfo.Duration.Milliseconds() ||
		codec != "h264" || codecString != "avc1.640028" || unplayable.Valid || appleOnly {
		t.Errorf("got size %d, mtime %d, duration %d, codec %q %q, unplayable %v, apple only %v",
			size, mtime, durationMS, codec, codecString, unplayable, appleOnly)
	}

	var audio []media.AudioTrack
	rows, err := l.db.Query("SELECT stream, codec, channels, layout, lang, title, is_default FROM audio_tracks ORDER BY stream")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a media.AudioTrack
		if err := rows.Scan(&a.Stream, &a.Codec, &a.Channels, &a.Layout, &a.Lang, &a.Title, &a.Default); err != nil {
			t.Fatal(err)
		}
		audio = append(audio, a)
	}
	_ = rows.Close()
	if !reflect.DeepEqual(audio, fakeInfo.Audio) {
		t.Errorf("audio = %+v, want %+v", audio, fakeInfo.Audio)
	}

	var sub media.SubtitleTrack
	if err := l.db.QueryRow("SELECT stream, codec, lang, title, is_default, forced, sdh FROM subtitle_tracks").
		Scan(&sub.Stream, &sub.Codec, &sub.Lang, &sub.Title, &sub.Default, &sub.Forced, &sub.SDH); err != nil {
		t.Fatal(err)
	}
	if sub != fakeInfo.Subtitles[0] {
		t.Errorf("subtitle = %+v, want %+v", sub, fakeInfo.Subtitles[0])
	}
}

func TestScanNameFields(t *testing.T) {
	tests := []struct {
		typ  Type
		path string
		want Video
	}{
		{Movies, "Dune (2021)/Dune (2021) {edition-IMAX} - 4K.mkv", Video{Title: "Dune", Year: 2021, Edition: "IMAX", Version: "4K"}},
		{TVShows, "Show (2020)/Season 01/Show (2020) - s01e02-e03 - Pilot.mkv",
			Video{Title: "Show", Year: 2020, Season: 1, Episode: 2, EpisodeEnd: 3, EpisodeTitle: "Pilot"}},
		{OtherVideos, "Trips/2024/Beach.mp4", Video{Title: "Beach", Group: "Trips/2024"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			l := newTestLib(t, tt.typ)
			l.write(tt.path, "x")
			l.scan()
			var v Video
			if err := l.db.QueryRow(`SELECT title, year, edition, version, season, episode, episode_end,
				episode_title, group_name FROM videos WHERE path = ?`, tt.path).Scan(
				&v.Title, &v.Year, &v.Edition, &v.Version, &v.Season, &v.Episode, &v.EpisodeEnd, &v.EpisodeTitle, &v.Group,
			); err != nil {
				t.Fatal(err)
			}
			if v != tt.want {
				t.Errorf("got %+v, want %+v", v, tt.want)
			}
		})
	}
}

func TestScanMissingAndBack(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	before := l.videos()["Heat (1995).mkv"]

	if err := os.Remove(filepath.Join(l.dir, "Heat (1995).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	gone := l.videos()["Heat (1995).mkv"]
	if !gone.Missing || gone.ID != before.ID || gone.Title != "Heat" {
		t.Errorf("after delete: %+v, want the same row, missing, title kept", gone)
	}

	l.write("Heat (1995).mkv", "heat")
	l.scan()
	back := l.videos()["Heat (1995).mkv"]
	if back.Missing || back.ID != before.ID {
		t.Errorf("after putting it back: %+v, want the same row, not missing", back)
	}
}

func TestScanRename(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	if err := os.Rename(filepath.Join(l.dir, "Heat (1995).mkv"), filepath.Join(l.dir, "Heat (1995) - 1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()

	videos := l.videos()
	old, renamed := videos["Heat (1995).mkv"], videos["Heat (1995) - 1080p.mkv"]
	if !old.Missing {
		t.Errorf("old path: %+v, want missing", old)
	}
	if renamed.ID == 0 || renamed.ID == old.ID || renamed.Missing {
		t.Errorf("new path: %+v, want a new row, not missing", renamed)
	}
}

func TestScanReprobesOnlyChanges(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()
	l.scan()
	if got := l.prober.calls; got["Heat (1995).mkv"] != 1 || got["Ronin (1998).mkv"] != 1 {
		t.Fatalf("probes after two scans = %v, want one each", got)
	}

	// New mtime, same size.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(l.dir, "Heat (1995).mkv"), later, later); err != nil {
		t.Fatal(err)
	}
	// New size, and back to the same mtime.
	p := filepath.Join(l.dir, "Ronin (1998).mkv")
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	l.write("Ronin (1998).mkv", "ronin, longer")
	if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	l.scan()
	if got := l.prober.calls; got["Heat (1995).mkv"] != 2 || got["Ronin (1998).mkv"] != 2 {
		t.Errorf("probes after changes = %v, want two each", got)
	}
}

func TestScanRetriesFailedProbe(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.prober.fail["Heat (1995).mkv"] = true
	l.scan()
	if v := l.videos()["Heat (1995).mkv"]; v.ProbeError != "broken file" || v.Missing {
		t.Fatalf("after a failed probe: %+v, want the error stored", v)
	}
	var unplayable string
	if err := l.db.QueryRow("SELECT unplayable FROM videos").Scan(&unplayable); err != nil {
		t.Fatal(err)
	}
	if unplayable != string(media.UnplayableProbeFailed) {
		t.Errorf("unplayable = %q, want %q", unplayable, media.UnplayableProbeFailed)
	}

	l.scan()
	if n := l.prober.calls["Heat (1995).mkv"]; n != 2 {
		t.Fatalf("probes = %d, want a retry on the next scan", n)
	}

	l.prober.fail["Heat (1995).mkv"] = false
	l.scan()
	if v := l.videos()["Heat (1995).mkv"]; v.ProbeError != "" || v.Codec != "avc1.640028" {
		t.Errorf("after a good probe: %+v, want the error cleared", v)
	}
	l.scan()
	if n := l.prober.calls["Heat (1995).mkv"]; n != 3 {
		t.Errorf("probes = %d, want no probe once it succeeded", n)
	}
}

func TestScanFailedProbeClearsTracks(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	l.prober.fail["Heat (1995).mkv"] = true
	l.write("Heat (1995).mkv", "heat, now broken")
	l.scan()
	var n int
	if err := l.db.QueryRow("SELECT (SELECT count(*) FROM audio_tracks) + (SELECT count(*) FROM subtitle_tracks)").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d tracks left after a failed probe, want 0", n)
	}
}

// A row written by an older parser gets the name the current one gives, without a probe.
func TestScanRefreshesNameFields(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	if _, err := l.db.Exec("UPDATE videos SET title = 'Old parse', year = 0"); err != nil {
		t.Fatal(err)
	}
	l.scan()
	if v := l.videos()["Heat (1995).mkv"]; v.Title != "Heat" || v.Year != 1995 {
		t.Errorf("got %+v, want title Heat, year 1995", v)
	}
	if n := l.prober.calls["Heat (1995).mkv"]; n != 1 {
		t.Errorf("probes = %d, want 1", n)
	}
}

// The library folder itself may be a symlink; links inside it are still not followed.
func TestScanSymlinkedLibraryFolder(t *testing.T) {
	l := newTestLib(t, Movies)
	target := filepath.Join(filepath.Dir(l.dir), "target")
	if err := os.Rename(l.dir, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, l.dir); err != nil {
		t.Fatal(err)
	}
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	if v, ok := l.videos()["Heat (1995).mkv"]; !ok || v.Missing {
		t.Errorf("Heat = %+v, found %v; want found, not missing", v, ok)
	}
}

// A library folder that is a symlink, pointed outside the media folder after it was added, is not
// walked, and its videos don't turn missing.
func TestScanSymlinkLeadsOutside(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "Ronin (1998).mkv"), []byte("ronin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(l.dir, l.dir+".away"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, l.dir); err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); !errors.Is(err, ErrBadPath) {
		t.Errorf("err = %v, want ErrBadPath", err)
	}
	if got := l.videos(); len(got) != 1 || got["Heat (1995).mkv"].Missing {
		t.Errorf("videos = %+v, want only Heat, not missing", got)
	}
}

func TestScanLibraryIsAFile(t *testing.T) {
	l := newTestLib(t, Movies)
	if err := os.Remove(l.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.dir, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); err == nil {
		t.Error("want an error")
	}
}

// An unplugged drive must not turn the whole library missing.
func TestScanMissingRoot(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.scan()
	if err := os.Rename(l.dir, l.dir+".away"); err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); err == nil {
		t.Error("scan of a missing library folder: want an error")
	}
	if v := l.videos()["Heat (1995).mkv"]; v.Missing {
		t.Errorf("Heat = %+v, want not missing", v)
	}
}

func TestScanUnreadableFolder(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()

	locked := filepath.Join(l.dir, "Heat (1995)")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if err := os.Remove(filepath.Join(l.dir, "Ronin (1998).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()

	videos := l.videos()
	if v := videos["Heat (1995)/Heat (1995).mkv"]; v.Missing {
		t.Errorf("Heat, in the unreadable folder = %+v, want not missing", v)
	}
	if v := videos["Ronin (1998).mkv"]; !v.Missing {
		t.Errorf("Ronin, deleted = %+v, want missing", v)
	}
}

// Shutdown cancels the scan. That's not the file's fault, so no probe error is stored.
func TestScanCancelled(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	ctx, cancel := context.WithCancel(context.Background())
	// Like shutdown mid-probe: exec.CommandContext kills ffprobe, and the probe fails.
	l.scanner.Prober = proberFunc(func(context.Context, string) (media.Info, error) {
		cancel()
		return media.Info{}, errors.New("signal: killed")
	})
	if err := l.scanner.ScanLibrary(ctx, l.lib, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if videos := l.videos(); len(videos) != 0 {
		t.Errorf("videos = %v, want none written", videos)
	}
}

// The whole path with real ffprobe, on the clips task testdata makes.
func TestScanTestdata(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "media"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".done")); err != nil {
		t.Fatalf("test clips missing, run task testdata: %v", err)
	}
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scanner := &Scanner{DB: db, Root: root, Prober: media.FFprobe{}}
	for i, lib := range []Library{{Path: "Movies", Type: Movies}, {Path: "TV", Type: TVShows}} {
		lib.ID = int64(i + 1)
		if _, err := db.Exec("INSERT INTO libraries (id, path, type) VALUES (?, ?, ?)", lib.ID, lib.Path, lib.Type); err != nil {
			t.Fatal(err)
		}
		if err := scanner.ScanLibrary(ctx, lib, nil); err != nil {
			t.Fatal(err)
		}
	}

	want := map[string]string{ // title → codec string prefix, or unplayable reason
		"Stereo Test":    "avc1.6400",
		"Surround Test":  "avc1.6400",
		"Seven One Test": "avc1.6400",
		"Two Audio Test": "avc1.6400",
		"Ten Bit Test":   string(media.UnplayableH264Profile),
		"Test Show":      "hvc1.1.6.L",
	}
	rows, err := db.Query("SELECT title, codec_string, COALESCE(unplayable, ''), COALESCE(probe_error, '') FROM videos")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := 0
	for rows.Next() {
		var title, codec, unplayable, probeErr string
		if err := rows.Scan(&title, &codec, &unplayable, &probeErr); err != nil {
			t.Fatal(err)
		}
		got++
		w, ok := want[title]
		switch {
		case !ok:
			t.Errorf("unexpected video %q", title)
		case probeErr != "":
			t.Errorf("%s: probe error %s", title, probeErr)
		case !strings.HasPrefix(codec+unplayable, w):
			t.Errorf("%s: codec %q, unplayable %q, want %q", title, codec, unplayable, w)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got != len(want) {
		t.Errorf("got %d videos, want %d", got, len(want))
	}
}
