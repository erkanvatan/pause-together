// The offset between this page's clock (performance.now()) and the server's, from ping/pong.
import { CLOCK_SAMPLES } from './timing';

type Sample = { rtt: number; offset: number };

export class Clock {
	private samples: Sample[] = [];

	// add records a pong: the ping left at sentAt, the server stamped serverTime, the pong came back at
	// receivedAt. The server's time is taken as reached halfway through the round trip.
	add(sentAt: number, serverTime: number, receivedAt: number) {
		const rtt = receivedAt - sentAt;
		this.samples.push({ rtt, offset: serverTime - (sentAt + rtt / 2) });
		if (this.samples.length > CLOCK_SAMPLES) this.samples.shift();
	}

	// offset is server time minus local time, from the lowest-RTT recent sample; null before any pong.
	offset(): number | null {
		if (this.samples.length === 0) return null;
		return this.samples.reduce((best, s) => (s.rtt < best.rtt ? s : best)).offset;
	}

	// serverNow is the server's time at local time localNow; null before any pong.
	serverNow(localNow: number): number | null {
		const o = this.offset();
		return o === null ? null : localNow + o;
	}

	// reset forgets every sample, on each reconnect: the server may have restarted with a new clock.
	reset() {
		this.samples = [];
	}
}
