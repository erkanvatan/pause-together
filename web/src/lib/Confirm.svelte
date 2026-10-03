<script lang="ts">
	// An inline "Are you sure?" row: the question, a danger button that does it, and Cancel. Ember
	// belongs here, not on the quiet button that opened it.
	import { onMount } from 'svelte';
	import { strings } from '$lib/strings';

	let {
		message,
		action,
		onconfirm,
		oncancel,
		disabled = false,
		focus = false,
		class: cls = ''
	}: {
		message: string;
		action: string; // the danger button's label: the action again, never "Yes"
		onconfirm: () => void;
		oncancel: () => void;
		disabled?: boolean; // the action can't run now (offline)
		focus?: boolean; // Cancel takes focus: the button that opened this row is gone
		class?: string;
	} = $props();

	let cancel: HTMLButtonElement;
	onMount(() => {
		if (focus) cancel.focus();
	});
</script>

<div class="flex flex-wrap items-center gap-2 text-sm {cls}" role="alert">
	<span>{message}</span>
	<button onclick={onconfirm} {disabled} class="btn btn-danger btn-small">{action}</button>
	<button bind:this={cancel} onclick={oncancel} class="btn btn-quiet btn-small">{strings.cancel}</button>
</div>
