<script lang="ts">
	// A modal: the browser's own <dialog>, so focus moves in and stays there, the page behind is inert to
	// keys and screen readers, and Escape closes it. It fills the window with the midnight wash; class
	// lays out the panel in it. Open while mounted; leaving gives focus back to where it was.
	import { onMount, type Snippet } from 'svelte';

	let {
		label,
		onclose,
		class: cls = '',
		children
	}: {
		label: string;
		onclose: () => void; // Escape, or the browser closed it
		class?: string;
		children: Snippet;
	} = $props();

	let dialog: HTMLDialogElement;
	let leaving = false;

	onMount(() => {
		const before = document.activeElement;
		dialog.showModal();
		return () => {
			leaving = true;
			dialog.close();
			// The browser gives focus back only to a dialog still on the page; this one is leaving it.
			if (before instanceof HTMLElement && before.isConnected) before.focus();
		};
	});
</script>

<dialog
	bind:this={dialog}
	aria-label={label}
	oncancel={(e) => {
		e.preventDefault();
		onclose();
	}}
	onclose={() => leaving || onclose()}
	class="fixed inset-0 m-0 size-full max-h-none max-w-none border-0 bg-midnight/80 p-0 text-moonlight backdrop:bg-transparent"
>
	<!-- The layout goes on a box inside: a display on the <dialog> itself would show it while closed. -->
	<div class="flex size-full {cls}">
		{@render children()}
	</div>
</dialog>
