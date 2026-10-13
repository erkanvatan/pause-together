import { afterEach, describe, expect, it, vi } from 'vitest';
import { forgetMe, joinPerson, loadMe, logOut, me, saveName } from './me.svelte';

function respond(status: number, body: string) {
	vi.stubGlobal('fetch', vi.fn(async () => new Response(body, { status })));
}

afterEach(() => {
	vi.unstubAllGlobals();
	me.loaded = false;
	me.id = null;
	me.name = null;
	me.isAdmin = false;
	me.guestUrl = '';
});

describe('saveName', () => {
	it('stores the name the server returns', async () => {
		respond(200, '{"id":4,"name":"Alice","isAdmin":true,"guestUrl":"http://100.101.102.103:8420"}');
		expect(await saveName('  Alice  ')).toBe('ok');
		expect(me).toEqual({
			loaded: true,
			id: 4,
			name: 'Alice',
			isAdmin: true,
			guestUrl: 'http://100.101.102.103:8420'
		});
	});

	it('reports a name the server refused', async () => {
		respond(400, 'bad name');
		expect(await saveName('a\tb')).toBe('invalid');
		expect(me.name).toBeNull();
	});

	it("reports another person's name, and joins them by id", async () => {
		respond(409, '{"error":"taken","id":2,"name":"Ali"}');
		expect(await saveName('ali')).toEqual({ taken: { id: 2, name: 'Ali' } });
		expect(me.name).toBeNull();

		respond(200, '{"id":2,"name":"Bob","isAdmin":false}');
		expect(await joinPerson(2)).toBe('ok');
		expect(vi.mocked(fetch).mock.calls[0][1]?.body).toBe('{"join":2}');
		expect(me.name).toBe('Bob');
	});

	it("doesn't blame the name when a join is refused", async () => {
		respond(404, 'unknown person');
		expect(await joinPerson(999)).toBe('failed');
	});

	it.each([
		['server error', () => respond(500, 'oops')],
		['taken, but not who', () => respond(409, '{"error":"taken","name":"Ali"}')],
		['refused (403)', () => respond(403, 'forbidden')],
		['not JSON', () => respond(200, '<html>proxy error</html>')],
		[
			'no connection',
			() =>
				vi.stubGlobal(
					'fetch',
					vi.fn(async () => {
						throw new TypeError('Failed to fetch');
					})
				)
		]
	])('fails without throwing: %s', async (_, setup) => {
		setup();
		expect(await saveName('Alice')).toBe('failed');
		expect(me.name).toBeNull();
	});
});

describe('loadMe', () => {
	it('loads a new visitor', async () => {
		respond(200, '{"id":null,"name":null,"isAdmin":false}');
		await loadMe();
		expect(me).toEqual({ loaded: true, id: null, name: null, isAdmin: false, guestUrl: '' });
	});

	it('loses to a rename that answered first', async () => {
		let answerLoad = (_: Response) => {};
		vi.stubGlobal(
			'fetch',
			vi.fn(async (_url: string, init?: RequestInit) =>
				init?.method === 'POST'
					? new Response('{"id":1,"name":"Bo","isAdmin":false}')
					: new Promise<Response>((resolve) => (answerLoad = resolve))
			)
		);
		const loading = loadMe();
		expect(await saveName('Bo')).toBe('ok');
		answerLoad(new Response('{"id":1,"name":"Ali","isAdmin":false}'));
		await loading;
		expect(me.name).toBe('Bo');
	});

	it('throws on a server error, so the page retries', async () => {
		respond(500, 'oops');
		await expect(loadMe()).rejects.toThrow();
		expect(me.loaded).toBe(false);
	});
});

describe('logOut', () => {
	it('forgets the name, so the page asks for one again', async () => {
		me.name = 'Alice';
		respond(200, '{"id":null,"name":null,"isAdmin":false}');
		expect(await logOut()).toBe(true);
		expect(me.name).toBeNull();
	});

	it('keeps the name when the server failed', async () => {
		me.name = 'Alice';
		respond(500, 'oops');
		expect(await logOut()).toBe(false);
		expect(me.name).toBe('Alice');
	});

	it('asks the server when the answer was lost, and it had logged out', async () => {
		me.name = 'Alice';
		vi.stubGlobal(
			'fetch',
			vi
				.fn()
				.mockImplementationOnce(async () => {
					throw new TypeError('Failed to fetch');
				})
				.mockImplementationOnce(async () => new Response('{"id":null,"name":null,"isAdmin":false}'))
		);
		expect(await logOut()).toBe(true);
		expect(me.name).toBeNull();
	});
});

describe('logOut wins', () => {
	it('over a look that started after it but answered first', async () => {
		me.name = 'Alice';
		let answerLogOut = (_: Response) => {};
		vi.stubGlobal(
			'fetch',
			vi.fn((_url: string, init?: RequestInit) =>
				init?.method === 'DELETE'
					? new Promise<Response>((resolve) => (answerLogOut = resolve))
					: Promise.resolve(new Response('{"id":1,"name":"Alice","isAdmin":false}'))
			)
		);
		const out = logOut();
		await loadMe();
		answerLogOut(new Response('{"id":null,"name":null,"isAdmin":false}'));
		expect(await out).toBe(true);
		expect(me.name).toBeNull();
	});
});

describe('forgetMe', () => {
	it('forgets the name, and an answer still on its way loses', async () => {
		me.id = 1;
		me.name = 'Alice';
		let answer = (_: Response) => {};
		vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => (answer = resolve))));
		const loading = loadMe();
		forgetMe();
		answer(new Response('{"id":1,"name":"Alice","isAdmin":false}'));
		await loading;
		expect([me.id, me.name]).toEqual([null, null]);
	});

	it('a failed rename leaves the look on its way', async () => {
		let answer = (_: Response) => {};
		vi.stubGlobal(
			'fetch',
			vi
				.fn()
				.mockImplementationOnce(() => new Promise<Response>((resolve) => (answer = resolve)))
				.mockImplementationOnce(async () => new Response('oops', { status: 500 }))
		);
		const loading = loadMe();
		expect(await saveName('Bo')).toBe('failed');
		answer(new Response('{"id":1,"name":"Ali","isAdmin":false}'));
		await loading;
		expect(me.name).toBe('Ali');
	});
});
