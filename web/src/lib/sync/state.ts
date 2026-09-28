// The room's playback state as the server sends it, and our own intents applied to it at once. The
// next state from the server replaces the local one as is: that's the snap.

// PlayState is the part of the room's state that says where the video should be. Times are server ms.
export type PlayState = {
	playing: boolean; // what people asked for
	positionMs: number; // at server time atMs
	atMs: number;
	durationMs: number; // 0: unknown
	waiting: readonly unknown[]; // who the room waits for
};

export type Intent = { type: 'play' } | { type: 'pause' } | { type: 'seek'; positionMs: number };

// running: the room's clock runs while it plays and nobody is waited for.
export function running(s: PlayState): boolean {
	return s.playing && s.waiting.length === 0;
}

// target is where the video should be at server time now.
export function target(s: PlayState, now: number): number {
	return clamp(running(s) ? s.positionMs + now - s.atMs : s.positionMs, s.durationMs);
}

// local applies one of our intents right away, before the server answers.
export function local(s: PlayState, i: Intent, now: number): PlayState {
	switch (i.type) {
		case 'play':
			return { ...s, playing: true, positionMs: target(s, now), atMs: now };
		case 'pause':
			return { ...s, playing: false, positionMs: target(s, now), atMs: now };
		case 'seek':
			return { ...s, positionMs: clamp(i.positionMs, s.durationMs), atMs: now };
	}
}

function clamp(p: number, durationMs: number): number {
	return Math.max(0, durationMs > 0 ? Math.min(p, durationMs) : p);
}
