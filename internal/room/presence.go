package room

import "slices"

// Presence tracks who is watching a room now: an open socket, or the last one closed less than
// PresenceGraceMs ago. Someone gone longer is forgotten. Each user shows once, however many tabs they
// have open. Pure: time is passed in.
type Presence struct {
	visitors []*visitor // in the order they came; someone forgotten who comes back goes last
}

type visitor struct {
	who     Who
	sockets int
	until   int64 // with no socket: watching until this time
}

// Join counts a socket of w's. It also takes w's current name.
func (p *Presence) Join(w Who) {
	v := p.find(w.UserID)
	if v == nil {
		v = &visitor{}
		p.visitors = append(p.visitors, v)
	}
	v.who = w
	v.sockets++
}

// Leave counts a socket of a user's as closed. With their last one gone, the grace starts, unless the
// page left on purpose (closed the socket cleanly): then they aren't coming back soon.
func (p *Presence) Leave(userID, now int64, left bool) {
	v := p.find(userID)
	if v == nil || v.sockets == 0 {
		return
	}
	v.sockets--
	if v.sockets == 0 {
		v.until = now
		if !left {
			v.until += PresenceGraceMs
		}
	}
}

// Watching returns who is watching at now, and forgets who is gone. Never nil, so it is sent as [].
func (p *Presence) Watching(now int64) []Who {
	p.visitors = slices.DeleteFunc(p.visitors, func(v *visitor) bool { return v.sockets == 0 && now >= v.until })
	watching := []Who{}
	for _, v := range p.visitors {
		watching = append(watching, v.who)
	}
	return watching
}

func (p *Presence) find(userID int64) *visitor {
	if i := slices.IndexFunc(p.visitors, func(v *visitor) bool { return v.who.UserID == userID }); i >= 0 {
		return p.visitors[i]
	}
	return nil
}
