import { describe, expect, it } from 'vitest';
import type { Prepare } from './protocol';
import {
	cuesAt,
	parseVtt,
	sameSubtitle,
	sanitizeCue,
	subtitleUrl,
	type Cue,
	type Span
} from './subtitles';

const plain = (text: string): Span => ({ text, i: false, b: false });
const ital = (text: string): Span => ({ text, i: true, b: false });
const bold = (text: string): Span => ({ text, i: false, b: true });
const both = (text: string): Span => ({ text, i: true, b: true });

describe('sanitizeCue', () => {
	it.each([
		['plain text', 'Hello', [[plain('Hello')]]],
		['italic', '<i>Hi</i> there', [[ital('Hi'), plain(' there')]]],
		['bold', 'a <b>big</b> one', [[plain('a '), bold('big'), plain(' one')]]],
		['upper-case tags, as in SRT', '<I>Hi</I>', [[ital('Hi')]]],
		['a class on a kept tag', '<i.loud>Hi</i>', [[ital('Hi')]]],
		['nested', '<i>a <b>b</b></i>', [[ital('a '), both('b')]]],
		['never closed', '<i>open', [[ital('open')]]],
		['stray close', 'a</i>b', [[plain('ab')]]],
		['italic across two lines', '<i>one\ntwo</i>', [[ital('one')], [ital('two')]]],
		['font dropped', '<font color="#ffff00">x</font>', [[plain('x')]]],
		['class span dropped', '<c.yellow>x</c>', [[plain('x')]]],
		['voice dropped', '<v Bob>Hi</v>', [[plain('Hi')]]],
		['underline dropped', '<u>x</u>', [[plain('x')]]],
		['timestamp dropped', 'a <00:00:01.000>b', [[plain('a b')]]],
		['ASS position dropped', '{\\an8}Top', [[plain('Top')]]],
		['ASS italic dropped too', '{\\i1}x{\\i0}', [[plain('x')]]],
		['braces without a backslash stay', '{sic}', [[plain('{sic}')]]],
		['entities', 'Tom &amp; Jerry &lt;3', [[plain('Tom & Jerry <3')]]],
		['escaped tags stay text', '&lt;i&gt;no&lt;/i&gt;', [[plain('<i>no</i>')]]],
		['nbsp and direction marks', 'a&nbsp;b&lrm;&rlm;', [[plain('a b‎‏')]]],
		['unknown entity stays', 'a &copy; b', [[plain('a &copy; b')]]],
		['a lone < stays', 'a < b', [[plain('a < b')]]],
		['line ends trimmed', '{\\an8} Top ', [[plain('Top')]]],
		['trimmed past a blank styled span', '<i> </i> Hello <b> </b>', [[plain('Hello')]]],
		['empty after stripping', '{\\an8}<i></i>', []],
		['empty lines dropped', 'one\n{\\an8}\ntwo', [[plain('one')], [plain('two')]]],
		['nothing', '', []]
	])('%s', (_, text, want) => {
		expect(sanitizeCue(text)).toEqual(want);
	});
});

describe('parseVtt', () => {
	const vtt = [
		'WEBVTT',
		'',
		'STYLE',
		'::cue { color: red }',
		'',
		'NOTE a comment',
		'',
		'1',
		'00:00:01.000 --> 00:00:02.500 align:start position:10%',
		'<i>Hi</i>',
		'there',
		'',
		'01:00:00.000 --> 01:00:01.000',
		'Late',
		'',
		'00:03.000 --> 00:04.250',
		'Short form',
		'',
		'00:05.000 --> 00:06.000',
		'{\\an8}',
		'',
		'bad --> times',
		'Skipped',
		''
	];
	const want: Cue[] = [
		{ startMs: 1000, endMs: 2500, lines: [[ital('Hi')], [plain('there')]] },
		{ startMs: 3_600_000, endMs: 3_601_000, lines: [[plain('Late')]] },
		{ startMs: 3000, endMs: 4250, lines: [[plain('Short form')]] }
	];

	it('reads cues, skipping headers, notes, styles, empty and broken cues', () => {
		expect(parseVtt(vtt.join('\n'))).toEqual(want);
	});

	it('reads CRLF files', () => {
		expect(parseVtt(vtt.join('\r\n'))).toEqual(want);
	});

	it('reads nothing from junk', () => {
		expect(parseVtt('not a subtitle')).toEqual([]);
	});
});

describe('cuesAt', () => {
	const a: Cue = { startMs: 1000, endMs: 2000, lines: [[plain('a')]] };
	const b: Cue = { startMs: 1500, endMs: 3000, lines: [[plain('b')]] };
	it.each([
		['before any', 999, 0, []],
		['start is in', 1000, 0, [a]],
		['two at once', 1600, 0, [a, b]],
		['end is out', 2000, 0, [b]],
		['positive offset shows it later', 1600, 500, [a]],
		['negative offset shows it sooner', 1600, -1000, [b]],
		['after all', 3000, 0, []]
	])('%s', (_, videoMs, offsetMs, want) => {
		expect(cuesAt([a, b], videoMs, offsetMs)).toEqual(want);
	});
});

describe('subtitleUrl', () => {
	const key = '0123456789abcdef0123456789abcdef';
	const prepare = (over: Partial<Prepare>): Prepare => ({
		state: 'ready',
		place: 0,
		progress: 0,
		error: '',
		key,
		subtitles: [3],
		...over
	});
	const sidecars = [{ id: 7, lang: 'tr', forced: false, sdh: false, key: 'fedcba9876543210fedcba9876543210' }];
	it.each([
		['off', null, prepare({}), ''],
		['embedded, in the copy', { stream: 3 }, prepare({}), `/stream/${key}/3.vtt`],
		['embedded, not in the copy', { stream: 4 }, prepare({}), ''],
		['embedded, copy not ready', { stream: 3 }, prepare({ state: 'running', key: '', subtitles: [] }), ''],
		['embedded, no prepare yet', { stream: 3 }, null, ''],
		['sidecar', { sidecar: 7 }, null, '/stream/fedcba9876543210fedcba9876543210/subtitle.vtt'],
		['sidecar gone', { sidecar: 8 }, prepare({}), '']
	])('%s', (_, choice, p, want) => {
		expect(subtitleUrl(choice, p, sidecars)).toBe(want);
	});
});

describe('sameSubtitle', () => {
	it.each([
		['both off', null, null, true],
		['off and a track', null, { stream: 3 }, false],
		['same stream', { stream: 3 }, { stream: 3 }, true],
		['other stream', { stream: 3 }, { stream: 4 }, false],
		['same sidecar', { sidecar: 3 }, { sidecar: 3 }, true],
		['stream and sidecar with one number', { stream: 3 }, { sidecar: 3 }, false]
	])('%s', (_, a, b, want) => {
		expect(sameSubtitle(a, b)).toBe(want);
	});
});
