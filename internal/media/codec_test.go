package media

import (
	"reflect"
	"testing"
	"time"
)

// Stream JSON as ffprobe 7.1 prints it, trimmed to the fields we read.
const (
	h264High    = `{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","level":31}`
	coverArt    = `{"index":0,"codec_type":"video","codec_name":"mjpeg","profile":"Baseline","pix_fmt":"yuvj420p","level":-99,"disposition":{"attached_pic":1}}`
	stereoAudio = `{"index":1,"codec_type":"audio","codec_name":"aac","channels":2,"channel_layout":"stereo"}`
)

func probeJSON(streams ...string) []byte {
	s := `{"streams":[`
	for i, st := range streams {
		if i > 0 {
			s += ","
		}
		s += st
	}
	return []byte(s + `],"format":{"duration":"1.021000"}}`)
}

func TestClassifyVideo(t *testing.T) {
	tests := []struct {
		name       string
		stream     string
		codec      string
		unplayable Unplayable
		appleOnly  bool
	}{
		{"H.264 High", h264High, "avc1.64001f", "", false},
		{"H.264 Main", `{"codec_type":"video","codec_name":"h264","profile":"Main","pix_fmt":"yuv420p","level":40}`, "avc1.4d0028", "", false},
		{"H.264 Baseline", `{"codec_type":"video","codec_name":"h264","profile":"Baseline","pix_fmt":"yuv420p","level":30}`, "avc1.42001e", "", false},
		{"H.264 Constrained Baseline", `{"codec_type":"video","codec_name":"h264","profile":"Constrained Baseline","pix_fmt":"yuv420p","level":30}`,
			"avc1.42401e", "", false},
		{"H.264 full-range 4:2:0", `{"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuvj420p","level":40}`, "avc1.640028", "", false},
		{"H.264 High 10", `{"codec_type":"video","codec_name":"h264","profile":"High 10","pix_fmt":"yuv420p10le","level":40}`,
			"", UnplayableH264Profile, false},
		{"H.264 4:2:2", `{"codec_type":"video","codec_name":"h264","profile":"High 4:2:2","pix_fmt":"yuv422p","level":40}`,
			"", UnplayableH264Profile, false},
		// x264 lossless: 4:2:0 pixels, but a profile browsers can't decode.
		{"H.264 lossless 4:2:0", `{"codec_type":"video","codec_name":"h264","profile":"High 4:4:4 Predictive","pix_fmt":"yuv420p","level":40}`,
			"", UnplayableH264Profile, false},

		{"HEVC Main", `{"codec_type":"video","codec_name":"hevc","profile":"Main","pix_fmt":"yuv420p","level":120}`, "hvc1.1.6.L120.B0", "", false},
		{"HEVC Main 10", `{"codec_type":"video","codec_name":"hevc","profile":"Main 10","pix_fmt":"yuv420p10le","level":153}`,
			"hvc1.2.4.L153.B0", "", false},
		{"HEVC Rext", `{"codec_type":"video","codec_name":"hevc","profile":"Rext","pix_fmt":"yuv444p","level":93}`, "hvc1.4.10.L93.B0", "", false},
		{"Dolby Vision profile 5", `{"codec_type":"video","codec_name":"hevc","profile":"Main 10","pix_fmt":"yuv420p10le","level":153,
			"side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":5,"dv_level":6}]}`,
			"hvc1.2.4.L153.B0", "", true},
		{"Dolby Vision profile 8", `{"codec_type":"video","codec_name":"hevc","profile":"Main 10","pix_fmt":"yuv420p10le","level":153,
			"side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":8,"dv_level":6}]}`,
			"hvc1.2.4.L153.B0", "", false},

		{"AV1 10-bit", `{"codec_type":"video","codec_name":"av1","profile":"Main","pix_fmt":"yuv420p10le","level":8}`, "av01.0.08M.10", "", false},
		{"AV1 level 0", `{"codec_type":"video","codec_name":"av1","profile":"Main","pix_fmt":"yuv420p","level":0}`, "av01.0.00M.08", "", false},
		{"AV1 unknown level", `{"codec_type":"video","codec_name":"av1","profile":"Main","pix_fmt":"yuv420p","level":-99}`, "av01.0.00M.08", "", false},
		{"AV1 High", `{"codec_type":"video","codec_name":"av1","profile":"High","pix_fmt":"yuv444p","level":12}`, "av01.1.12M.08", "", false},

		{"VP9 unknown level", `{"codec_type":"video","codec_name":"vp9","profile":"Profile 0","pix_fmt":"yuv420p","level":-99}`, "vp09.00.10.08", "", false},
		{"VP9 profile 2", `{"codec_type":"video","codec_name":"vp9","profile":"Profile 2","pix_fmt":"yuv420p10le","level":41}`, "vp09.02.41.10", "", false},

		{"VP8", `{"codec_type":"video","codec_name":"vp8","profile":"0","pix_fmt":"yuv420p","level":-99}`, "", UnplayableCodec, false},
		{"MPEG-2", `{"codec_type":"video","codec_name":"mpeg2video","profile":"Main","pix_fmt":"yuv420p","level":8}`, "", UnplayableCodec, false},
		{"VC-1", `{"codec_type":"video","codec_name":"vc1","profile":"Advanced","pix_fmt":"yuv420p","level":3}`, "", UnplayableCodec, false},
		{"XviD", `{"codec_type":"video","codec_name":"mpeg4","profile":"Advanced Simple Profile","pix_fmt":"yuv420p","level":5}`,
			"", UnplayableCodec, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseProbe(probeJSON(tt.stream))
			if err != nil {
				t.Fatal(err)
			}
			if info.CodecString != tt.codec || info.Unplayable != tt.unplayable || info.AppleOnly != tt.appleOnly {
				t.Errorf("got codec %q, unplayable %q, apple only %v; want %q, %q, %v",
					info.CodecString, info.Unplayable, info.AppleOnly, tt.codec, tt.unplayable, tt.appleOnly)
			}
		})
	}
}

