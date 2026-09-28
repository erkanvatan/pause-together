import { describe, expect, it } from 'vitest';
import { Clock } from './clock';

// ping sends at sentAt, the server answers with its time, the pong arrives rtt later.
function ping(c: Clock, sentAt: number, rtt: number, serverTime: number) {
	c.add(sentAt, serverTime, sentAt + rtt);
}

describe('Clock', () => {
	it('has no offset before the first pong', () => {
		const c = new Clock();
		expect(c.offset()).toBeNull();
		expect(c.serverNow(100)).toBeNull();
	});

	it('takes the server time as reached halfway through the round trip', () => {
		const c = new Clock();
		ping(c, 100, 40, 5000);
		expect(c.offset()).toBe(4880);
		expect(c.serverNow(200)).toBe(5080);
	});

	it('keeps the lowest-RTT sample', () => {
		const c = new Clock();
		ping(c, 0, 40, 5000); // offset 4980
		ping(c, 1000, 10, 6100); // offset 5095, rtt 10
		ping(c, 2000, 80, 7000); // offset 4960
		expect(c.offset()).toBe(5095);
	});

	it('11th ping pushes out the oldest: a stale best sample stops counting', () => {
		const c = new Clock();
		ping(c, 0, 2, 1000); // offset 999, the best RTT
		for (let i = 1; i <= 9; i++) ping(c, i * 1000, 50, i * 1000 + 2000); // offset 1975
		expect(c.offset()).toBe(999);
		ping(c, 10_000, 50, 12_000);
		expect(c.offset()).toBe(1975);
	});

	it('reconnect → no offset until the first pong', () => {
		const c = new Clock();
		ping(c, 0, 10, 1000);
		c.reset();
		expect(c.offset()).toBeNull();
		ping(c, 5000, 60, 100); // the server restarted: its clock is back near 0
		expect(c.offset()).toBe(-4930);
	});
});
