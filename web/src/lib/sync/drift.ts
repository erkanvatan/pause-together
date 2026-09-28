// How to bring the video back to where the room is.
import { DRIFT_IGNORE_MS, DRIFT_SEEK_MS, NUDGE_BIG, NUDGE_BIG_FROM_MS, NUDGE_SMALL } from './timing';
import { running, target, type PlayState } from './state';

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

// Step is what the video should do now: play or pause, at what rate, and where to seek, if at all.
export type Step = Correction & { play: boolean };

// follow says how the video at videoMs follows the room at server time now. A running room plays and
// corrects drift. A paused or waiting one pauses, and seeks when it's off by DRIFT_IGNORE_MS or more:
// a still picture has no rate to nudge. endMs is where the video file itself ends: never aim past it.
export function follow(s: PlayState, now: number, videoMs: number, endMs = Infinity): Step {
	const t = Math.min(target(s, now), endMs);
	if (running(s)) return { play: true, ...correct(videoMs, t) };
	return Math.abs(videoMs - t) < DRIFT_IGNORE_MS ? { play: false, rate: 1 } : { play: false, rate: 1, seekTo: t };
}
