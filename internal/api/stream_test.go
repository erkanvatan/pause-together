package api

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erkanvatan/pause-together/internal/media"
)

// streamHandlers returns both listeners' handlers over a cache holding one prepared video, its key
// and its bytes.
func streamHandlers(t *testing.T) (map[string]http.Handler, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	key := media.Job{VideoID: 1}.Key()
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	if err := os.Mkdir(filepath.Join(dir, key), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, key, "video.mp4"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	d := deps(testUsers(t))
	d.Jobs = media.NewJobs(dir, media.FFmpeg{})
	build := fakeBuild()
	return map[string]http.Handler{"guest": Guest(build, d), "admin": Admin(build, d)}, key, data
}

func TestStreamRange(t *testing.T) {
	hs, key, data := streamHandlers(t)
	for name, h := range hs {
		t.Run(name, func(t *testing.T) {
			rec := do(h, http.MethodGet, "/stream/"+key+"/video.mp4", http.Header{"Range": {"bytes=10-19"}})
			if rec.Code != http.StatusPartialContent {
				t.Fatalf("status = %d, want 206", rec.Code)
			}
			if !bytes.Equal(rec.Body.Bytes(), data[10:20]) {
				t.Errorf("body = %v, want %v", rec.Body.Bytes(), data[10:20])
			}
			if got := rec.Header().Get("Content-Range"); got != "bytes 10-19/100" {
				t.Errorf("Content-Range = %q", got)
			}
			if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
				t.Errorf("Content-Type = %q, want video/mp4", got)
			}

			if rec := do(h, http.MethodGet, "/stream/"+key+"/video.mp4", nil); rec.Code != http.StatusOK ||
				!bytes.Equal(rec.Body.Bytes(), data) {
				t.Errorf("whole file: status = %d, %d bytes", rec.Code, rec.Body.Len())
			}
		})
	}
}

func TestStreamNotFound(t *testing.T) {
	hs, key, _ := streamHandlers(t)
	other := media.Job{VideoID: 2}.Key()
	for _, target := range []string{
		"/stream/" + other + "/video.mp4",                // unknown key
		"/stream/" + strings.ToUpper(key) + "/video.mp4", // malformed
		"/stream/" + key[:31] + "/video.mp4",             // too short
		"/stream/%2e%2e/video.mp4",                       // ..
		"/stream/" + key + ".tmp/video.mp4",              // half-written
		"/stream/" + key + "/other.mp4",                  // another name
		"/stream/" + key,                                 // the folder
		"/stream/" + key + "/",                           // the folder
	} {
		for name, h := range hs {
			if rec := do(h, http.MethodGet, target, nil); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: status = %d, want 404", name, target, rec.Code)
			}
		}
	}
}
