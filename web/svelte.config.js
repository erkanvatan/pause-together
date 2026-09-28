import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA mode: every unknown path gets index.html, and Go does the same.
		adapter: adapter({ fallback: 'index.html' }),
		// The build ID the server compares on each socket connect. Dev fixes it (compose.dev.yml), so it
		// matches Go's; a production build keeps SvelteKit's default, the build time.
		version: { name: process.env.BUILD_ID || undefined }
	}
};

export default config;
