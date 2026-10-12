// Who this visitor is. The server decides: the token lives in an HttpOnly cookie the page can't read.

export const me = $state({
	loaded: false,
	id: null as number | null, // the person's; a rename keeps it, a move to another person's name doesn't
	name: null as string | null, // null until the visitor picks a name
	isAdmin: false,
	guestUrl: '' // the address to give guests, host only; '' when they can't reach it yet
});

type MeResponse = { id: number | null; name: string | null; isAdmin: boolean; guestUrl?: string };

// A look (GET) takes a number as it starts; its answer counts unless a later one already did. A
// rename or log out is the browser's newest truth when it answers, so its answer takes the next number:
// a look still on its way can't undo it.
let started = 0;
let applied = 0;

function apply(call: number, r: MeResponse) {
	if (call < applied) return;
	applied = call;
	me.id = r.id;
	me.name = r.name;
	me.isAdmin = r.isAdmin;
	me.guestUrl = r.guestUrl ?? '';
	me.loaded = true;
}

// loadMe also refreshes the cookie, so it never expires while in use.
export async function loadMe(): Promise<void> {
	const call = ++started;
	const res = await fetch('/api/me');
	if (!res.ok) throw new Error(`GET /api/me: ${res.status}`);
	const r = await res.json();
	apply(call, r);
}

// logOut forgets this browser: the page asks for a name again. It reports whether this browser is
// logged out now; it never throws. When the answer is lost, the server may have done it all the same:
// ask it.
export async function logOut(): Promise<boolean> {
	try {
		const res = await fetch('/api/me', { method: 'DELETE' });
		if (res.ok) {
			const r = await res.json();
			apply(++started, r);
			return true;
		}
	} catch {
		// asked below
	}
	await loadMe().catch(() => {});
	return me.name === null;
}

// forgetMe is a log out on another page of this browser: the server said so on the room socket.
export function forgetMe() {
	applied = ++started;
	me.id = me.name = null;
}

export type SaveResult = 'ok' | 'invalid' | 'failed';

// saveName creates the user on first use and renames it after. It never throws.
export async function saveName(name: string): Promise<SaveResult> {
	try {
		const res = await fetch('/api/me', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name })
		});
		if (res.status === 400) return 'invalid';
		if (!res.ok) return 'failed';
		const r = await res.json();
		apply(++started, r);
		return 'ok';
	} catch {
		return 'failed'; // no connection, or a body that isn't JSON
	}
}
