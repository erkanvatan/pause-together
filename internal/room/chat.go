package room

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/erkanvatan/pause-together/internal/library"
)

const (
	// MaxMessageRunes is the longest chat message, in runes after trimming. web/src/lib/chat.ts counts
	// the same way.
	MaxMessageRunes = 1000
	// ChatPageSize is how many messages load at once: on joining, and per page on scrolling up.
	ChatPageSize = 100
)

var (
	// ErrBadMessage: blank, too long, or a reply to a message from another room.
	ErrBadMessage = errors.New("bad chat message")
	// ErrNotYours: a delete of someone else's message, or of one that isn't there (any more).
	ErrNotYours = errors.New("no such message of yours")
)

// Message is a chat message as the room shows it. Deleted messages are never shown.
type Message struct {
	ID   int64  `json:"id"`
	From Who    `json:"from"` // the sender's current name
	Text string `json:"text"`
	// SentAt is the wall clock when it was sent, in Unix milliseconds.
	SentAt int64 `json:"sentAt"`
	// Video and PositionMs are what the room played, and where, when it was sent.
	Video      library.VideoSummary `json:"video"`
	PositionMs int64                `json:"positionMs"`
	ReplyTo    *Quote               `json:"replyTo"`
}

// Quote is the message a reply answers.
type Quote struct {
	ID      int64  `json:"id"`
	From    Who    `json:"from"`
	Text    string `json:"text"` // "" once deleted
	Deleted bool   `json:"deleted"`
}

// NewMessage is a message to add. The room's loop fills in the video, position and time.
type NewMessage struct {
	RoomID, UserID int64
	Text           string
	ReplyTo        *int64
	VideoID        int64
	PositionMs     int64
	SentAt         int64
}

// AddMessage stores a chat message and returns it. Text is trimmed, then must be 1 to MaxMessageRunes
// runes. A reply may answer a deleted message: it was there when its sender started typing. An archived
// room's chat is read-only: ErrArchived.
func (r *Rooms) AddMessage(ctx context.Context, n NewMessage) (Message, error) {
	text := strings.TrimSpace(n.Text)
	if c := utf8.RuneCountInString(text); c < 1 || c > MaxMessageRunes {
		return Message{}, ErrBadMessage
	}
	if err := r.writable(ctx, n.RoomID); err != nil {
		return Message{}, err
	}
	if n.ReplyTo != nil {
		var ok bool
		if err := r.DB.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM messages WHERE id = ? AND room_id = ?)",
			*n.ReplyTo, n.RoomID).Scan(&ok); err != nil {
			return Message{}, err
		}
		if !ok {
			return Message{}, ErrBadMessage
		}
	}
	var id int64
	if err := r.DB.QueryRowContext(ctx, `
		INSERT INTO messages (room_id, user_id, text, reply_to, video_id, position_ms, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		n.RoomID, n.UserID, text, n.ReplyTo, n.VideoID, n.PositionMs, n.SentAt).Scan(&id); err != nil {
		return Message{}, err
	}
	ms, err := r.queryMessages(ctx, "m.id = ?", id)
	if err != nil {
		return Message{}, err
	}
	if len(ms) != 1 {
		return Message{}, errors.New("added message not found")
	}
	return ms[0], nil
}

// Messages returns up to ChatPageSize of a room's messages sent before the one with id before (0: the
// newest), oldest first. Never nil.
func (r *Rooms) Messages(ctx context.Context, roomID, before int64) ([]Message, error) {
	if _, err := r.get(ctx, roomID); err != nil {
		return nil, err
	}
	if before == 0 {
		before = math.MaxInt64
	}
	// Newest first, to take the page next to the cursor, then turned around.
	ms, err := r.queryMessages(ctx, "m.room_id = ? AND NOT m.deleted AND m.id < ? ORDER BY m.id DESC LIMIT ?",
		roomID, before, ChatPageSize)
	for i, j := 0, len(ms)-1; i < j; i, j = i+1, j-1 {
		ms[i], ms[j] = ms[j], ms[i]
	}
	return ms, err
}

// DeleteMessage deletes one of the user's own messages in a room. Its row stays, text wiped, so
// replies to it can say it was deleted.
func (r *Rooms) DeleteMessage(ctx context.Context, roomID, userID, id int64) error {
	if err := r.writable(ctx, roomID); err != nil {
		return err
	}
	res, err := r.DB.ExecContext(ctx, `
		UPDATE messages SET text = '', deleted = 1
		WHERE id = ? AND room_id = ? AND user_id = ? AND NOT deleted`, id, roomID, userID)
	if err := notFound(res, err); errors.Is(err, ErrNotFound) {
		return ErrNotYours
	} else if err != nil {
		return err
	}
	return nil
}

// writable returns ErrArchived when a room's chat is read-only, or ErrNotFound.
func (r *Rooms) writable(ctx context.Context, roomID int64) error {
	var archived bool
	err := r.DB.QueryRowContext(ctx, "SELECT archived FROM rooms WHERE id = ?", roomID).Scan(&archived)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return err
	case archived:
		return ErrArchived
	}
	return nil
}

// queryMessages returns the messages matching where (which may end in ORDER BY and LIMIT), with their
// senders, quotes and videos.
func (r *Rooms) queryMessages(ctx context.Context, where string, args ...any) ([]Message, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT m.id, m.user_id, u.name, m.text, m.sent_at, m.video_id, m.position_ms,
			q.id, q.user_id, qu.name, q.text, q.deleted
		FROM messages m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN messages q ON q.id = m.reply_to
		LEFT JOIN users qu ON qu.id = q.user_id
		WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	ms := []Message{}
	var videoIDs []int64
	for rows.Next() {
		var m Message
		var videoID int64
		var qID, qUser sql.NullInt64
		var qName, qText sql.NullString
		var qDeleted sql.NullBool
		if err := rows.Scan(&m.ID, &m.From.UserID, &m.From.Name, &m.Text, &m.SentAt, &videoID, &m.PositionMs,
			&qID, &qUser, &qName, &qText, &qDeleted); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if qID.Valid {
			m.ReplyTo = &Quote{ID: qID.Int64, From: Who{UserID: qUser.Int64, Name: qName.String}, Text: qText.String,
				Deleted: qDeleted.Bool}
		}
		ms = append(ms, m)
		videoIDs = append(videoIDs, videoID)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	// A page spans few videos: read each once.
	videos := map[int64]library.VideoSummary{}
	for i, id := range videoIDs {
		v, ok := videos[id]
		if !ok {
			ref, err := r.Library.Video(ctx, id)
			if err != nil {
				return nil, err
			}
			v = ref.VideoSummary
			videos[id] = v
		}
		ms[i].Video = v
	}
	return ms, nil
}
