package room

// Timing of the sync rules and the room loop, in milliseconds of server time.
const (
	// StallGraceMs: a client buffering or away for longer than this pauses the room.
	StallGraceMs = 3000
	// CaughtUpMs: a skipped client ready within this of the room has caught up. It matches the client's
	// seek line (DRIFT_SEEK_MS in web/src/lib/sync/timing.ts): closer than that, drift is only nudged.
	CaughtUpMs = 1000
	// SaveEveryMs: how often the position is saved while the room plays.
	SaveEveryMs = 5000

	// HeartbeatTimeoutMs: a socket that sends nothing for this long counts as disconnected. Clients ping
	// more often than that (PING_EVERY_MS in web/src/lib/sync/timing.ts).
	HeartbeatTimeoutMs = 10_000
	// PresenceGraceMs: someone whose last socket closed stays "watching now" this long, so a flaky phone
	// that reconnects doesn't flicker to "was here".
	PresenceGraceMs = 15_000
	// TickMs: how often a room's loop applies what time alone changes, and checks its prepare job.
	TickMs = 250
)
