package api

import (
	"errors"
	"net/http"

	"github.com/erkanvatan/pause-together/internal/library"
)

// listVideos lists every video for the picker, without tracks: a big library would make a big list.
func (s *server) listVideos(w http.ResponseWriter, r *http.Request) {
	videos, err := s.Libraries.Videos(r.Context())
	if err != nil {
		internalError(w, "list videos", err)
		return
	}
	writeJSON(w, videos)
}

// getVideo returns one video with its tracks, once the picker has picked it.
func (s *server) getVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	v, err := s.Libraries.VideoDetail(r.Context(), id)
	switch {
	case errors.Is(err, library.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		internalError(w, "get video", err)
	default:
		writeJSON(w, v)
	}
}

// getLanguages returns the host's language defaults. Guests need them too: the picker preselects
// from them.
func (s *server) getLanguages(w http.ResponseWriter, r *http.Request) {
	langs, err := s.Libraries.GetLanguages(r.Context())
	if err != nil {
		internalError(w, "get languages", err)
		return
	}
	writeJSON(w, langs)
}
