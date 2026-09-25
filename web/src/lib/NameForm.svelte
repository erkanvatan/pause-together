<script lang="ts">
	import { saveName } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let {
		initial = '',
		ondone,
		oncancel
	}: { initial?: string; ondone?: () => void; oncancel?: () => void } = $props();

	// svelte-ignore state_referenced_locally (only the starting value is wanted)
	let name = $state(initial);
	let error = $state('');
	let saving = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		const result = await saveName(name);
		saving = false;
		if (result === 'ok') {
			error = '';
			ondone?.();
		} else {
			error = result === 'invalid' ? strings.nameInvalid : strings.saveFailed;
		}
	}
</script>

<form onsubmit={submit} class="flex w-full max-w-xs flex-col gap-3">
	<label for="name" class="text-lg font-medium">{strings.namePrompt}</label>
	<!-- No maxlength: it counts UTF-16 units, not runes. The server decides. -->
	<!-- svelte-ignore a11y_autofocus -->
	<input
		id="name"
		bind:value={name}
		placeholder={strings.namePlaceholder}
		autocomplete="nickname"
		autofocus
		class="rounded-md border border-neutral-700 bg-neutral-900 px-3 py-2 text-neutral-100 outline-none focus:border-neutral-400"
	/>
	{#if error}
		<p class="text-sm text-red-400" role="alert">{error}</p>
	{/if}
	<div class="flex gap-2">
		<button
			type="submit"
			disabled={saving || name.trim() === ''}
			class="flex-1 rounded-md bg-neutral-100 px-3 py-2 font-medium text-neutral-950 disabled:opacity-40"
		>
			{strings.save}
		</button>
		{#if oncancel}
			<button
				type="button"
				onclick={oncancel}
				class="flex-1 rounded-md border border-neutral-700 px-3 py-2 text-neutral-300"
			>
				{strings.cancel}
			</button>
		{/if}
	</div>
</form>
