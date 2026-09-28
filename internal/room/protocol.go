package room

// The room socket's messages: JSON {"type": ...}. web/src/lib/protocol.ts defines the same ones; change
// both together. testdata/protocol.json holds one of each, checked on both sides.

import "github.com/coder/websocket"

// CloseNotFound closes a socket whose room doesn't exist (any more): deleted while the client was away,
// so it missed MsgDeleted.
const CloseNotFound websocket.StatusCode = 4404

// Messages a client sends.
const (
	MsgPing       = "ping" // the heartbeat and clock sample; answered with a pong
	MsgPlay       = "play"
	MsgPause      = "pause"
	MsgSeek       = "seek"
	MsgPlayAnyway = "playAnyway"
	MsgSubtitle   = "subtitle"
	MsgOffset     = "offset"
	MsgStatus     = "status"
)

// Messages the server sends.
const (
	MsgHello    = "hello" // always first
	MsgState    = "state"
	MsgRoom     = "room"
	MsgPresence = "presence"
	MsgPrepare  = "prepare"
	MsgPaused   = "paused" // for the "Alice paused" note
	MsgPong     = "pong"
	MsgDeleted  = "deleted"
)

// ClientMsg is any message a client sends. Only its type's fields are set.
type ClientMsg struct {
	Type string `json:"type"`
	// T is the client's clock when it sent a ping (performance.now()). The pong sends it back.
	T float64 `json:"t,omitempty"`
	// PositionMs and Ms may have a fraction (video.currentTime * 1000): the loop rounds them.
	PositionMs float64   `json:"positionMs,omitempty"` // seek: where to; status: where the client's video is
	Subtitle   *Subtitle `json:"subtitle,omitempty"`   // subtitle: nil = off
	Ms         float64   `json:"ms,omitempty"`         // offset
	Status     string    `json:"status,omitempty"`     // status: a statusNames key
}

// statusNames are the status values on the wire.
var statusNames = map[string]Status{
	"ready":     Ready,
	"buffering": Buffering,
	"away":      Away,
	"cantPlay":  CantPlay,
}

type HelloMsg struct {
	Type string `json:"type"`
	// BuildID is the server's web build. A page from another build reloads.
	BuildID string `json:"buildId"`
	UserID  int64  `json:"userId"`
}

type StateMsg struct {
	Type  string `json:"type"`
	State State  `json:"state"`
}

// RoomMsg sends the room on join, and after a rename, switch or unarchive.
type RoomMsg struct {
	Type string `json:"type"`
	Room Room   `json:"room"`
}

type PresenceMsg struct {
	Type     string `json:"type"`
	Watching []Who  `json:"watching"`
	WasHere  []Who  `json:"wasHere"`
}

type PrepareMsg struct {
	Type    string  `json:"type"`
	Prepare Prepare `json:"prepare"`
}

// Prepare is where the room's prepared copy stands.
type Prepare struct {
	// State is media.JobReady, JobRunning, JobQueued or JobFailed; "" = no job: the video is gone or
	// can't play, or the room is archived.
	State    string  `json:"state"`
	Place    int     `json:"place"`    // queued: 1 = next in line
	Progress float64 `json:"progress"` // running: 0 to 1
	Error    string  `json:"error"`    // failed: media.FailNoSpace or FailPrepare
	Key      string  `json:"key"`      // ready: the copy's /stream key
}

type PausedMsg struct {
	Type string `json:"type"`
	By   Who    `json:"by"`
}

type PongMsg struct {
	Type     string  `json:"type"`
	T        float64 `json:"t"`        // the ping's
	ServerMs int64   `json:"serverMs"` // server time when it answered
}

// TypeMsg is a message with nothing but its type: deleted.
type TypeMsg struct {
	Type string `json:"type"`
}
