package room

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/coder/websocket"

	"github.com/erkanvatan/pause-together/internal/media"
)

// loop owns one room's state. Everything reaches it through in, and runs on its goroutine: no locks.
// It stops when the room is deleted, the hub stops, or nobody has been in the room for PresenceGraceMs.
type loop struct {
	hub  *Hub
	id   int64
	in   chan func() // unbuffered: a send succeeds only while the loop runs
	done chan struct{}

	room      Room // as the pages show it; its position and subtitle may lag behind sync's
	sync      *Sync
	presence  Presence
	sockets   []*socket
	key       string  // the cache key of the room's prepared copy; "" when it can't play
	prepare   Prepare // as last sent
	watching  []Who   // as last sent
	gone      bool    // deleted
	touchedMs int64   // when the loop last marked its copy used; 0 = not yet
}

func newLoop(h *Hub, rm Room) *loop {
	return &loop{hub: h, id: rm.ID, in: make(chan func()), done: make(chan struct{}), room: rm,
		prepare: Prepare{Subtitles: []int{}}}
}

// do runs f on the loop. It reports false when the loop has stopped: f never runs.
func (l *loop) do(f func()) bool {
	select {
	case l.in <- f:
		return true
	case <-l.done:
		return false
	}
}

func (l *loop) run(ctx context.Context) {
	defer close(l.done)
	// Read the room again: the joiner opened it before the hub counted the join, so an archive or a
	// delete could slip in between. From here on the hub refuses the archive and tells the loop of
	// the delete.
	switch rm, err := l.hub.Rooms.opened(ctx, l.id); {
	case errors.Is(err, ErrNotFound):
		l.deleted()
		return
	case err != nil:
		slog.Error("reload room", "room", l.id, "err", err)
	default:
		l.room = rm
	}
	l.sync = NewSync(l.room, l.hub.now())
	tick := time.NewTicker(TickMs * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			// The server is stopping. Pause the room and tell the pages, or they play on without it and
			// jump back when it returns. Then save where the room is: a playing room last saved up to
			// SaveEveryMs ago. ctx is done, but the database stays open until every loop has stopped.
			now := l.hub.now()
			l.sync.Pause(0, now)
			l.broadcast(StateMsg{Type: MsgState, State: l.sync.State(now)})
			l.save(context.WithoutCancel(ctx), now)
			for _, s := range l.sockets {
				s.closeWith(websocket.StatusGoingAway)
			}
			return
		case f := <-l.in:
			f()
		case <-tick.C:
			now := l.hub.now()
			l.apply(l.sync.Tick(now), now)
			// A ready copy stays: the cache clean-up keeps a copy while touch marks it used. Joins, switches
			// and unarchiving check again.
			if l.prepare.State != media.JobReady {
				l.checkPrepare()
			}
			l.touch(now)
		}
		l.publish(l.hub.now())
		if l.gone || l.idle() {
			return
		}
	}
}

// join adds s. It gets the whole picture; everyone else, only what changed.
func (l *loop) join(s *socket) {
	now := l.hub.now()
	l.presence.Join(s.who)
	e := l.sync.Join(s.id, s.who, s.browser, now)
	// The file may have changed since the key was found: a rescan, or a server restart.
	l.findKey()
	l.checkPrepare()
	l.apply(e, now)
	l.publish(now)
	s.send(encode(RoomMsg{Type: MsgRoom, Room: l.room}))
	s.send(encode(StateMsg{Type: MsgState, State: l.sync.State(now)}))
	s.send(encode(PrepareMsg{Type: MsgPrepare, Prepare: l.prepare}))
	if ms, err := l.hub.Rooms.Messages(l.hub.ctx, l.id, 0); err != nil {
		slog.Error("load chat", "room", l.id, "err", err)
	} else {
		s.send(encode(ChatHistoryMsg{Type: MsgChatHistory, Messages: ms}))
	}
	s.send(encode(PresenceMsg{Type: MsgPresence, Watching: l.watching}))
	l.sockets = append(l.sockets, s)
}

