<script lang="ts">
	// A button that opens a small panel of rows under it. The panel closes on Escape, giving focus back
	// to the button, and on a press anywhere outside it. Items get close(), to call when one is picked.
	import type { Snippet } from 'svelte';

	let {
		label,
		buttonClass,
		button,
		items
	}: {
		label?: string; // the button's name when it shows only an icon
		buttonClass: string;
		button: Snippet;
		items: Snippet<[close: () => void]>;
	} = $props();

	let open = $state(false);
	let root: HTMLDivElement;
	let trigger: HTMLButtonElement;

	const close = () => (open = false);

	function key(e: KeyboardEvent) {
		if (!open || e.key !== 'Escape') return;
		open = false;
		trigger.focus();
	}

	function outside(e: PointerEvent) {
		if (open && !root.contains(e.target as Node)) open = false;
	}

	// Tab past the last row closes it too. Only when focus went somewhere: Safari doesn't focus a
	// clicked button, so a click on a row would close the panel before the click lands.
	function focusOut(e: FocusEvent) {
		const to = e.relatedTarget as Node | null;
		if (to && !root.contains(to)) open = false;
	}

	// The arrows stay in the menu: on the room page they'd skip the whole room's video.
	function arrows(e: KeyboardEvent) {
		if (open && e.key.startsWith('Arrow')) e.stopPropagation();
	}
</script>

<svelte:window onkeydown={key} onpointerdown={outside} />

<div bind:this={root} onfocusout={focusOut} onkeydown={arrows} role="none" class="relative min-w-0">
	<button
		bind:this={trigger}
		onclick={() => (open = !open)}
		aria-expanded={open}
		aria-label={label}
		title={label}
		class={buttonClass}
	>
		{@render button()}
	</button>
	{#if open}
		<div class="absolute right-0 z-20 mt-1 min-w-48 rounded-control border border-line bg-dusk p-1">
			{@render items(close)}
		</div>
	{/if}
</div>
