package media

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPrepareArgs(t *testing.T) {
	const src, dir = "/media/Movies/A (2020).mkv", "/data/cache/k.tmp"
	head := []string{"-nostdin", "-v", "error", "-progress", "pipe:1", "-nostats", "-fflags", "+genpts", "-i", src,
		"-map", "0:V:0"}
	tail := []string{"-movflags", "+faststart", "-f", "mp4", dir + "/video.mp4"}
	reencode := []string{"-c:a", "aac", "-b:a", "192k"}

	tests := []struct {
		name  string
		codec string
		audio *AudioTrack
		want  []string // between head and tail
		subs  []int
		after []string // after tail
	}{
		{"stereo AAC copied", "h264", &AudioTrack{Stream: 1, Codec: "aac", Channels: 2},
			[]string{"-map", "0:1", "-c:v", "copy", "-c:a", "copy"}, nil, nil},
		{"mono AAC copied", "h264", &AudioTrack{Stream: 1, Codec: "aac", Channels: 1},
			[]string{"-map", "0:1", "-c:v", "copy", "-c:a", "copy"}, nil, nil},
		{"HEVC tagged hvc1", "hevc", &AudioTrack{Stream: 1, Codec: "aac", Channels: 2},
			[]string{"-map", "0:1", "-c:v", "copy", "-tag:v", "hvc1", "-c:a", "copy"}, nil, nil},
		{"stereo AC3 to AAC", "h264", &AudioTrack{Stream: 1, Codec: "ac3", Channels: 2},
			slices.Concat([]string{"-map", "0:1", "-c:v", "copy"}, reencode, []string{"-ac", "2"}), nil, nil},
		{"mono MP3 to stereo AAC", "vp9", &AudioTrack{Stream: 1, Codec: "mp3", Channels: 1},
			slices.Concat([]string{"-map", "0:1", "-c:v", "copy"}, reencode, []string{"-ac", "2"}), nil, nil},
		{"5.1 AC3 downmixed", "h264", &AudioTrack{Stream: 1, Codec: "ac3", Channels: 6, Layout: "5.1(side)"},
			slices.Concat([]string{"-map", "0:1", "-c:v", "copy", "-af", downmix}, reencode), nil, nil},
		{"7.1 AAC downmixed, not copied", "av1", &AudioTrack{Stream: 1, Codec: "aac", Channels: 8, Layout: "7.1"},
			slices.Concat([]string{"-map", "0:1", "-c:v", "copy", "-af", downmix}, reencode), nil, nil},
		{"chosen stream index mapped", "h264", &AudioTrack{Stream: 3, Codec: "aac", Channels: 2},
			[]string{"-map", "0:3", "-c:v", "copy", "-c:a", "copy"}, nil, nil},
		{"no audio", "h264", nil, []string{"-c:v", "copy"}, nil, nil},
		{"subtitles as more outputs", "h264", &AudioTrack{Stream: 1, Codec: "aac", Channels: 2},
			[]string{"-map", "0:1", "-c:v", "copy", "-c:a", "copy"}, []int{2, 5},
			[]string{"-map", "0:2", "-c:s", "webvtt", "-f", "webvtt", dir + "/2.vtt",
				"-map", "0:5", "-c:s", "webvtt", "-f", "webvtt", dir + "/5.vtt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prepareArgs(Job{Source: src, VideoCodec: tt.codec, Audio: tt.audio, Subtitles: tt.subs}, dir)
			want := slices.Concat(head, tt.want, tail, tt.after)
			if !slices.Equal(got, want) {
				t.Errorf("args =\n  %q\nwant\n  %q", got, want)
			}
		})
	}
}

func TestJobKey(t *testing.T) {
	base := Job{VideoID: 1, Size: 100, Mtime: 200, Audio: &AudioTrack{Stream: 1}, Name: "a", Source: "/a"}
	key := base.Key()
	if !validKey(key) {
		t.Fatalf("key %q is not 32 lowercase hex", key)
	}

	same := base
	same.Name, same.Source, same.Duration = "b", "/b", time.Hour // not part of the key
	same.Audio = &AudioTrack{Stream: 1, Codec: "ac3"}
	same.Subtitles = []int{2}
	if same.Key() != key {
		t.Errorf("key changed with fields outside the key")
	}

	changes := map[string]func(*Job){
		"video":    func(j *Job) { j.VideoID = 2 },
		"audio":    func(j *Job) { j.Audio = &AudioTrack{Stream: 2} },
		"no audio": func(j *Job) { j.Audio = nil },
		"size":     func(j *Job) { j.Size = 101 },
		"mtime":    func(j *Job) { j.Mtime = 201 },
	}
	for name, change := range changes {
		j := base
		change(&j)
		if j.Key() == key {
			t.Errorf("%s: key didn't change", name)
		}
	}
}

func TestParseProgress(t *testing.T) {
	tests := []struct {
		line string
		want time.Duration
		ok   bool
	}{
		{"out_time_us=1500000", 1500 * time.Millisecond, true},
		{"out_time_us=N/A", 0, false},
		{"out_time_ms=1500000", 0, false},
		{"progress=continue", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseProgress(tt.line)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: got %v, %v; want %v, %v", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFFmpegRefusesRelativePaths(t *testing.T) {
	for _, j := range []struct{ src, dir string }{{"-x.mkv", "/tmp/k.tmp"}, {"/x.mkv", "-k.tmp"}} {
		err := FFmpeg{}.Prepare(context.Background(), Job{Source: j.src}, j.dir, nil)
		if err == nil || !strings.Contains(err.Error(), "not absolute") {
			t.Errorf("%q → %q: err = %v, want a not-absolute error", j.src, j.dir, err)
		}
	}
}

// probed is the part of ffprobe's JSON the real-ffmpeg tests check.
type probed struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		CodecName string            `json:"codec_name"`
		CodecTag  string            `json:"codec_tag_string"`
		Channels  int               `json:"channels"`
		Tags      map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
	} `json:"format"`
}

