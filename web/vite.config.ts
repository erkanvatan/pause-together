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
		// Regex keys match only what Go routes: /api (without a slash) or /streams stay SPA pages, as in prod.
		proxy: {
			'^/api/': api,
			'^/stream/': api,
			'^/ws(\\?|$)': { ...api, ws: true }
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