// leave takes s out. left: the page closed it on purpose.
func (l *loop) leave(s *socket, left bool) {
	now := l.hub.now()
	l.sockets = slices.DeleteFunc(l.sockets, func(o *socket) bool { return o == s })
	l.presence.Leave(s.who.UserID, now, left)
	l.apply(l.sync.Leave(s.id, now), now)
	l.hub.mu.Lock()
	if r := l.hub.rooms[l.id]; r != nil && r.loop == l {
		r.sockets--
	}
	l.hub.mu.Unlock()
}

// intent applies a client's message. In an archived room, which can't play, intents only get the
// state back, so the sender snaps to it.
func (l *loop) intent(s *socket, m ClientMsg) {
	now := l.hub.now()
	switch m.Type {
	case MsgStatus:
		if st, ok := statusNames[m.Status]; ok {
			l.apply(l.sync.Status(s.id, st, ms(m.PositionMs), now), now)
		}
		return
	case MsgChat:
		l.chat(s, m, now)
		return
	case MsgDeleteChat:
		// Only its sender deletes a message, and never in an archived room. The page offers nothing
		// else, so a refusal is dropped.
		if err := l.hub.Rooms.DeleteMessage(l.hub.ctx, l.id, s.who.UserID, m.ID); err != nil {
			slog.Debug("delete chat message", "room", l.id, "err", err)
			return
		}
		l.broadcast(ChatDeletedMsg{Type: MsgChatDeleted, ID: m.ID})
		return
	}
	snap := func() { s.send(encode(StateMsg{Type: MsgState, State: l.sync.State(now)})) }
	if l.room.Archived {
		snap()
		return
	}
	var e Effect
	switch m.Type {
	case MsgPlay:
		e = l.sync.Play(now)
	case MsgPause:
		e = l.sync.Pause(s.id, now)
	case MsgSeek:
		e = l.sync.SeekTo(ms(m.PositionMs), now)
	case MsgPlayAnyway:
		e = l.sync.PlayAnyway(now)
	case MsgSubtitle:
		if err := l.hub.Rooms.checkSubtitle(l.hub.ctx, l.room.Video.ID, m.Subtitle); err != nil {
			snap()
			return
		}
		e = l.sync.SetSubtitle(m.Subtitle, now)
	case MsgOffset:
		e = l.sync.SetOffset(ms(m.Ms), now)
	default:
		return
	}
	l.apply(e, now)
}

// chat stores a message with what the room plays, and where, and sends it to everyone. The page never
// sends one the server refuses (blank, too long, an archived room), so a refusal is dropped.
func (l *loop) chat(s *socket, m ClientMsg, now int64) {
	st := l.sync.State(now)
	msg, err := l.hub.Rooms.AddMessage(l.hub.ctx, NewMessage{RoomID: l.id, UserID: s.who.UserID, Text: m.Text,
		ReplyTo: m.ReplyTo, VideoID: st.VideoID, PositionMs: st.Position(now), SentAt: time.Now().UnixMilli()})
	if err != nil {
		slog.Debug("add chat message", "room", l.id, "err", err)
		return
	}
	l.broadcast(ChatMsg{Type: MsgChat, Message: msg})
}

// switched puts on the room's new video.
func (l *loop) switched(rm Room) {
	now := l.hub.now()
	l.room = rm
	l.broadcast(RoomMsg{Type: MsgRoom, Room: rm})
	l.apply(l.sync.Switch(rm, now), now)
	l.findKey()
	l.checkPrepare()
}

// changed shows a renamed or unarchived room. It takes only the name and the archived flag: rm was
// read before this call, and a switch may have reached the loop since. An archived room's copy may have
// been cleaned up, so unarchiving checks it again.
func (l *loop) changed(rm Room) {
	unarchived := l.room.Archived && !rm.Archived
	l.room.Name, l.room.Archived = rm.Name, rm.Archived
	l.broadcast(RoomMsg{Type: MsgRoom, Room: l.room})
	if unarchived {
		l.findKey()
		l.checkPrepare()
	}
}

