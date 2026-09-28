package room

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/erkanvatan/pause-together/internal/library"
)

var (
	alice = Who{UserID: 1, Name: "Alice"}
	bob   = Who{UserID: 2, Name: "Bob"}
	carol = Who{UserID: 3, Name: "Carol"}
)

const hour = 3_600_000

// scene drives one room's Sync through a scenario. Time only moves with at, which ticks the room as
// its loop does.
type scene struct {
	t    *testing.T
	s    *Sync
	now  int64
	next int64
	e    Effect // the last event's
}

func newScene(t *testing.T, durationMs int64) *scene {
	return &scene{t: t, s: NewSync(videoRoom(1, durationMs, 0), 0)}
}

func videoRoom(video, durationMs, positionMs int64) Room {
	return Room{
		ID:         1,
		Video:      library.VideoRef{VideoSummary: library.VideoSummary{ID: video, DurationMs: durationMs}},
		PositionMs: positionMs,
	}
}

func (sc *scene) join(w Who) int64 {
	sc.next++
	sc.e = sc.s.Join(sc.next, w, sc.now)
	return sc.next
}

// at moves time to ms and ticks the room.
func (sc *scene) at(ms int64) *scene {
	sc.t.Helper()
	if ms < sc.now {
		sc.t.Fatalf("time runs backwards: %d after %d", ms, sc.now)
	}
	sc.now = ms
	sc.e = sc.s.Tick(ms)
	return sc
}

// status reports a socket's status at the room's current position, or at pos when given.
func (sc *scene) status(id int64, st Status, pos ...int64) {
	p := sc.pos()
	if len(pos) > 0 {
		p = pos[0]
	}
	sc.e = sc.s.Status(id, st, p, sc.now)
}

func (sc *scene) play()           { sc.e = sc.s.Play(sc.now) }
func (sc *scene) pause(id int64)  { sc.e = sc.s.Pause(id, sc.now) }
func (sc *scene) seek(pos int64)  { sc.e = sc.s.SeekTo(pos, sc.now) }
func (sc *scene) playAnyway()     { sc.e = sc.s.PlayAnyway(sc.now) }
func (sc *scene) leave(id int64)  { sc.e = sc.s.Leave(id, sc.now) }
func (sc *scene) state() State    { return sc.s.State(sc.now) }
func (sc *scene) pos() int64      { return sc.state().Position(sc.now) }
func (sc *scene) running() bool   { st := sc.state(); return st.Playing && len(st.Waiting) == 0 }
func (sc *scene) waiting() []Who  { return sc.state().Waiting }
func (sc *scene) behind() []Lag   { return sc.state().Behind }
func (sc *scene) wantPos(p int64) { sc.t.Helper(); sc.check("position", sc.pos(), p) }

func (sc *scene) check(what string, got, want any) {
	sc.t.Helper()
	if !reflect.DeepEqual(got, want) {
		sc.t.Errorf("at %d ms: %s = %+v, want %+v", sc.now, what, got, want)
	}
}

// watching: Alice and Bob tapped in, ready, and the room playing from 0:00 at time 0.
func watching(t *testing.T) (sc *scene, a, b int64) {
	sc = newScene(t, hour)
	a, b = sc.join(alice), sc.join(bob)
	sc.status(a, Ready)
	sc.status(b, Ready)
	sc.play()
	return sc, a, b
}

