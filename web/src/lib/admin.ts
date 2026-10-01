// The admin API. Only the admin port serves it; elsewhere every call fails with 404.
import { call, type Languages, type LibraryType, type Result } from '$lib/api';
import type { Prepare } from '$lib/protocol';
import { reasonText, strings } from '$lib/strings';

export type ScanStatus = {
	state: '' | 'queued' | 'scanning';
	done: number; // while scanning: videos done so far
	total: number; // while scanning: videos found
	error: string; // why the last scan failed; '' if it worked
	gone: boolean; // the last scan failed because the library's folder is gone
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

// A prepare job: one video + audio track turned into a cached MP4.
export type Job = {
	key: string;
	name: string;
	state: 'running' | 'queued' | 'failed';
	place: number; // queued: 1 = next in line
	progress: number; // running: 0 to 1
	error: string; // failed: a code; jobText turns it into text
	detail: string; // failed: sizes, or ffmpeg's message
};

export type Jobs = { jobs: Job[]; cacheBytes: number; freeBytes: number };

export const listLibraries = () => call<Library[]>('GET', '/api/admin/libraries');

export const addLibrary = (path: string, type: LibraryType) =>
	call<Library>('POST', '/api/admin/libraries', { path, type });

export const removeLibrary = (id: number) => call<void>('DELETE', `/api/admin/libraries/${id}`);

export const rescanLibrary = (id: number) => call<void>('POST', `/api/admin/libraries/${id}/scan`);

export const listProblems = () => call<Problems>('GET', '/api/admin/problems');

export const listJobs = () => call<Jobs>('GET', '/api/admin/jobs');

// deleteRoom deletes a room for good. Its videos stay.
export const deleteRoom = (id: number) => call<void>('DELETE', `/api/admin/rooms/${id}`);

// setLanguages returns the defaults as saved: codes normalized, duplicates dropped.
export const setLanguages = (langs: Languages) =>
	call<Languages>('PUT', '/api/admin/languages', langs);

// The cache clean-up setting: a prepared copy nobody has used for unusedDays is deleted.
export type CacheSettings = { unusedDays: number };

// The bounds of unusedDays (media.MinUnusedDays, media.MaxUnusedDays).
export const minUnusedDays = 1;
export const maxUnusedDays = 365;

export const getCache = () => call<CacheSettings>('GET', '/api/admin/cache');

export const setCache = (c: CacheSettings) => call<CacheSettings>('PUT', '/api/admin/cache', c);

// clearCache deletes every prepared copy. Converted sidecar subtitles and a running job stay.
export const clearCache = () => call<void>('DELETE', '/api/admin/cache');

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
	const text = p.reason ? reasonText(p.reason) : '';
	return p.reason === 'codec' && p.codec ? `${text} (${p.codec})` : text;
}

// guessType guesses a library's type from its folder's name, so the add form starts on the likely one:
// "TV Shows" and "Series" hold shows, "Movies" and "Films" movies. null: no guess.
export function guessType(path: string): LibraryType | null {
	const name = (path.split('/').pop() ?? '').toLowerCase();
	if (/\b(tv|shows?|series|diziler|dizi)\b/.test(name)) return 'tv';
	if (/\b(movies?|films?|filmler)\b/.test(name)) return 'movies';
	return null;
}

// jobText says where a job, or a room's prepared copy, stands; '' when it's ready or there's none.
// Unknown failure codes show as they are.
export function jobText(j: Pick<Prepare, 'state' | 'place' | 'progress' | 'error'>): string {
	switch (j.state) {
		case 'running':
			return strings.jobRunning(Math.floor(j.progress * 100));
		case 'queued':
			return strings.jobQueued(j.place);
		case 'failed':
			return strings.jobErrors[j.error] ?? j.error;
		default:
			return '';
	}
}

const byteUnits = ['B', 'kB', 'MB', 'GB', 'TB'];

// formatBytes writes a size in SI units (1 GB = 10⁹ bytes, like the server's messages): whole bytes,
// then one decimal below 10 and none above.
export function formatBytes(n: number): string {
	let i = 0;
	// The limits are checked after rounding, so 999,999 B is "1.0 MB", not "1000 kB".
	while (n >= 999.5 && i < byteUnits.length - 1) {
		n /= 1000;
		i++;
	}
	const digits = i > 0 && n < 9.95 ? 1 : 0;
	return `${n.toFixed(digits)} ${byteUnits[i]}`;
}