func TestMainVideoStream(t *testing.T) {
	tests := []struct {
		name       string
		streams    []string
		codec      string
		unplayable Unplayable
	}{
		{"cover art first", []string{coverArt, h264High}, "h264", ""},
		{"cover art only", []string{coverArt, stereoAudio}, "", UnplayableNoVideo},
		{"audio only", []string{stereoAudio}, "", UnplayableNoVideo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseProbe(probeJSON(tt.streams...))
			if err != nil {
				t.Fatal(err)
			}
			if info.VideoCodec != tt.codec || info.Unplayable != tt.unplayable {
				t.Errorf("got video codec %q, unplayable %q; want %q, %q", info.VideoCodec, info.Unplayable, tt.codec, tt.unplayable)
			}
		})
	}
}

func TestParseProbeTracks(t *testing.T) {
	info, err := parseProbe(probeJSON(
		h264High,
		`{"index":1,"codec_type":"audio","codec_name":"eac3","channels":6,"channel_layout":"5.1(side)",
			"disposition":{"default":1},"tags":{"language":"tur","title":"Türkçe"}}`,
		`{"index":2,"codec_type":"audio","codec_name":"aac","channels":2,"channel_layout":"stereo","tags":{"language":"und"}}`,
		`{"index":3,"codec_type":"subtitle","codec_name":"subrip","disposition":{"forced":1},"tags":{"language":"eng","title":"Forced"}}`,
		`{"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","disposition":{"default":1,"hearing_impaired":1},"tags":{"language":"eng"}}`,
		`{"index":5,"codec_type":"attachment","codec_name":"ttf"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	want := Info{
		Duration:    1021 * time.Millisecond,
		VideoCodec:  "h264",
		CodecString: "avc1.64001f",
		Audio: []AudioTrack{
			{Stream: 1, Codec: "eac3", Channels: 6, Layout: "5.1(side)", Lang: "tur", Title: "Türkçe", Default: true},
			{Stream: 2, Codec: "aac", Channels: 2, Layout: "stereo"}, // "und" means no language
		},
		Subtitles: []SubtitleTrack{
			{Stream: 3, Codec: "subrip", Lang: "eng", Title: "Forced", Forced: true},
			{Stream: 4, Codec: "hdmv_pgs_subtitle", Lang: "eng", Default: true, SDH: true, Unavailable: SubtitleImage},
		},
	}
	if !reflect.DeepEqual(info, want) {
		t.Errorf("\n got %+v\nwant %+v", info, want)
	}
}

func TestParseProbeNoDuration(t *testing.T) {
	info, err := parseProbe([]byte(`{"streams":[` + h264High + `],"format":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if info.Duration != 0 {
		t.Errorf("duration = %v, want 0", info.Duration)
	}
}

func TestParseProbeBadJSON(t *testing.T) {
	if _, err := parseProbe([]byte("not json")); err == nil {
		t.Error("want an error")
	}
}
