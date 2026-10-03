package room

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"
)

var ErrWatching = errors.New("someone is watching the room")

const (
	// sendBuffer is how many messages a socket may fall behind by. A socket further behind is dropped,
	// so a slow one never holds up its room; it reconnects and gets the full state.
	sendBuffer = 64
	// readLimit caps a client message. The biggest is a chat message: MaxMessageRunes runes, which the
	// page's JSON.stringify writes in up to 6 bytes each (a control character as \u0001).
	readLimit = 8 << 10
)

// Hub runs one loop per room with people in it, and connects sockets to them.
type Hub struct {
	Rooms   *Rooms
	BuildID string
	// Heartbeat is HeartbeatTimeoutMs. Tests shorten it.
	Heartbeat time.Duration

	ctx   context.Context // stops every loop
	start time.Time       // server time is milliseconds since then
	loops sync.WaitGroup

	mu         sync.Mutex
	rooms      map[int64]*hubRoom
	lastSocket int64
}

// hubRoom is what the hub knows of a room's loop, under Hub.mu.
type hubRoom struct {
	loop *loop
	// sockets counts joined sockets that haven't left. A join counts itself here before its loop hears
	// of it, so neither SetArchived nor the loop stopping when idle can miss it.
	sockets  int
	watching []Who // the loop's latest presence
}

// NewHub returns a hub whose loops stop when ctx is done.
func NewHub(ctx context.Context, rooms *Rooms, buildID string) *Hub {
	return &Hub{
		Rooms:     rooms,
		BuildID:   buildID,
		Heartbeat: HeartbeatTimeoutMs * time.Millisecond,
		ctx:       ctx,
		start:     time.Now(),
		rooms:     map[int64]*hubRoom{},
	}
}

// Wait waits for every loop to stop, after the hub's context is done.
func (h *Hub) Wait() { h.loops.Wait() }

// now is server time: monotonic, in milliseconds since the hub started.
func (h *Hub) now() int64 { return time.Since(h.start).Milliseconds() }

// Serve runs one socket of who's in room rm, just opened, until it closes. browser tells one browser
// from another (see client.browser).
func (h *Hub) Serve(ctx context.Context, conn *websocket.Conn, rm Room, who Who, browser string) {
	conn.SetReadLimit(readLimit)
	ctx, drop := context.WithCancel(ctx)
	defer drop()
	s := &socket{who: who, browser: browser, out: make(chan []byte, sendBuffer), quit: make(chan struct{}), drop: drop}
	s.send(encode(HelloMsg{Type: MsgHello, BuildID: h.BuildID, UserID: who.UserID}))
	l := h.join(rm, s)

	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		s.write(ctx, conn)
	}()
	left := h.read(ctx, conn, l, s)
	drop()
	<-wrote
	_ = conn.CloseNow()
	l.do(func() { l.leave(s, left) })
}

// join counts s in, starting rm's loop if it has none, and hands s to the loop.
func (h *Hub) join(rm Room, s *socket) *loop {
	h.mu.Lock()
	h.lastSocket++
	s.id = h.lastSocket
	r := h.rooms[rm.ID]
	if r == nil {
		r = &hubRoom{loop: newLoop(h, rm)}
		h.rooms[rm.ID] = r
		l := r.loop
		h.loops.Go(func() { l.run(h.ctx) })
	}
	r.sockets++
	l := r.loop
	h.mu.Unlock()
	if !l.do(func() { l.join(s) }) {
		h.gone(s) // deleted, or shutting down, since it was looked up
	}
	return l
}

// read passes s's messages to its loop, and answers pings itself. It returns when the socket closes,
// or sends nothing for Heartbeat. It reports whether the page left on purpose: a clean close, which a
// browser sends when the tab closes or the page moves on, unlike a dropped connection.
func (h *Hub) read(ctx context.Context, conn *websocket.Conn, l *loop, s *socket) (left bool) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, h.Heartbeat)
		_, b, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			code := websocket.CloseStatus(err)
			return code == websocket.StatusNormalClosure || code == websocket.StatusGoingAway
		}
		var m ClientMsg
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if m.Type == MsgPing {
			s.send(encode(PongMsg{Type: MsgPong, T: m.T, ServerMs: h.now()}))
			continue
		}
		if !l.do(func() { l.intent(s, m) }) {
			return false
		}
	}
}

