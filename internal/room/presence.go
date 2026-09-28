package room

import "slices"

// Presence tracks who is in a room: "watching now" (an open socket, or the last one closed less than
// PresenceGraceMs ago) and "was here" (joined before, gone now). Each user shows once, however many
// tabs they have open. Pure: time is passed in.
type Presence struct {
	visitors []*visitor // in the order they first showed up
}

type visitor struct {
	who     Who
	sockets int
	until   int64 // with no socket: watching until this time
}

// Load adds who was here before, from the database.
func (p *Presence) Load(wasHere []Who) {
	for _, w := range wasHere {
		if p.find(w.UserID) == nil {
			p.visitors = append(p.visitors, &visitor{who: w})
		}
	}
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

// Leave counts a socket of a user's as closed. With their last one gone, the grace starts.
func (p *Presence) Leave(userID, now int64) {
	v := p.find(userID)
	if v == nil || v.sockets == 0 {
		return
	}
	v.sockets--
	if v.sockets == 0 {
		v.until = now + PresenceGraceMs
	}
}

// Snapshot returns who is watching now and who was here, at now. Neither is nil, so both are sent as [].
func (p *Presence) Snapshot(now int64) (watching, wasHere []Who) {
	watching, wasHere = []Who{}, []Who{}
	for _, v := range p.visitors {
		if v.sockets > 0 || now < v.until {
			watching = append(watching, v.who)
		} else {
			wasHere = append(wasHere, v.who)
		}
	}
	return watching, wasHere
}

func (p *Presence) find(userID int64) *visitor {
	if i := slices.IndexFunc(p.visitors, func(v *visitor) bool { return v.who.UserID == userID }); i >= 0 {
		return p.visitors[i]
	}
	return nil
}
