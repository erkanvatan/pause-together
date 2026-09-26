package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/unicode"
)

// SidecarVersion is part of every sidecar key. Bump it when the conversion changes, so old copies are
// made again.
const SidecarVersion = 2

// sidecarFile is a converted sidecar's name inside its key's folder.
const sidecarFile = "subtitle.vtt"

// maxSidecarBytes stops a huge file that only looks like a subtitle from filling memory. Real ones
// are well under 1 MB.
const maxSidecarBytes = 10 << 20

// convertTimeout stops one hung conversion from stalling a whole scan.
const convertTimeout = time.Minute

// ErrUnreadable marks a sidecar subtitle the file's own fault: no permission to read it, too big, not
// UTF-8 with no code page for its language, rejected by ffmpeg, or no cues. Other errors (a disk
// hiccup, ffmpeg killed or too slow, writing the cache) say nothing about the file, so its row stays.
var ErrUnreadable = errors.New("unreadable subtitle")

// SubtitleUnavailable says why a subtitle track can't become WebVTT. It's a code, not English: the
// web strings file turns it into text.
type SubtitleUnavailable string

const (
	SubtitleImage SubtitleUnavailable = "image" // pictures (PGS, VobSub, DVB): text only with OCR
	SubtitleCodec SubtitleUnavailable = "codec" // some other codec we don't convert
)

// textSubtitles are the embedded codecs ffmpeg turns into WebVTT.
var textSubtitles = map[string]bool{"subrip": true, "ass": true, "ssa": true, "mov_text": true, "webvtt": true}

var imageSubtitles = map[string]bool{"hdmv_pgs_subtitle": true, "dvd_subtitle": true, "dvb_subtitle": true, "xsub": true}

func subtitleUnavailable(codec string) SubtitleUnavailable {
	switch {
	case textSubtitles[codec]:
		return ""
	case imageSubtitles[codec]:
		return SubtitleImage
	default:
		return SubtitleCodec
	}
}

// SidecarKey names a sidecar subtitle's converted copy: the file (library + path in it), its size
// and mtime, and SidecarVersion. It's 32 lowercase hex characters, like a Job's key.
func SidecarKey(libraryID int64, rel string, size, mtime int64) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "sidecar|%d|%d|%s|%d|%d", SidecarVersion, libraryID, rel, size, mtime))
	return hex.EncodeToString(sum[:16])
}

// Subtitles converts sidecar subtitle files into WebVTT in the cache folder, one folder per key.
type Subtitles struct {
	Dir string
}

// Sidecar converts the sidecar subtitle at src into key's copy, unless it's there already. lang is
// the language code from the file name; it picks the code page of a file that isn't UTF-8. src must
// be absolute. A problem with the file itself matches ErrUnreadable.
func (s Subtitles) Sidecar(ctx context.Context, src, lang, key string) error {
	if !validKey(key) {
		return fmt.Errorf("bad subtitle key %q", key)
	}
	dir := filepath.Join(s.Dir, key)
	out := filepath.Join(dir, sidecarFile)
	if _, err := os.Stat(out); err == nil {
		return nil
	}
	vtt, err := convertSidecar(ctx, src, lang)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	// Not a .tmp folder: the job queue deletes those at start, maybe while a startup scan writes here.
	// A .part left by a crash is overwritten next time.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out+".part", vtt, 0o644); err != nil {
		return err
	}
	return os.Rename(out+".part", out)
}

// Remove deletes key's copy.
func (s Subtitles) Remove(key string) error {
	if !validKey(key) {
		return fmt.Errorf("bad subtitle key %q", key)
	}
	return os.RemoveAll(filepath.Join(s.Dir, key))
}

// sidecarFormats maps a sidecar's extension to ffmpeg's format name.
var sidecarFormats = map[string]string{".srt": "srt", ".ass": "ass", ".vtt": "webvtt"}

// unreadable marks err as the file's own fault.
func unreadable(err error) error {
	return fmt.Errorf("%w: %w", ErrUnreadable, err)
}

