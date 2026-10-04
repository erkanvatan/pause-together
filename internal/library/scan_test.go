package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
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
	Subtitles: []media.SubtitleTrack{
		{Stream: 3, Codec: "subrip", Lang: "eng", Forced: true},
		{Stream: 4, Codec: "hdmv_pgs_subtitle", Lang: "tur", Unavailable: media.SubtitleImage},
	},
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

// fakeSubtitler keeps its copies as a set of keys. It counts conversions per file name and fails for
// the names in fail. cacheErr, if set, fails every conversion as if the cache couldn't be written.
type fakeSubtitler struct {
	copies   map[string]bool
	calls    map[string]int
	fail     map[string]bool
	cacheErr error
}

func newFakeSubtitler() *fakeSubtitler {
	return &fakeSubtitler{copies: map[string]bool{}, calls: map[string]int{}, fail: map[string]bool{}}
}

func (f *fakeSubtitler) Sidecar(_ context.Context, src, _, key string) error {
	if !filepath.IsAbs(src) {
		return errors.New("path not absolute")
	}
	if f.copies[key] {
		return nil
	}
	name := filepath.Base(src)
	f.calls[name]++
	switch {
	case f.fail[name]:
		return fmt.Errorf("%w: no cues found", media.ErrUnreadable)
	case f.cacheErr != nil:
		return f.cacheErr
	}
	f.copies[key] = true
	return nil
}

func (f *fakeSubtitler) Remove(key string) error {
	delete(f.copies, key)
	return nil
}

type proberFunc func(context.Context, string) (media.Info, error)

func (f proberFunc) Probe(ctx context.Context, path string) (media.Info, error) { return f(ctx, path) }

type testLib struct {
	t       *testing.T
	db      *sql.DB
	scanner *Scanner
	prober  *fakeProber
	subs    *fakeSubtitler
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
	prober, subs := newFakeProber(), newFakeSubtitler()
	return &testLib{
		t: t, db: db, prober: prober, subs: subs, lib: lib, dir: dir,
		scanner: &Scanner{DB: db, Root: root, Prober: prober, Subtitles: subs},
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

	var subs []media.SubtitleTrack
	rows, err = l.db.Query(`SELECT stream, codec, lang, title, is_default, forced, sdh, COALESCE(unavailable, '')
		FROM subtitle_tracks ORDER BY stream`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sub media.SubtitleTrack
		if err := rows.Scan(&sub.Stream, &sub.Codec, &sub.Lang, &sub.Title, &sub.Default, &sub.Forced, &sub.SDH,
			&sub.Unavailable); err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}
	_ = rows.Close()
	if !reflect.DeepEqual(subs, fakeInfo.Subtitles) {
		t.Errorf("subtitles = %+v, want %+v", subs, fakeInfo.Subtitles)
	}
}

type savedSidecar struct {
	ID    int64
	Video string // its video's path
	Lang  string
	Flags string // "forced", "sdh", both or ""
	Key   string
}

// sidecars returns the library's sidecar subtitle rows by their file's path.
func (l *testLib) sidecars() map[string]savedSidecar {
	l.t.Helper()
	rows, err := l.db.Query(`SELECT s.id, v.path, s.name, s.lang, s.forced, s.sdh, s.cache_key
		FROM sidecar_subtitles s JOIN videos v ON v.id = s.video_id WHERE v.library_id = ?`, l.lib.ID)
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]savedSidecar{}
	for rows.Next() {
		var r savedSidecar
		var name string
		var forced, sdh bool
		if err := rows.Scan(&r.ID, &r.Video, &name, &r.Lang, &forced, &sdh, &r.Key); err != nil {
			l.t.Fatal(err)
		}
		switch {
		case forced && sdh:
			r.Flags = "forced sdh"
		case forced:
			r.Flags = "forced"
		case sdh:
			r.Flags = "sdh"
		}
		got[path.Join(path.Dir(r.Video), name)] = r
	}
	if err := rows.Err(); err != nil {
		l.t.Fatal(err)
	}
	return got
}

// key is the sidecar key of a file in the library as it is on disk now.
func (l *testLib) key(rel string) string {
	l.t.Helper()
	st, err := os.Stat(filepath.Join(l.dir, rel))
	if err != nil {
		l.t.Fatal(err)
	}
	return media.SidecarKey(l.lib.ID, rel, st.Size(), st.ModTime().UnixNano())
}

