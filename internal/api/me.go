package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/erkanvatan/pause-together/internal/user"
)

const (
	// tokenMaxAge is the longest cookie life browsers allow (Chrome caps it at 400 days).
	tokenMaxAge = 400 * 24 * 60 * 60 // seconds
	// maxBodyBytes caps JSON request bodies.
	maxBodyBytes = 4 << 10
)

// setTokenCookie sets or refreshes the user's token cookie. Not Secure: guests use plain HTTP over
// Tailscale, and their browser would drop it.
func (s *server) setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.TokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   tokenMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// currentUser returns the user whose token the request carries. No cookie or an unknown token
// reports false: that's a new visitor.
func (s *server) currentUser(r *http.Request) (u user.User, token string, ok bool, err error) {
	c, err := r.Cookie(s.TokenCookie)
	if err != nil {
		return user.User{}, "", false, nil // no cookie
	}
	u, ok, err = s.Users.ByToken(r.Context(), c.Value)
	return u, c.Value, ok, err
}

type meResponse struct {
	Name    *string `json:"name"` // null until the visitor picks a name
	IsAdmin bool    `json:"isAdmin"`
}

// getMe says who the visitor is, and refreshes their cookie so it never expires while in use.
func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	u, token, ok, err := s.currentUser(r)
	if err != nil {
		internalError(w, "look up user", err)
		return
	}
	resp := meResponse{IsAdmin: s.admin}
	if ok {
		s.setTokenCookie(w, token)
		resp.Name = &u.Name
	}
	writeJSON(w, resp)
}

// postMe sets the visitor's name: a rename for a known user, a new user otherwise.
func (s *server) postMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	name, err := user.CleanName(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	u, token, ok, err := s.currentUser(r)
	if err != nil {
		internalError(w, "look up user", err)
		return
	}
	if ok {
		err = s.Users.Rename(r.Context(), u.ID, name)
	} else {
		_, token, err = s.Users.Create(r.Context(), name)
	}
	if err != nil {
		internalError(w, "save user", err)
		return
	}
	s.setTokenCookie(w, token)
	writeJSON(w, meResponse{Name: &name, IsAdmin: s.admin})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write JSON", "err", err)
	}
}

func internalError(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
