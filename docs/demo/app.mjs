// Talks to the throwaway app that `task demo` and `task screenshots` start. The scripts share its
// network, so its ports are localhost here.

export const GUEST = 'http://localhost:8080';
export const ADMIN = 'http://localhost:8081';

export async function api(method, path, body) {
	const res = await fetch(ADMIN + path, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: body && JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`);
	return res.json();
}

export async function until(what, check) {
	for (let i = 0; i < 300; i++) {
		const v = await check();
		if (v) return v;
		await new Promise((r) => setTimeout(r, 1000));
	}
	throw new Error(`timed out waiting for ${what}`);
}

// Waits until no prepare job is left and something is in the cache.
export const prepared = () =>
	until('the prepare', async () => {
		const c = await api('GET', '/api/admin/jobs');
		return c.jobs.length === 0 && c.cacheBytes > 0;
	});
