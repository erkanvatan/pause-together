package api

import (
	"errors"
	"io/fs"
	"net/http"
)

// streamVideo serves a prepared video. http.ServeContent answers Range requests, which is what lets
// <video> seek. The URL names the copy by its cache key, never by a path.
func (s *server) streamVideo(w http.ResponseWriter, r *http.Request) {
	f, err := s.Jobs.Open(r.PathValue("key"))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, "open prepared video", err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		internalError(w, "stat prepared video", err)
		return
	}
	http.ServeContent(w, r, "video.mp4", info.ModTime(), f)
}
