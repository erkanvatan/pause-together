// Subtitles for our own overlay: WebVTT cues, their text made safe, and which ones show when. Pure: no
// fetch, no DOM.
import type { Sidecar, SubtitleChoice } from '$lib/api';
import type { Prepare } from '$lib/protocol';

// A run of text in one style. Only italic and bold survive; the page draws them as <i> and <b>.
export type Span = { text: string; i: boolean; b: boolean };
export type Line = Span[];
export type Cue = { startMs: number; endMs: number; lines: Line[] };

const entities: Record<string, string> = {
	'&amp;': '&',
	'&lt;': '<',
	'&gt;': '>',
	'&nbsp;': ' ',
	'&lrm;': '‎',
	'&rlm;': '‏'
};

// A tag: <i>, </b>, <c.yellow>, <v Bob>, <00:00:01.000>. A '<' not followed by a name is plain text.
const tagPattern = /<(\/?)([A-Za-z0-9][^\s<>]*)[^<>]*>/g;

// sanitizeCue turns a cue's text into lines of styled spans. <i> and <b> are kept; every other tag and
// ASS override blocks like {\an8} are dropped, keeping their text. Entities are decoded after the tags
// are taken out, so &lt;i&gt; stays text. Lines left empty are dropped.
export function sanitizeCue(text: string): Line[] {
	let i = 0; // open <i> tags
	let b = 0;
	const lines: Line[] = [];
	for (const raw of text.replace(/\{\\[^}]*\}/g, '').split('\n')) {
		const line: Line = [];
		const add = (s: string) => {
			const t = s.replace(/&(?:amp|lt|gt|nbsp|lrm|rlm);/g, (e) => entities[e]);
			if (!t) return;
			const last = line.at(-1);
			if (last && last.i === i > 0 && last.b === b > 0) last.text += t;
			else line.push({ text: t, i: i > 0, b: b > 0 });
		};
		let at = 0;
		for (const m of raw.matchAll(tagPattern)) {
			add(raw.slice(at, m.index));
			at = m.index + m[0].length;
			const step = m[1] ? -1 : 1;
			const name = m[2].split('.')[0].toLowerCase();
			if (name === 'i') i = Math.max(0, i + step);
			else if (name === 'b') b = Math.max(0, b + step);
		}
		add(raw.slice(at));
		trim(line);
		if (line.length > 0) lines.push(line);
	}
	return lines;
}

// trim takes the white space off a line's two ends, and the spans that leaves empty.
function trim(line: Line) {
	while (line.length > 0 && !(line[0].text = line[0].text.trimStart())) line.shift();
	while (line.length > 0 && !(line[line.length - 1].text = line[line.length - 1].text.trimEnd())) line.pop();
}

// A cue's timing line: start --> end, then settings we ignore. Hours are optional.
const timingPattern = /^((?:\d+:)?\d{2}:\d{2}\.\d{3})\s+-->\s+((?:\d+:)?\d{2}:\d{2}\.\d{3})(?:\s|$)/;

function ms(t: string): number {
	const [clock, frac] = t.split('.');
	const secs = clock.split(':').reduce((sum, n) => sum * 60 + Number(n), 0);
	return secs * 1000 + Number(frac);
}

// parseVtt reads a WebVTT file's cues, in file order. Blocks without a timing line (the header, NOTE,
// STYLE, REGION) and cues with no text left are skipped.
export function parseVtt(text: string): Cue[] {
	const cues: Cue[] = [];
	for (const block of text.replace(/\r\n?/g, '\n').split(/\n{2,}/)) {
		const lines = block.split('\n');
		// The timing line is the first, or the second after a cue identifier.
		const at = lines.findIndex((l) => l.includes('-->'));
		if (at < 0 || at > 1) continue;
		const m = timingPattern.exec(lines[at]);
		if (!m) continue;
		const body = sanitizeCue(lines.slice(at + 1).join('\n'));
		if (body.length > 0) cues.push({ startMs: ms(m[1]), endMs: ms(m[2]), lines: body });
	}
	return cues;
}

// cuesAt returns the cues showing at video time videoMs. A positive offset shows every cue later.
export function cuesAt(cues: Cue[], videoMs: number, offsetMs: number): Cue[] {
	const t = videoMs - offsetMs;
	return cues.filter((c) => c.startMs <= t && t < c.endMs);
}

// subtitleUrl is where the room's subtitle is served, or '' when there's none to show. An embedded
// track is in the prepared copy's folder, if that run kept it; a sidecar has its own copy.
export function subtitleUrl(
	choice: SubtitleChoice | null,
	prepare: Prepare | null,
	sidecars: Sidecar[]
): string {
	if (choice === null) return '';
	if ('sidecar' in choice) {
		const s = sidecars.find((s) => s.id === choice.sidecar);
		return s ? `/stream/${s.key}/subtitle.vtt` : '';
	}
	if (prepare?.state !== 'ready' || !prepare.subtitles.includes(choice.stream)) return '';
	return `/stream/${prepare.key}/${choice.stream}.vtt`;
}

// sameSubtitle says whether two subtitle choices name the same track (null: off).
export function sameSubtitle(a: SubtitleChoice | null, b: SubtitleChoice | null): boolean {
	if (a === null || b === null) return a === b;
	if ('stream' in a) return 'stream' in b && a.stream === b.stream;
	return 'sidecar' in b && a.sidecar === b.sidecar;
}
