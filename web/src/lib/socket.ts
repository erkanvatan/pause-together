// The room page's socket: joins the room, pings for the heartbeat and the clock, and reconnects.
import { version } from '$app/environment';
import { CLOSE_NOT_FOUND, type ClientMessage, type ServerMessage } from '$lib/protocol';
import { Clock } from '$lib/sync/clock';
import {
	PING_EVERY_MS,
	PONG_TIMEOUT_MS,
	RECONNECT_MAX_MS,
	RECONNECT_MIN_MS
} from '$lib/sync/timing';

// CLOSE_LEFT is WebSocket's normal closure. The server takes it, like a closing tab's "going away", as
// the page leaving on purpose.
const CLOSE_LEFT = 1000;

// reconnectDelay is how long to wait before reconnect attempt n (0 = the first after a drop).
export function reconnectDelay(attempt: number): number {
	return Math.min(RECONNECT_MIN_MS * 2 ** attempt, RECONNECT_MAX_MS);
}

export class RoomSocket {
	// The server's clock. Reset on every reconnect: the server may have restarted with a new one.
	readonly clock = new Clock();
	private ws: WebSocket | null = null;
	private attempt = 0; // reconnects since the last good connection
	private lastPong = 0;
	private pingTimer: ReturnType<typeof setInterval> | undefined;
	private retryTimer: ReturnType<typeof setTimeout> | undefined;
	private closed = false;

	constructor(
		private roomId: number,
		private onmessage: (m: ServerMessage) => void,
		// ondrop is called when the connection is lost; the next hello says it's back.
		private ondrop: () => void = () => {}
	) {
		this.connect();
	}

	// send drops the message while disconnected, and says so: the full state arrives on reconnect.
	send(m: ClientMessage): boolean {
		if (this.ws?.readyState !== WebSocket.OPEN) return false;
		this.ws.send(JSON.stringify(m));
		return true;
	}

	// close leaves the room: the clean close tells the server so, and it drops us from the list at once.
	close() {
		this.closed = true;
		clearTimeout(this.retryTimer);
		this.drop(CLOSE_LEFT);
	}

	private connect() {
		const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${scheme}//${location.host}/ws?room=${this.roomId}`);
		this.ws = ws;
		this.clock.reset();
		ws.onopen = () => {
			this.lastPong = performance.now();
			this.ping();
			this.pingTimer = setInterval(() => this.ping(), PING_EVERY_MS);
		};
		ws.onmessage = (e) => {
			const m = JSON.parse(e.data) as ServerMessage;
			if (m.type === 'hello') {
				// A page from another build must not run against this server.
				if (m.buildId !== version) {
					this.close();
					location.reload();
					return;
				}
				this.attempt = 0;
			} else if (m.type === 'pong') {
				const now = performance.now();
				this.lastPong = now;
				this.clock.add(m.t, m.serverMs, now);
			}
			this.onmessage(m);
		};
		ws.onclose = (e) => {
			if (e.code !== CLOSE_NOT_FOUND) {
				this.lost();
				return;
			}
			// The room was deleted while this page was away, so it missed the message saying so.
			this.close();
			this.onmessage({ type: 'deleted' });
		};
	}

	private ping() {
		if (performance.now() - this.lastPong > PONG_TIMEOUT_MS) {
			this.lost();
			return;
		}
		this.send({ type: 'ping', t: performance.now() });
	}

	// lost drops the connection and tries again after the backoff.
	private lost() {
		this.drop();
		if (this.closed) return;
		this.ondrop();
		this.retryTimer = setTimeout(() => this.connect(), reconnectDelay(this.attempt++));
	}

	// drop closes the socket without waiting for it: a dead connection may take long to report closing.
	// Without a code the server counts it as a dropped connection, and gives the page time to come back.
	private drop(code?: number) {
		clearInterval(this.pingTimer);
		const ws = this.ws;
		this.ws = null;
		if (!ws) return;
		ws.onopen = ws.onmessage = ws.onclose = null;
		ws.close(code);
	}
}
