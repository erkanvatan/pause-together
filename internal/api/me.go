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

// postMe gives this browser's person the name: a new browser gets a token, a known one renames its
// person (see user.Store.Rename). A name another person has answers 409 with who has it, so the page
// can ask first; {join: id} then makes this browser that person.
func (s *server) postMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Join int64  `json:"join"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	token := ""
	if c, err := r.Cookie(s.TokenCookie); err == nil {
		token = c.Value
	}
	if req.Join != 0 {
		s.join(w, r, token, req.Join)
		return
	}
	name, err := user.CleanName(req.Name)
	if err != nil {
		writeBadName(w, err)
		return
	}

	if token != "" {
		from, to, err := s.Users.Rename(r.Context(), token, name)
		switch {
		case errors.Is(err, user.ErrUnknownToken): // a new browser after all: its dead cookie goes below
		case errors.Is(err, user.ErrNameTaken):
			writeTaken(w, to)
			return
		case err != nil:
			internalError(w, "rename user", err)
			return
		default:
			if to.Name != from.Name { // every page of theirs, and everyone else's, shows the new name
				s.Hub.Renamed(to.ID)
			}
			s.setTokenCookie(w, token)
			writeJSON(w, s.me(&to))
			return
		}
	}
	u, newToken, err := s.Users.Create(r.Context(), name)
	if errors.Is(err, user.ErrNameTaken) {
		if token != "" {
			s.setTokenCookie(w, "") // unknown, or Rename would have answered
		}
		writeTaken(w, u)
		return
	}
	if err != nil {
		internalError(w, "create user", err)
		return
	}
	s.setTokenCookie(w, newToken)
	writeJSON(w, s.me(&u))
}

// join makes this browser person id, the one a 409 named.
func (s *server) join(w http.ResponseWriter, r *http.Request, token string, id int64) {
	from, to, newToken, err := s.Users.Join(r.Context(), token, id)
	if errors.Is(err, user.ErrUnknownPerson) {
		http.Error(w, "unknown person", http.StatusNotFound)
		return
	}
	if err != nil {
		internalError(w, "join user", err)
		return
	}
	if newToken != "" {
		token = newToken
	} else if from.ID != to.ID { // this browser's other pages join again as the new person
		s.Hub.Moved(browserOf(token))
	}
	s.setTokenCookie(w, token)
	// The person's spelling now: they may have renamed since the page asked.
	writeJSON(w, s.me(&to))
}

// errTaken refuses a name another person has.
const errTaken = "taken"

// takenResponse says who has the name: their id, to join them, and their spelling, to ask.
type takenResponse struct {
	Error string `json:"error"` // errTaken
	ID    int64  `json:"id"`
	Name  string `json:"name"`
}

func writeTaken(w http.ResponseWriter, holder user.User) {
	writeJSONStatus(w, http.StatusConflict, takenResponse{errTaken, holder.ID, holder.Name})
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

// writeBadName answers a refused name with why, so the page can say what to fix: 400
// {error: reason, symbol}, the symbol only for name-symbol.
func writeBadName(w http.ResponseWriter, err error) {
	ne := &user.NameError{Reason: user.ReasonOther}
	errors.As(err, &ne)
	writeJSONStatus(w, http.StatusBadRequest, struct {
		Error  string `json:"error"`
		Symbol string `json:"symbol,omitempty"`
	}{ne.Reason, ne.Symbol})
}

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
