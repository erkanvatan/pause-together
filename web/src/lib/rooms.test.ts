import { describe, expect, it } from 'vitest';
import type { Room, RoomCard } from './api';
import type { Wait } from './protocol';
import {
	needsConfirm,
	roomProgress,
	roomSubtitle,
	roomTitle,
	splitRooms,
	usedAgo,
	roomsByVideo,
	waitText,
	type WaitText
} from './rooms';

function room(id: number, over: Partial<Room> = {}): Room {
	return {
		id,
		name: '',
		video: {
			id: 7,
			type: 'movies',
			title: 'Heat',
			year: 1995,
			edition: '',
			version: '',
			season: 0,
			episode: 0,
			episodeEnd: 0,
			episodeTitle: '',
			group: '',
			durationMs: 0,
			codecString: 'avc1.640028',
			unplayable: '',
			appleOnly: false,
			missing: false
		},
		audio: 1,
		subtitle: null,
		positionMs: 0,
		subtitleOffsetMs: 0,
		archived: false,
		...over
	};
}

describe('roomTitle', () => {
	it.each([
		['its name', room(1, { name: 'Movie night' }), 'Movie night'],
		['the video without a name', room(1), 'Heat (1995)'],
		[
			'a missing video by its stored name',
			room(1, { video: { ...room(1).video, missing: true } }),
			'Heat (1995)'
		],
		['the video without its version label', room(1, { video: { ...room(1).video, version: '4K' } }), 'Heat (1995)']
	])('shows %s', (_, r, want) => {
		expect(roomTitle(r)).toBe(want);
	});
});

describe('roomSubtitle', () => {
	const heat4k = { ...room(1).video, version: '4K', edition: "Director's Cut" };
	it.each([
		['the whole video name under a room name', room(1, { name: 'Movie night', video: heat4k }), "Heat (1995) · Director's Cut · 4K"],
		['the version label under a video title', room(1, { video: heat4k }), '4K'],
		['nothing when there is no more to say', room(1), '']
	])('shows %s', (_, r, want) => {
		expect(roomSubtitle(r)).toBe(want);
	});
});

describe('roomProgress', () => {
	const at = (positionMs: number, durationMs: number) =>
		room(1, { positionMs, video: { ...room(1).video, durationMs } });
	it.each([
		['not started at 0:00', at(0, 6_000_000), 'Not started'],
		['not started under a second in', at(999, 6_000_000), 'Not started'],
		['where it is, of how long', at(3_733_000, 9_267_000), '1:02:13 of 2:34:27'],
		['finished at the end', at(6_000_000, 6_000_000), 'Finished'],
		['only where it is without a duration', at(65_000, 0), '1:05']
	])('says %s', (_, r, want) => {
		expect(roomProgress(r)).toBe(want);
	});
});

const card = (id: number, over: Partial<RoomCard> = {}): RoomCard => ({
	...room(id),
	watching: [],
	gone: false,
	usedAt: 0,
	...over
});

describe('splitRooms', () => {
	const ann = [{ userId: 1, name: 'Ann' }];
	const ids = (rs: RoomCard[]) => rs.map((r) => r.id);

	it('splits active, gone and archived', () => {
		const rooms = [card(4), card(3, { archived: true }), card(2, { gone: true }), card(1, { archived: true, gone: true })];
		const { active, gone, archived } = splitRooms(rooms, null);
		expect(ids(active)).toEqual([4]);
		expect(ids(gone)).toEqual([2]);
		expect(ids(archived)).toEqual([3, 1]);
	});

	it('keeps a gone room with people in it active: they may be swapping its video', () => {
		const { active, gone } = splitRooms([card(2, { gone: true, watching: ann })], null);
		expect(ids(active)).toEqual([2]);
		expect(gone).toEqual([]);
	});

	it('puts rooms with people first, then the most recently used, then the newest', () => {
		const rooms = [
			card(1, { usedAt: 500 }),
			card(2, { usedAt: 100, watching: ann }),
			card(3, { usedAt: 900 }),
			card(4),
			card(5),
			card(6, { usedAt: 300, watching: ann })
		];
		expect(ids(splitRooms(rooms, null).active)).toEqual([6, 2, 3, 1, 5, 4]);
	});

	it('keeps a frozen order while managing, rooms not in it first', () => {
		const rooms = [card(1, { usedAt: 900 }), card(2, { watching: ann }), card(3), card(4), card(5)];
		expect(ids(splitRooms(rooms, [3, 1, 2]).active)).toEqual([5, 4, 3, 1, 2]);
	});
});

