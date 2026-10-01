<script lang="ts">
	// The address the host gives guests: the guest port's IP, never the localhost the host browses on.
	// Host only: /api/me sends it on the admin port. Shown where me.guestUrl is set.
	import { onDestroy } from 'svelte';
	import { me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	// How long "Copied" or "Couldn't copy" shows.
	const noteMs = 2000;

	let { path = '' }: { path?: string } = $props(); // the page to open, like /rooms/15

	const url = $derived(me.guestUrl + path);
	// The host browses on localhost, a secure context, so the clipboard works there.
	const canCopy = !!navigator.clipboard;
	let note = $state(''); // after a press: "Copied", or "Couldn't copy" so an old clipboard isn't sent
	let timer: ReturnType<typeof setTimeout>;

	async function copy() {
		try {
			await navigator.clipboard.writeText(url);
			note = strings.copied;
		} catch {
			note = strings.copyFailed; // the text stays selectable
		}
		clearTimeout(timer);
		timer = setTimeout(() => (note = ''), noteMs);
	}

	onDestroy(() => clearTimeout(timer));
</script>

<p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
	<span class="text-haze">{strings.guestLink}</span>
	<span class="break-all select-all">{url}</span>
	{#if canCopy}
		<button onclick={copy} class="btn btn-quiet btn-small" aria-live="polite">
			{note || strings.copy}
		</button>
	{/if}
</p>
