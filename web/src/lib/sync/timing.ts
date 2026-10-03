// Timing of the sync rules. The server's are in internal/room/timing.go.

// Drift under this is left alone.
export const DRIFT_IGNORE_MS = 200;
// Drift up to this is fixed by nudging playbackRate; beyond it, the video seeks. The server's
// CaughtUpMs matches it.
export const DRIFT_SEEK_MS = 1000;
// playbackRate nudge: NUDGE_SMALL below NUDGE_BIG_FROM_MS of drift, NUDGE_BIG from there.
export const NUDGE_SMALL = 0.05;
export const NUDGE_BIG = 0.1;
export const NUDGE_BIG_FROM_MS = 500;
// The clock offset comes from the lowest-RTT ping of this many recent ones.
export const CLOCK_SAMPLES = 10;

// The socket pings this often: the heartbeat, and a clock sample. The server drops a socket silent
// for 10 s (HeartbeatTimeoutMs in internal/room/timing.go).
export const PING_EVERY_MS = 3000;
// No pong for this long: the connection is dead, even if the browser hasn't noticed.
export const PONG_TIMEOUT_MS = 10000;
// Reconnect backoff: the first retry waits RECONNECT_MIN_MS, each next one twice as long, at most
// RECONNECT_MAX_MS.
export const RECONNECT_MIN_MS = 500;
export const RECONNECT_MAX_MS = 10000;

// The player checks its video against the room this often (and on every new state and video event).
export const FOLLOW_EVERY_MS = 250;
// While the room plays, the player reports its status and position at least this often.
export const STATUS_EVERY_MS = 2000;
// How long the "Alice paused" note stays up.
export const PAUSED_NOTE_MS = 2000;
// The host counts as offline once the socket has been down this long, so a blip that reconnects at
// once never says so.
export const OFFLINE_NOTE_MS = 2000;
// After a network error, the player loads the video again this much later.
export const MEDIA_RETRY_MS = 2000;
