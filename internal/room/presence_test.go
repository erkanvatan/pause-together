package room

import (
	"reflect"
	"testing"
)

func TestPresence(t *testing.T) {
	type step struct {
		join  *Who  // a socket of this user's opens
		leave int64 // or: a socket of this user's closes (0 = no leave)
		left  bool  // the page closed it cleanly
		at    int64
	}
	join := func(w Who) step { return step{join: &w} }
	drop := func(w Who, at int64) step { return step{leave: w.UserID, at: at} }
	closeTab := func(w Who, at int64) step { return step{leave: w.UserID, left: true, at: at} }
	alicia := Who{UserID: alice.UserID, Name: "Alicia"}

	tests := []struct {
		name  string
		steps []step
		at    int64 // when the list is read
		want  []Who
	}{
		{"nobody yet → empty, not nil", nil, 0, []Who{}},
		{"Alice opens two tabs → one entry", []step{join(alice), join(alice)}, 0, []Who{alice}},
		{"Alice closes one of two tabs → still in",
			[]step{join(alice), join(alice), closeTab(alice, 1000)}, 60_000, []Who{alice}},
		{"Alice drops for 10 s and reconnects → in all along",
			[]step{join(alice), drop(alice, 1000), join(alice)}, 60_000, []Who{alice}},
		{"Alice drops, 14.999 s later → still in", []step{join(alice), drop(alice, 1000)}, 15_999, []Who{alice}},
		{"Alice drops, 15 s later → out", []step{join(alice), drop(alice, 1000)}, 16_000, []Who{}},
		{"Alice closes her tab → out at once", []step{join(alice), closeTab(alice, 1000)}, 1000, []Who{}},
		{"Alice closes one of two tabs, the other drops → in for the grace",
			[]step{join(alice), join(alice), closeTab(alice, 1000), drop(alice, 2000)}, 16_999, []Who{alice}},
		{"Alice closes one of two tabs, the other drops, 15 s later → out",
			[]step{join(alice), join(alice), closeTab(alice, 1000), drop(alice, 2000)}, 17_000, []Who{}},
		{"Alice renamed herself and rejoins → her new name, one entry",
			[]step{join(alice), join(alicia)}, 0, []Who{alicia}},
		{"Bob, then Alice → in the order they came", []step{join(bob), join(alice)}, 0, []Who{bob, alice}},
		{"a leave with no socket open changes nothing", []step{drop(alice, 1000)}, 2000, []Who{}},
		{"Alice drops, a stray second leave → the grace isn't pushed back",
			[]step{join(alice), drop(alice, 1000), drop(alice, 5000)}, 16_000, []Who{}},
		{"Alice drops, a stray second leave, she rejoins → in",
			[]step{join(alice), drop(alice, 1000), drop(alice, 2000), join(alice)}, 60_000, []Who{alice}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Presence
			for _, s := range tt.steps {
				if s.join != nil {
					p.Join(*s.join)
				} else {
					p.Leave(s.leave, s.at, s.left)
				}
			}
			if got := p.Watching(tt.at); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("watching at %d ms = %v, want %v", tt.at, got, tt.want)
			}
		})
	}

	// Someone gone past the grace is forgotten: back later, they join at the end, once.
	t.Run("Alice gone, Bob joins, Alice comes back → Bob, then Alice", func(t *testing.T) {
		var p Presence
		p.Join(alice)
		p.Leave(alice.UserID, 1000, true)
		p.Watching(2000)
		p.Join(bob)
		p.Join(alice)
		if got, want := p.Watching(3000), []Who{bob, alice}; !reflect.DeepEqual(got, want) {
			t.Errorf("watching = %v, want %v", got, want)
		}
		if n := len(p.visitors); n != 2 {
			t.Errorf("%d visitors kept, want 2", n)
		}
	})
}
