// Records the README's demo: Alice on a laptop and Sam on a phone, in one room. Run through
// `task demo`, in Playwright's image, sharing the network of a throwaway app container: its ports
// are localhost here. Writes laptop.webm, phone.webm and times.env (where the story starts in each
// video, and how long it runs) into OUT, for webp.sh.
import { chromium } from 'playwright';
import { writeFileSync } from 'node:fs';

const GUEST = 'http://localhost:8080';
const ADMIN = 'http://localhost:8081';
const OUT = process.env.OUT;

async function api(method, path, body) {
	const res = await fetch(ADMIN + path, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: body && JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`);
	return res.json();
}

async function until(what, check) {
	for (let i = 0; i < 300; i++) {
		const v = await check();
		if (v) return v;
		await new Promise((r) => setTimeout(r, 1000));
	}
	throw new Error(`timed out waiting for ${what}`);
}

// The film, in a room, prepared.
await api('POST', '/api/admin/libraries', { path: 'Movies', type: 'movies' });
const [video] = await until('the scan', async () => {
	const videos = await api('GET', '/api/videos');
	return videos.length && videos;
});
const { audio } = await api('GET', `/api/videos/${video.id}`);
const room = await api('POST', '/api/rooms', { videoId: video.id, audio: audio[0].stream });
await api('GET', `/api/rooms/${room.id}`);
await until('the prepare', async () => {
	const c = await api('GET', '/api/admin/jobs');
	return c.jobs.length === 0 && c.cacheBytes > 0;
});

const browser = await chromium.launch();

// A visible mouse pointer and touch ripple: Playwright's video shows neither.
const pointer = (mouse) => {
	addEventListener('DOMContentLoaded', () => {
		const style = document.createElement('style');
		style.textContent = `
			#demo-cursor { position: fixed; z-index: 2147483647; pointer-events: none; width: 22px; height: 22px;
				left: -50px; top: -50px; }
			.demo-tap { position: fixed; z-index: 2147483647; pointer-events: none; width: 44px; height: 44px;
				margin: -22px 0 0 -22px; border-radius: 50%; background: rgba(255,255,255,.55);
				animation: demo-tap .5s ease-out forwards; }
			@keyframes demo-tap { to { transform: scale(1.6); opacity: 0; } }`;
		document.head.append(style);
		const c = document.createElement('div');
		c.id = 'demo-cursor';
		c.innerHTML = `<svg viewBox="0 0 22 22" width="22" height="22"><path d="M3 2 L3 18 L7.5 13.8 L10.5 20.5 L13.3 19.3
			L10.4 12.8 L16.5 12.8 Z" fill="#fff" stroke="#000" stroke-width="1.4" stroke-linejoin="round"/></svg>`;
		if (mouse) document.body.append(c);
		addEventListener('mousemove', (e) => { c.style.left = e.clientX - 3 + 'px'; c.style.top = e.clientY - 2 + 'px'; }, true);
		addEventListener('pointerdown', (e) => {
			if (e.pointerType !== 'touch') return;
			const t = document.createElement('div');
			t.className = 'demo-tap';
			t.style.left = e.clientX + 'px';
			t.style.top = e.clientY + 'px';
			document.body.append(t);
			setTimeout(() => t.remove(), 600);
		}, true);
	});
};

async function person(name, file, opts) {
	const ctx = await browser.newContext({ ...opts, recordVideo: { dir: `${OUT}/rec`, size: opts.viewport } });
	await ctx.addInitScript(pointer, !opts.hasTouch);
	const page = await ctx.newPage();
	const created = Date.now();
	await page.request.post(`${GUEST}/api/me`, { data: { name } });
	await page.goto(`${GUEST}/rooms/${room.id}`);
	return { ctx, page, created, file };
}

const alice = await person('Alice', 'laptop', { viewport: { width: 1024, height: 640 } });
const sam = await person('Sam', 'phone', {
	viewport: { width: 360, height: 640 },
	isMobile: true,
	hasTouch: true,
	deviceScaleFactor: 1
});
const a = alice.page;
const s = sam.page;

// Moves the pointer there smoothly, then clicks.
async function click(locator) {
	const b = await locator.boundingBox();
	await a.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 20 });
	await a.waitForTimeout(150);
	await a.mouse.down();
	await a.mouse.up();
}
const button = (page, name) => page.getByRole('button', { name, exact: true }).first();

// Setup, cut from the demo: both join, and the room moves to 3:53.
await button(a, 'Click to join').click();
await button(s, 'Tap to join').tap();
await a.getByRole('slider', { name: 'Position' }).fill('233000');
await a.mouse.move(980, 300);
await a.waitForTimeout(3000);

// The story.
const story = Date.now();
await a.waitForTimeout(1000);
await click(button(a, 'Play'));
await a.waitForTimeout(4500);
await s.touchscreen.tap(180, 250); // brings the phone's controls back
await a.waitForTimeout(500);
await button(s, 'Pause').tap();
await a.waitForTimeout(1500);
await click(a.getByPlaceholder('Message'));
await a.keyboard.type('Haha, look at his face', { delay: 55 });
await a.waitForTimeout(300);
await a.keyboard.press('Enter');
await a.mouse.move(980, 300, { steps: 15 });
await a.waitForTimeout(1800);
await button(s, 'Play').tap();
await a.waitForTimeout(3500);
const end = Date.now();

let times = '';
for (const p of [alice, sam]) {
	await p.ctx.close(); // the video is written on close
	await p.page.video().saveAs(`${OUT}/${p.file}.webm`);
	times += `${p.file}=${(story - p.created) / 1000}\n`;
}
times += `duration=${(end - story) / 1000}\n`;
writeFileSync(`${OUT}/times.env`, times);
await browser.close();
