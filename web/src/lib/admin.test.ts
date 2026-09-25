import { afterEach, describe, expect, it, vi } from 'vitest';
import { addLibrary, listFolders, problemText, removeLibrary } from './admin';

function respond(status: number, body: string | null, type = 'application/json') {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(body, { status, headers: body ? { 'Content-Type': type } : {} }))
	);
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('call results', () => {
	it('returns the body on success', async () => {
		respond(201, '{"id":1,"path":"Movies","type":"movies"}');
		expect(await addLibrary('Movies', 'movies')).toEqual({
			ok: true,
			value: { id: 1, path: 'Movies', type: 'movies' }
		});
	});

	it('succeeds with no body', async () => {
		respond(204, null);
		expect(await removeLibrary(1)).toEqual({ ok: true, value: undefined });
	});

	it.each([
		[409, '{"error":"overlap"}', 'overlap'],
		[400, '{"error":"not-folder"}', 'not-folder'],
		[404, '404 page not found', 'failed'],
		[500, 'Internal Server Error', 'failed'],
		[400, '{"error":42}', 'failed']
	])('maps status %i with body %s to %s', async (status, body, want) => {
		respond(status, body, body.startsWith('{') ? 'application/json' : 'text/plain');
		expect(await addLibrary('Movies', 'movies')).toEqual({ ok: false, error: want });
	});

	it('reports no connection as failed', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => {
				throw new TypeError('Failed to fetch');
			})
		);
		expect(await removeLibrary(1)).toEqual({ ok: false, error: 'failed' });
	});

	it('sends the folder path escaped and unwraps the list', async () => {
		const fetch = vi.fn(
			async () =>
				new Response('{"folders":["A","B"]}', { headers: { 'Content-Type': 'application/json' } })
		);
		vi.stubGlobal('fetch', fetch);
		expect(await listFolders('Movies/A & B')).toEqual({ ok: true, value: ['A', 'B'] });
		expect(fetch).toHaveBeenCalledWith('/api/admin/folders?path=Movies%2FA%20%26%20B', {
			method: 'GET',
			headers: undefined,
			body: undefined
		});
	});
});

describe('problemText', () => {
	it('turns a reason code into text', () => {
		expect(problemText({ libraryId: 1, path: 'x', reason: 'movie-no-year' })).toContain('year');
	});

	it('names the codec', () => {
		expect(problemText({ libraryId: 1, path: 'x', reason: 'codec', codec: 'vp8' })).toMatch(
			/\(vp8\)$/
		);
	});

	it('shows an unknown code as it is', () => {
		expect(problemText({ libraryId: 1, path: 'x', reason: 'new-reason' })).toBe('new-reason');
	});
});
