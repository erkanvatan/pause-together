package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/erkanvatan/pause-together/internal/room"
)

// Error codes the room pages turn into text.
const (
	errBadPick  = "bad-pick"
	errBadName  = "bad-name"
	errArchived = "archived"
	// The room page tells a room that is gone from a failed connection by this code.
	errRoomNotFound = "not-found"
)

func (s *server) listRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.Rooms.List(r.Context())
	if err != nil {
		internalError(w, "list rooms", err)
		return
	}
	writeJSON(w, rooms)
}

func (s *server) createRoom(w http.ResponseWriter, r *http.Request) {
	var p room.Pick
	if !decodeBody(w, r, &p) {
		return
	}
	rm, err := s.Rooms.Create(r.Context(), p)
	if err != nil {
		roomError(w, "create room", err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, rm)
}

// openRoom returns a room for its page, and queues its prepare.
func (s *server) openRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := roomID(w, r)
	if !ok {
		return
	}
	rm, err := s.Rooms.Open(r.Context(), id)
	writeRoom(w, "open room", rm, err)
}

func (s *server) switchVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := roomID(w, r)
	if !ok {
		return
	}
	var p room.Pick
	if !decodeBody(w, r, &p) {
		return
	}
	rm, err := s.Rooms.Switch(r.Context(), id, p)
	writeRoom(w, "switch video", rm, err)
}

func (s *server) renameRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := roomID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	rm, err := s.Rooms.Rename(r.Context(), id, req.Name)
	writeRoom(w, "rename room", rm, err)
}

// setArchived returns the handler that archives a room, or unarchives it.
func (s *server) setArchived(archived bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := roomID(w, r)
		if !ok {
			return
		}
		rm, err := s.Rooms.SetArchived(r.Context(), id, archived)
		writeRoom(w, "archive room", rm, err)
	}
}

// deleteRoom is on the admin API: only the host deletes rooms.
func (s *server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := roomID(w, r)
	if !ok {
		return
	}
	if err := s.Rooms.Delete(r.Context(), id); err != nil {
		roomError(w, "delete room", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeRoom sends a room, or its error.
func writeRoom(w http.ResponseWriter, what string, rm room.Room, err error) {
	if err != nil {
		roomError(w, what, err)
		return
	}
	writeJSON(w, rm)
}

// roomID reads the {id} path value. A malformed one gets the same answer as an unknown room.
func roomID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, errRoomNotFound)
	}
	return id, ok
}

func roomError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, room.ErrNotFound):
		writeError(w, http.StatusNotFound, errRoomNotFound)
	case errors.Is(err, room.ErrBadPick):
		writeError(w, http.StatusBadRequest, errBadPick)
	case errors.Is(err, room.ErrBadName):
		writeError(w, http.StatusBadRequest, errBadName)
	case errors.Is(err, room.ErrArchived):
		writeError(w, http.StatusConflict, errArchived)
	default:
		internalError(w, what, err)
	}
}

// decodeBody reads a JSON body into v. On failure it sends bad-request and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, errBadRequest)
		return false
	}
	return true
}
