package room

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// chatRooms returns rooms with Alice (1) and Bob (2), and two rooms: one on Heat, one on Ronin.
func chatRooms(t *testing.T) (r *Rooms, heat, ronin Room) {
	t.Helper()
	r = newTestRooms(t)
	if _, err := r.DB.Exec(`INSERT INTO users (id, name, name_key) VALUES (1, 'Alice', 'alice'), (2, 'Bob', 'bob')`); err != nil {
		t.Fatal(err)
	}
	return r, mustCreate(t, r, pick(1, stream(1))), mustCreate(t, r, pick(2, stream(1)))
}

// post adds a message on Heat at 1:00, and fails the test on an error.
func post(t *testing.T, r *Rooms, roomID, userID int64, text string, replyTo *int64) Message {
	t.Helper()
	m, err := r.AddMessage(context.Background(), NewMessage{RoomID: roomID, UserID: userID, Text: text,
		ReplyTo: replyTo, VideoID: 1, PositionMs: 60_000, SentAt: 1_700_000_000_000})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func messages(t *testing.T, r *Rooms, roomID, before int64) []Message {
	t.Helper()
	ms, err := r.Messages(context.Background(), roomID, before)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestAddMessage(t *testing.T) {
	r, heat, _ := chatRooms(t)
	m := post(t, r, heat.ID, 1, "  hello  ", nil)
	if m.Text != "hello" || m.From != alice || m.Video.ID != 1 || m.Video.Title != "Heat" ||
		m.PositionMs != 60_000 || m.SentAt != 1_700_000_000_000 || m.ReplyTo != nil {
		t.Errorf("message = %+v", m)
	}
	if got := messages(t, r, heat.ID, 0); len(got) != 1 || got[0].ID != m.ID || got[0].Text != "hello" {
		t.Errorf("stored = %+v", got)
	}
}

// The limit counts runes, not bytes: ş takes two bytes, the emoji four.
func TestMessageLength(t *testing.T) {
	r, heat, _ := chatRooms(t)
	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{"1000 letters", strings.Repeat("a", 1000), true},
		{"1000 ş", strings.Repeat("ş", 1000), true},
		{"1000 emoji", strings.Repeat("😀", 1000), true},
		{"1001 letters", strings.Repeat("a", 1001), false},
		{"1000 letters, spaces around", "  " + strings.Repeat("a", 1000) + "\n", true},
		{"empty", "", false},
		{"blank", " \n\t ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.AddMessage(context.Background(), NewMessage{RoomID: heat.ID, UserID: 1, Text: tt.text, VideoID: 1})
			if tt.ok && err != nil {
				t.Errorf("refused: %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrBadMessage) {
				t.Errorf("err = %v, want ErrBadMessage", err)
			}
		})
	}
}

// Pages go back from the newest, 100 at a time, oldest first within a page. Deleted messages are
// skipped.
func TestMessagesPaging(t *testing.T) {
	r, heat, ronin := chatRooms(t)
	var ids []int64
	for i := range 250 {
		ids = append(ids, post(t, r, heat.ID, 1, "m", nil).ID)
		if i%50 == 0 {
			post(t, r, ronin.ID, 2, "elsewhere", nil)
		}
	}
	span := func(ms []Message) (first, last int64, n int) {
		if len(ms) == 0 {
			return 0, 0, 0
		}
		return ms[0].ID, ms[len(ms)-1].ID, len(ms)
	}
	tests := []struct {
		name        string
		before      int64
		first, last int64
		n           int
	}{
		{"newest", 0, ids[150], ids[249], 100},
		{"before the newest page", ids[150], ids[50], ids[149], 100},
		{"the last page", ids[50], ids[0], ids[49], 50},
		{"before the first", ids[0], 0, 0, 0},
	}
	for _, tt := range tests {
		first, last, n := span(messages(t, r, heat.ID, tt.before))
		if first != tt.first || last != tt.last || n != tt.n {
			t.Errorf("%s: %d messages, %d to %d; want %d, %d to %d", tt.name, n, first, last, tt.n, tt.first, tt.last)
		}
	}

	if err := r.DeleteMessage(context.Background(), heat.ID, 1, ids[249]); err != nil {
		t.Fatal(err)
	}
	if first, last, n := span(messages(t, r, heat.ID, 0)); first != ids[149] || last != ids[248] || n != 100 {
		t.Errorf("after deleting the newest: %d messages, %d to %d", n, first, last)
	}
}

func TestMessagesUnknownRoom(t *testing.T) {
	r, _, _ := chatRooms(t)
	if _, err := r.Messages(context.Background(), 99, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestReply(t *testing.T) {
	r, heat, ronin := chatRooms(t)
	ctx := context.Background()
	first := post(t, r, heat.ID, 1, "first", nil)
	reply := post(t, r, heat.ID, 2, "reply", &first.ID)
	want := Quote{ID: first.ID, From: alice, Text: "first"}
	if reply.ReplyTo == nil || *reply.ReplyTo != want {
		t.Errorf("reply quotes %+v, want %+v", reply.ReplyTo, want)
	}

	other := post(t, r, ronin.ID, 1, "other room", nil)
	_, err := r.AddMessage(ctx, NewMessage{RoomID: heat.ID, UserID: 2, Text: "x", ReplyTo: &other.ID, VideoID: 1})
	if !errors.Is(err, ErrBadMessage) {
		t.Errorf("reply to another room's message: err = %v, want ErrBadMessage", err)
	}
	unknown := int64(999)
	_, err = r.AddMessage(ctx, NewMessage{RoomID: heat.ID, UserID: 2, Text: "x", ReplyTo: &unknown, VideoID: 1})
	if !errors.Is(err, ErrBadMessage) {
		t.Errorf("reply to an unknown message: err = %v, want ErrBadMessage", err)
	}
}

// A deleted message is gone from the list. Replies to it, older and newer, quote it as deleted.
func TestDeleteThenReply(t *testing.T) {
	r, heat, _ := chatRooms(t)
	first := post(t, r, heat.ID, 1, "first", nil)
	before := post(t, r, heat.ID, 2, "reply before", &first.ID)
	if err := r.DeleteMessage(context.Background(), heat.ID, 1, first.ID); err != nil {
		t.Fatal(err)
	}
	after := post(t, r, heat.ID, 2, "reply after", &first.ID)
	deleted := Quote{ID: first.ID, From: alice, Deleted: true}
	if after.ReplyTo == nil || *after.ReplyTo != deleted {
		t.Errorf("new reply quotes %+v, want %+v", after.ReplyTo, deleted)
	}
	got := messages(t, r, heat.ID, 0)
	if len(got) != 2 || got[0].ID != before.ID || got[1].ID != after.ID {
		t.Fatalf("messages = %+v, want the two replies", got)
	}
	for _, m := range got {
		if m.ReplyTo == nil || *m.ReplyTo != deleted {
			t.Errorf("%q quotes %+v, want %+v", m.Text, m.ReplyTo, deleted)
		}
	}
}

func TestDeleteMessage(t *testing.T) {
	r, heat, ronin := chatRooms(t)
	ctx := context.Background()
	m := post(t, r, heat.ID, 1, "mine", nil)
	tests := []struct {
		name           string
		room, user, id int64
	}{
		{"someone else's", heat.ID, 2, m.ID},
		{"through another room", ronin.ID, 1, m.ID},
		{"unknown", heat.ID, 1, 999},
	}
	for _, tt := range tests {
		if err := r.DeleteMessage(ctx, tt.room, tt.user, tt.id); !errors.Is(err, ErrNotYours) {
			t.Errorf("%s: err = %v, want ErrNotYours", tt.name, err)
		}
	}
	if got := messages(t, r, heat.ID, 0); len(got) != 1 || got[0].Text != "mine" {
		t.Errorf("after refused deletes: %+v, want the message still there", got)
	}

	if err := r.DeleteMessage(ctx, heat.ID, 1, m.ID); err != nil {
		t.Fatal(err)
	}
	var text string
	if err := r.DB.QueryRow("SELECT text FROM messages WHERE id = ?", m.ID).Scan(&text); err != nil || text != "" {
		t.Errorf("deleted row keeps text %q (%v), want it wiped", text, err)
	}
	if err := r.DeleteMessage(ctx, heat.ID, 1, m.ID); !errors.Is(err, ErrNotYours) {
		t.Errorf("delete again: err = %v, want ErrNotYours", err)
	}
}

func TestArchivedChatIsReadOnly(t *testing.T) {
	r, heat, _ := chatRooms(t)
	ctx := context.Background()
	m := post(t, r, heat.ID, 1, "before", nil)
	if _, err := r.SetArchived(ctx, heat.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AddMessage(ctx, NewMessage{RoomID: heat.ID, UserID: 1, Text: "x", VideoID: 1}); !errors.Is(err, ErrArchived) {
		t.Errorf("add: err = %v, want ErrArchived", err)
	}
	if err := r.DeleteMessage(ctx, heat.ID, 1, m.ID); !errors.Is(err, ErrArchived) {
		t.Errorf("delete: err = %v, want ErrArchived", err)
	}
	if got := messages(t, r, heat.ID, 0); len(got) != 1 {
		t.Errorf("archived room's chat: %+v, want it still readable", got)
	}
}

// Deleting a room deletes its chat, replies included, and leaves other rooms' alone.
func TestDeleteRoomDeletesChat(t *testing.T) {
	r, heat, ronin := chatRooms(t)
	first := post(t, r, heat.ID, 1, "first", nil)
	post(t, r, heat.ID, 2, "reply", &first.ID)
	post(t, r, ronin.ID, 1, "stays", nil)
	if err := r.Delete(context.Background(), heat.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := r.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE room_id = ?", heat.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("deleted room keeps %d messages (%v)", n, err)
	}
	if got := messages(t, r, ronin.ID, 0); len(got) != 1 {
		t.Errorf("other room: %+v, want its message", got)
	}
}

// A message names its sender by their current name.
func TestMessageSenderRenamed(t *testing.T) {
	r, heat, _ := chatRooms(t)
	post(t, r, heat.ID, 1, "hi", nil)
	if _, err := r.DB.Exec("UPDATE users SET name = 'Alicia' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if got := messages(t, r, heat.ID, 0); got[0].From.Name != "Alicia" {
		t.Errorf("from = %+v, want Alicia", got[0].From)
	}
}
