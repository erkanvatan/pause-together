package room

// Timing of the sync rules, in milliseconds of server time.
const (
	// StallGraceMs: a client buffering or away for longer than this pauses the room.
	StallGraceMs = 3000
	// CaughtUpMs: a skipped client ready within this of the room has caught up. It matches the client's
	// seek line (DRIFT_SEEK_MS in web/src/lib/sync/timing.ts): closer than that, drift is only nudged.
	CaughtUpMs = 1000
	// SaveEveryMs: how often the position is saved while the room plays.
	SaveEveryMs = 5000
)
