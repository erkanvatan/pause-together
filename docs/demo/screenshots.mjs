// Takes the README's screenshots of the pages around the player: the welcome, the homepage, the
// picker and the admin page. Run through `task screenshots`, in Playwright's image, sharing the
// network of a throwaway app container (library.sh's library). Writes PNGs into OUT, for the task to
// turn into docs/screenshots/*.webp.
import { chromium } from 'playwright';
import { ADMIN, GUEST, api, prepared, until } from './app.mjs';

const OUT = process.env.OUT;

// The library, and two rooms: Alice and Sam in "Movie night", and an episode nobody is in.
for (const [path, type] of [['Movies', 'movies'], ['TV', 'tv'], ['Home Videos', 'other']]) {
	await api('POST', '/api/admin/libraries', { path, type });
}
const videos = await until('the scan', async () => {
	const v = await api('GET', '/api/videos');
	return v.length === 13 && v;
});
async function room(video) {
	const { audio } = await api('GET', `/api/videos/${video.id}`);
	const r = await api('POST', '/api/rooms', { videoId: video.id, audio: audio[0].stream });
	await api('GET', `/api/rooms/${r.id}`); // queues its prepare
	return r;
}
const night = await room(videos.find((v) => v.title === 'Sintel'));
await api('PUT', `/api/rooms/${night.id}/name`, { name: 'Movie night' });
const episode = await room(videos.find((v) => v.episodeTitle === 'Gran Dillama'));
await prepared();

const browser = await chromium.launch();
const desktop = { viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 };

async function person(name, base = GUEST) {
	const ctx = await browser.newContext(desktop);
	if (name) await ctx.request.post(`${base}/api/me`, { data: { name } });
	return ctx.newPage();
}

// Joins the room and moves it to ms.
async function seek(page, id, ms) {
	await page.goto(`${GUEST}/rooms/${id}`);
	await page.getByRole('button', { name: 'Click to join', exact: true }).click();
	await page.getByRole('slider', { name: 'Position' }).fill(String(ms));
	await page.waitForTimeout(1000);
}

const alice = await person('Alice');
await seek(alice, episode.id, 74_000);
await seek(alice, night.id, 372_000);
const sam = await person('Sam');
await sam.goto(`${GUEST}/rooms/${night.id}`);

async function shot(page, file, fullPage = false) {
	await page.waitForTimeout(500);
	await page.screenshot({ path: `${OUT}/${file}.png`, fullPage });
}

// A room link opened for the first time.
const guest = await person(null);
await guest.goto(`${GUEST}/rooms/${night.id}`);
await guest.getByRole('textbox').waitFor();
await shot(guest, 'welcome');

// Two days on, so the room nobody is in says when it was used.
const dad = await person('Dad');
await dad.clock.setSystemTime(Date.now() + 2 * 24 * 3600 * 1000);
await dad.goto(GUEST);
await dad.getByText('Movie night').waitFor();
await shot(dad, 'home');

await dad.getByRole('button', { name: 'Watch something' }).click();
await dad.getByText('Sintel').first().waitFor();
await shot(dad, 'picker');

const host = await person('Dad', ADMIN);
await host.goto(`${ADMIN}/admin`);
await host.getByText('Night of the Living Dead').first().waitFor();
await shot(host, 'admin', true); // it's long

await browser.close();