func TestScanSidecars(t *testing.T) {
	l := newTestLib(t, Movies)
	const video, en, tr = "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Heat (1995).en.srt", "Heat (1995)/Heat (1995).tur.forced.srt"
	l.write(video, "heat")
	l.write(en, "english")
	l.write(tr, "türkçe")
	l.write("Heat (1995)/Heat (1995).sdh.srt", "no language: skipped")
	l.scan()

	got := l.sidecars()
	want := map[string]savedSidecar{
		en: {Video: video, Lang: "en", Key: l.key(en)},
		tr: {Video: video, Lang: "tur", Flags: "forced", Key: l.key(tr)},
	}
	for rel, w := range want {
		g := got[rel]
		w.ID = g.ID
		if g != w || !l.subs.copies[w.Key] {
			t.Errorf("%s: row %+v, want %+v; copy made: %v", rel, g, w, l.subs.copies[w.Key])
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d sidecars, want %d: %v", len(got), len(want), got)
	}

	t.Run("unchanged: not converted again", func(t *testing.T) {
		l.scan()
		if n := l.subs.calls["Heat (1995).en.srt"]; n != 1 {
			t.Errorf("converted %d times, want 1", n)
		}
		if again := l.sidecars(); !reflect.DeepEqual(again, got) {
			t.Errorf("rows changed:\n%v\nwant\n%v", again, got)
		}
	})

	t.Run("changed: new copy, old one removed, same row", func(t *testing.T) {
		oldKey := got[en].Key
		l.write(en, "english, fixed")
		l.scan()
		row := l.sidecars()[en]
		if row.ID != got[en].ID || row.Key != l.key(en) || row.Key == oldKey {
			t.Errorf("row = %+v, want id %d with the new key", row, got[en].ID)
		}
		if l.subs.copies[oldKey] || !l.subs.copies[row.Key] {
			t.Errorf("copies = %v, want only the new key", l.subs.copies)
		}
	})

	t.Run("deleted: row and copy gone", func(t *testing.T) {
		key := got[tr].Key
		if err := os.Remove(filepath.Join(l.dir, tr)); err != nil {
			t.Fatal(err)
		}
		l.scan()
		if _, ok := l.sidecars()[tr]; ok || l.subs.copies[key] {
			t.Errorf("row or copy still there")
		}
	})

	t.Run("cache error: row kept, not listed", func(t *testing.T) {
		before := l.sidecars()[en]
		l.subs.cacheErr = errors.New("no space left on device")
		l.write(en, "english, fixed again")
		l.scan()
		l.subs.cacheErr = nil
		if after := l.sidecars()[en]; after != before {
			t.Errorf("row = %+v, want it unchanged: %+v", after, before)
		}
		if _, ok := l.skipped()[en]; ok {
			t.Errorf("listed as skipped")
		}
	})

	t.Run("can't convert: listed, row and copy gone", func(t *testing.T) {
		key := l.sidecars()[en].Key
		l.subs.fail["Heat (1995).en.srt"] = true
		l.write(en, "broken")
		l.scan()
		if _, ok := l.sidecars()[en]; ok || l.subs.copies[key] {
			t.Errorf("row or copy still there")
		}
		if r := l.skipped()[en]; r != string(ReasonSubUnreadable) {
			t.Errorf("skipped reason = %q, want %q", r, ReasonSubUnreadable)
		}

		delete(l.subs.fail, "Heat (1995).en.srt")
		l.scan()
		if _, ok := l.sidecars()[en]; !ok {
			t.Errorf("not back after a good scan")
		}
		if _, ok := l.skipped()[en]; ok {
			t.Errorf("still listed as skipped")
		}
	})
}

// A video renamed (mkv to mp4) is a new video. Its sidecar keeps its name, row and copy, and follows it.
func TestScanSidecarFollowsRenamedVideo(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Heat (1995).en.srt", "english")
	l.scan()
	before := l.sidecars()["Heat (1995).en.srt"]

	if err := os.Rename(filepath.Join(l.dir, "Heat (1995).mkv"), filepath.Join(l.dir, "Heat (1995).mp4")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	after := l.sidecars()["Heat (1995).en.srt"]
	if after.ID != before.ID || after.Key != before.Key || after.Video != "Heat (1995).mp4" {
		t.Errorf("after = %+v, want %+v on the mp4", after, before)
	}
	if n := l.subs.calls["Heat (1995).en.srt"]; n != 1 {
		t.Errorf("converted %d times, want 1", n)
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
	l.write("Ronin (1998).mkv", "ronin") // an emptied folder is a gone folder, not a missing video
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

// An unmounted drive leaves its mount point behind, empty: that's a gone folder too, not a library
// whose every video went missing.
func TestScanEmptiedFolder(t *testing.T) {
	l := newTestLib(t, Movies)
	l.scan() // a new library may be empty
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.scan()
	if err := os.RemoveAll(filepath.Join(l.dir, "Heat (1995)")); err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.ScanLibrary(context.Background(), l.lib, nil); !errors.Is(err, ErrFolderGone) {
		t.Errorf("scan of an emptied library folder: err = %v, want ErrFolderGone", err)
	}
	if v := l.videos()["Heat (1995)/Heat (1995).mkv"]; v.Missing {
		t.Errorf("Heat = %+v, want not missing", v)
	}
}

// The media folder gone is not the library's folder gone: removing the library wouldn't help.
func TestScanMissingMediaFolder(t *testing.T) {
	l := newTestLib(t, Movies)
	l.scanner.Root = filepath.Join(l.scanner.Root, "away")
	err := l.scanner.ScanLibrary(context.Background(), l.lib, nil)
	if err == nil || errors.Is(err, ErrFolderGone) {
		t.Errorf("err = %v, want an error that isn't ErrFolderGone", err)
	}
}

func TestScanUnreadableFolder(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.write("Heat (1995)/Heat (1995).en.srt", "english")
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
	if _, ok := l.sidecars()["Heat (1995)/Heat (1995).en.srt"]; !ok {
		t.Errorf("Heat's subtitle, in the unreadable folder: row deleted")
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
	cache := t.TempDir()
	scanner := &Scanner{DB: db, Root: root, Prober: media.FFprobe{}, Subtitles: media.Subtitles{Dir: cache}}
	for i, lib := range []Library{{Path: "Movies", Type: Movies}, {Path: "TV", Type: TVShows}, {Path: "Other", Type: OtherVideos}} {
		lib.ID = int64(i + 1)
		if _, err := db.Exec("INSERT INTO libraries (id, path, type) VALUES (?, ?, ?)", lib.ID, lib.Path, lib.Type); err != nil {
			t.Fatal(err)
		}
		if err := scanner.ScanLibrary(ctx, lib, nil); err != nil {
			t.Fatal(err)
		}
	}

	const h264, show = "avc1.6400", "TV/Test Show (2024)/"
	checkPrefixes(t, "videos", queryPairs(t, db, `SELECT l.path || '/' || v.path, v.codec_string || COALESCE(v.unplayable, '')
		FROM videos v JOIN libraries l ON l.id = v.library_id`), map[string]string{ // path → codec string prefix, or unplayable reason
		"Movies/Stereo Test (2020)/Stereo Test (2020).mkv":                              h264,
		"Movies/Surround Test (2021).mkv":                                               h264,
		"Movies/Seven One Test (2022).mkv":                                              h264,
		"Movies/Two Audio Test (2025).mkv":                                              h264,
		"Movies/Ten Bit Test (2023).mkv":                                                string(media.UnplayableH264Profile),
		"Movies/VP9 Test (2015).webm":                                                   "vp09.00.",
		"Movies/AV1 Test (2014).mp4":                                                    "av01.0.",
		"Movies/VP8 Test (2013).webm":                                                   string(media.UnplayableCodec),
		"Movies/MPEG-2 Test (2012).ts":                                                  string(media.UnplayableCodec),
		"Movies/No Video Test (2011).mkv":                                               string(media.UnplayableNoVideo),
		"Movies/Broken Test (2010).mkv":                                                 string(media.UnplayableProbeFailed),
		"Movies/Subtitle Tracks Test (2009).mkv":                                        h264,
		"Movies/Versions Test (2019)/Versions Test (2019).mkv":                          h264,
		"Movies/Versions Test (2019)/Versions Test (2019) {edition-Director's Cut}.mkv": h264,
		"Movies/Versions Test (2019)/Versions Test (2019) - 4K.mkv":                     h264,
		"Movies/Test Collection/Collection Test (2018).mp4":                             h264,
		"Movies/Shorts/Shorts Collection Test (2017).mkv":                               h264,
		"Movies/Sidecar Test (2008)/Sidecar Test (2008).mkv":                            h264,
		show + "Season 01/Test Show (2024) - s01e01 - Pilot.mkv":                        "hvc1.1.6.L",
		show + "Season 01/Test Show (2024) - s01e02-e03 - Double.mkv":                   h264,
		show + "Season 2/Test Show (2024) - s02e01.mkv":                                 h264,
		show + "Test Show (2024) - s02e02 - Loose.mkv":                                  h264,
		show + "Specials/Test Show (2024) - s00e01 - Special.mkv":                       h264,
		"TV/No Year Show/Season 01/No Year Show - s01e01.mkv":                           h264,
		"Other/Root Clip.mp4":                                                           h264,
		"Other/Silent Clip.mkv":                                                         h264,
		"Other/Trailers/Trailer Clip.mkv":                                               h264,
		"Other/Holidays/2023/Beach.mkv":                                                 h264,
	})

	sidecar := "Movies/Sidecar Test (2008)/"
	checkPrefixes(t, "skipped files", queryPairs(t, db, `SELECT l.path || '/' || s.path, s.reason
		FROM skipped_files s JOIN libraries l ON l.id = s.library_id`), map[string]string{
		"Movies/No Year Test.mkv":                               string(ReasonMovieNoYear),
		"Movies/Split Test (2016)/Split Test (2016) - pt1.mkv":  string(ReasonMovieSplit),
		"Movies/Split Test (2016)/Split Test (2016) - cd2.mkv":  string(ReasonMovieSplit),
		sidecar + "Sidecar Test (2008).srt":                     string(ReasonSubNoLang),
		sidecar + "Sidecar Test (2008).english.srt":             string(ReasonSubBadName),
		sidecar + "Nothing Here (2008).en.srt":                  string(ReasonSubNoVideo),
		sidecar + "Sidecar Test (2008).zh.srt":                  string(ReasonSubUnreadable),
		sidecar + "Sidecar Test (2008).fr.srt":                  string(ReasonSubUnreadable),
		"TV/Loose Show - s01e01.mkv":                            string(ReasonTVNoShowFolder),
		show + "Season 01/Disc 1/Test Show (2024) - s01e04.mkv": string(ReasonTVBadFolder),
		show + "Season 01/Test Show (2024) - 05.mkv":            string(ReasonTVNoEpisode),
		show + "Test Show (2024) - 2024-05-01.mkv":              string(ReasonTVDate),
	})

	// The sidecars, converted with real ffmpeg.
	checkPrefixes(t, "sidecars", queryPairs(t, db, `SELECT name,
		lang || IIF(forced, ' forced', '') || IIF(sdh, ' sdh', '') || ' ' || cache_key FROM sidecar_subtitles`), map[string]string{
		"Stereo Test (2020).tr.srt":         "tr ",
		"Stereo Test (2020).tur.srt":        "tur ",
		"Stereo Test (2020).en.srt":         "en ",
		"Stereo Test (2020).en.sdh.ass":     "en sdh ",
		"Sidecar Test (2008).en.srt":        "en ",
		"Sidecar Test (2008).en.forced.srt": "en forced ",
		"Sidecar Test (2008).en.hi.srt":     "en sdh ",
		"Sidecar Test (2008).hi.srt":        "hi ",
		"Sidecar Test (2008).de.vtt":        "de ",
	})
	keys, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if _, err := os.Stat(filepath.Join(cache, k.Name(), "subtitle.vtt")); err != nil {
			t.Errorf("sidecar copy %s: %v", k.Name(), err)
		}
	}
	if len(keys) != 9 {
		t.Errorf("got %d sidecar copies, want 9", len(keys))
	}
}

// queryPairs runs a query of two text columns and returns them as a map.
func queryPairs(t *testing.T, db *sql.DB, query string) map[string]string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		got[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

// checkPrefixes wants the same keys in got and want, each value in got starting with want's.
func checkPrefixes(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	for k, v := range got {
		if w, ok := want[k]; !ok {
			t.Errorf("%s: unexpected %q", what, k)
		} else if !strings.HasPrefix(v, w) {
			t.Errorf("%s: %q = %q, want prefix %q", what, k, v, w)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("%s: missing %q", what, k)
		}
	}
}
