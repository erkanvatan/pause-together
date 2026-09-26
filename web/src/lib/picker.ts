// The library picker's logic: browsing, search, what this device can play, and the default audio
// and subtitle. Pure: no fetch, no DOM.
import type {
	AudioTrack,
	Languages,
	LibraryType,
	VideoDetail,
	VideoSummary
} from '$lib/api';
import { reasonText, strings } from '$lib/strings';

// Pick is what the picker hands back: a video, its audio track (null: the video has none) and its
// subtitle (null: off).
export type Pick = { videoId: number; audio: number | null; subtitle: SubtitleChoice | null };

// A subtitle the picker offers: an embedded track or a sidecar file, in one list.
export type SubtitleChoice = { stream: number } | { sidecar: number };

export type SubtitleOption = {
	key: string; // unique in its video, for <select> values
	choice: SubtitleChoice;
	lang: string;
	title: string;
	default: boolean;
	forced: boolean;
	sdh: boolean;
	unavailable: string; // a code; '' = can be shown
};

// subtitleOptions lists a video's subtitles: embedded tracks by stream, then sidecar files.
export function subtitleOptions(v: VideoDetail): SubtitleOption[] {
	return [
		...v.subtitles.map((s) => ({
			key: `s${s.stream}`,
			choice: { stream: s.stream },
			lang: s.lang,
			title: s.title,
			default: s.default,
			forced: s.forced,
			sdh: s.sdh,
			unavailable: s.unavailable
		})),
		...v.sidecars.map((s) => ({
			key: `f${s.id}`,
			choice: { sidecar: s.id },
			lang: s.lang,
			title: '',
			default: false,
			forced: s.forced,
			sdh: s.sdh,
			unavailable: ''
		}))
	];
}

// defaultAudio: the first track in the host's language, else the file's default track, else the
// first. A default-flagged track wins among several in one language.
export function defaultAudio(tracks: AudioTrack[], langs: Languages): AudioTrack | null {
	const pick = (list: AudioTrack[]) => list.find((t) => t.default) ?? list[0];
	if (langs.audio) {
		const match = tracks.filter((t) => t.lang === langs.audio);
		if (match.length > 0) return pick(match);
	}
	return pick(tracks) ?? null;
}

// defaultSubtitle walks the host's subtitle languages in order and takes the first one with a full
// (not forced) track, even if it's the audio's language. Within a language: not SDH first, then the
// default-flagged one. If no language matches: a forced track in the audio's language (it translates
// the few lines in another language), then the file's default track, then none.
export function defaultSubtitle(
	options: SubtitleOption[],
	audioLang: string,
	langs: Languages
): SubtitleOption | null {
	const usable = options.filter((o) => !o.unavailable);
	const rank = (o: SubtitleOption) => (o.sdh ? 2 : 0) + (o.default ? 0 : 1);
	for (const lang of langs.subtitles) {
		const full = usable.filter((o) => o.lang === lang && !o.forced);
		if (full.length > 0) return full.reduce((best, o) => (rank(o) < rank(best) ? o : best));
	}
	if (audioLang) {
		const forced = usable.find((o) => o.forced && o.lang === audioLang);
		if (forced) return forced;
	}
	return usable.find((o) => o.default) ?? null;
}

// canPlay asks the browser whether it can decode the video. Prepared copies are always MP4, and the
// full codec string matters: a bare 'hvc1' answers "maybe" to almost anything.
export function canPlay(codecString: string, canPlayType: (type: string) => string): boolean {
	return codecString !== '' && canPlayType(`video/mp4; codecs="${codecString}"`) !== '';
}

// whyUnplayable says why a video can't be picked: the server found it unplayable, or this device
// can't decode it. '' = it can be picked.
export function whyUnplayable(v: VideoSummary, canPlayType: (type: string) => string): string {
	if (v.unplayable) return reasonText(v.unplayable);
	return canPlay(v.codecString, canPlayType) ? '' : strings.cantPlayHere(codecName(v.codecString));
}

// codecName names a codec string's codec for people: 'hvc1.2.4.L120.90' is HEVC.
export function codecName(codecString: string): string {
	const names: Record<string, string> = {
		avc1: 'H.264',
		avc3: 'H.264',
		hvc1: 'HEVC',
		hev1: 'HEVC',
		av01: 'AV1',
		vp09: 'VP9'
	};
	const prefix = codecString.split('.')[0];
	return names[prefix] ?? prefix;
}

// searchKey folds text for search: no accents, lower case, Turkish dotless ı as i. So "sehir" finds
// "Şehir" and "istanbul" finds "İstanbul".
export function searchKey(s: string): string {
	return s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().replaceAll('ı', 'i');
}

export type SearchEntry = { video: VideoSummary; text: string };

// searchIndex folds each video's searchable text once, so typing doesn't redo it per keystroke.
export function searchIndex(videos: VideoSummary[]): SearchEntry[] {
	return videos.map((v) => ({
		video: v,
		text: searchKey(
			[v.title, v.year || '', v.edition, v.version, v.episodeTitle, v.group, episodeCode(v)].join(' ')
		)
	}));
}

// search returns the videos that hold every word of the query, in the index's order.
export function search(index: SearchEntry[], query: string): VideoSummary[] {
	const words = searchKey(query).split(/\s+/).filter(Boolean);
	if (words.length === 0) return [];
	return index.filter((e) => words.every((w) => e.text.includes(w))).map((e) => e.video);
}

