import { describe, expect, it } from 'vitest';
import type { Room } from './api';
import { needsConfirm, roomTitle, splitArchived } from './rooms';

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
		]
	])('shows %s', (_, r, want) => {
		expect(roomTitle(r)).toBe(want);
	});
});

describe('splitArchived', () => {
	it('keeps the order in each part', () => {
		const rooms = [room(4), room(3, { archived: true }), room(2), room(1, { archived: true })];
		const { active, archived } = splitArchived(rooms);
		expect(active.map((r) => r.id)).toEqual([4, 2]);
		expect(archived.map((r) => r.id)).toEqual([3, 1]);
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
