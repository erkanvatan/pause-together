<script lang="ts">
	import Icon from '$lib/Icon.svelte';
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

<form onsubmit={submit} class="flex w-full max-w-80 flex-col gap-4">
	<Icon name="pause" class="size-10 text-lamp" />
	<label for="name" class="font-display text-2xl font-bold text-balance">{strings.namePrompt}</label>
	<!-- No maxlength: it counts UTF-16 units, not runes. The server decides. -->
	<!-- svelte-ignore a11y_autofocus -->
	<input
		id="name"
		bind:value={name}
		placeholder={strings.namePlaceholder}
		autocomplete="nickname"
		autofocus
		class="field"
	/>
	{#if error}
		<p class="text-sm text-ember" role="alert">{error}</p>
	{/if}
	<div class="flex gap-2">
		<button type="submit" disabled={saving || name.trim() === ''} class="btn btn-primary flex-1">
			{strings.save}
		</button>
		{#if oncancel}
			<button type="button" onclick={oncancel} class="btn btn-quiet flex-1">
				{strings.cancel}
			</button>
		{/if}
	</div>
</form>
