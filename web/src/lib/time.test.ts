import { describe, expect, it } from 'vitest';
import { formatTime } from './time';

describe('formatTime', () => {
	it.each([
		[0, '0:00'],
		[5000, '0:05'],
		[5999, '0:05'],
		[723_000, '12:03'],
		[3_599_000, '59:59'],
		[3_600_000, '1:00:00'],
		[6_000_000, '1:40:00'],
		[-500, '0:00']
	])('%d ms → %s', (ms, want) => {
		expect(formatTime(ms)).toBe(want);
	});
});
