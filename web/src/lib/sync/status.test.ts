import { describe, expect, it } from 'vitest';
import { reportDue, statusOf, type Facts } from './status';

// A joined, visible, loaded player in a playing room.
const ok: Facts = {
	joined: true,
	blocked: false,
	cantPlay: false,
	hidden: false,
	seeking: false,
	loaded: true,
	shouldPlay: true
};

describe('statusOf', () => {
	it.each([
		['all good → ready', ok, 'ready'],
		['not joined yet → nothing to say', { ...ok, joined: false }, null],
		['not joined, but can\'t play → cantPlay: it never blocks', { ...ok, joined: false, cantPlay: true }, 'cantPlay'],
		['can\'t play wins over hidden', { ...ok, cantPlay: true, hidden: true }, 'cantPlay'],
		['tab hidden → away', { ...ok, hidden: true }, 'away'],
		['play refused, needs a fresh tap → away', { ...ok, blocked: true }, 'away'],
		['hidden wins over loading', { ...ok, hidden: true, loaded: false }, 'away'],
		['seeking → buffering', { ...ok, seeking: true }, 'buffering'],
		['seeking in a paused room → buffering', { ...ok, seeking: true, shouldPlay: false }, 'buffering'],
		['should play, not enough data → buffering, also while the room waits for it', { ...ok, loaded: false }, 'buffering'],
		['paused room, not loaded → ready: nothing to wait for yet', { ...ok, loaded: false, shouldPlay: false }, 'ready']
	])('%s', (_, f, want) => {
		expect(statusOf(f)).toBe(want);
	});
});

describe('reportDue', () => {
	it.each([
		['nothing sent yet (or reconnected) → send', null, 'ready', 100, false, true],
		['status changed → send', { status: 'ready', at: 0 }, 'buffering', 100, true, true],
		['same status, paused room → no', { status: 'ready', at: 0 }, 'ready', 60_000, false, false],
		['same status, playing, 1 s since → no', { status: 'ready', at: 0 }, 'ready', 1000, true, false],
		['same status, playing, 2 s since → send the position', { status: 'ready', at: 0 }, 'ready', 2000, true, true]
	] as const)('%s', (_, last, status, now, running, want) => {
		expect(reportDue(last, status, now, running)).toBe(want);
	});
});
