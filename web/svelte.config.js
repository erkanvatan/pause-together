import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA mode: every unknown path gets index.html, and Go does the same.
		adapter: adapter({ fallback: 'index.html' })
	}
};

export default config;
