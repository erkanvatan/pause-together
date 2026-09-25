package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clip returns the absolute path of a clip made by task testdata. Missing clips fail the test instead
// of skipping it, so the real-ffmpeg tests never pass by not running.
func clip(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", "media", rel))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("test clip missing, run task testdata: %v", err)
	}
	return p
}

func TestFFprobeClips(t *testing.T) {
	tests := []struct {
		clip       string
		codec      string // CodecString prefix: the level depends on the encoder
		unplayable Unplayable
		channels   int
	}{
		{"Movies/Stereo Test (2020)/Stereo Test (2020).mkv", "avc1.6400", "", 2},
		{"Movies/Surround Test (2021).mkv", "avc1.6400", "", 6},
		{"Movies/Seven One Test (2022).mkv", "avc1.6400", "", 8},
		{"Movies/Ten Bit Test (2023).mkv", "", UnplayableH264Profile, 2},
		{"TV/Test Show (2024)/Season 01/Test Show (2024) - s01e01 - Pilot.mkv", "hvc1.1.6.L", "", 2},
	}
	for _, tt := range tests {
		t.Run(tt.clip, func(t *testing.T) {
			info, err := FFprobe{}.Probe(context.Background(), clip(t, tt.clip))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(info.CodecString, tt.codec) || tt.codec == "" && info.CodecString != "" {
				t.Errorf("codec string = %q, want prefix %q", info.CodecString, tt.codec)
			}
			if info.Unplayable != tt.unplayable {
				t.Errorf("unplayable = %q, want %q", info.Unplayable, tt.unplayable)
			}
			if len(info.Audio) != 1 || info.Audio[0].Channels != tt.channels {
				t.Errorf("audio = %+v, want one track with %d channels", info.Audio, tt.channels)
			}
			if info.Duration <= 0 {
				t.Errorf("duration = %v, want > 0", info.Duration)
			}
		})
	}
}

func TestFFprobeSubtitleTrack(t *testing.T) {
	info, err := FFprobe{}.Probe(context.Background(), clip(t, "Movies/Stereo Test (2020)/Stereo Test (2020).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleTrack{{Stream: 2, Codec: "subrip", Lang: "eng"}}
	if len(info.Subtitles) != 1 || info.Subtitles[0] != want[0] {
		t.Errorf("subtitles = %+v, want %+v", info.Subtitles, want)
	}
}

// A relative "-x.mkv" or "concat:x.mkv" would be read as an option or a protocol.
func TestFFprobeRefusesRelativePath(t *testing.T) {
	for _, p := range []string{"x.mkv", "-x.mkv", "concat:x.mkv"} {
		if _, err := (FFprobe{}).Probe(context.Background(), p); err == nil || !strings.Contains(err.Error(), "not absolute") {
			t.Errorf("%q: err = %v, want a not-absolute error", p, err)
		}
	}
}

func TestFFprobeError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "broken.mkv")
	if err := os.WriteFile(p, []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := FFprobe{}.Probe(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "Invalid data") {
		t.Errorf("err = %v, want ffprobe's message", err)
	}
}