// episodeCode writes S01E02, or S01E02-E03 for a two-episode file. '' for anything not an episode.
export function episodeCode(v: VideoSummary): string {
	if (v.type !== 'tv') return '';
	const pad = (n: number) => String(n).padStart(2, '0');
	const code = `S${pad(v.season)}E${pad(v.episode)}`;
	return v.episodeEnd > v.episode ? `${code}-E${pad(v.episodeEnd)}` : code;
}

// withYear writes "Title (Year)", or just the title with no year.
export function withYear(title: string, year: number): string {
	return year ? `${title} (${year})` : title;
}

// videoName names a video on its own, as in search results: "Heat (1995) · Director's Cut",
// "Dark (2017) · S01E02 · Secrets", "Trips/Rome · Day 1".
export function videoName(v: VideoSummary): string {
	switch (v.type) {
		case 'movies':
			return [withYear(v.title, v.year), v.edition, v.version].filter(Boolean).join(' · ');
		case 'tv':
			return [withYear(v.title, v.year), episodeCode(v), v.episodeTitle].filter(Boolean).join(' · ');
		case 'other':
			return [v.group, v.title].filter(Boolean).join(' · ');
	}
}

export type Sort = 'title' | 'recent';

export type Season = { number: number; episodes: VideoSummary[] };
// newest is the id of the newest video in it, for the "recent" sort.
export type Show = { key: string; title: string; year: number; newest: number; seasons: Season[] };
export type Folder = { name: string; newest: number; videos: VideoSummary[] }; // name '' = the top folder

// Shelf is one tab of the picker: every video of one library type, grouped.
export type Shelf =
	| { type: 'movies'; movies: VideoSummary[] }
	| { type: 'tv'; shows: Show[] }
	| { type: 'other'; folders: Folder[] };

const collator = new Intl.Collator(strings.locale, { numeric: true, sensitivity: 'base' });

// shelves groups videos by library type, in the order Movies, TV Shows, Other Videos, leaving out
// types with no videos. Libraries of one type share a shelf: a show split across two disks is one
// show. The "title" sort is A to Z; "recent" puts the newest first, and a show or folder counts as
// new as its newest video. Episodes are always in order, specials last.
export function shelves(videos: VideoSummary[], sort: Sort): Shelf[] {
	const byName = <T extends { newest: number }>(list: T[], name: (x: T) => string) =>
		list.sort((a, b) =>
			sort === 'recent' ? b.newest - a.newest : collator.compare(name(a), name(b))
		);
	const sortVideos = (list: VideoSummary[], name: (v: VideoSummary) => string) =>
		list.sort((a, b) => (sort === 'recent' ? b.id - a.id : collator.compare(name(a), name(b))));

	const of = (type: LibraryType) => videos.filter((v) => v.type === type);
	const out: Shelf[] = [];

	const movies = of('movies');
	if (movies.length > 0) {
		out.push({
			type: 'movies',
			movies: sortVideos(movies, (v) => `${v.title} ${v.year} ${v.edition} ${v.version}`)
		});
	}

	const episodes = of('tv');
	if (episodes.length > 0) {
		const shows = new Map<string, Show>();
		for (const v of episodes) {
			const key = `${v.title}\u0000${v.year}`;
			let show = shows.get(key);
			if (!show) {
				show = { key, title: v.title, year: v.year, newest: 0, seasons: [] };
				shows.set(key, show);
			}
			let season = show.seasons.find((s) => s.number === v.season);
			if (!season) {
				season = { number: v.season, episodes: [] };
				show.seasons.push(season);
			}
			season.episodes.push(v);
			show.newest = Math.max(show.newest, v.id);
		}
		for (const show of shows.values()) {
			show.seasons.sort((a, b) => (a.number || Infinity) - (b.number || Infinity));
			for (const s of show.seasons) s.episodes.sort((a, b) => a.episode - b.episode || a.id - b.id);
		}
		out.push({ type: 'tv', shows: byName([...shows.values()], (s) => withYear(s.title, s.year)) });
	}

	const others = of('other');
	if (others.length > 0) {
		const folders = new Map<string, Folder>();
		for (const v of others) {
			let f = folders.get(v.group);
			if (!f) {
				f = { name: v.group, newest: 0, videos: [] };
				folders.set(v.group, f);
			}
			f.videos.push(v);
			f.newest = Math.max(f.newest, v.id);
		}
		for (const f of folders.values()) sortVideos(f.videos, (v) => v.title);
		const list = byName([...folders.values()], (f) => f.name);
		// The top folder's videos come first, as they sit above every sub-folder.
		list.sort((a, b) => Number(b.name === '') - Number(a.name === ''));
		out.push({ type: 'other', folders: list });
	}
	return out;
}

// langName names a language in the UI's language: 'tr' is "Turkish". An unknown code shows as it is.
export function langName(code: string): string {
	if (!code) return strings.unknownLanguage;
	try {
		return new Intl.DisplayNames([strings.locale], { type: 'language' }).of(code) ?? code;
	} catch {
		return code; // not a well-formed language tag
	}
}

// audioLabel: "English · 5.1 · Director's commentary".
export function audioLabel(t: AudioTrack): string {
	return [langName(t.lang), strings.channels(t.channels), t.title].filter(Boolean).join(' · ');
}

// subtitleLabel: "English · Forced · SDH · Signs".
export function subtitleLabel(o: SubtitleOption): string {
	return [
		langName(o.lang),
		o.forced ? strings.forced : '',
		o.sdh ? strings.sdh : '',
		o.title,
		'sidecar' in o.choice ? strings.sidecarFile : ''
	]
		.filter(Boolean)
		.join(' · ');
}