// gone closes a socket whose loop has stopped: its room was deleted, or the server is stopping.
func (h *Hub) gone(s *socket) {
	if h.ctx.Err() != nil {
		s.closeWith(websocket.StatusGoingAway)
		return
	}
	s.send(encode(TypeMsg{Type: MsgDeleted}))
	s.closeWith(websocket.StatusNormalClosure)
}

// loop returns a room's loop, or nil when nobody is in it.
func (h *Hub) loop(id int64) *loop {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[id]; r != nil {
		return r.loop
	}
	return nil
}

// Switched tells a room's loop that the room switched to another video.
func (h *Hub) Switched(rm Room) {
	if l := h.loop(rm.ID); l != nil {
		l.do(func() { l.switched(rm) })
	}
}

// Changed tells a room's loop that the room was renamed or unarchived.
func (h *Hub) Changed(rm Room) {
	if l := h.loop(rm.ID); l != nil {
		l.do(func() { l.changed(rm) })
	}
}

// Deleted tells everyone in a deleted room, and closes their sockets.
func (h *Hub) Deleted(id int64) {
	if l := h.loop(id); l != nil {
		l.do(l.deleted)
	}
}

// Watching returns who is watching a room now. Never nil.
func (h *Hub) Watching(id int64) []Who {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[id]; r != nil && r.watching != nil {
		return slices.Clone(r.watching)
	}
	return []Who{}
}

// SetArchived archives or unarchives a room. Archiving is refused with ErrWatching while anyone is
// watching it, their reconnect grace included. The hub's lock is held from the check to the write, so
// nobody joins in between.
func (h *Hub) SetArchived(ctx context.Context, id int64, archived bool) (Room, error) {
	h.mu.Lock()
	if r := h.rooms[id]; archived && r != nil && (r.sockets > 0 || len(r.watching) > 0) {
		h.mu.Unlock()
		return Room{}, ErrWatching
	}
	rm, err := h.Rooms.SetArchived(ctx, id, archived)
	h.mu.Unlock()
	if err == nil {
		h.Changed(rm)
	}
	return rm, err
}

// socket is one open room page.
type socket struct {
	id      int64
	who     Who
	browser string
	out     chan []byte
	drop    context.CancelFunc // closes the socket at once

	quit      chan struct{} // closed by closeWith: send what's queued, then close
	closeOnce sync.Once
	code      websocket.StatusCode
}

// send queues a message. A socket too far behind is dropped.
func (s *socket) send(b []byte) {
	if b == nil {
		return
	}
	select {
	case s.out <- b:
	default:
		s.drop()
	}
}

// closeWith closes the socket with code, after the messages already queued.
func (s *socket) closeWith(code websocket.StatusCode) {
	s.closeOnce.Do(func() {
		s.code = code
		close(s.quit)
	})
}

// write sends queued messages until ctx is done, a write fails or closeWith is called.
func (s *socket) write(ctx context.Context, conn *websocket.Conn) {
	defer s.drop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-s.out:
			if conn.Write(ctx, websocket.MessageText, b) != nil {
				return
			}
		case <-s.quit:
			for {
				select {
				case b := <-s.out:
					if conn.Write(ctx, websocket.MessageText, b) != nil {
						return
					}
				default:
					_ = conn.Close(s.code, "")
					return
				}
			}
		}
	}
}

// encode returns a message's JSON. Every message type encodes; nil only on a bug.
func encode(msg any) []byte {
	b, err := json.Marshal(msg)
	if err != nil {
		slog.Error("encode message", "err", err)
		return nil
	}
	return b
}
