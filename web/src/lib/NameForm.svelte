<script lang="ts">
	// The name form: a first visit's welcome, or a rename from the menu (oncancel set).
	import { onDestroy, onMount, tick } from 'svelte';
	import { listRooms } from '$lib/api';
	import Brand from '$lib/Brand.svelte';
	import Icon from '$lib/Icon.svelte';
	import { joinPerson, saveName, type SaveResult } from '$lib/me.svelte';
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
	// Who has the name, until "If that's you, go on" is answered.
	let taken = $state<{ id: number; name: string } | null>(null);
	// The answers stay off this long after the question shows, so a double tap or a second Enter on
	// Continue can't answer it, or wave it away, unread.
	const READ_MS = 500;
	let reading = $state(false);
	let readTimer: ReturnType<typeof setTimeout> | undefined;
	onDestroy(() => clearTimeout(readTimer));
	let input = $state<HTMLInputElement>();
	let pickButton = $state<HTMLButtonElement>();

	async function save(send: () => Promise<SaveResult>) {
		saving = true;
		const result = await send();
		saving = false;
		if (result === 'ok') {
			error = '';
			ondone?.();
		} else if (typeof result === 'object') {
			error = '';
			taken = result.taken;
			reading = true;
			clearTimeout(readTimer);
			// Then "Pick another name" takes the focus (a button that's off can't): the next Enter lands on
			// the safe answer. Not the field, or a phone's keyboard would cover the question.
			readTimer = setTimeout(async () => {
				reading = false;
				await tick();
				pickButton?.focus();
			}, READ_MS);
		} else {
			// A failed join keeps its question, to try again.
			error = result === 'invalid' ? strings.nameInvalid : strings.saveFailed;
		}
	}

	// Enter in the field while the question is up points at its answers, rather than asking it again.
	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (taken) pickButton?.focus();
		else save(() => saveName(name));
	}

	function join() {
		const id = taken?.id;
		if (id !== undefined) save(() => joinPerson(id));
	}

	// Typing or "Pick another name" drops the question.
	function clearTaken() {
		taken = null;
		error = '';
	}

	function pickAnother() {
		clearTaken();
		input?.focus();
		input?.select();
	}
</script>

<form onsubmit={submit} class="flex w-full max-w-80 flex-col gap-4">
	{#if welcome}
		<div class="mb-4 flex flex-col gap-1">
			<p class="flex items-center gap-1.5">
				<Brand />
			</p>
			<!-- The tagline is the brand's voice; the line under it says plainly what the app is. -->
			<p class="text-lg">{strings.tagline}</p>
			<p class="mt-1 text-haze">{strings.about}</p>
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
		bind:this={input}
		bind:value={name}
		oninput={() => taken && clearTaken()}
		readonly={saving}
		placeholder={strings.namePlaceholder}
		autocomplete="nickname"
		autofocus
		class="field"
	/>
	{#if taken}
		<div class="flex flex-col gap-3">
			<p class="break-words" role="alert">
				{strings.nameTaken(taken.name)}
				<span class="text-haze">{strings.nameTakenAsk}</span>
			</p>
			{#if error}
				<p class="text-sm text-ember" role="alert">{error}</p>
			{/if}
			<button
				type="button"
				onclick={join}
				disabled={saving || reading}
				class="btn btn-primary break-words py-2"
			>
				{strings.continueAs(taken.name)}
			</button>
			<button
				type="button"
				bind:this={pickButton}
				onclick={pickAnother}
				disabled={saving || reading}
				class="btn btn-quiet"
			>
				{strings.pickAnotherName}
			</button>
			{#if oncancel}
				<button type="button" onclick={oncancel} disabled={saving} class="btn btn-quiet">
					{strings.cancel}
				</button>
			{/if}
		</div>
	{:else}
		{#if error}
			<p class="text-sm text-ember" role="alert">{error}</p>
		{:else if welcome}
			<p class="-mt-2 text-sm text-haze">{strings.nameWhy}</p>
		{/if}
		<div class="flex gap-2">
			<!-- A rename's Save stays off until the name changes: a form at rest shows no lit lamp. -->
			<button
				type="submit"
				disabled={saving || name.trim() === '' || (!welcome && name.trim() === initial)}
				class="btn btn-primary flex-1"
			>
				{welcome ? strings.continue : strings.save}
			</button>
			{#if oncancel}
				<button type="button" onclick={oncancel} disabled={saving} class="btn btn-quiet flex-1">
					{strings.cancel}
				</button>
			{/if}
		</div>
	{/if}
</form>
