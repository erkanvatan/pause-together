// How to bring the video back to where the room is.
import { DRIFT_IGNORE_MS, DRIFT_SEEK_MS, NUDGE_BIG, NUDGE_BIG_FROM_MS, NUDGE_SMALL } from './timing';

// Correction is the playbackRate to set, and where to seek, if the video should.
export type Correction = { rate: number; seekTo?: number };

// correct compares the video's position with the room's (targetMs): small drift is ignored, medium
// drift nudges playbackRate, large drift seeks.
export function correct(videoMs: number, targetMs: number): Correction {
	const drift = videoMs - targetMs; // > 0: ahead
	const size = Math.abs(drift);
	if (size < DRIFT_IGNORE_MS) return { rate: 1 };
	if (size > DRIFT_SEEK_MS) return { rate: 1, seekTo: targetMs };
	const nudge = size < NUDGE_BIG_FROM_MS ? NUDGE_SMALL : NUDGE_BIG;
	return { rate: drift > 0 ? 1 - nudge : 1 + nudge };
}
