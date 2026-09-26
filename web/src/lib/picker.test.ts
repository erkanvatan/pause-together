import { describe, expect, it } from 'vitest';
import type { AudioTrack, Languages, VideoDetail, VideoSummary } from './api';
import {
	canPlay,
	codecName,
	defaultAudio,
	defaultSubtitle,
	episodeCode,
	langName,
	search,
	searchIndex,
	searchKey,
	shelves,
	subtitleOptions,
	videoName,
	whyUnplayable,
	type SubtitleOption
} from './picker';

const langs = (audio: string, ...subtitles: string[]): Languages => ({ audio, subtitles });

function audio(stream: number, lang: string, isDefault = false): AudioTrack {
	return { stream, codec: 'aac', channels: 2, lang, title: '', default: isDefault };
}

describe('defaultAudio', () => {
	const tracks = [audio(1, 'en', true), audio(2, 'tr'), audio(3, 'tr'), audio(4, 'de')];
	it.each([
		['preferred language found', tracks, langs('tr'), 2],
		['default-flagged wins within the language', [audio(1, 'tr'), audio(2, 'tr', true)], langs('tr'), 2],
		['preferred missing → default flag', tracks, langs('fr'), 1],
		['original → default flag', tracks, langs(''), 1],
		['no default flag → first', [audio(1, 'de'), audio(2, 'en')], langs(''), 1],
		['no default flag, preferred missing → first', [audio(1, 'de'), audio(2, 'en')], langs('fr'), 1]
	])('%s', (_, tracks, prefs, want) => {
		expect(defaultAudio(tracks, prefs)?.stream).toBe(want);
	});

	it('returns null with no tracks', () => {
		expect(defaultAudio([], langs('tr'))).toBeNull();
	});
});

let nextKey = 0;
function sub(lang: string, flags: Partial<SubtitleOption> = {}): SubtitleOption {
	const key = `k${nextKey++}`;
	return {
		key,
		choice: { stream: nextKey },
		lang,
		title: '',
		default: false,
		forced: false,
		sdh: false,
		unavailable: '',
		...flags
	};
}

describe('defaultSubtitle', () => {
	const enForced = sub('en', { forced: true });
	const en = sub('en');
	const enSDH = sub('en', { sdh: true });
	const tr = sub('tr');
	const trDefault = sub('tr', { default: true });
	const trImage = sub('tr', { unavailable: 'image' });
	const de = sub('de', { default: true });

	it.each([
		['first list language wins', [en, tr], 'ja', langs('', 'tr', 'en'), tr],
		['list order, not track order', [tr, en], 'ja', langs('', 'en', 'tr'), en],
		['next language when the first has none', [en, de], 'ja', langs('', 'tr', 'en'), en],
		['full track even in the audio language', [enForced, en], 'en', langs('', 'en'), en],
		['plain before SDH', [enSDH, en], 'ja', langs('', 'en'), en],
		['SDH when it is all there is', [enSDH, de], 'ja', langs('', 'en'), enSDH],
		['default-flagged first within a language', [tr, trDefault], 'ja', langs('', 'tr'), trDefault],
		['forced tracks are not full tracks', [enForced, de], 'ja', langs('', 'en'), de],
		['nothing in the list → forced in audio language', [enForced, de], 'en', langs('', 'tr'), enForced],
		['nothing in the list → default flag', [en, de], 'ja', langs('', 'tr'), de],
		['empty list → default flag', [en, de], 'ja', langs(''), de],
		['unavailable never picked', [trImage, en], 'ja', langs('', 'tr', 'en'), en],
		['unavailable default not picked', [sub('de', { default: true, unavailable: 'image' })], 'ja', langs(''), null],
		['nothing fits → off', [en], 'ja', langs('', 'tr'), null],
		['no audio language → no forced match', [sub('', { forced: true })], '', langs(''), null],
		['no subtitles → off', [], 'en', langs('', 'en'), null]
	])('%s', (_, options, audioLang, prefs, want) => {
		expect(defaultSubtitle(options, audioLang, prefs)).toBe(want);
	});
});

const summary = (v: Partial<VideoSummary>): VideoSummary => ({
	id: 1,
	type: 'movies',
	title: '',
	year: 0,
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
	...v
});

