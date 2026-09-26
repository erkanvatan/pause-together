package media

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RecipeVersion is part of every cache key. Bump it when the ffmpeg arguments change, so old copies
// are made again with the new ones.
const RecipeVersion = 2

// downmix turns 3 or more channels into stereo that keeps voices clear on phone speakers. aformat
// first brings every layout to plain 5.1 (7.1 folds its extra pair in, side becomes back), so the pan
// can name its channels. The pan weights the center (voices) over the rest; '<' scales each side so its
// gains add up to 1, which keeps it from clipping. ffmpeg's default downmix makes voices quiet. The light
// compressor keeps explosions from drowning voices, and its makeup gain brings the level back up.
const downmix = "aformat=channel_layouts=5.1," +
	"pan=stereo|FL<FC+0.30*FL+0.30*BL|FR<FC+0.30*FR+0.30*BR," +
	"acompressor=threshold=0.125:ratio=2:attack=20:release=250:makeup=2"

// stderrTail is how much of ffmpeg's error output a failed job keeps. A broken file can print an
// error per packet, and the last lines say why it stopped.
const stderrTail = 4 << 10

// Job is one video + audio track to prepare into a cached MP4.
type Job struct {
	VideoID    int64
	Name       string        // shown on the admin page
	Source     string        // absolute path of the video file
	VideoCodec string        // ffprobe's name, e.g. "hevc"
	Duration   time.Duration // for progress
	Size       int64         // the source's size, for the key and the free-space check
	Mtime      int64         // the source's mtime, Unix nanoseconds
	Audio      *AudioTrack   // the room's audio track; nil when the video has none
	// Subtitles are the embedded subtitle streams to turn into WebVTT: every track that isn't
	// Unavailable. They're part of the video, so not part of the key.
	Subtitles []int
}

// Key names the job's prepared copy: video + audio track + source size and mtime + recipe version.
// Rooms with the same key share one copy. It's 32 lowercase hex characters.
func (j Job) Key() string {
	audio := -1
	if j.Audio != nil {
		audio = j.Audio.Stream
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%d|%d|%d|%d", RecipeVersion, j.VideoID, audio, j.Size, j.Mtime))
	return hex.EncodeToString(sum[:16])
}

// Preparer turns a job's source into files in the folder dir: the MP4 and a WebVTT file per subtitle
// track. FFmpeg is the real one.
type Preparer interface {
	Prepare(ctx context.Context, j Job, dir string, progress func(done time.Duration)) error
}

// FFmpeg prepares videos with the ffmpeg binary.
type FFmpeg struct{}

// Prepare runs ffmpeg once. Both paths must be absolute: a relative "-x.mkv" or "concat:x.mkv" would
// be read as an option or a protocol. progress, if not nil, hears how much of the video is done.
func (FFmpeg) Prepare(ctx context.Context, j Job, dir string, progress func(time.Duration)) error {
	for _, p := range []string{j.Source, dir} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("ffmpeg: path %q is not absolute", p)
		}
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", prepareArgs(j, dir)...)
	stderr := &tail{max: stderrTail}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if d, ok := parseProgress(sc.Text()); ok && progress != nil {
			progress(d)
		}
	}
	// If the scanner stopped early, ffmpeg would block on a full pipe and Wait would never return.
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if msg := strings.TrimSpace(string(stderr.buf)); msg != "" {
			return fmt.Errorf("ffmpeg: %w: %s", err, msg)
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}

// prepareArgs builds the ffmpeg arguments: the main video stream copied, only the chosen audio track,
// and the index at the front so playback starts at once. Each subtitle track is another output of the
// same run: its packets are spread across the whole file, so reading it apart would cost another full
// read.
func prepareArgs(j Job, dir string) []string {
	args := []string{"-nostdin", "-v", "error", "-progress", "pipe:1", "-nostats",
		// AVI and some MPEG-TS packets carry no pts; MP4 needs one on every packet.
		"-fflags", "+genpts", "-i", j.Source,
		// V, not v: video that isn't cover art, the same stream the probe picked.
		"-map", "0:V:0"}
	if j.Audio != nil {
		args = append(args, "-map", "0:"+strconv.Itoa(j.Audio.Stream))
	}
	args = append(args, "-c:v", "copy")
	if j.VideoCodec == "hevc" {
		args = append(args, "-tag:v", "hvc1") // Apple devices refuse HEVC tagged hev1
	}
	if j.Audio != nil {
		args = append(args, audioArgs(*j.Audio)...)
	}
	// ffmpeg guesses the format from the extension; name it anyway, like the subtitles.
	args = append(args, "-movflags", "+faststart", "-f", "mp4", filepath.Join(dir, videoFile))
	for _, s := range j.Subtitles {
		args = append(args, "-map", "0:"+strconv.Itoa(s), "-c:s", "webvtt", "-f", "webvtt",
			filepath.Join(dir, subtitleName(s)))
	}
	return args
}

// subtitleExt ends every WebVTT file in the cache.
const subtitleExt = ".vtt"

// subtitleName is an embedded subtitle track's WebVTT file in its key's folder.
func subtitleName(stream int) string {
	return strconv.Itoa(stream) + subtitleExt
}

// audioArgs keeps AAC with at most two channels as it is. Everything else becomes stereo AAC.
func audioArgs(a AudioTrack) []string {
	reencode := []string{"-c:a", "aac", "-b:a", "192k"}
	switch {
	case a.Codec == "aac" && a.Channels <= 2:
		return []string{"-c:a", "copy"}
	case a.Channels <= 2:
		return append(reencode, "-ac", "2")
	default:
		return append([]string{"-af", downmix}, reencode...)
	}
}

// parseProgress reads how far ffmpeg is from one line of its -progress output. out_time_us is in
// microseconds (so, oddly, is out_time_ms); it is "N/A" before the first packet.
func parseProgress(line string) (time.Duration, bool) {
	v, ok := strings.CutPrefix(line, "out_time_us=")
	if !ok {
		return 0, false
	}
	us, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(us) * time.Microsecond, true
}

// tail keeps the last max bytes written to it.
type tail struct {
	max int
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(p), nil
}
