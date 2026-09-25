package media

import (
	"fmt"
	"strings"
)

// Unplayable says why no browser can play a video. It's a code, not English: the web strings file
// turns it into text.
type Unplayable string

const (
	UnplayableNoVideo     Unplayable = "no-video"     // no video stream, or only cover art
	UnplayableCodec       Unplayable = "codec"        // not H.264, HEVC, AV1 or VP9; Info.VideoCodec says which
	UnplayableH264Profile Unplayable = "h264-profile" // H.264 that isn't 8-bit 4:2:0 in a profile browsers decode
	UnplayableProbeFailed Unplayable = "probe-failed" // ffprobe failed; the scan stores its error and retries
)

// h264Profiles maps the H.264 profiles browsers decode to the profile_idc and constraint bytes of an
// "avc1.PPCCLL" string. High 10, 4:2:2 and 4:4:4 are left out: browsers can't decode them. x264's
// lossless mode is "High 4:4:4 Predictive" even with 4:2:0 pixels.
var h264Profiles = map[string]string{
	"Constrained Baseline": "4240",
	"Baseline":             "4200",
	"Main":                 "4d00",
	"High":                 "6400",
}

// hevcProfiles maps HEVC profiles to the profile_idc and compatibility flags of an "hvc1" string.
// Anything else is written as Main: canPlayType() and the <video> error event still catch it.
var hevcProfiles = map[string]string{
	"Main":    "1.6",
	"Main 10": "2.4",
	"Rext":    "4.10",
}

var av1Profiles = map[string]int{"Main": 0, "High": 1, "Professional": 2}

// When ffprobe doesn't know the level (it reports -99 for most VP9 files), the codec string gets the
// lowest one, so canPlayType() checks only the profile and bit depth.
const (
	av1LowestLevel = 0
	vp9LowestLevel = 10
)

// classify checks a video stream against the codec allowlist and builds its codec string.
func classify(s stream) (string, Unplayable) {
	switch s.CodecName {
	case "h264":
		pc, ok := h264Profiles[s.Profile]
		if !ok || s.PixFmt != "yuv420p" && s.PixFmt != "yuvj420p" {
			return "", UnplayableH264Profile
		}
		return fmt.Sprintf("avc1.%s%02x", pc, s.Level), ""
	case "hevc":
		p, ok := hevcProfiles[s.Profile]
		if !ok {
			p = hevcProfiles["Main"]
		}
		// ffprobe doesn't report the tier; L (Main tier) is by far the most common.
		return fmt.Sprintf("hvc1.%s.L%d.B0", p, s.Level), ""
	case "av1":
		level := s.Level
		if level < 0 {
			level = av1LowestLevel
		}
		// ffprobe doesn't report the tier; M (Main tier) is by far the most common.
		return fmt.Sprintf("av01.%d.%02dM.%02d", av1Profiles[s.Profile], level, bitDepth(s.PixFmt)), ""
	case "vp9":
		var profile int
		_, _ = fmt.Sscanf(s.Profile, "Profile %d", &profile)
		level := s.Level
		if level <= 0 {
			level = vp9LowestLevel
		}
		return fmt.Sprintf("vp09.%02d.%02d.%02d", profile, level, bitDepth(s.PixFmt)), ""
	}
	return "", UnplayableCodec
}

// bitDepth reads the bit depth from a pixel format name like "yuv420p10le".
func bitDepth(pixFmt string) int {
	switch {
	case strings.Contains(pixFmt, "p10"):
		return 10
	case strings.Contains(pixFmt, "p12"):
		return 12
	}
	return 8
}

func isDolbyVision5(s stream) bool {
	for _, sd := range s.SideData {
		if sd.Type == "DOVI configuration record" && sd.DVProfile == 5 {
			return true
		}
	}
	return false
}
