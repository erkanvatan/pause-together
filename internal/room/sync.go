package room

import (
	"reflect"
	"slices"
)

// Who is a user as the room shows them.
type Who struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
}

// Lag is how far a skipped client is behind the room, in whole seconds: "Alice is 3 s behind".
type Lag struct {
	Who
	Ms int64 `json:"ms"`
}

// Status is what a client reports about its player. It can make the room wait, but never changes
// Playing: only intents do.
type Status int

const (
	Ready     Status = iota + 1 // can play at the room's position
	Buffering                   // loading, stalled or seeking
	Away                        // tab hidden or app in the background
	CantPlay                    // this device can't decode the video
)

// State is a room's playback state, as sent to everyone in it.
type State struct {
	VideoID          int64     `json:"videoId"`
	Audio            *int      `json:"audio"`
	Subtitle         *Subtitle `json:"subtitle"`
	SubtitleOffsetMs int64     `json:"subtitleOffsetMs"`
	DurationMs       int64     `json:"durationMs"`
	// Playing is what people asked for. The room's clock runs only while Playing and nobody is waiting.
	Playing bool `json:"playing"`
	// PositionMs is the position at server time AtMs.
	PositionMs int64 `json:"positionMs"`
	AtMs       int64 `json:"atMs"`
	// Waiting lists who the room waits for, once per user, only while Playing. Never nil, so it's sent as [].
	Waiting []Who `json:"waiting"`
	// Behind lists the clients skipped by "Play anyway", once per user. Never nil, like Waiting.
	Behind []Lag `json:"behind"`
}

// Position is where the video is at server time now.
func (st State) Position(now int64) int64 {
	p := st.PositionMs
	if st.Playing && len(st.Waiting) == 0 {
		p += now - st.AtMs
	}
	return clamp(p, st.DurationMs)
}

// Effect is what an event asks of the room's owner.
type Effect struct {
	Changed  bool // send State to everyone: it changed, or an intent's sender needs the snap
	Save     bool // write the position now
	PausedBy *Who // who paused, for the "Alice paused" note; nil for automatic pauses
}

// client is one socket in the room.
type client struct {
	id        int64
	who       Who
	status    Status // 0: nothing reported yet
	since     int64  // when its current stall began
	posMs     int64  // reported position
	posAt     int64  // when it was reported
	readyOnce bool
	skipped   bool // by "Play anyway", until it catches up
}

// Sync holds one room's sync rules: pure, no IO. Time is milliseconds of server time, passed in.
type Sync struct {
	videoID          int64
	audio            *int
	subtitle         *Subtitle
	subtitleOffsetMs int64
	durationMs       int64
	playing          bool
	running          bool // the clock runs: playing and nobody to wait for
	positionMs, atMs int64
	waiting          []Who
	clients          []*client // in join order
	savedAt          int64
	sent             State
}

// NewSync loads a room paused at its stored position.
func NewSync(r Room, now int64) *Sync {
	s := &Sync{atMs: now}
	s.load(r)
	s.positionMs = clamp(r.PositionMs, s.durationMs)
	s.subtitleOffsetMs = r.SubtitleOffsetMs
	s.sent = s.State(now)
	return s
}

func (s *Sync) load(r Room) {
	s.videoID, s.audio, s.subtitle = r.Video.ID, r.Audio, r.Subtitle
	s.durationMs = r.Video.DurationMs
}

// State returns the room's state at server time now.
func (s *Sync) State(now int64) State {
	st := State{
		VideoID:          s.videoID,
		Audio:            s.audio,
		Subtitle:         s.subtitle,
		SubtitleOffsetMs: s.subtitleOffsetMs,
		DurationMs:       s.durationMs,
		Playing:          s.playing,
		PositionMs:       s.positionMs,
		AtMs:             s.atMs,
		Waiting:          append([]Who{}, s.waiting...),
		Behind:           []Lag{},
	}
	room := s.pos(now)
	for _, c := range s.clients {
		if !c.skipped {
			continue
		}
		lag := room - s.clientPos(c, now)
		if lag <= CaughtUpMs {
			continue
		}
		lag = lag / 1000 * 1000
		if i := slices.IndexFunc(st.Behind, func(l Lag) bool { return l.UserID == c.who.UserID }); i >= 0 {
			st.Behind[i].Ms = max(st.Behind[i].Ms, lag)
		} else {
			st.Behind = append(st.Behind, Lag{Who: c.who, Ms: lag})
		}
	}
	return st
}

// Join adds a socket. It blocks nobody until it has been ready once.
func (s *Sync) Join(id int64, w Who, now int64) Effect {
	s.clients = append(s.clients, &client{id: id, who: w})
	return s.settle(now, Effect{})
}

// Leave removes a socket. When the last one goes, the room pauses.
func (s *Sync) Leave(id, now int64) Effect {
	s.clients = slices.DeleteFunc(s.clients, func(c *client) bool { return c.id == id })
	var e Effect
	if len(s.clients) == 0 && s.playing {
		s.rebase(now)
		s.playing = false
		e.Save = true
	}
	return s.settle(now, e)
}

// Play starts the room, unless it sits at the end of its video.
func (s *Sync) Play(now int64) Effect {
	if s.durationMs == 0 || s.pos(now) < s.durationMs {
		s.playing = true
	}
	return s.settle(now, Effect{Changed: true})
}

