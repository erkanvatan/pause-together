import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { ClientMessage, ServerMessage } from './protocol';

// The fixture Go's protocol test checks too: what one side sends is what the other reads.
const fixture = JSON.parse(
	readFileSync(new URL('../../../internal/room/testdata/protocol.json', import.meta.url), 'utf8')
);

const alice = { userId: 1, name: 'Alice' };
const bob = { userId: 2, name: 'Bob' };

const client: ClientMessage[] = [
	{ type: 'ping', t: 1234.5 },
	{ type: 'play' },
	{ type: 'pause' },
	{ type: 'seek', positionMs: 90000 },
	{ type: 'playAnyway' },
	{ type: 'subtitle', subtitle: { stream: 3 } },
	{ type: 'subtitle', subtitle: { sidecar: 7 } },
	{ type: 'subtitle', subtitle: null },
	{ type: 'offset', ms: -1500 },
	{ type: 'status', status: 'ready', positionMs: 12345.678 },
	{ type: 'status', status: 'buffering', positionMs: 0 },
	{ type: 'status', status: 'away', positionMs: 5000 },
	{ type: 'status', status: 'cantPlay', positionMs: 0 }
];

const server: ServerMessage[] = [
	{ type: 'hello', buildId: '1727000000000', userId: 1 },
	{
		type: 'state',
		state: {
			videoId: 7,
			audio: 1,
			subtitle: { stream: 3 },
			subtitleOffsetMs: 500,
			durationMs: 3600000,
			playing: true,
			positionMs: 90000,
			atMs: 4000,
			waiting: [alice],
			behind: [{ ...bob, ms: 3000 }]
		}
	},
	{
		type: 'room',
		room: {
			id: 1,
			name: 'Movie night',
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
				durationMs: 3600000,
				codecString: 'avc1.640028',
				unplayable: '',
				appleOnly: false,
				missing: false
			},
			audio: null,
			subtitle: { sidecar: 7 },
			positionMs: 0,
			subtitleOffsetMs: 0,
			archived: false
		}
	},
	{ type: 'presence', watching: [alice], wasHere: [] },
	{ type: 'prepare', prepare: { state: 'queued', place: 2, progress: 0, error: '', key: '' } },
	{
		type: 'prepare',
		prepare: { state: 'ready', place: 0, progress: 0, error: '', key: '0123456789abcdef0123456789abcdef' }
	},
	{ type: 'paused', by: bob },
	{ type: 'pong', t: 1234.5, serverMs: 98765 },
	{ type: 'deleted' }
];

describe('protocol', () => {
	it('sends what the server reads', () => {
		expect(client).toEqual(fixture.client);
	});

	it('reads what the server sends', () => {
		expect(server).toEqual(fixture.server);
	});
});