// convertSidecar reads a sidecar subtitle, decodes it to UTF-8 and has ffmpeg turn it into WebVTT.
func convertSidecar(ctx context.Context, src, lang string) ([]byte, error) {
	if !filepath.IsAbs(src) {
		return nil, fmt.Errorf("subtitle: path %q is not absolute", src)
	}
	format, ok := sidecarFormats[strings.ToLower(filepath.Ext(src))]
	if !ok {
		return nil, unreadable(fmt.Errorf("subtitle: unknown format %q", filepath.Ext(src)))
	}
	f, err := os.Open(src)
	if errors.Is(err, fs.ErrPermission) {
		return nil, unreadable(err)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxSidecarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSidecarBytes {
		return nil, unreadable(fmt.Errorf("subtitle: larger than %d MB", maxSidecarBytes>>20))
	}
	text, err := decodeSubtitle(data, lang)
	if err != nil {
		return nil, unreadable(err)
	}
	vtt, err := toWebVTT(ctx, format, text)
	if err != nil {
		return nil, err
	}
	// ffmpeg turns a file it can't parse into an empty WebVTT. The admin page should list that file,
	// not the picker offer it.
	if !bytes.Contains(vtt, []byte("-->")) {
		return nil, unreadable(errors.New("subtitle: no cues found"))
	}
	return vtt, nil
}

// toWebVTT has ffmpeg turn UTF-8 subtitle text into WebVTT. The text goes in on stdin, so ffmpeg
// never sees a path. ffmpeg keeps <i>, <b> and <u> and drops the rest of the styling. Only ffmpeg
// exiting with an error code blames the text; not starting, a timeout or a kill don't.
func toWebVTT(ctx context.Context, format, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", format, "-i", "pipe:0", "-f", "webvtt", "pipe:1")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return out, nil
	case ctx.Err() != nil:
		return nil, fmt.Errorf("ffmpeg: %w", ctx.Err())
	case errors.As(err, &exitErr) && exitErr.Exited():
		return nil, unreadable(fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr))))
	default:
		return nil, fmt.Errorf("ffmpeg: %w", err) // didn't start, or killed by a signal
	}
}

// langCodePages are the legacy code pages of languages, by 2- and 3-letter code (both 3-letter forms
// where there are two, like ger and deu). Languages whose old files never had one code page are left
// out: Chinese (GBK or Big5), and scripts that went straight to Unicode, like Hindi.
var langCodePages = codePages(map[encoding.Encoding][]string{
	charmap.Windows1250: {"cs", "cze", "ces", "hr", "hrv", "hu", "hun", "pl", "pol", "ro", "rum", "ron",
		"sk", "slo", "slk", "sl", "slv", "bs", "bos", "sq", "alb", "sqi",
		"sr", "srp"}, // Serbian subtitles are mostly in Latin letters
	charmap.Windows1251: {"ru", "rus", "uk", "ukr", "bg", "bul", "be", "bel", "mk", "mac", "mkd"},
	charmap.Windows1252: {"en", "eng", "fr", "fre", "fra", "de", "ger", "deu", "es", "spa", "it", "ita",
		"pt", "por", "nl", "dut", "nld", "sv", "swe", "da", "dan", "no", "nor", "nb", "nob", "nn", "nno",
		"fi", "fin", "is", "ice", "isl", "ca", "cat", "eu", "baq", "eus", "gl", "glg", "ga", "gle",
		"af", "afr", "id", "ind", "ms", "may", "msa", "sw", "swa", "tl", "tgl", "fil"},
	charmap.Windows1253: {"el", "gre", "ell"},
	charmap.Windows1254: {"tr", "tur", "az", "aze"},
	charmap.Windows1255: {"he", "heb"},
	charmap.Windows1256: {"ar", "ara", "fa", "per", "fas", "ur", "urd"},
	charmap.Windows1257: {"et", "est", "lv", "lav", "lt", "lit"},
	charmap.Windows1258: {"vi", "vie"},
	charmap.Windows874:  {"th", "tha"},
	japanese.ShiftJIS:   {"ja", "jpn"},
	korean.EUCKR:        {"ko", "kor"},
})

func codePages(byEncoding map[encoding.Encoding][]string) map[string]encoding.Encoding {
	m := make(map[string]encoding.Encoding)
	for enc, langs := range byEncoding {
		for _, l := range langs {
			m[l] = enc
		}
	}
	return m
}

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// decodeSubtitle turns a subtitle file into UTF-8 text without a BOM. A BOM or valid UTF-8 is
// trusted. Otherwise lang, the language code from the file name, picks the code page.
func decodeSubtitle(data []byte, lang string) (string, error) {
	var enc encoding.Encoding
	switch {
	case bytes.HasPrefix(data, utf8BOM):
		return string(data[len(utf8BOM):]), nil
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}), bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		enc = unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM) // ExpectBOM follows the BOM's order
	case utf8.Valid(data):
		return string(data), nil
	case langCodePages[lang] != nil:
		enc = langCodePages[lang]
	default:
		return "", fmt.Errorf("subtitle: not UTF-8, and no code page known for language %q", lang)
	}
	text, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return "", fmt.Errorf("subtitle: decode: %w", err)
	}
	return string(text), nil
}
