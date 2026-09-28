package room

import (
	"reflect"
	"testing"
)

func TestPresence(t *testing.T) {
	check := func(t *testing.T, p *Presence, now int64, watching, wasHere []Who) {
		t.Helper()
		w, h := p.Snapshot(now)
		if !reflect.DeepEqual(w, watching) || !reflect.DeepEqual(h, wasHere) {
			t.Errorf("at %d ms: watching %v, was here %v; want %v, %v", now, w, h, watching, wasHere)
		}
	}

	t.Run("nobody yet → both lists empty, not nil", func(t *testing.T) {
		check(t, &Presence{}, 0, []Who{}, []Who{})
	})

	t.Run("Alice was here before a restart, Bob joins → Bob watching, Alice was here", func(t *testing.T) {
		var p Presence
		p.Load([]Who{alice})
		p.Join(bob)
		check(t, &p, 0, []Who{bob}, []Who{alice})
	})

	t.Run("Alice opens two tabs → one entry", func(t *testing.T) {
		var p Presence
		p.Join(alice)
		p.Join(alice)
		check(t, &p, 0, []Who{alice}, []Who{})
	})

	t.Run("Alice closes one of two tabs → still watching", func(t *testing.T) {
		var p Presence
		p.Join(alice)
		p.Join(alice)
		p.Leave(alice.UserID, 1000)
		check(t, &p, 60_000, []Who{alice}, []Who{})
	})

	t.Run("Alice drops for 10 s and reconnects → watching all along", func(t *testing.T) {
		var p Presence
		p.Join(alice)
		p.Leave(alice.UserID, 1000)
		check(t, &p, 11_000, []Who{alice}, []Who{})
		p.Join(alice)
		check(t, &p, 60_000, []Who{alice}, []Who{})
	})

	t.Run("Alice gone 15 s → was here", func(t *testing.T) {
		var p Presence
		p.Join(alice)
		p.Leave(alice.UserID, 1000)
		check(t, &p, 15_999, []Who{alice}, []Who{})
		check(t, &p, 16_000, []Who{}, []Who{alice})
	})

	t.Run("Alice renamed herself and rejoins → her new name, still one entry", func(t *testing.T) {
		var p Presence
		p.Load([]Who{alice})
		p.Join(Who{UserID: alice.UserID, Name: "Alicia"})
		check(t, &p, 0, []Who{{UserID: alice.UserID, Name: "Alicia"}}, []Who{})
	})

	t.Run("a leave with no socket open changes nothing", func(t *testing.T) {
		var p Presence
		p.Load([]Who{alice})
		p.Leave(alice.UserID, 1000)
		p.Leave(bob.UserID, 1000)
		check(t, &p, 2000, []Who{}, []Who{alice})
	})
}
