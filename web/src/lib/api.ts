// Calls to the API both ports serve.

export type LibraryType = 'movies' | 'tv' | 'other';

// A video as the picker lists it. Name fields: 0 and '' mean none, except season 0, which is specials.
export type VideoSummary = {
	id: number; // ids only grow, so a higher id was added later
	type: LibraryType; // its library's
	title: string; // movie title, show name, or other video's file name
	year: number;
	edition: string;
	version: string;
	season: number;
	episode: number;
	episodeEnd: number; // last episode in the file; equals episode for one-episode files
	episodeTitle: string;
	group: string; // other videos: folder path in the library, '' at its root
	durationMs: number;
	codecString: string; // for canPlayType(), e.g. 'avc1.640028'
	unplayable: string; // a reason code; '' = browsers can play the codec
	appleOnly: boolean; // Dolby Vision profile 5: other screens show it purple and green
};

// Every lang is a code the server normalized: 2 letters where one exists ('en', not 'eng').
export type AudioTrack = {
	stream: number;
	codec: string;
	channels: number;
	lang: string;
	title: string;
	default: boolean;
};

export type SubtitleTrack = {
	stream: number;
	lang: string;
	title: string;
	default: boolean;
	forced: boolean;
	sdh: boolean;
	unavailable: string; // a code; '' = can be shown
};

export type Sidecar = { id: number; lang: string; forced: boolean; sdh: boolean };

export type VideoDetail = VideoSummary & {
	audio: AudioTrack[];
	subtitles: SubtitleTrack[];
	sidecars: Sidecar[];
};

// The host's language defaults for new picks.
export type Languages = {
	audio: string; // '' = original: the file's default track
	subtitles: string[]; // in order of preference
};

// The server's error code, or 'failed' for no connection, a server error, or an unexpected body.
export type Result<T> = { ok: true; value: T } | { ok: false; error: string };

export async function call<T>(method: string, url: string, body?: unknown): Promise<Result<T>> {
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

export const listVideos = () => call<VideoSummary[]>('GET', '/api/videos');

export const getVideo = (id: number) => call<VideoDetail>('GET', `/api/videos/${id}`);

export const getLanguages = () => call<Languages>('GET', '/api/languages');
