<script lang="ts">
	import Picker from '$lib/Picker.svelte';
	import { strings } from '$lib/strings';

	let picking = $state(false);
</script>

<svelte:head>
	<title>{strings.appName}</title>
</svelte:head>

<main class="flex flex-col items-center justify-center gap-2 pt-24">
	<h1 class="text-4xl font-bold">{strings.appName}</h1>
	<p class="text-neutral-400">{strings.tagline}</p>
	<button
		onclick={() => (picking = true)}
		class="mt-6 rounded-md bg-neutral-100 px-4 py-2 font-medium text-neutral-950"
	>
		{strings.watchSomething}
	</button>
</main>

<!-- Rooms come next: for now a pick just closes the picker. -->
{#if picking}
	<Picker onpick={() => (picking = false)} onclose={() => (picking = false)} />
{/if}
