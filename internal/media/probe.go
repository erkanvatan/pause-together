// Package media runs ffprobe, decides whether browsers can play a video, and prepares videos into
// cached MP4s with ffmpeg.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// probeTimeout stops one hung file (a network drive gone away) from stalling a whole scan.
const probeTimeout = time.Minute

// Info is what ffprobe says about a video file.
type Info struct {
	Duration    time.Duration
	VideoCodec  string     // ffprobe's name for the main video stream's codec, e.g. "vp8"; "" = none
	CodecString string     // for canPlayType(), e.g. "avc1.640028"; "" when Unplayable is set
	Unplayable  Unplayable // "" = the codec is on the allowlist
	AppleOnly   bool       // Dolby Vision profile 5: other screens show it purple and green
	Audio       []AudioTrack
	Subtitles   []SubtitleTrack
}

// AudioTrack is one audio stream. Stream is ffprobe's stream index, as in ffmpeg's "-map 0:N".
type AudioTrack struct {
	Stream   int
	Codec    string
	Channels int
	Layout   string // e.g. "5.1(side)"
	Lang     string // as tagged, e.g. "eng"; "" = none
	Title    string
	Default  bool
}

// SubtitleTrack is one embedded subtitle stream.
type SubtitleTrack struct {
	Stream  int
	Codec   string // e.g. "subrip", "ass", "hdmv_pgs_subtitle"
	Lang    string
	Title   string
	Default bool
	Forced  bool
	SDH     bool
}

// FFprobe probes files with the ffprobe binary.
type FFprobe struct{}

// Probe runs ffprobe on path, which must be absolute: a relative "-x.mkv" or "concat:x.mkv" would be
// read as an option or a protocol.
func (FFprobe) Probe(ctx context.Context, path string) (Info, error) {
	if !filepath.IsAbs(path) {
		return Info{}, fmt.Errorf("ffprobe: path %q is not absolute", path)
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-of", "json", "-show_format", "-show_streams", path,
	).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return Info{}, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return Info{}, fmt.Errorf("ffprobe: %w", err)
	}
	return parseProbe(out)
}

// probeOutput is the part of ffprobe's JSON we read.
type probeOutput struct {
	Streams []stream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"` // seconds, e.g. "1.021000"; missing for some files
	} `json:"format"`
}

type stream struct {
	Index         int               `json:"index"`
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	Profile       string            `json:"profile"`
	Level         int               `json:"level"`
	PixFmt        string            `json:"pix_fmt"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	Disposition   map[string]int    `json:"disposition"`
	Tags          map[string]string `json:"tags"`
	SideData      []struct {
		Type      string `json:"side_data_type"`
		DVProfile int    `json:"dv_profile"`
	} `json:"side_data_list"`
}

// lang returns the stream's language tag. "und" (undetermined, MP4's default) counts as none.
func (s stream) lang() string {
	if l := s.Tags["language"]; l != "und" {
		return l
	}
	return ""
}

func parseProbe(data []byte) (Info, error) {
	var out probeOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return Info{}, fmt.Errorf("ffprobe output: %w", err)
	}

	var info Info
	if out.Format.Duration != "" {
		secs, err := strconv.ParseFloat(out.Format.Duration, 64)
		if err != nil {
			return Info{}, fmt.Errorf("ffprobe duration %q: %w", out.Format.Duration, err)
		}
		info.Duration = time.Duration(secs * float64(time.Second)).Round(time.Millisecond)
	}

	var main *stream
	for i, s := range out.Streams {
		switch s.CodecType {
		case "video":
			// Cover art is a one-picture video stream; it's never the movie.
			if main == nil && s.Disposition["attached_pic"] == 0 {
				main = &out.Streams[i]
			}
		case "audio":
			info.Audio = append(info.Audio, AudioTrack{
				Stream: s.Index, Codec: s.CodecName, Channels: s.Channels, Layout: s.ChannelLayout,
				Lang: s.lang(), Title: s.Tags["title"], Default: s.Disposition["default"] == 1,
			})
		case "subtitle":
			info.Subtitles = append(info.Subtitles, SubtitleTrack{
				Stream: s.Index, Codec: s.CodecName, Lang: s.lang(), Title: s.Tags["title"],
				Default: s.Disposition["default"] == 1, Forced: s.Disposition["forced"] == 1,
				SDH: s.Disposition["hearing_impaired"] == 1,
			})
		}
	}

	if main == nil {
		info.Unplayable = UnplayableNoVideo
		return info, nil
	}
	info.VideoCodec = main.CodecName
	info.CodecString, info.Unplayable = classify(*main)
	info.AppleOnly = isDolbyVision5(*main)
	return info, nil
}
