// The admin API. Only the admin port serves it; elsewhere every call fails with 404.
import { strings } from '$lib/strings';

export type LibraryType = keyof typeof strings.libraryTypes;

export type ScanStatus = {
	state: '' | 'queued' | 'scanning';
	done: number; // while scanning: videos done so far
	total: number; // while scanning: videos found
	error: string; // why the last scan failed; '' if it worked
};

export type Library = {
	id: number;
	path: string; // relative to the media folder
	type: LibraryType;
	videos: number; // videos that aren't missing
	scan: ScanStatus;
};

// A file the admin page lists: skipped by the scan, unplayable, or Apple devices only.
export type Problem = {
	libraryId: number;
	path: string; // relative to the library folder
	reason?: string; // a code; problemText turns it into text
	codec?: string;
	probeError?: string;
};

export type Problems = { skipped: Problem[]; unplayable: Problem[]; appleOnly: Problem[] };

// The server's error code, or 'failed' for no connection, a server error, or an unexpected body.
export type Result<T> = { ok: true; value: T } | { ok: false; error: string };

async function call<T>(method: string, url: string, body?: unknown): Promise<Result<T>> {
	try {
		const res = await fetch(url, {
			method,
			headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
			body: body === undefined ? undefined : JSON.stringify(body)
		});
		if (res.ok) {
			const value = res.headers.get('Content-Type')?.startsWith('application/json')
				? await res.json()
				: undefined;
			return { ok: true, value };
		}
		const code = await res
			.json()
			.then((b) => b?.error)
			.catch(() => undefined);
		return { ok: false, error: typeof code === 'string' ? code : 'failed' };
	} catch {
		return { ok: false, error: 'failed' };
	}
}

export const listLibraries = () => call<Library[]>('GET', '/api/admin/libraries');

export const addLibrary = (path: string, type: LibraryType) =>
	call<Library>('POST', '/api/admin/libraries', { path, type });

export const removeLibrary = (id: number) => call<void>('DELETE', `/api/admin/libraries/${id}`);

export const rescanLibrary = (id: number) => call<void>('POST', `/api/admin/libraries/${id}/scan`);

export const listProblems = () => call<Problems>('GET', '/api/admin/problems');

// listFolders lists the folders inside path, relative to the media folder ('' is the media folder).
export async function listFolders(path: string): Promise<Result<string[]>> {
	const r = await call<{ folders: string[] }>(
		'GET',
		`/api/admin/folders?path=${encodeURIComponent(path)}`
	);
	return r.ok ? { ok: true, value: r.value.folders } : r;
}

// problemText says why a file can't be used. Unknown codes show as they are.
export function problemText(p: Problem): string {
	const reasons: Record<string, string> = strings.reasons;
	const text = (p.reason && reasons[p.reason]) || p.reason || '';
	return p.reason === 'codec' && p.codec ? `${text} (${p.codec})` : text;
}
