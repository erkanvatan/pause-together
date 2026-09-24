// Package api holds the HTTP handlers for both listeners.
package api

import (
	"io/fs"
	"net/http"
)

// Guest returns the handler for the guest port: the whole app except the admin API.
func Guest(web fs.FS) http.Handler {
	return http.NewCrossOriginProtection().Handler(newMux(web))
}

// Admin returns the handler for the admin port: the app plus the admin API, for local Host headers only.
func Admin(web fs.FS) http.Handler {
	return localHostOnly(http.NewCrossOriginProtection().Handler(newMux(web)))
}

// newMux registers the routes shared by both ports. "/", "/api/" and "/stream/" carry no method:
// "GET /" next to "/api/" is a pattern conflict, and ServeMux panics on it.
func newMux(web fs.FS) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/stream/", http.NotFoundHandler())
	mux.Handle("/", spa(web))
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
