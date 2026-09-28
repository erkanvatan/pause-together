import { describe, expect, it } from 'vitest';
import { local, running, target, type PlayState } from './state';

const paused: PlayState = { playing: false, positionMs: 60_000, atMs: 1000, durationMs: 120_000, waiting: [] };
const playing: PlayState = { ...paused, playing: true };

describe('target', () => {
	it.each([
		['paused → stays put', paused, 60_000],
		['playing → runs on from the server timestamp', playing, 64_000],
		['waiting for someone → stays put', { ...playing, waiting: [{ name: 'Alice' }] }, 60_000],
		['past the end → at the end', { ...playing, positionMs: 119_000 }, 120_000]
	])('%s', (_, s, want) => {
		expect(target(s, 5000)).toBe(want);
	});

	it('runs only while playing and nobody is waited for', () => {
		expect([running(paused), running(playing), running({ ...playing, waiting: [{}] })]).toEqual([
			false,
			true,
			false
		]);
	});
});

describe('local', () => {
	it('play applies at once, from where the video is', () => {
		expect(local(paused, { type: 'play' }, 5000)).toEqual({ ...paused, playing: true, atMs: 5000 });
	});

	it('pause applies at once, where the video is now', () => {
		expect(local(playing, { type: 'pause' }, 5000)).toEqual({
			...playing,
			playing: false,
			positionMs: 64_000,
			atMs: 5000
		});
	});

	it('seek applies at once, inside the video, still playing', () => {
		expect(local(playing, { type: 'seek', positionMs: 200_000 }, 5000)).toEqual({
			...playing,
			positionMs: 120_000,
			atMs: 5000
		});
	});
});
