// Chat helpers, pure: message length, name colors, and keeping the room's list of messages.
import type { ChatMessage, Who } from '$lib/api';

// MAX_MESSAGE_CHARS is the longest message, counted as the server does (MaxMessageRunes in
// internal/room/chat.go): in code points, after trimming.
export const MAX_MESSAGE_CHARS = 1000;
// CHAT_PAGE_SIZE is how many messages the server sends at once (ChatPageSize). A shorter page is the last.
export const CHAT_PAGE_SIZE = 100;
// A new message pops up over the video for TOAST_MS while the chat is closed. At most MAX_TOASTS show.
export const TOAST_MS = 5000;
export const MAX_TOASTS = 3;

// messageLength counts code points, like Go counts runes. An emoji is one; text.length says two.
export function messageLength(text: string): number {
	return [...text.trim()].length;
}

export function canSend(text: string): boolean {
	const n = messageLength(text);
	return n > 0 && n <= MAX_MESSAGE_CHARS;
}

// Full class names, so Tailwind finds them. Nine, so a room of ten never shares one. More would look alike.
const personColors = [
	'text-person-1',
	'text-person-2',
	'text-person-3',
	'text-person-4',
	'text-person-5',
	'text-person-6',
	'text-person-7',
	'text-person-8',
	'text-person-9'
];

// Names are colored first come first served, so nobody in a room shares a color until a tenth other
// person shows up. A person keeps theirs while the page is open; another screen, or a reload, may hand
// them out in another order. NameColors maps a user ID to its place in personColors.
export type NameColors = Map<number, number>;

// withPeople gives each person not colored yet the next color, in the order they show up: messages
// oldest first, each with whoever it quotes, then who's watching. You get none: yours is the lamp. It
// returns the same map when nobody is new.
export function withPeople(colors: NameColors, messages: ChatMessage[], watching: Who[], me: number): NameColors {
	let next = colors;
	const add = (userId: number) => {
		if (userId === me || next.has(userId)) return;
		if (next === colors) next = new Map(colors);
		next.set(userId, next.size % personColors.length);
	};
	for (const m of messages) {
		add(m.from.userId);
		if (m.replyTo) add(m.replyTo.from.userId);
	}
	for (const w of watching) add(w.userId);
	return next;
}

// nameColor is the text color of a person's name: the lamp for you, else the color withPeople gave them.
// Someone it hasn't met keeps the plain text color, so they never pass for whoever has the first one.
export function nameColor(userId: number, me: number, colors: NameColors): string {
	if (userId === me) return 'text-lamp';
	const at = colors.get(userId);
	return at === undefined ? '' : personColors[at];
}

// withMessage adds a new message at the end, unless the list has it.
export function withMessage(list: ChatMessage[], m: ChatMessage): ChatMessage[] {
	return list.some((o) => o.id === m.id) ? list : [...list, m];
}

// withOlder puts a page of older messages in front.
export function withOlder(list: ChatMessage[], page: ChatMessage[]): ChatMessage[] {
	const have = new Set(list.map((m) => m.id));
	return [...page.filter((m) => !have.has(m.id)), ...list];
}

// withDeleted takes a deleted message out, and turns the quotes of replies to it into "deleted".
export function withDeleted(list: ChatMessage[], id: number): ChatMessage[] {
	return list
		.filter((m) => m.id !== id)
		.map((m) =>
			m.replyTo?.id === id ? { ...m, replyTo: { ...m.replyTo, text: '', deleted: true } } : m
		);
}

// focusAfter is the message that takes focus when the focused one (id) leaves the list: the nearest
// newer one that stays, else the nearest older one. null: none stays.
export function focusAfter(before: ChatMessage[], now: ChatMessage[], id: number): number | null {
	const at = before.findIndex((m) => m.id === id);
	if (at < 0) return null;
	const stays = new Set(now.map((m) => m.id));
	const newer = before.slice(at + 1).find((m) => stays.has(m.id));
	const older = before.slice(0, at).findLast((m) => stays.has(m.id));
	return (newer ?? older)?.id ?? null;
}

// withToast adds a toast, keeping the newest MAX_TOASTS.
export function withToast(toasts: ChatMessage[], m: ChatMessage): ChatMessage[] {
	return [...toasts, m].slice(-MAX_TOASTS);
}

// withHistory takes the newest messages a (re)joined socket gets. Older ones loaded before stay when
// they join up with the history; otherwise messages sent in between would be missing, so the history
// replaces the list. keptOlder: the loaded older ones stayed.
export function withHistory(
	list: ChatMessage[],
	history: ChatMessage[]
): { list: ChatMessage[]; keptOlder: boolean } {
	const first = history[0];
	const joins = first !== undefined && list.some((m) => m.id >= first.id);
	const older = joins ? list.filter((m) => m.id < first.id) : [];
	return { list: [...older, ...history], keptOlder: older.length > 0 };
}
