// Package api holds the HTTP handlers for both listeners.
package api

import (
	"io/fs"
	"net/http"

	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/room"
	"github.com/erkanvatan/pause-together/internal/user"
)

// Deps is what the handlers need, the same on both listeners.
type Deps struct {
	Users *user.Store
	// TokenCookie names the user's cookie. Browsers ignore the port when storing cookies, so dev
	// (localhost:5173) and prod (localhost:8421) need different names or they overwrite each other.
	TokenCookie string
	Libraries   *library.Libraries
	Scans       *library.Scans // admin API only
	Jobs        *media.Jobs    // prepared videos and subtitles, served under /stream
	Rooms       *room.Rooms
	Hub         *room.Hub // room sockets, and who's watching
}

// Guest returns the handler for the guest port: the whole app except the admin API.
func Guest(web fs.FS, d Deps) http.Handler {
	s := &server{Deps: d}
	return http.NewCrossOriginProtection().Handler(s.mux(web))
}

// Admin returns the handler for the admin port: the app plus the admin API, for local Host headers only.
func Admin(web fs.FS, d Deps) http.Handler {
	s := &server{Deps: d, admin: true}
	return localHostOnly(http.NewCrossOriginProtection().Handler(s.mux(web)))
}

// server holds what the handlers need. admin is true only on the admin listener.
type server struct {
	Deps
	admin bool
}

// mux registers the routes: the admin API on the admin port only, the rest on both. "/", "/api/"
// and "/stream/" carry no method: "GET /" next to "/api/" is a pattern conflict, and ServeMux panics
// on it.
func (s *server) mux(web fs.FS) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.HandleFunc("GET /api/me", s.getMe)
	mux.HandleFunc("POST /api/me", s.postMe)
	mux.HandleFunc("GET /api/videos", s.listVideos)
	mux.HandleFunc("GET /api/videos/{id}", s.getVideo)
	mux.HandleFunc("GET /api/languages", s.getLanguages)
	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("POST /api/rooms", s.createRoom)
	mux.HandleFunc("GET /api/rooms/{id}", s.openRoom)
	mux.HandleFunc("PUT /api/rooms/{id}/video", s.switchVideo)
	mux.HandleFunc("PUT /api/rooms/{id}/name", s.renameRoom)
	mux.HandleFunc("POST /api/rooms/{id}/archive", s.setArchived(true))
	mux.HandleFunc("POST /api/rooms/{id}/unarchive", s.setArchived(false))
	mux.HandleFunc("GET /api/rooms/{id}/messages", s.roomMessages)
	mux.HandleFunc("GET /stream/{key}/{name}", s.streamFile)
	mux.HandleFunc("GET /ws", s.roomSocket)
	if s.admin {
		s.adminRoutes(mux)
	}
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/stream/", http.NotFoundHandler())
	mux.Handle("/", spa(web))
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}
