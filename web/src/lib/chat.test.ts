import { describe, expect, it } from 'vitest';
import type { ChatMessage, VideoSummary } from './api';
import {
	canSend,
	MAX_MESSAGE_CHARS,
	MAX_TOASTS,
	messageLength,
	withDeleted,
	withHistory,
	withMessage,
	withOlder,
	withToast
} from './chat';

const alice = { userId: 1, name: 'Alice' };
const video = { id: 7, title: 'Heat' } as VideoSummary;

function message(id: number, replyTo: number | null = null): ChatMessage {
	return {
		id,
		from: alice,
		text: `m${id}`,
		sentAt: 0,
		video,
		positionMs: 0,
		replyTo: replyTo === null ? null : { id: replyTo, from: alice, text: `m${replyTo}`, deleted: false }
	};
}

const ids = (list: ChatMessage[]) => list.map((m) => m.id);

describe('messageLength', () => {
	it.each([
		['', 0],
		['abc', 3],
		['şğü', 3],
		['😀', 1], // two UTF-16 units, one character: the server counts runes
		['👍🏽', 2] // thumbs up and a skin tone: two code points, like Go
	])('%s is %i', (text, n) => expect(messageLength(text)).toBe(n));
});

describe('canSend', () => {
	it.each([
		['hello', true],
		['', false],
		['  \n ', false],
		['a'.repeat(MAX_MESSAGE_CHARS), true],
		['😀'.repeat(MAX_MESSAGE_CHARS), true],
		['a'.repeat(MAX_MESSAGE_CHARS + 1), false],
		[`  ${'a'.repeat(MAX_MESSAGE_CHARS)}  `, true] // the server trims first
	])('%j → %s', (text, ok) => expect(canSend(text)).toBe(ok));
});

describe('withMessage', () => {
	it('appends', () => expect(ids(withMessage([message(1)], message(2)))).toEqual([1, 2]));
	it('skips one it has', () => expect(ids(withMessage([message(1)], message(1)))).toEqual([1]));
});

describe('withOlder', () => {
	it('puts the page first', () =>
		expect(ids(withOlder([message(3), message(4)], [message(1), message(2)]))).toEqual([1, 2, 3, 4]));
	it('skips ones it has', () =>
		expect(ids(withOlder([message(2), message(3)], [message(1), message(2)]))).toEqual([1, 2, 3]));
});

describe('withHistory', () => {
	const range = (a: number, b: number) => Array.from({ length: b - a + 1 }, (_, k) => message(a + k));
	it('keeps older pages loaded before a reconnect', () => {
		const got = withHistory(range(1, 150), range(60, 160));
		expect(ids(got.list)).toEqual(ids(range(1, 160)));
		expect(got.keptOlder).toBe(true);
	});
	it('replaces the list when the history leaves a gap', () => {
		const got = withHistory(range(1, 50), range(100, 199));
		expect(ids(got.list)).toEqual(ids(range(100, 199)));
		expect(got.keptOlder).toBe(false);
	});
	it('takes the history on the first join', () => {
		expect(withHistory([], range(1, 3))).toEqual({ list: range(1, 3), keptOlder: false });
	});
	it('takes an empty history as it is', () => {
		expect(withHistory(range(1, 3), [])).toEqual({ list: [], keptOlder: false });
	});
	it('drops loaded messages deleted while away', () => {
		expect(ids(withHistory(range(1, 5), [message(1), message(2), message(4)]).list)).toEqual([1, 2, 4]);
	});
});

describe('withDeleted', () => {
	it('drops the message and marks replies to it', () => {
		const list = withDeleted([message(1), message(2, 1), message(3, 2)], 1);
		expect(ids(list)).toEqual([2, 3]);
		expect(list[0].replyTo).toEqual({ id: 1, from: alice, text: '', deleted: true });
		expect(list[1].replyTo).toEqual(message(3, 2).replyTo);
	});
	it('marks replies to one not loaded', () => {
		const list = withDeleted([message(5, 1)], 1);
		expect(list[0].replyTo?.deleted).toBe(true);
	});
});

describe('withToast', () => {
	it('keeps the newest few', () => {
		let toasts: ChatMessage[] = [];
		for (let id = 1; id <= MAX_TOASTS + 2; id++) toasts = withToast(toasts, message(id));
		expect(ids(toasts)).toEqual(Array.from({ length: MAX_TOASTS }, (_, k) => k + 3));
	});
});
