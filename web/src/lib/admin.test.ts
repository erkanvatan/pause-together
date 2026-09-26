import { afterEach, describe, expect, it, vi } from 'vitest';
import { addLibrary, formatBytes, jobText, listFolders, problemText, removeLibrary } from './admin';

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

describe('formatBytes', () => {
	it.each([
		[0, '0 B'],
		[999, '999 B'],
		[999_999, '1.0 MB'],
		[9.96e9, '10 GB'],
		[1500, '1.5 kB'],
		[1_234_567_890, '1.2 GB'],
		[12_345_678_901, '12 GB'],
		[300e9, '300 GB'],
		[3.25e12, '3.3 TB'],
		[5e15, '5000 TB']
	])('%d → %s', (n, want) => {
		expect(formatBytes(n)).toBe(want);
	});
});

describe('jobText', () => {
	const job = {
		key: 'k',
		name: 'x',
		place: 0,
		progress: 0,
		error: '',
		detail: ''
	};

	it('shows progress as a whole percent', () => {
		expect(jobText({ ...job, state: 'running', progress: 0.426 })).toBe('Preparing… 42%');
	});

	it.each([
		[1, 'Queued, next in line'],
		[2, 'Queued, 2nd in line'],
		[3, 'Queued, 3rd in line'],
		[4, 'Queued, 4th in line'],
		[11, 'Queued, 11th in line'],
		[12, 'Queued, 12th in line'],
		[21, 'Queued, 21st in line'],
		[102, 'Queued, 102nd in line']
	])('place %i → %s', (place, want) => {
		expect(jobText({ ...job, state: 'queued', place })).toBe(want);
	});

	it('turns a failure code into text', () => {
		expect(jobText({ ...job, state: 'failed', error: 'no-space' })).toContain('disk space');
	});

	it('shows an unknown failure code as it is', () => {
		expect(jobText({ ...job, state: 'failed', error: 'new-code' })).toBe('new-code');
	});
});
