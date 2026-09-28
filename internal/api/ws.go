package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"

	"github.com/erkanvatan/pause-together/internal/room"
)

// errNoUser: the visitor hasn't picked a name yet, so they can't join a room.
const errNoUser = "no-user"

// roomSocket joins a room page's socket, /ws?room={id}, to the room's loop. Opening the room queues its
// prepare again, which matters after a server restart: the page reconnects without reloading.
func (s *server) roomSocket(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("room"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, errRoomNotFound)
		return
	}
	u, _, ok, err := s.currentUser(r)
	if err != nil {
		internalError(w, "look up user", err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, errNoUser)
		return
	}
	// Default options: the Origin check stays on. A page on another origin can't join as our user.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has answered
	}
	// Only past the Origin check: opening queues the prepare, which another origin mustn't trigger.
	// A browser can't read a refused upgrade's status, so a missing room gets a close code instead.
	rm, err := s.Rooms.Open(r.Context(), id)
	if err != nil {
		code := room.CloseNotFound
		if !errors.Is(err, room.ErrNotFound) {
			slog.Error("open room", "err", err)
			code = websocket.StatusInternalError
		}
		_ = conn.Close(code, "")
		return
	}
	s.Hub.Serve(r.Context(), conn, rm, room.Who{UserID: u.ID, Name: u.Name})
}
