package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/erkanvatan/pause-together/internal/media"
)

func TestPrepareJob(t *testing.T) {
	ctx := context.Background()
	l := newTestLib(t, Movies)
	l.write("Heat (1995)/Heat (1995).mkv", "heat")
	l.scan()
	id := l.videos()["Heat (1995)/Heat (1995).mkv"].ID
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}
	info, err := os.Stat(filepath.Join(l.dir, "Heat (1995)/Heat (1995).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	stream := func(n int) *int { return &n }

	tests := []struct {
		name        string
		audio       *int
		wantAudio   *media.AudioTrack
		wantMissing bool
	}{
		{"5.1 track", stream(1), &fakeInfo.Audio[0], false},
		{"no audio", nil, nil, false},
		{"track gone since the pick", stream(9), &media.AudioTrack{Stream: 9}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, missing, err := libs.PrepareJob(ctx, id, tt.audio)
			if err != nil {
				t.Fatal(err)
			}
			want := media.Job{
				VideoID:    id,
				Name:       "Library/Heat (1995)/Heat (1995).mkv",
				Source:     filepath.Join(l.dir, "Heat (1995)/Heat (1995).mkv"),
				VideoCodec: "h264",
				Duration:   fakeInfo.Duration,
				Size:       info.Size(),
				Mtime:      info.ModTime().UnixNano(),
				Audio:      tt.wantAudio,
				Subtitles:  []int{3}, // the PGS track is unavailable
			}
			if !reflect.DeepEqual(j, want) {
				t.Errorf("job = %+v\nwant %+v", j, want)
			}
			if missing != tt.wantMissing {
				t.Errorf("missing = %v, want %v", missing, tt.wantMissing)
			}
		})
	}

	// A library folder that is gone leaves the file out of reach.
	if err := os.RemoveAll(l.dir); err != nil {
		t.Fatal(err)
	}
	if _, missing, err := libs.PrepareJob(ctx, id, stream(1)); err != nil || !missing {
		t.Errorf("library folder gone: missing = %v, err = %v; want true, nil", missing, err)
	}

	if _, _, err := libs.PrepareJob(ctx, id+100, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown video: err = %v, want ErrNotFound", err)
	}
}

func TestVideo(t *testing.T) {
	ctx := context.Background()
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin") // an emptied folder is a gone folder, not a missing video
	l.scan()
	id := l.videos()["Heat (1995).mkv"].ID
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}

	if err := os.Remove(filepath.Join(l.dir, "Heat (1995).mkv")); err != nil {
		t.Fatal(err)
	}
	l.scan()
	got, err := libs.Video(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Missing || got.Title != "Heat" || got.Year != 1995 {
		t.Errorf("got %+v, want Heat (1995), missing", got)
	}
	if _, err := libs.Video(ctx, id+100); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown video: err = %v, want ErrNotFound", err)
	}
}
