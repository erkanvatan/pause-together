// Chat helpers, pure: message length, and keeping the room's list of messages.
import type { ChatMessage } from '$lib/api';

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
