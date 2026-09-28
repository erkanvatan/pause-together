import { describe, expect, it } from 'vitest';
import { correct, follow } from './drift';
import type { PlayState } from './state';

describe('correct', () => {
	it.each([
		['150 ms behind → left alone', 9850, { rate: 1 }],
		['150 ms ahead → left alone', 10_150, { rate: 1 }],
		['300 ms behind → sped up 5 %', 9700, { rate: 1.05 }],
		['300 ms ahead → slowed down 5 %', 10_300, { rate: 0.95 }],
		['700 ms behind → sped up 10 %', 9300, { rate: 1.1 }],
		['700 ms ahead → slowed down 10 %', 10_700, { rate: 0.9 }],
		['exactly 1 s behind → still nudged', 9000, { rate: 1.1 }],
		['1.5 s behind → seek', 8500, { rate: 1, seekTo: 10_000 }],
		['1.5 s ahead → seek', 11_500, { rate: 1, seekTo: 10_000 }]
	])('%s', (_, videoMs, want) => {
		expect(correct(videoMs, 10_000)).toEqual(want);
	});
});

describe('follow', () => {
	// At server time 5000 a playing room is at 64 s, a paused one at 60 s.
	const paused: PlayState = { playing: false, positionMs: 60_000, atMs: 1000, durationMs: 120_000, waiting: [] };
	const playing: PlayState = { ...paused, playing: true };

	it.each([
		['paused, 100 ms off → stays paused, no seek', paused, 60_100, { play: false, rate: 1 }],
		['paused, 300 ms off → seeks: a paused room matches exactly', paused, 60_300, { play: false, rate: 1, seekTo: 60_000 }],
		['playing, in step → plays', playing, 64_000, { play: true, rate: 1 }],
		['playing, 300 ms behind → plays sped up', playing, 63_700, { play: true, rate: 1.05 }],
		['playing, 2 s behind → plays and seeks', playing, 62_000, { play: true, rate: 1, seekTo: 64_000 }],
		['waiting for someone → pauses where the room is', { ...playing, waiting: [{}] }, 60_000, { play: false, rate: 1 }],
		['playing past the end → held at the end, no seek', { ...playing, positionMs: 119_000 }, 120_000, { play: true, rate: 1 }]
	])('%s', (_, s, videoMs, want) => {
		expect(follow(s, 5000, videoMs)).toEqual(want);
	});

	it('file ends before the room\'s duration → aims at the file\'s end, no seek loop there', () => {
		const atEnd = { ...paused, positionMs: 120_000 };
		expect(follow(atEnd, 5000, 119_000, 119_000)).toEqual({ play: false, rate: 1 });
		expect(follow(playing, 5000, 30_000, 50_000)).toEqual({ play: true, rate: 1, seekTo: 50_000 });
	});
});
