package room

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// testSocket is a socket with no connection: its messages stay in out.
func testSocket(w Who) *socket {
	return &socket{who: w, out: make(chan []byte, sendBuffer), quit: make(chan struct{}), drop: func() {}}
}

// nextMsg returns the socket's next message, decoded loosely.
func nextMsg(t *testing.T, s *socket) map[string]any {
	t.Helper()
	select {
	case b := <-s.out:
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no message")
		return nil
	}
}

// A joiner opens its room before the hub counts it. An archive or delete in between must not be lost
// when the join starts the room's loop.
func TestHubJoinStaleRoom(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	if _, err := r.DB.Exec(`INSERT INTO users (id, token_hash, name) VALUES (1, x'01', 'Alice')`); err != nil {
		t.Fatal(err)
	}
	h := NewHub(t.Context(), r, "build")
	t.Cleanup(h.Wait)

	t.Run("archived in between → the loop has it archived", func(t *testing.T) {
		rm := mustCreate(t, r, pick(1, stream(1)))
		if _, err := r.SetArchived(ctx, rm.ID, true); err != nil {
			t.Fatal(err)
		}
		s := testSocket(alice)
		h.join(rm, s) // rm still says not archived
		if m := nextMsg(t, s); m["type"] != MsgRoom || m["room"].(map[string]any)["archived"] != true {
			t.Errorf("first message = %v, want the room, archived", m)
		}
	})

	t.Run("deleted in between → deleted, and closed", func(t *testing.T) {
		rm := mustCreate(t, r, pick(2, stream(1)))
		if err := r.Delete(ctx, rm.ID); err != nil {
			t.Fatal(err)
		}
		s := testSocket(alice)
		h.join(rm, s)
		if m := nextMsg(t, s); m["type"] != MsgDeleted {
			t.Errorf("first message = %v, want deleted", m)
		}
		select {
		case <-s.quit:
		case <-time.After(5 * time.Second):
			t.Error("socket not closed")
		}
		if l := h.loop(rm.ID); l != nil {
			t.Error("the deleted room still has a loop")
		}
	})
}
