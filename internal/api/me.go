package api

import (
	"encoding/json"
	"errors"
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
// Tailscale, and their browser would drop it. An empty token deletes the cookie.
func (s *server) setTokenCookie(w http.ResponseWriter, token string) {
	maxAge := tokenMaxAge
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.TokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
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
	// ID is the person's, so a page can tell a rename from a move to another person. null with Name.
	ID      *int64  `json:"id"`
	Name    *string `json:"name"` // null until the visitor picks a name
	IsAdmin bool    `json:"isAdmin"`
	// GuestURL is the address to give guests, on the admin port only; "" when they can't reach it yet.
	GuestURL string `json:"guestUrl,omitempty"`
}

// me is the answer to /api/me for this user: nil for a new visitor.
func (s *server) me(u *user.User) meResponse {
	resp := meResponse{IsAdmin: s.admin}
	if u != nil {
		resp.ID, resp.Name = &u.ID, &u.Name
	}
	if s.admin {
		resp.GuestURL = s.GuestURL
	}
	return resp
}

// getMe says who the visitor is, and refreshes their cookie so it never expires while in use.
func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	u, token, ok, err := s.currentUser(r)
	if err != nil {
		internalError(w, "look up user", err)
		return
	}
	if !ok {
		writeJSON(w, s.me(nil))
		return
	}
	s.setTokenCookie(w, token)
	writeJSON(w, s.me(&u))
}

// postMe gives this browser's person the name, or makes this browser the person who has it (see
// user.Store.Rename). A new browser gets a token.
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

	var u user.User
	token := ""
	if c, err := r.Cookie(s.TokenCookie); err == nil {
		from, to, err := s.Users.Rename(r.Context(), c.Value, name)
		switch {
		case errors.Is(err, user.ErrUnknownToken): // a new browser after all
		case err != nil:
			internalError(w, "rename user", err)
			return
		default:
			u, token = to, c.Value
			switch {
			case to.ID != from.ID: // this browser's other pages join again as the new person
				s.Hub.Moved(browserOf(c.Value))
			case to.Name != from.Name: // every page of theirs, and everyone else's, shows the new name
				s.Hub.Renamed(to.ID)
			}
		}
	}
	if token == "" {
		if u, token, err = s.Users.Create(r.Context(), name); err != nil {
			internalError(w, "create user", err)
			return
		}
	}
	s.setTokenCookie(w, token)
	// The person's spelling, which may not be the one typed: "ali" joins "Ali".
	writeJSON(w, s.me(&u))
}

// deleteMe logs this browser out: its token goes, and the page asks for a name again. The person stays.
func (s *server) deleteMe(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.TokenCookie); err == nil {
		forgot, err := s.Users.Forget(r.Context(), c.Value)
		if err != nil {
			internalError(w, "forget token", err)
			return
		}
		if forgot {
			s.Hub.LoggedOut(browserOf(c.Value))
		}
	}
	s.setTokenCookie(w, "")
	writeJSON(w, s.me(nil))
}

// browserOf is how rooms tell one browser from another: its token's hash. They keep it in memory only,
// and never send it.
func browserOf(token string) string { return string(user.TokenHash(token)) }

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write JSON", "err", err)
	}
}

func internalError(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
