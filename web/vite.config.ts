import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

// Go's admin listener, so localhost:5173 is the host's view. No changeOrigin: it would rewrite Host
// to the Docker service name, and Go's admin Host check would reject every call. Keep these objects:
// Vite turns the string shorthand ('/api': url) into changeOrigin: true.
const api = { target: process.env.API_URL ?? 'http://localhost:8081', changeOrigin: false };

// BrowserStack's tunnel serves the page as bs-local.com, which Go's admin Host check refuses, so a remote
// phone's room socket would never open. Only a page on bs-local.com itself passes as localhost: another
// site opening a socket to bs-local.com (it points at 127.0.0.1) still fails Go's Origin check.
const bsLocal = 'bs-local.com:5173';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		strictPort: true,
		// Regex keys match only what Go routes, as in prod: /api and /stream go to Go (it redirects them to
		// /api/ and /stream/), /streams stays an SPA page.
		proxy: {
			'^/api(/|$)': api,
			'^/stream(/|$)': api,
			'^/ws(\\?|$)': {
				...api,
				ws: true,
				configure: (proxy) =>
					proxy.on('proxyReqWs', (proxyReq, req) => {
						if (req.headers.host !== bsLocal || req.headers.origin !== `http://${bsLocal}`) return;
						proxyReq.setHeader('host', 'localhost:5173');
						proxyReq.setHeader('origin', 'http://localhost:5173');
					})
			}
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