// Pause stops the room. Pausing a paused room changes nothing, and names nobody.
func (s *Sync) Pause(id, now int64) Effect {
	if !s.playing {
		return s.settle(now, Effect{Changed: true})
	}
	s.rebase(now)
	s.playing = false
	e := Effect{Changed: true, Save: true}
	if c := s.client(id); c != nil {
		e.PausedBy = &c.who
	}
	return s.settle(now, e)
}

// SeekTo moves to posMs, kept inside the video.
func (s *Sync) SeekTo(posMs, now int64) Effect {
	s.positionMs, s.atMs = clamp(posMs, s.durationMs), now
	return s.settle(now, Effect{Changed: true, Save: true})
}

// PlayAnyway skips everyone the room waits for, until each catches up or comes back.
func (s *Sync) PlayAnyway(now int64) Effect {
	for _, c := range s.clients {
		if s.blocks(c, now) {
			c.skipped = true
		}
	}
	return s.settle(now, Effect{Changed: true})
}

// Switch puts on the room's new video at 0:00, paused. Who has been ready once stays so: pressing
// play waits for everyone to load the new video.
func (s *Sync) Switch(r Room, now int64) Effect {
	s.load(r)
	s.playing, s.running = false, false
	s.positionMs, s.atMs = 0, now
	for _, c := range s.clients {
		c.skipped = false
	}
	return s.settle(now, Effect{Changed: true, Save: true})
}

func (s *Sync) SetSubtitle(sub *Subtitle, now int64) Effect {
	s.subtitle = sub
	return s.settle(now, Effect{Changed: true, Save: true})
}

func (s *Sync) SetOffset(ms, now int64) Effect {
	s.subtitleOffsetMs = ms
	return s.settle(now, Effect{Changed: true, Save: true})
}

// Status records what a socket reports, with its position. A skipped socket is no longer skipped once
// it is ready within CaughtUpMs of the room, or can't play at all. Going from buffering to away, or
// back, doesn't restart the stall.
func (s *Sync) Status(id int64, st Status, posMs, now int64) Effect {
	c := s.client(id)
	if c == nil {
		return Effect{}
	}
	if stalled(st) && !stalled(c.status) {
		c.since = now
	}
	c.status, c.posMs, c.posAt = st, posMs, now
	if st == Ready {
		c.readyOnce = true
		if lag := s.pos(now) - posMs; c.skipped && lag <= CaughtUpMs && lag >= -CaughtUpMs {
			c.skipped = false
		}
	}
	if st == CantPlay {
		c.skipped = false
	}
	return s.settle(now, Effect{})
}

// Tick applies what time alone changes: a stall passing its grace, the video ending, the 5 s save.
func (s *Sync) Tick(now int64) Effect {
	return s.settle(now, Effect{})
}

// settle applies the rules that follow from any event, then fills in the rest of e.
func (s *Sync) settle(now int64, e Effect) Effect {
	if s.running && s.durationMs > 0 && s.pos(now) >= s.durationMs {
		s.rebase(now)
		s.playing = false
		e.Save = true
	}
	s.waiting = nil
	for _, c := range s.clients {
		if s.blocks(c, now) && !slices.ContainsFunc(s.waiting, func(w Who) bool { return w.UserID == c.who.UserID }) {
			s.waiting = append(s.waiting, c.who)
		}
	}
	if running := s.playing && len(s.waiting) == 0; running != s.running {
		s.rebase(now)
		s.running = running
		if running {
			s.savedAt = now
		} else {
			e.Save = true
		}
	}
	if s.running && now-s.savedAt >= SaveEveryMs {
		e.Save = true
	}
	if e.Save {
		s.savedAt = now
	}
	if st := s.State(now); !reflect.DeepEqual(st, s.sent) {
		e.Changed = true
		s.sent = st
	}
	return e
}

// blocks reports whether the room waits for c: it wants to play, and c has been ready once, isn't
// skipped, and has been buffering or away for longer than StallGraceMs.
func (s *Sync) blocks(c *client, now int64) bool {
	return s.playing && c.readyOnce && !c.skipped && stalled(c.status) && now-c.since > StallGraceMs
}

func stalled(st Status) bool { return st == Buffering || st == Away }

// pos is the room's position at now.
func (s *Sync) pos(now int64) int64 {
	p := s.positionMs
	if s.running {
		p += now - s.atMs
	}
	return clamp(p, s.durationMs)
}

// rebase moves the stored position to now, before the clock starts or stops.
func (s *Sync) rebase(now int64) {
	s.positionMs, s.atMs = s.pos(now), now
}

// clientPos is where c's video is at now: it runs on only while c is ready and the room plays.
func (s *Sync) clientPos(c *client, now int64) int64 {
	if c.status == Ready && s.running {
		return c.posMs + now - c.posAt
	}
	return c.posMs
}

func (s *Sync) client(id int64) *client {
	if i := slices.IndexFunc(s.clients, func(c *client) bool { return c.id == id }); i >= 0 {
		return s.clients[i]
	}
	return nil
}

// clamp keeps p inside a video of durationMs (0: unknown, so no end).
func clamp(p, durationMs int64) int64 {
	if durationMs > 0 {
		p = min(p, durationMs)
	}
	return max(p, 0)
}