func TestSync(t *testing.T) {
	t.Run("the server loads every room paused at its stored position", func(t *testing.T) {
		s := NewSync(videoRoom(1, hour, 90_000), 500)
		st := s.State(10_000)
		if st.Playing || st.PositionMs != 90_000 || st.Position(10_000) != 90_000 {
			t.Errorf("state = %+v, want paused at 90000", st)
		}
	})

	t.Run("Bob presses play: the server applies it and sends the full state", func(t *testing.T) {
		sc := newScene(t, hour)
		sc.join(alice)
		sc.join(bob)
		sc.at(1000).play()
		sc.check("changed", sc.e.Changed, true)
		st := sc.state()
		sc.check("playing, position, time", []any{st.Playing, st.PositionMs, st.AtMs}, []any{true, int64(0), int64(1000)})
		sc.at(3000).wantPos(2000)
		sc.check("changed on a quiet tick", sc.e.Changed, false)
	})

	t.Run("Alice buffers 4 s → room pauses, waiting for Alice", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(12_000)
		sc.check("running after 2 s", sc.running(), true)
		sc.at(13_500)
		sc.check("waiting", sc.waiting(), []Who{alice})
		sc.check("changed", sc.e.Changed, true)
		sc.check("still wants to play", sc.state().Playing, true)
		sc.at(20_000).wantPos(13_500)
	})

	t.Run("Alice buffers 2 s → room keeps playing", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(12_000).status(a, Ready)
		sc.at(14_000)
		sc.check("running", sc.running(), true)
		sc.wantPos(14_000)
	})

	t.Run("Alice is away 4 s → room pauses; she comes back → it resumes where it stopped", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Away)
		sc.at(14_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
		sc.at(30_000).status(a, Ready, 14_000)
		sc.check("waiting", sc.waiting(), []Who{})
		sc.check("running", sc.running(), true)
		sc.at(31_000).wantPos(15_000)
	})

	t.Run("Alice has two tabs, one buffers → room waits for Alice, listed once", func(t *testing.T) {
		sc, a, _ := watching(t)
		a2 := sc.join(alice)
		sc.status(a2, Ready)
		sc.at(10_000).status(a2, Buffering)
		sc.at(14_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
		sc.status(a, Buffering)
		sc.at(18_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
	})

	t.Run("Alice left her tab hidden, Bob presses play → room waits for Alice at once", func(t *testing.T) {
		sc := newScene(t, hour)
		a, b := sc.join(alice), sc.join(bob)
		sc.status(a, Ready)
		sc.status(b, Ready)
		sc.at(1000).status(a, Away)
		sc.at(600_000).play()
		sc.check("waiting", sc.waiting(), []Who{alice})
		sc.at(610_000).wantPos(0)
	})

	t.Run("Alice leaves while the room waits for her → it resumes", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(14_000).leave(a)
		sc.check("running", sc.running(), true)
		sc.at(15_000).wantPos(15_000)
	})

	t.Run("Play anyway → Alice is skipped until she catches up, shown behind meanwhile", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(14_000).playAnyway()
		sc.check("running", sc.running(), true)
		// The room froze at 14000; Alice is stuck at 10000.
		sc.at(17_000)
		sc.check("behind", sc.behind(), []Lag{{Who: alice, Ms: 7000}})
		// Ready, but 3.5 s back: still skipped, and she runs along with the room.
		sc.status(a, Ready, 13_500)
		sc.at(19_000)
		sc.check("behind", sc.behind(), []Lag{{Who: alice, Ms: 3000}})
		// Ready within 1 s: caught up.
		sc.status(a, Ready, sc.pos()-800)
		sc.check("behind", sc.behind(), []Lag{})
		// No longer skipped: a new 4 s stall blocks again.
		sc.at(20_000).status(a, Buffering)
		sc.at(24_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
	})

	t.Run("Alice buffers 2 s, then locks her phone 2 s → one stall of 4 s, room pauses", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(12_000).status(a, Away)
		sc.at(14_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
	})

	t.Run("skipped Alice turns out unable to play → no longer shown behind", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(14_000).playAnyway()
		sc.at(20_000).status(a, CantPlay, 10_000)
		sc.check("behind", sc.behind(), []Lag{})
	})

	t.Run("skipped Alice stays away → how far behind she is keeps growing", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(10_000).status(a, Away)
		sc.at(14_000).playAnyway()
		sc.at(20_000)
		sc.check("behind", sc.behind(), []Lag{{Who: alice, Ms: 10_000}})
		sc.at(30_000)
		sc.check("behind", sc.behind(), []Lag{{Who: alice, Ms: 20_000}})
		sc.check("changed", sc.e.Changed, true)
	})

	t.Run("a device that can't play never blocks and is never behind", func(t *testing.T) {
		sc, _, _ := watching(t)
		c := sc.join(carol)
		sc.status(c, Ready)
		sc.at(5000).status(c, CantPlay, 0)
		sc.at(20_000)
		sc.check("waiting", sc.waiting(), []Who{})
		sc.check("behind", sc.behind(), []Lag{})
	})

	t.Run("someone who hasn't tapped \"Tap to join\" never blocks", func(t *testing.T) {
		sc, _, _ := watching(t)
		c := sc.join(carol)
		sc.at(20_000)
		sc.check("waiting", sc.waiting(), []Who{})
		sc.status(c, Away)
		sc.at(30_000)
		sc.check("waiting", sc.waiting(), []Who{})
	})

	t.Run("a new joiner doesn't block until it has been ready once", func(t *testing.T) {
		sc, _, _ := watching(t)
		c := sc.join(carol)
		sc.at(5000).status(c, Buffering, 0)
		sc.at(20_000)
		sc.check("waiting before ready", sc.waiting(), []Who{})
		sc.status(c, Ready)
		sc.at(21_000).status(c, Buffering)
		sc.at(25_000)
		sc.check("waiting after ready", sc.waiting(), []Who{carol})
	})

	t.Run("status is never a command: a paused room stays paused, a stall under 3 s doesn't pause", func(t *testing.T) {
		sc := newScene(t, hour)
		a := sc.join(alice)
		sc.status(a, Ready)
		sc.at(5000)
		sc.check("playing", sc.state().Playing, false)
		sc2, a2, _ := watching(t)
		sc2.at(1000).status(a2, Away)
		sc2.at(4000)
		sc2.check("running", sc2.running(), true)
		sc2.check("changed", sc2.e.Changed, false)
	})

	t.Run("Bob pauses → everyone sees \"Bob paused\", position saved", func(t *testing.T) {
		sc, _, b := watching(t)
		sc.at(7000).pause(b)
		sc.check("paused by", sc.e.PausedBy, &bob)
		sc.check("save", sc.e.Save, true)
		sc.check("playing", sc.state().Playing, false)
		sc.at(9000).wantPos(7000)
	})

	t.Run("Bob pauses a paused room → no note, no save, but the state is sent back", func(t *testing.T) {
		sc, a, b := watching(t)
		sc.at(7000).pause(a)
		sc.pause(b)
		sc.check("effect", sc.e, Effect{Changed: true})
	})

	t.Run("every intent sends the state, so its sender snaps to it even when nothing changed", func(t *testing.T) {
		sc, _, _ := watching(t)
		sc.at(1000).play()
		sc.check("changed", sc.e.Changed, true)
	})

	t.Run("the room pausing for Alice names nobody, and saves the position", func(t *testing.T) {
		sc, a, _ := watching(t)
		sc.at(1000).status(a, Buffering)
		sc.at(4500)
		sc.check("paused by", sc.e.PausedBy, (*Who)(nil))
		sc.check("save", sc.e.Save, true)
	})

	t.Run("the video reaches its end → paused there, saved", func(t *testing.T) {
		sc := newScene(t, 60_000)
		sc.status(sc.join(alice), Ready)
		sc.play()
		sc.at(59_000)
		sc.check("playing at 59 s", sc.state().Playing, true)
		sc.at(61_000)
		sc.check("playing after the end", sc.state().Playing, false)
		sc.check("save", sc.e.Save, true)
		sc.wantPos(60_000)
	})

	t.Run("play at the end of the video → stays paused there", func(t *testing.T) {
		sc := newScene(t, 60_000)
		sc.status(sc.join(alice), Ready)
		sc.seek(60_000)
		sc.at(1000).play()
		sc.check("effect", sc.e, Effect{Changed: true})
		sc.check("playing", sc.state().Playing, false)
	})

	t.Run("seek lands inside the video", func(t *testing.T) {
		sc := newScene(t, 60_000)
		sc.seek(90_000)
		sc.wantPos(60_000)
		sc.seek(-5)
		sc.wantPos(0)
	})

	t.Run("the last one leaves → room pauses, saved", func(t *testing.T) {
		sc, a, b := watching(t)
		sc.at(5000).leave(a)
		sc.check("playing with Bob left", sc.state().Playing, true)
		sc.at(8000).leave(b)
		sc.check("playing", sc.state().Playing, false)
		sc.check("save", sc.e.Save, true)
		sc.at(20_000).wantPos(8000)
	})

	t.Run("position is saved every 5 s while playing, not while paused or waiting", func(t *testing.T) {
		sc, a, b := watching(t)
		var saves []int64
		for ms := int64(1000); ms <= 12_000; ms += 1000 {
			if sc.at(ms).e.Save {
				saves = append(saves, ms)
			}
		}
		sc.check("saves", saves, []int64{5000, 10_000})
		sc.status(a, Buffering)
		sc.at(16_000) // waiting: this pause saves once
		saves = nil
		for ms := int64(17_000); ms <= 30_000; ms += 1000 {
			if sc.at(ms).e.Save {
				saves = append(saves, ms)
			}
		}
		sc.pause(b)
		for ms := int64(31_000); ms <= 40_000; ms += 1000 {
			if sc.at(ms).e.Save {
				saves = append(saves, ms)
			}
		}
		sc.check("saves while waiting or paused", saves, []int64(nil))
	})

	t.Run("seek, subtitle and offset changes are saved and sent", func(t *testing.T) {
		sc := newScene(t, hour)
		sc.join(alice)
		sc.seek(30_000)
		sc.check("seek", sc.e, Effect{Changed: true, Save: true})
		n := 3
		sc.e = sc.s.SetSubtitle(&Subtitle{Stream: &n}, sc.now)
		sc.check("subtitle", sc.e, Effect{Changed: true, Save: true})
		sc.e = sc.s.SetOffset(-500, sc.now)
		sc.check("offset", sc.e, Effect{Changed: true, Save: true})
		st := sc.state()
		sc.check("state", []any{*st.Subtitle.Stream, st.SubtitleOffsetMs}, []any{3, int64(-500)})
	})

	t.Run("switch → new video at 0:00, paused; play waits for everyone to load it", func(t *testing.T) {
		sc, a, b := watching(t)
		sc.at(10_000).status(a, Buffering)
		sc.at(14_000).playAnyway()
		sc.e = sc.s.Switch(videoRoom(2, 30*60_000, 0), sc.now)
		sc.check("switch", sc.e, Effect{Changed: true, Save: true})
		st := sc.state()
		sc.check("video, playing, position", []any{st.VideoID, st.Playing, st.DurationMs}, []any{int64(2), false, int64(30 * 60_000)})
		sc.wantPos(0)
		sc.check("behind", sc.behind(), []Lag{})
		// Both load the new video; Bob is ready first and presses play.
		sc.status(b, Buffering, 0)
		sc.at(15_000).status(b, Ready, 0)
		sc.play()
		sc.at(19_000)
		sc.check("waiting", sc.waiting(), []Who{alice})
	})
}

func TestStateJSON(t *testing.T) {
	b, err := json.Marshal(NewSync(videoRoom(1, hour, 0), 0).State(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"playing":false`, `"positionMs":0`, `"atMs":0`, `"waiting":[]`, `"behind":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("%s has no %s", b, want)
		}
	}
}
