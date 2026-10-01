<script lang="ts">
	// The name form: a first visit's welcome, or a rename from the menu (oncancel set).
	import { onMount } from 'svelte';
	import { listRooms } from '$lib/api';
	import Brand from '$lib/Brand.svelte';
	import Icon from '$lib/Icon.svelte';
	import { saveName } from '$lib/me.svelte';
	import { roomTitle } from '$lib/rooms';
	import { strings } from '$lib/strings';

	let {
		initial = '',
		roomId,
		ondone,
		oncancel
	}: {
		initial?: string;
		roomId?: number; // a first visit by a room's link: say which room
		ondone?: () => void;
		oncancel?: () => void;
	} = $props();

	const welcome = $derived(!oncancel);
	let room = $state(''); // the linked room's title, once known

	// The room list, not the room: opening a room would start preparing its video. Without it the form
	// still works, so a failure just leaves the line out.
	onMount(async () => {
		if (roomId === undefined) return;
		const r = await listRooms();
		const found = r.ok ? r.value.find((c) => c.id === roomId) : undefined;
		if (found) room = roomTitle(found);
	});

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
	{#if welcome}
		<div class="mb-4 flex flex-col gap-1">
			<p class="flex items-center gap-1.5">
				<Brand />
			</p>
			<p class="text-haze">{strings.tagline}</p>
		</div>
		{#if room}
			<p class="break-words">{strings.joining(room)}</p>
		{/if}
	{:else}
		<Icon name="pause" class="size-10 text-lamp" />
	{/if}
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
	{:else if welcome}
		<p class="-mt-2 text-sm text-haze">{strings.nameWhy}</p>
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
