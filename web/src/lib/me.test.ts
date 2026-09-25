import { afterEach, describe, expect, it, vi } from 'vitest';
import { loadMe, me, saveName } from './me.svelte';

function respond(status: number, body: string) {
	vi.stubGlobal('fetch', vi.fn(async () => new Response(body, { status })));
}

afterEach(() => {
	vi.unstubAllGlobals();
	me.loaded = false;
	me.name = null;
	me.isAdmin = false;
});

describe('saveName', () => {
	it('stores the name the server returns', async () => {
		respond(200, '{"name":"Alice","isAdmin":true}');
		expect(await saveName('  Alice  ')).toBe('ok');
		expect(me).toEqual({ loaded: true, name: 'Alice', isAdmin: true });
	});

	it('reports a name the server refused', async () => {
		respond(400, 'bad name');
		expect(await saveName('a\tb')).toBe('invalid');
		expect(me.name).toBeNull();
	});

	it.each([
		['server error', () => respond(500, 'oops')],
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
		respond(200, '{"name":null,"isAdmin":false}');
		await loadMe();
		expect(me).toEqual({ loaded: true, name: null, isAdmin: false });
	});

	it('throws on a server error, so the page retries', async () => {
		respond(500, 'oops');
		await expect(loadMe()).rejects.toThrow();
		expect(me.loaded).toBe(false);
	});
});
