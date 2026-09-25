// Who this visitor is. The server decides: the token lives in an HttpOnly cookie the page can't read.

export const me = $state({
	loaded: false,
	name: null as string | null, // null until the visitor picks a name
	isAdmin: false
});

type MeResponse = { name: string | null; isAdmin: boolean };

function apply(r: MeResponse) {
	me.name = r.name;
	me.isAdmin = r.isAdmin;
	me.loaded = true;
}

// loadMe also refreshes the cookie, so it never expires while in use.
export async function loadMe(): Promise<void> {
	const res = await fetch('/api/me');
	if (!res.ok) throw new Error(`GET /api/me: ${res.status}`);
	apply(await res.json());
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
		apply(await res.json());
		return 'ok';
	} catch {
		return 'failed'; // no connection, or a body that isn't JSON
	}
}
