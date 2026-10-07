import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reconnectDelay, RoomSocket } from './socket';
import { OPEN_TIMEOUT_MS, PING_EVERY_MS, PONG_TIMEOUT_MS, RECONNECT_MIN_MS } from './sync/timing';

describe('reconnectDelay', () => {
	it('starts short and doubles, up to 10 s', () => {
		expect([0, 1, 2, 3, 4, 5, 6, 20].map(reconnectDelay)).toEqual([
			500, 1000, 2000, 4000, 8000, 10000, 10000, 10000
		]);
	});
});

// FakeSocket never opens nor closes on its own, like a socket stuck opening.
class FakeSocket {
	static CONNECTING = 0;
	static OPEN = 1;
	static made: FakeSocket[] = [];
	readyState = FakeSocket.CONNECTING;
	onopen: (() => void) | null = null;
	onmessage: ((e: { data: string }) => void) | null = null;
	onclose: ((e: { code: number }) => void) | null = null;
	closed = false;
	sent: string[] = [];
	constructor(readonly url: string) {
		FakeSocket.made.push(this);
	}
	send(m: string) {
		this.sent.push(m);
	}
	close() {
		this.closed = true;
	}
}

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	FakeSocket.made = [];
});

describe('RoomSocket', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.stubGlobal('WebSocket', FakeSocket);
		vi.stubGlobal('location', { protocol: 'http:', host: '100.101.102.103:8420' });
	});

	it('drops a socket stuck opening, says so, and tries again', () => {
		const ondrop = vi.fn();
		const s = new RoomSocket(15, () => {}, ondrop);

		vi.advanceTimersByTime(OPEN_TIMEOUT_MS - 1);
		expect(ondrop).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(ondrop).toHaveBeenCalledOnce();
		expect(FakeSocket.made[0].closed).toBe(true);

		vi.advanceTimersByTime(RECONNECT_MIN_MS);
		expect(FakeSocket.made).toHaveLength(2);
		expect(FakeSocket.made[1].url).toBe('ws://100.101.102.103:8420/ws?room=15');
		s.close();
	});

	it('keeps a socket that opens late, and gives its first pong the full wait', () => {
		const ondrop = vi.fn();
		const s = new RoomSocket(15, () => {}, ondrop);
		const ws = FakeSocket.made[0];

		vi.advanceTimersByTime(OPEN_TIMEOUT_MS - 500);
		ws.readyState = FakeSocket.OPEN;
		ws.onopen!();
		expect(ws.sent.map((m) => JSON.parse(m).type)).toEqual(['ping']);
		vi.advanceTimersByTime(PONG_TIMEOUT_MS);
		expect(ondrop).not.toHaveBeenCalled();
		expect(ws.closed).toBe(false);
		vi.advanceTimersByTime(PING_EVERY_MS);
		expect(ondrop).toHaveBeenCalledOnce();
		s.close();
	});
});
