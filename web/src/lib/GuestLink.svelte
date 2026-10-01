<script lang="ts">
	// The address the host gives guests: the guest port's IP, never the localhost the host browses on.
	// Host only: /api/me sends it on the admin port.
	import { onDestroy } from 'svelte';
	import { me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	// How long "Copied" shows.
	const copiedMs = 2000;

	let { path = '' }: { path?: string } = $props(); // the page to open, like /rooms/15

	const url = $derived(me.guestUrl ? me.guestUrl + path : '');
	// The host browses on localhost, a secure context, so the clipboard works there.
	const canCopy = !!navigator.clipboard;
	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout>;

	async function copy() {
		try {
			await navigator.clipboard.writeText(url);
		} catch {
			return; // the text stays selectable
		}
		copied = true;
		clearTimeout(timer);
		timer = setTimeout(() => (copied = false), copiedMs);
	}

	onDestroy(() => clearTimeout(timer));
</script>

{#if url}
	<p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
		<span class="text-haze">{strings.guestLink}</span>
		<span class="break-all select-all">{url}</span>
		{#if canCopy}
			<button onclick={copy} class="btn btn-quiet btn-small" aria-live="polite">
				{copied ? strings.copied : strings.copy}
			</button>
		{/if}
	</p>
{/if}
