// What this player reports to the room. Status is never a command: it can make the room wait, but only
// our own controls play, pause or seek.
import type { Status } from '$lib/protocol';
import { STATUS_EVERY_MS } from './timing';

// Facts is what the page knows about its player.
export type Facts = {
	joined: boolean; // pressed "Tap to join"
	blocked: boolean; // the browser refused play(): it needs a fresh tap
	cantPlay: boolean; // this device can't decode the video
	hidden: boolean; // tab hidden or app in the background
	seeking: boolean;
	loaded: boolean; // enough data to play on (readyState >= HAVE_FUTURE_DATA)
	shouldPlay: boolean; // people asked the room to play, even while it waits for someone
};

// statusOf is what to report; null before "Tap to join", so a newcomer never blocks the room. A player
// that needs a fresh tap counts as away: its person isn't watching.
export function statusOf(f: Facts): Status | null {
	if (f.cantPlay) return 'cantPlay';
	if (!f.joined) return null;
	if (f.hidden || f.blocked) return 'away';
	// Loading counts even while the room waits: saying ready then would release the wait at once.
	if (f.seeking || (f.shouldPlay && !f.loaded)) return 'buffering';
	return 'ready';
}

// Report is the last status sent, and when (performance.now()).
export type Report = { status: Status; at: number };

// reportDue says whether to send status now: nothing sent yet on this connection, a change, or
// STATUS_EVERY_MS passed while the room runs, so the server knows where the video is.
export function reportDue(last: Report | null, status: Status, now: number, running: boolean): boolean {
	if (!last || last.status !== status) return true;
	return running && now - last.at >= STATUS_EVERY_MS;
}
