package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
)

// encode turns UTF-8 text into another encoding's bytes.
func encode(t *testing.T, enc encoding.Encoding, s string) string {
	t.Helper()
	b, err := enc.NewEncoder().String(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeSubtitle(t *testing.T) {
	turkish := "Şişli'de ğüşıöç, İstanbul'un ılık akşamı"
	tr := encode(t, charmap.Windows1254, turkish)
	ru := encode(t, charmap.Windows1251, "Привет, как дела?")
	el := encode(t, charmap.Windows1253, "Καλημέρα κόσμε")
	de := encode(t, charmap.Windows1252, "Grüß dich, schöne Grüße")
	pl := encode(t, charmap.Windows1250, "Zażółć gęślą jaźń")
	ja := encode(t, japanese.ShiftJIS, "こんにちは世界")
	utf16le := encode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), "Grüße ş")
	utf16be := encode(t, unicode.UTF16(unicode.BigEndian, unicode.UseBOM), "Grüße ş")

	tests := []struct {
		name string
		data string
		lang string
		want string
	}{
		{"UTF-8", "Şişli ğ", "tr", "Şişli ğ"},
		{"UTF-8 wins over the language", "Şişli ğ", "ru", "Şişli ğ"},
		{"UTF-8 with no known code page", "नमस्ते", "hi", "नमस्ते"},
		{"UTF-8 BOM dropped", "\xef\xbb\xbfŞişli", "en", "Şişli"},
		{"UTF-8 BOM trusted over the language", "\xef\xbb\xbfŞişli", "tr", "Şişli"},
		{"UTF-16 LE BOM", utf16le, "en", "Grüße ş"},
		{"UTF-16 BE BOM", utf16be, "tr", "Grüße ş"},
		{"Windows-1254 as tr", tr, "tr", turkish},
		{"Windows-1254 as tur", tr, "tur", turkish},
		{"Windows-1251 as ru", ru, "ru", "Привет, как дела?"},
		{"Windows-1251 as rus", ru, "rus", "Привет, как дела?"},
		{"Windows-1253 as el", el, "el", "Καλημέρα κόσμε"},
		{"Windows-1253 as gre", el, "gre", "Καλημέρα κόσμε"},
		{"Windows-1252 as de", de, "de", "Grüß dich, schöne Grüße"},
		{"Windows-1252 as ger", de, "ger", "Grüß dich, schöne Grüße"},
		{"Windows-1250 as pl", pl, "pl", "Zażółć gęślą jaźń"},
		{"Shift JIS as ja", ja, "ja", "こんにちは世界"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSubtitle([]byte(tt.data), tt.lang)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	// Not UTF-8, and the language has no code page: guessing would show garbage.
	if _, err := decodeSubtitle([]byte(tr), "hi"); err == nil {
		t.Errorf("Windows-1254 bytes as hi: want an error")
	}
}

func TestSubtitleUnavailable(t *testing.T) {
	tests := map[string]SubtitleUnavailable{
		"subrip": "", "ass": "", "ssa": "", "mov_text": "", "webvtt": "",
		"hdmv_pgs_subtitle": SubtitleImage, "dvd_subtitle": SubtitleImage, "dvb_subtitle": SubtitleImage,
		"eia_608": SubtitleCodec, "dvb_teletext": SubtitleCodec,
	}
	for codec, want := range tests {
		if got := subtitleUnavailable(codec); got != want {
			t.Errorf("%s: got %q, want %q", codec, got, want)
		}
	}
}

func TestSidecarKey(t *testing.T) {
	key := SidecarKey(1, "A (2020)/A (2020).en.srt", 100, 200)
	if !validKey(key) {
		t.Fatalf("key %q is not 32 lowercase hex", key)
	}
	for name, other := range map[string]string{
		"library": SidecarKey(2, "A (2020)/A (2020).en.srt", 100, 200),
		"path":    SidecarKey(1, "A (2020)/A (2020).tr.srt", 100, 200),
		"size":    SidecarKey(1, "A (2020)/A (2020).en.srt", 101, 200),
		"mtime":   SidecarKey(1, "A (2020)/A (2020).en.srt", 100, 201),
	} {
		if other == key {
			t.Errorf("%s: key didn't change", name)
		}
	}
}

// Real ffmpeg on the sidecars task testdata makes.
func TestSidecarClips(t *testing.T) {
	const movie = "Movies/Stereo Test (2020)/Stereo Test (2020)"
	tests := []struct {
		file string
		lang string
		want string // a line of the WebVTT
		not  string // must not be in it; "" = don't check
	}{
		{movie + ".tr.srt", "tr", "ş ğ ı İ", ""},
		{movie + ".tur.srt", "tur", "ş ğ ı İ", ""},
		{movie + ".en.srt", "en", "With a BOM", "\ufeff"},
		{movie + ".en.sdh.ass", "en", "<i>Up top</i> and plain", `\an8`},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			s := Subtitles{Dir: t.TempDir()}
			key := SidecarKey(1, tt.file, 1, 1)
			if err := s.Sidecar(context.Background(), clip(t, tt.file), tt.lang, key); err != nil {
				t.Fatal(err)
			}
			vtt, err := os.ReadFile(filepath.Join(s.Dir, key, sidecarFile))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(vtt, []byte("WEBVTT\n")) {
				t.Errorf("no WEBVTT header:\n%s", vtt)
			}
			if !strings.Contains(string(vtt), "\n"+tt.want+"\n") {
				t.Errorf("want line %q in:\n%s", tt.want, vtt)
			}
			if tt.not != "" && strings.Contains(string(vtt), tt.not) {
				t.Errorf("%q still in:\n%s", tt.not, vtt)
			}
			if !strings.Contains(string(vtt), "00:00.000 --> 00:00.900") {
				t.Errorf("timing lost:\n%s", vtt)
			}
		})
	}
}

