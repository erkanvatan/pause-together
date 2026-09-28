import { describe, expect, it } from 'vitest';
import { reconnectDelay } from './socket';

describe('reconnectDelay', () => {
	it('starts short and doubles, up to 10 s', () => {
		expect([0, 1, 2, 3, 4, 5, 6, 20].map(reconnectDelay)).toEqual([
			500, 1000, 2000, 4000, 8000, 10000, 10000, 10000
		]);
	});
});