describe('subtitleOptions', () => {
	it('lists embedded tracks, then sidecars', () => {
		const v: VideoDetail = {
			...summary({}),
			audio: [],
			subtitles: [
				{ stream: 3, lang: 'en', title: 'Signs', default: true, forced: true, sdh: false, unavailable: '' },
				{ stream: 4, lang: 'tr', title: '', default: false, forced: false, sdh: false, unavailable: 'image' }
			],
			sidecars: [{ id: 9, lang: 'tr', forced: false, sdh: true }]
		};
		expect(subtitleOptions(v)).toEqual([
			{ key: 's3', choice: { stream: 3 }, lang: 'en', title: 'Signs', default: true, forced: true, sdh: false, unavailable: '' },
			{ key: 's4', choice: { stream: 4 }, lang: 'tr', title: '', default: false, forced: false, sdh: false, unavailable: 'image' },
			{ key: 'f9', choice: { sidecar: 9 }, lang: 'tr', title: '', default: false, forced: false, sdh: true, unavailable: '' }
		]);
	});
});

describe('canPlay', () => {
	const browser = (type: string) =>
		type === 'video/mp4; codecs="avc1.640028"' ? 'probably' : type.includes('hvc1') ? 'maybe' : '';
	it.each([
		['avc1.640028', true],
		['hvc1.2.4.L120.90', true],
		['av01.0.08M.08', false],
		['', false]
	])('%s → %s', (codec, want) => {
		expect(canPlay(codec, browser)).toBe(want);
	});
});

describe('whyUnplayable', () => {
	const browser = (type: string) => (type.includes('avc1') ? 'probably' : '');
	it.each([
		['plays', summary({}), ''],
		['server reason', summary({ unplayable: 'codec', codecString: '' }), "Browsers can't play this codec."],
		['unknown server reason', summary({ unplayable: 'new-code' }), 'new-code'],
		['this device', summary({ codecString: 'hvc1.2.4.L120.90' }), "This device can't play HEVC."]
	])('%s', (_, v, want) => {
		expect(whyUnplayable(v, browser)).toBe(want);
	});
});

describe('codecName', () => {
	it.each([
		['avc1.640028', 'H.264'],
		['hvc1.2.4.L120.90', 'HEVC'],
		['av01.0.08M.08', 'AV1'],
		['vp09.00.10.08', 'VP9'],
		['xyz9.1', 'xyz9']
	])('%s → %s', (codec, want) => {
		expect(codecName(codec)).toBe(want);
	});
});

describe('search', () => {
	it.each([
		['Şehir', 'sehir'],
		['İstanbul', 'istanbul'],
		['Işık', 'isik'],
		['Amélie', 'amelie'],
		['DARK', 'dark']
	])('searchKey(%s) = %s', (s, want) => {
		expect(searchKey(s)).toBe(want);
	});

	const videos = [
		summary({ id: 1, title: 'Heat', year: 1995 }),
		summary({ id: 2, type: 'tv', title: 'Dark', season: 1, episode: 2, episodeEnd: 2, episodeTitle: 'Lies' }),
		summary({ id: 3, type: 'other', title: 'Day 1', group: 'Trips/Kapadokya' }),
		summary({ id: 4, title: 'Şahsiyet', year: 2018 })
	];
	it.each([
		['heat', [1]],
		['1995', [1]],
		['dark s01e02', [2]],
		['lies', [2]],
		['kapadokya day', [3]],
		['sahsiyet', [4]],
		['dark s01e03', []],
		['  ', []]
	])('%s → %j', (q, want) => {
		expect(search(searchIndex(videos), q).map((v) => v.id)).toEqual(want);
	});
});