func TestSidecarReadyIsKept(t *testing.T) {
	s := Subtitles{Dir: t.TempDir()}
	key := SidecarKey(1, "x.srt", 1, 1)
	out := filepath.Join(s.Dir, key, sidecarFile)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("WEBVTT\n\nready"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The source doesn't exist: a ready copy means it's never read.
	if err := s.Sidecar(context.Background(), "/nope/x.srt", "", key); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "WEBVTT\n\nready" {
		t.Errorf("ready copy changed: %q", got)
	}

	if err := s.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(out)); !os.IsNotExist(err) {
		t.Errorf("key folder still there: %v", err)
	}
}

func TestSidecarErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	noPerm := write("noperm.srt", []byte("1\n00:00:00,000 --> 00:00:00,900\nHi\n"))
	if err := os.Chmod(noPerm, 0); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		src        string
		lang       string
		want       string
		unreadable bool // the file's own fault
	}{
		{"no cues", write("a.srt", []byte("\x00\x01 not a subtitle")), "en", "no cues", true},
		{"too big", write("big.srt", make([]byte, maxSidecarBytes+1)), "en", "larger than", true},
		{"unknown extension", write("a.sub", []byte("x")), "en", "unknown format", true},
		{"no code page", write("hi.srt", []byte("\xfe\xfd")), "hi", "no code page", true},
		{"no permission", noPerm, "en", "permission denied", true},
		{"relative path", "-x.srt", "en", "not absolute", false},
		{"gone", filepath.Join(dir, "gone.srt"), "en", "no such file", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Subtitles{Dir: t.TempDir()}
			key := SidecarKey(1, tt.name, 1, 1)
			err := s.Sidecar(context.Background(), tt.src, tt.lang, key)
			if err == nil || errors.Is(err, ErrUnreadable) != tt.unreadable || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q, ErrUnreadable %v", err, tt.want, tt.unreadable)
			}
			if entries, _ := os.ReadDir(s.Dir); len(entries) != 0 {
				t.Errorf("cache not empty after a failure: %v", entries)
			}
		})
	}
	if err := (Subtitles{Dir: dir}).Sidecar(context.Background(), "/x.srt", "", "../x"); err == nil {
		t.Errorf("bad key accepted")
	}
}

// ffmpeg that doesn't finish is not the text's fault.
func TestToWebVTTCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := toWebVTT(ctx, "srt", "1\n00:00:00,000 --> 00:00:00,900\nHi\n")
	if err == nil || errors.Is(err, ErrUnreadable) {
		t.Errorf("err = %v, want an error that isn't ErrUnreadable", err)
	}
}

// A cache it can't write to is not the file's fault.
func TestSidecarCacheError(t *testing.T) {
	src := clip(t, "Movies/Stereo Test (2020)/Stereo Test (2020).en.srt")
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Subtitles{Dir: notDir}.Sidecar(context.Background(), src, "en", SidecarKey(1, "x", 1, 1))
	if err == nil || errors.Is(err, ErrUnreadable) {
		t.Errorf("err = %v, want an error that isn't ErrUnreadable", err)
	}
}