describe('roomsByVideo', () => {
	const other = { ...room(1).video, id: 8 };
	const ann = [{ userId: 1, name: 'Ann' }];
	const ids = (rs: RoomCard[] = []) => rs.map((r) => r.id);

	it("lists each video's rooms, people first, then the most recently used", () => {
		const byVideo = roomsByVideo([
			card(1, { usedAt: 900 }),
			card(2, { usedAt: 100, watching: ann }),
			card(3, { usedAt: 500 }),
			card(4, { usedAt: 999, video: other })
		]);
		expect(ids(byVideo.get(7))).toEqual([2, 1, 3]);
		expect(ids(byVideo.get(8))).toEqual([4]);
	});

	it("leaves out rooms that can't play, or are at the end", () => {
		const byVideo = roomsByVideo([
			card(1, { archived: true }),
			card(2, { gone: true }),
			card(3, { gone: true, watching: ann }),
			card(4, { positionMs: 5000, video: { ...room(4).video, durationMs: 5000 } })
		]);
		expect(byVideo.get(7)).toBeUndefined();
	});
});

describe('usedAgo', () => {
	const now = Date.UTC(2026, 9, 2, 12);
	const min = 60_000;
	it.each([
		[0, ''],
		[now, 'just now'],
		[now + 5 * min, 'just now'], // a clock a little ahead
		[now - 5 * min, '5 minutes ago'],
		[now - 3 * 60 * min, '3 hours ago'],
		[now - 30 * 60 * min, '1 day ago'], // never "yesterday": 30 h may be two calendar days back
		[now - 3 * 24 * 60 * min, '3 days ago'],
		[now - 15 * 24 * 60 * min, '2 weeks ago'],
		[now - 70 * 24 * 60 * min, '2 months ago'],
		[now - 360 * 24 * 60 * min, '11 months ago'],
		[now - 400 * 24 * 60 * min, '1 year ago']
	])('%i → %s', (usedAt, want) => {
		expect(usedAgo(usedAt, now)).toBe(want);
	});
});

describe('needsConfirm', () => {
	const hour = 3_600_000;
	it.each([
		['mid-video', 5000, hour, false, true],
		['at 0:00', 0, hour, false, false],
		['under a second: still shows 0:00', 999, hour, false, false],
		['at the end', hour, hour, false, false],
		['duration unknown', 5000, 0, false, true],
		['the video is missing: the swap keeps the position', 5000, hour, true, false]
	])('%s', (_, positionMs, durationMs, missing, want) => {
		expect(needsConfirm(positionMs, durationMs, missing)).toBe(want);
	});
});

describe('waitText', () => {
	const alice: Wait = { userId: 1, name: 'Alice', reason: 'away', sinceMs: 10_000 };
	const bob: Wait = { userId: 2, name: 'Bob', reason: 'buffering', sinceMs: 18_000 };
	const carol: Wait = { userId: 3, name: 'Carol', reason: 'left', sinceMs: 0 };
	it.each([
		[
			'one other person: named once, with why and how long',
			[alice],
			{ headline: 'Waiting for Alice', lines: ['stepped away · 0:12'], action: 'Play without Alice' }
		],
		[
			'several: a line each',
			[alice, bob, carol],
			{
				headline: 'Waiting for Alice, Bob, and Carol',
				lines: ['Alice: stepped away · 0:12', 'Bob: loading · 0:04', 'Carol: left the room · 0:22'],
				action: 'Play without Alice, Bob, and Carol'
			}
		],
		[
			'me, loading',
			[{ ...alice, userId: 9, reason: 'buffering' }],
			{ headline: "Everyone's waiting for you", lines: ['Your video is still loading…'], action: "Don't wait for me" }
		],
		[
			'me and Bob',
			[bob, { ...alice, userId: 9, reason: 'buffering' }],
			{
				headline: "Everyone's waiting for you",
				lines: ['Your video is still loading…', 'Bob: loading · 0:04'],
				action: 'Play anyway'
			}
		]
	] as [string, Wait[], WaitText][])('%s', (_, waiting, want) => {
		expect(waitText(waiting, 9, 22_000, false)).toEqual(want);
	});

	// The room may wait for this page, or for another socket of the same person.
	it.each([
		['away, this page needs a tap: "Tap to join" says the rest', 'away', true, []],
		['away, this page is fine: another screen is away', 'away', false, ['Another of your screens stepped away.']],
		['left, this page needs a tap: its old socket left', 'left', true, []],
		['left, this page is loading', 'left', false, ['Your video is still loading…']],
		['buffering', 'buffering', false, ['Your video is still loading…']]
	] as [string, Wait['reason'], boolean, string[]][])('me, %s', (_, reason, needsTap, lines) => {
		expect(waitText([{ ...alice, userId: 9, reason }], 9, 22_000, needsTap).lines).toEqual(lines);
	});

	it('leaves out times before the server clock is known', () => {
		expect(waitText([alice], 9, null, false).lines).toEqual(['stepped away']);
	});
});
