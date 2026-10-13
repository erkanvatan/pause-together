<script lang="ts">
	// Picking a name: the field, "Someone here is already called …" when it's taken, and the buttons.
	// The welcome and the rename dialog each put their own words around it.
	import { onDestroy, tick } from 'svelte';
	import { joinPerson, saveName, type SaveResult } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let {
		label,
		action,
		hint = '',
		initial = '',
		ondone,
		oncancel
	}: {
		label: string; // the question, as the field's label
		action: string; // the submit button
		hint?: string; // under the field, until an error takes its place
		initial?: string; // a rename's current name: Save stays off until it changes
		ondone?: () => void;
		oncancel?: () => void;
	} = $props();

	// svelte-ignore state_referenced_locally (only the starting value is wanted)
	let name = $state(initial);
	const trimmed = $derived(name.trim());
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

<form onsubmit={submit} class="flex w-full flex-col gap-4">
	<label for="name" class="font-display text-2xl font-bold text-balance">{label}</label>
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
		{:else if hint}
			<p class="-mt-2 text-sm text-haze">{hint}</p>
		{/if}
		<div class="flex gap-2">
			<!-- A rename's Save stays off until the name changes: a form at rest shows no lit lamp. -->
			<button
				type="submit"
				disabled={saving || trimmed === '' || trimmed === initial}
				class="btn btn-primary flex-1"
			>
				{action}
			</button>
			{#if oncancel}
				<button type="button" onclick={oncancel} disabled={saving} class="btn btn-quiet flex-1">
					{strings.cancel}
				</button>
			{/if}
		</div>
	{/if}
</form>