func probeOut(t *testing.T, path string) probed {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-of", "json", "-show_format", "-show_streams", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var p probed
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// topBoxes lists the top-level MP4 box types in file order.
func topBoxes(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var boxes []string
	var pos int64
	for {
		var hdr [16]byte
		if _, err := f.ReadAt(hdr[:8], pos); err == io.EOF {
			return boxes
		} else if err != nil {
			t.Fatal(err)
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		if size == 1 { // 64-bit size follows the type
			if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
				t.Fatal(err)
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		}
		if size < 8 {
			t.Fatalf("bad box size %d at %d", size, pos)
		}
		boxes = append(boxes, string(hdr[4:8]))
		pos += size
	}
}

// The ffmpeg arguments are where the bugs will be, so these run real ffmpeg on the testdata clips.
func TestFFmpegPrepareClips(t *testing.T) {
	tests := []struct {
		clip     string
		codec    string
		audio    AudioTrack
		wantTag  string // video codec_tag_string; "" = don't check
		wantLang string // audio language; "" = don't check
	}{
		{"Movies/Stereo Test (2020)/Stereo Test (2020).mkv", "h264",
			AudioTrack{Stream: 1, Codec: "aac", Channels: 2}, "", ""},
		{"Movies/Surround Test (2021).mkv", "h264",
			AudioTrack{Stream: 1, Codec: "ac3", Channels: 6, Layout: "5.1(side)"}, "", ""},
		{"Movies/Seven One Test (2022).mkv", "h264",
			AudioTrack{Stream: 1, Codec: "aac", Channels: 8, Layout: "7.1"}, "", ""},
		{"Movies/Two Audio Test (2025).mkv", "h264",
			AudioTrack{Stream: 2, Codec: "ac3", Channels: 6, Layout: "5.1(side)", Lang: "tur"}, "", "tur"},
		{"Movies/VP9 Test (2015).webm", "vp9",
			AudioTrack{Stream: 1, Codec: "opus", Channels: 1, Layout: "mono"}, "", ""},
		{"Movies/AV1 Test (2014).mp4", "av1",
			AudioTrack{Stream: 1, Codec: "aac", Channels: 2}, "", ""},
		{"TV/Test Show (2024)/Season 01/Test Show (2024) - s01e01 - Pilot.mkv", "hevc",
			AudioTrack{Stream: 1, Codec: "aac", Channels: 2}, "hvc1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.clip, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, videoFile)
			job := Job{Source: clip(t, tt.clip), VideoCodec: tt.codec, Duration: time.Second, Audio: &tt.audio}
			var last time.Duration
			if err := (FFmpeg{}).Prepare(context.Background(), job, dir, func(d time.Duration) { last = d }); err != nil {
				t.Fatal(err)
			}
			if last <= 0 {
				t.Errorf("progress never reported")
			}

			p := probeOut(t, out)
			if !strings.Contains(p.Format.FormatName, "mp4") {
				t.Errorf("format = %q, want mp4", p.Format.FormatName)
			}
			var video, audio int
			for _, s := range p.Streams {
				switch s.CodecType {
				case "video":
					video++
					if tt.wantTag != "" && s.CodecTag != tt.wantTag {
						t.Errorf("video tag = %q, want %q", s.CodecTag, tt.wantTag)
					}
				case "audio":
					audio++
					if s.CodecName != "aac" || s.Channels != 2 {
						t.Errorf("audio = %s, %d channels; want aac, 2", s.CodecName, s.Channels)
					}
					if tt.wantLang != "" && s.Tags["language"] != tt.wantLang {
						t.Errorf("audio language = %q, want %q", s.Tags["language"], tt.wantLang)
					}
				default:
					t.Errorf("unexpected %s stream", s.CodecType)
				}
			}
			if video != 1 || audio != 1 {
				t.Errorf("got %d video and %d audio streams, want 1 and 1", video, audio)
			}

			boxes := topBoxes(t, out)
			moov, mdat := slices.Index(boxes, "moov"), slices.Index(boxes, "mdat")
			if moov < 0 || mdat < 0 || moov > mdat {
				t.Errorf("boxes = %v, want moov before mdat (faststart)", boxes)
			}
		})
	}
}

func TestFFmpegError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "broken.mkv")
	if err := os.WriteFile(src, []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := FFmpeg{}.Prepare(context.Background(), Job{Source: src, VideoCodec: "h264"}, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "Invalid data") {
		t.Errorf("err = %v, want ffmpeg's message", err)
	}
}

// The embedded SRT of the stereo clip comes out as WebVTT next to the MP4, in the same run.
func TestFFmpegPrepareSubtitles(t *testing.T) {
	dir := t.TempDir()
	job := Job{Source: clip(t, "Movies/Stereo Test (2020)/Stereo Test (2020).mkv"), VideoCodec: "h264",
		Audio: &AudioTrack{Stream: 1, Codec: "aac", Channels: 2}, Subtitles: []int{2}}
	if err := (FFmpeg{}).Prepare(context.Background(), job, dir, nil); err != nil {
		t.Fatal(err)
	}
	vtt, err := os.ReadFile(filepath.Join(dir, "2.vtt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(vtt), "WEBVTT\n") || !strings.Contains(string(vtt), "\nHello\n") {
		t.Errorf("2.vtt =\n%s\nwant WebVTT with Hello", vtt)
	}
	for _, s := range probeOut(t, filepath.Join(dir, videoFile)).Streams {
		if s.CodecType == "subtitle" {
			t.Errorf("subtitle stream in the MP4")
		}
	}
}