describe('names', () => {
	it.each([
		[summary({ title: 'Heat', year: 1995, edition: "Director's Cut", version: '4K' }), "Heat (1995) · Director's Cut · 4K"],
		[summary({ type: 'tv', title: 'Dark', year: 2017, season: 1, episode: 2, episodeEnd: 3, episodeTitle: 'Lies' }), 'Dark (2017) · S01E02-E03 · Lies'],
		[summary({ type: 'tv', title: 'Dark', season: 0, episode: 1, episodeEnd: 1 }), 'Dark · S00E01'],
		[summary({ type: 'other', title: 'Day 1', group: 'Trips' }), 'Trips · Day 1'],
		[summary({ type: 'other', title: 'Day 1' }), 'Day 1']
	])('%#', (v, want) => {
		expect(videoName(v)).toBe(want);
	});

	it('episodeCode is empty outside TV', () => {
		expect(episodeCode(summary({ episode: 1 }))).toBe('');
	});

	it.each([
		['tr', 'Turkish'],
		['en', 'English'],
		['fil', 'Filipino'],
		['', 'Unknown language'],
		['x!', 'x!']
	])('langName(%s) = %s', (code, want) => {
		expect(langName(code)).toBe(want);
	});
});

describe('shelves', () => {
	const videos = [
		summary({ id: 1, title: 'Ronin', year: 1998 }),
		summary({ id: 2, title: 'heat', year: 1995 }),
		summary({ id: 3, type: 'tv', title: 'Dark', year: 2017, season: 1, episode: 2 }),
		summary({ id: 4, type: 'tv', title: 'Dark', year: 2017, season: 0, episode: 1 }),
		summary({ id: 5, type: 'tv', title: 'Dark', year: 2017, season: 1, episode: 1 }),
		summary({ id: 6, type: 'tv', title: 'Atlanta', year: 2016, season: 1, episode: 1 }),
		summary({ id: 7, type: 'tv', title: 'Dark', year: 2017, season: 2, episode: 1 }),
		summary({ id: 8, type: 'other', title: 'b', group: 'Trips' }),
		summary({ id: 9, type: 'other', title: 'a', group: '' }),
		summary({ id: 10, type: 'other', title: 'Day 10', group: 'Birthdays' }),
		summary({ id: 11, type: 'other', title: 'Day 2', group: 'Birthdays' })
	];

	it('groups and sorts A to Z', () => {
		const got = shelves(videos, 'title');
		expect(got.map((s) => s.type)).toEqual(['movies', 'tv', 'other']);
		const [movies, tv, other] = got;
		expect(movies.type === 'movies' && movies.movies.map((v) => v.id)).toEqual([2, 1]);
		if (tv.type !== 'tv') throw new Error('not tv');
		expect(tv.shows.map((s) => s.title)).toEqual(['Atlanta', 'Dark']);
		expect(tv.shows[1].seasons.map((s) => [s.number, s.episodes.map((e) => e.id)])).toEqual([
			[1, [5, 3]],
			[2, [7]],
			[0, [4]]
		]);
		if (other.type !== 'other') throw new Error('not other');
		expect(other.folders.map((f) => [f.name, f.videos.map((v) => v.id)])).toEqual([
			['', [9]],
			['Birthdays', [11, 10]], // numeric: Day 2 before Day 10
			['Trips', [8]]
		]);
	});

	it('sorts recently added first', () => {
		const [movies, tv, other] = shelves(videos, 'recent');
		expect(movies.type === 'movies' && movies.movies.map((v) => v.id)).toEqual([2, 1]);
		if (tv.type !== 'tv') throw new Error('not tv');
		expect(tv.shows.map((s) => s.title)).toEqual(['Dark', 'Atlanta']); // Dark's newest is 7
		expect(tv.shows[0].seasons[0].episodes.map((e) => e.id)).toEqual([5, 3]); // still in order
		if (other.type !== 'other') throw new Error('not other');
		expect(other.folders.map((f) => f.name)).toEqual(['', 'Birthdays', 'Trips']);
		expect(other.folders[1].videos.map((v) => v.id)).toEqual([11, 10]);
	});

	it('keeps shows with the same name but different years apart', () => {
		const [tv] = shelves(
			[
				summary({ id: 1, type: 'tv', title: 'Doctor Who', year: 1963, season: 1, episode: 1 }),
				summary({ id: 2, type: 'tv', title: 'Doctor Who', year: 2005, season: 1, episode: 1 })
			],
			'title'
		);
		expect(tv.type === 'tv' && tv.shows.map((s) => s.year)).toEqual([1963, 2005]);
	});

	it('leaves out empty types', () => {
		expect(shelves([summary({ type: 'other', title: 'x' })], 'title').map((s) => s.type)).toEqual(['other']);
	});
});
