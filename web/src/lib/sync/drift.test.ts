import { describe, expect, it } from 'vitest';
import { correct } from './drift';

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