func (l *loop) deleted() {
	l.gone = true
	for _, s := range l.sockets {
		l.hub.gone(s)
	}
	l.hub.mu.Lock()
	if r := l.hub.rooms[l.id]; r != nil && r.loop == l {
		delete(l.hub.rooms, l.id)
	}
	l.hub.mu.Unlock()
}

// apply carries out what an event asks of the loop.
func (l *loop) apply(e Effect, now int64) {
	st := l.sync.State(now)
	if e.Changed {
		l.broadcast(StateMsg{Type: MsgState, State: st})
	}
	if e.PausedBy != nil {
		l.broadcast(PausedMsg{Type: MsgPaused, By: *e.PausedBy})
	}
	if e.Save {
		l.save(l.hub.ctx, now)
	}
}

// save stores the room's position, subtitle and offset.
func (l *loop) save(ctx context.Context, now int64) {
	st := l.sync.State(now)
	st.PositionMs = st.Position(now)
	if err := l.hub.Rooms.SaveState(ctx, l.id, st); err != nil {
		slog.Error("save room state", "room", l.id, "err", err)
	}
}

// touch marks the room's copy used, at once and then every TouchEveryMs, so the cache clean-up keeps
// it while the room is in use. Opening a room marks it too, but not when the copy plays on after its
// source is gone. An archived room can't play, so it doesn't count.
func (l *loop) touch(now int64) {
	if l.key == "" || l.room.Archived || (l.touchedMs != 0 && now-l.touchedMs < TouchEveryMs) {
		return
	}
	l.hub.Rooms.Jobs.Touch(l.key)
	l.touchedMs = now
}

// publish sends presence when it changed, and gives the hub its watching list.
func (l *loop) publish(now int64) {
	watching := l.presence.Watching(now)
	if reflect.DeepEqual(watching, l.watching) {
		return
	}
	l.watching = watching
	l.broadcast(PresenceMsg{Type: MsgPresence, Watching: watching})
	l.hub.mu.Lock()
	if r := l.hub.rooms[l.id]; r != nil && r.loop == l {
		r.watching = watching
	}
	l.hub.mu.Unlock()
}

// findKey looks up the cache key of the room's video and audio track. A copy that is ready still plays
// when its source is gone.
func (l *loop) findKey() {
	key, err := l.hub.Rooms.playableKey(l.hub.ctx, l.room.Video.ID, l.room.Audio)
	if err != nil {
		slog.Error("find prepare job", "room", l.id, "err", err)
	}
	if key != l.key {
		l.touchedMs = 0 // a new copy: touch it on the next tick
	}
	l.key = key
}

// checkPrepare sends where the room's prepared copy stands, when that changed.
func (l *loop) checkPrepare() {
	p := Prepare{Subtitles: []int{}}
	if l.key != "" {
		j := l.hub.Rooms.Jobs.Status(l.key)
		// Whole percents: finer steps would send a message on every tick.
		p.State, p.Place, p.Progress, p.Error = j.State, j.Place, math.Round(j.Progress*100)/100, j.Error
		if j.State == media.JobReady {
			p.Key = l.key
			p.Subtitles = l.hub.Rooms.Jobs.Subtitles(l.key)
		}
	}
	if reflect.DeepEqual(p, l.prepare) {
		return
	}
	l.prepare = p
	l.broadcast(PrepareMsg{Type: MsgPrepare, Prepare: p})
}

// idle reports whether the loop can stop: nobody in the room, nobody within their reconnect grace,
// and no join on its way. It then takes the loop out of the hub; the next join starts a new one.
func (l *loop) idle() bool {
	if len(l.sockets) > 0 || len(l.watching) > 0 {
		return false
	}
	l.hub.mu.Lock()
	defer l.hub.mu.Unlock()
	r := l.hub.rooms[l.id]
	if r == nil || r.loop != l {
		return true
	}
	if r.sockets > 0 {
		return false
	}
	delete(l.hub.rooms, l.id)
	return true
}

func (l *loop) broadcast(msg any) {
	b := encode(msg)
	for _, s := range l.sockets {
		s.send(b)
	}
}

// ms rounds a client's milliseconds to whole ones.
func ms(v float64) int64 { return int64(math.Round(v)) }
