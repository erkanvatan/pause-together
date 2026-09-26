package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestVideos(t *testing.T) {
	l := newTestLib(t, TVShows)
	l.write("Dark (2017)/Season 01/Dark (2017) - s01e01 - Secrets.mkv", "e1")
	l.write("Dark (2017)/Season 01/Dark (2017) - s01e02-e03.mkv", "e2")
	l.write("Dark (2017)/Season 01/Dark (2017) - s01e04.mkv", "e4")
	l.prober.fail["Dark (2017) - s01e02-e03.mkv"] = true
	l.scan()
	if err := os.Remove(filepath.Join(l.dir, "Dark (2017)/Season 01/Dark (2017) - s01e04.mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	rows := l.videos()
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}

	got, err := libs.Videos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []VideoSummary{
		{ID: rows["Dark (2017)/Season 01/Dark (2017) - s01e01 - Secrets.mkv"].ID, Type: TVShows, Title: "Dark",
			Year: 2017, Season: 1, Episode: 1, EpisodeEnd: 1, EpisodeTitle: "Secrets",
			DurationMs: fakeInfo.Duration.Milliseconds(), CodecString: "avc1.640028"},
		{ID: rows["Dark (2017)/Season 01/Dark (2017) - s01e02-e03.mkv"].ID, Type: TVShows, Title: "Dark",
			Year: 2017, Season: 1, Episode: 2, EpisodeEnd: 3, Unplayable: "probe-failed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}

	// A removed library's videos are missing, so they're left out.
	if err := libs.Remove(context.Background(), l.lib.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := libs.Videos(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("after remove: %+v, %v; want none", got, err)
	}
}

func TestVideoDetail(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.write("Heat (1995)/Heat (1995).tur.srt", "sub")
	l.write("Heat (1995)/Heat (1995).en.forced.srt", "sub")
	l.write("Heat (1995)/Heat (1995).ger.sdh.srt", "sub")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()
	if _, err := l.db.Exec("UPDATE videos SET apple_only = 1 WHERE path = 'Heat (1995)/Heat (1995).mkv'"); err != nil {
		t.Fatal(err)
	}
	rows, sidecars := l.videos(), l.sidecars()
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}
	ctx := context.Background()

	got, err := libs.VideoDetail(ctx, rows["Heat (1995)/Heat (1995).mkv"].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := VideoDetail{
		VideoSummary: VideoSummary{ID: rows["Heat (1995)/Heat (1995).mkv"].ID, Type: Movies, Title: "Heat",
			Year: 1995, DurationMs: fakeInfo.Duration.Milliseconds(), CodecString: "avc1.640028", AppleOnly: true},
		Audio: []AudioInfo{
			{Stream: 1, Codec: "eac3", Channels: 6, Lang: "en", Title: "Surround", Default: true},
			{Stream: 2, Codec: "aac", Channels: 2, Lang: "tr"},
		},
		Subtitles: []SubtitleInfo{
			{Stream: 3, Lang: "en", Forced: true},
			{Stream: 4, Lang: "tr", Unavailable: "image"},
		},
		// By file name.
		Sidecars: []SidecarInfo{
			{ID: sidecars["Heat (1995)/Heat (1995).en.forced.srt"].ID, Lang: "en", Forced: true},
			{ID: sidecars["Heat (1995)/Heat (1995).ger.sdh.srt"].ID, Lang: "de", SDH: true},
			{ID: sidecars["Heat (1995)/Heat (1995).tur.srt"].ID, Lang: "tr"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}

	// No sidecars: an empty list, not nil, so JSON says [].
	ronin, err := libs.VideoDetail(ctx, rows["Ronin (1998).mkv"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if ronin.Sidecars == nil || len(ronin.Sidecars) != 0 {
		t.Errorf("sidecars = %#v, want empty list", ronin.Sidecars)
	}

	if err := os.Remove(filepath.Join(l.dir, "Ronin (1998).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	for name, id := range map[string]int64{"missing": rows["Ronin (1998).mkv"].ID, "unknown": 999} {
		if _, err := libs.VideoDetail(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s video: err = %v, want ErrNotFound", name, err)
		}
	}
}
