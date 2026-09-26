package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
)

// streamFile serves a file of a prepared copy: the video, or a subtitle as WebVTT. http.ServeContent
// answers Range requests, which is what lets <video> seek. The URL names the copy by its cache key,
// never by a path.
func (s *server) streamFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, err := s.Jobs.Open(r.PathValue("key"), name)
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, "open prepared file", err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		internalError(w, "stat prepared file", err)
		return
	}
	// Go's own type table has no .vtt, and sniffing would say text/plain.
	if path.Ext(name) == ".vtt" {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	}
	http.ServeContent(w, r, name, info.ModTime(), f)
}
