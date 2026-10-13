// Records the README's demo: Alice on a laptop and Bob on a phone, in one room. Run through
// `task demo`, in Playwright's image, sharing the network of a throwaway app container: its ports
// are localhost here. Writes laptop.webm, phone.webm and times.env (where the story starts in each
// video, and how long it runs) into OUT, for webp.sh.
import { chromium } from 'playwright';
import { writeFileSync } from 'node:fs';
import { GUEST, api, pickName, prepared, until } from './app.mjs';

const OUT = process.env.OUT;

// The film, in a room, prepared.
await api('POST', '/api/admin/libraries', { path: 'Movies', type: 'movies' });
const [video] = await until('the scan', async () => {
	const videos = await api('GET', '/api/videos');
	return videos.length && videos;
});
const { audio } = await api('GET', `/api/videos/${video.id}`);
const room = await api('POST', '/api/rooms', { videoId: video.id, audio: audio[0].stream });
await api('GET', `/api/rooms/${room.id}`);
await prepared();

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
	await pickName(page.request, GUEST, name);
	await page.goto(`${GUEST}/rooms/${room.id}`);
	return { ctx, page, created, file };
}

const alice = await person('Alice', 'laptop', { viewport: { width: 1024, height: 640 } });
const bob = await person('Bob', 'phone', {
	viewport: { width: 360, height: 640 },
	isMobile: true,
	hasTouch: true,
	deviceScaleFactor: 1
});
const a = alice.page;
const b = bob.page;

// Moves the pointer there smoothly.
async function hover(locator) {
	const box = await locator.boundingBox();
	await a.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 20 });
	await a.waitForTimeout(150);
}
async function click(locator) {
	await hover(locator);
	await a.mouse.down();
	await a.mouse.up();
}
// Picks an option the way a viewer sees it: the pointer goes there, the value changes. A headless
// browser draws no open dropdown.
async function choose(locator, option) {
	await hover(locator);
	await locator.selectOption(option);
}
// Drags the seek bar's thumb to ms. The room seeks when the button lets go.
async function drag(ms) {
	await a.mouse.move(600, 420, { steps: 10 }); // brings the faded controls back
	await a.waitForTimeout(300);
	const bar = a.getByRole('slider', { name: 'Position' });
	const box = await bar.boundingBox();
	const max = Number(await bar.getAttribute('max'));
	const x = (v) => box.x + (box.width * v) / max;
	const y = box.y + box.height / 2;
	await a.mouse.move(x(Number(await bar.inputValue())), y, { steps: 20 });
	await a.mouse.down();
	await a.mouse.move(x(ms), y, { steps: 25 });
	await a.waitForTimeout(150);
	await a.mouse.up();
}
// Bob's phone locks: the page is hidden, as the browser says when a phone locks, under a lock
// screen so the demo shows it.
async function lock(locked) {
	await b.evaluate((locked) => {
		for (const [name, value] of [['hidden', locked], ['visibilityState', locked ? 'hidden' : 'visible']]) {
			Object.defineProperty(document, name, { configurable: true, get: () => value });
		}
		document.dispatchEvent(new Event('visibilitychange'));
		if (!locked) return document.getElementById('demo-lock').remove();
		const l = document.createElement('div');
		l.id = 'demo-lock';
		l.style.cssText = `position: fixed; inset: 0; z-index: 2147483646; display: flex; flex-direction: column;
			align-items: center; padding-top: 120px; gap: 6px; color: #fff; font-family: sans-serif;
			background: linear-gradient(#1c1838, #0b0916);`;
		l.innerHTML = `<svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="#fff" stroke-width="2">
			<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>
			<div style="font-size: 72px; font-weight: 300">21:04</div><div style="opacity: .8">Saturday, 4 October</div>`;
		document.body.append(l);
	}, locked);
}
const button = (page, name) => page.getByRole('button', { name, exact: true }).first();

// Setup, cut from the demo: both join, and the room moves to 3:47.
await button(a, 'Click to join').click();
await button(b, 'Tap to join').tap();
await a.getByRole('slider', { name: 'Position' }).fill('227000');
await a.mouse.move(980, 300);
await a.waitForTimeout(3000);

// The story. Alice presses play: both screens play.
const story = Date.now();
await a.waitForTimeout(1000);
await click(button(a, 'Play'));
await a.waitForTimeout(4000);

// She skips ahead: Bob's phone jumps too, and says who moved it.
await drag(248000);
await a.waitForTimeout(2500);

// Subtitles for everyone, a touch later; a bigger size on her screen only.
await click(button(a, 'Subtitles'));
await a.waitForTimeout(600);
await choose(a.getByRole('combobox', { name: 'Subtitles' }), { index: 1 });
await a.waitForTimeout(1500);
const later = a.getByRole('button', { name: /later/ });
await click(later);
await a.waitForTimeout(300);
await click(later);
await a.waitForTimeout(1200);
await choose(a.getByRole('combobox', { name: 'Size' }), 'large');
await a.waitForTimeout(1200);
await click(button(a, 'Subtitles'));
await a.mouse.move(980, 300, { steps: 15 });
await a.waitForTimeout(1500);

// Bob's phone locks. 3 s on, the room pauses and waits for him, and plays on when he's back.
await lock(true);
await a.waitForTimeout(6500);
await lock(false);
await a.waitForTimeout(2500);

// A message from the laptop lands on the phone.
await click(a.getByPlaceholder('Message'));
await a.keyboard.type('Uh oh. He has a plan', { delay: 55 });
await a.waitForTimeout(300);
await a.keyboard.press('Enter');
await a.mouse.move(980, 300, { steps: 15 });
await a.waitForTimeout(3000);
const end = Date.now();

let times = '';
for (const p of [alice, bob]) {
	await p.ctx.close(); // the video is written on close
	await p.page.video().saveAs(`${OUT}/${p.file}.webm`);
	times += `${p.file}=${(story - p.created) / 1000}\n`;
}
times += `duration=${(end - story) / 1000}\n`;
writeFileSync(`${OUT}/times.env`, times);
await browser.close();
