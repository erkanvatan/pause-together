import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

// Go's admin listener, so localhost:5173 is the host's view. No changeOrigin: it would rewrite Host
// to the Docker service name, and Go's admin Host check would reject every call. Keep these objects:
// Vite turns the string shorthand ('/api': url) into changeOrigin: true.
const api = { target: process.env.API_URL ?? 'http://localhost:8081', changeOrigin: false };

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		strictPort: true,
		// Regex keys match only what Go routes, as in prod: /api and /stream go to Go (it redirects them to
		// /api/ and /stream/), /streams stays an SPA page.
		proxy: {
			'^/api(/|$)': api,
			'^/stream(/|$)': api,
			'^/ws(\\?|$)': { ...api, ws: true }
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
