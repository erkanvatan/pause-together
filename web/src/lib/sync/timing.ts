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
