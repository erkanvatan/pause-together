<script lang="ts">
	import { strings } from '$lib/strings';
</script>

<svelte:head>
	<title>{strings.appName}</title>
</svelte:head>

<main class="flex min-h-screen flex-col items-center justify-center gap-2 bg-neutral-950 text-neutral-100">
	<h1 class="text-4xl font-bold">{strings.appName}</h1>
	<p class="text-neutral-400">{strings.tagline}</p>
</main>
