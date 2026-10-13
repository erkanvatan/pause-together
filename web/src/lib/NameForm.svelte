<script lang="ts">
	// Picking a name: the field, "Someone here is already called …" when it's taken, and the buttons.
	// The welcome and the rename dialog each put their own words around it.
	import { onDestroy, tick } from 'svelte';
	import { joinPerson, MAX_NAME, nameLength, saveName, type SaveResult } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let {
		label,
		heading = 'h2',
		action,
		hint = '',
		describedby = '',
		enterkeyhint = 'go',
		initial = '',
		ondone,
		oncancel
	}: {
		label: string; // the question, as the field's label
		heading?: 'h1' | 'h2'; // the label is the page's or the dialog's heading
		action: string; // the submit button
		hint?: string; // under the field, until an error takes its place
		describedby?: string; // ids of the lines around the form, read out with the field
		enterkeyhint?: 'go' | 'done'; // the label on a phone keyboard's Enter: go to a page, or done in a dialog
		initial?: string; // a rename's current name: Save stays off until it changes
		ondone?: () => void;
		oncancel?: () => void;
	} = $props();

	// svelte-ignore state_referenced_locally (only the starting value is wanted)
	let name = $state(initial);
	const trimmed = $derived(name.trim());
	// invalid: the name itself is wrong, not just the save. spoken: what a screen reader hears instead.
	type FormError = { text: string; spoken?: string; invalid: boolean };
	let error = $state<FormError | null>(null);
	// Too long says so while typing, before any save. The other rules wait for the server's answer.
	const over = $derived(nameLength(name) - MAX_NAME);
	const shown = $derived(
		over > 0
			? { text: strings.nameTooLong(over), spoken: strings.nameTooLongSpoken, invalid: true }
			: error
	);

	let saving = $state(false);
	// "Saving…" shows only on a slow answer, so a quick one never flashes it.
	const SLOW_MS = 300;
	let slow = $state(false);
	let slowTimer: ReturnType<typeof setTimeout> | undefined;
	let readTimer: ReturnType<typeof setTimeout> | undefined;
	onDestroy(() => {
		clearTimeout(readTimer);
		clearTimeout(slowTimer);
	});
	const id = $props.id();
	// Who has the name, until "If that's you, continue" is answered.
	let taken = $state<{ id: number; name: string } | null>(null);
	// The answers stay off this long after the question shows, so a double tap or a second Enter on
	// Continue can't answer it, or wave it away, unread.
	const READ_MS = 500;
	let reading = $state(false);
	let input = $state<HTMLInputElement>();
	let pickButton = $state<HTMLButtonElement>();

	async function save(send: () => Promise<SaveResult>) {
		saving = true;
		error = null; // a retry that fails the same way shows its error anew
		slowTimer = setTimeout(() => (slow = true), SLOW_MS);
		const result = await send();
		clearTimeout(slowTimer);
		saving = slow = false;
		if (result === 'ok') {
			error = null;
			ondone?.();
		} else if (typeof result === 'object' && 'taken' in result) {
			error = null;
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
		} else if (typeof result === 'object') {
			error = { text: strings.nameRefused(result.invalid.reason, result.invalid.symbol), invalid: true };
		} else {
			// A failed join keeps its question, to try again.
			error = { text: strings.saveFailed, invalid: false };
		}
	}

	// Enter in the field while the question is up points at its answers, rather than asking it again.
	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (taken) pickButton?.focus();
		else save(() => saveName(name));
	}

	function join() {
		const person = taken?.id;
		if (person !== undefined) save(() => joinPerson(person));
	}

	// Typing or "Pick another name" drops the question, and an error about the name before.
	function clearTaken() {
		taken = null;
		error = null;
	}

	function pickAnother() {
		clearTaken();
		input?.focus();
		input?.select();
	}
</script>

<form onsubmit={submit} class="flex w-full flex-col gap-4">
	<svelte:element this={heading} class="font-display text-2xl font-bold text-balance">
		<label for="{id}-name" class="block">{label}</label>
	</svelte:element>
	<!-- No maxlength: it counts UTF-16 units, not characters. nameLength counts as the server does. -->
	<!-- svelte-ignore a11y_autofocus -->
	<input
		id="{id}-name"
		bind:this={input}
		bind:value={name}
		oninput={clearTaken}
		readonly={saving}
		aria-describedby={[describedby, shown ? `${id}-error` : hint && !taken ? `${id}-hint` : '']
			.filter(Boolean)
			.join(' ') || undefined}
		aria-invalid={shown?.invalid || undefined}
		placeholder={strings.namePlaceholder}
		autocomplete="nickname"
		autocorrect="off"
		spellcheck="false"
		{enterkeyhint}
		autofocus
		class="field"
	/>
	{#if taken}
		<div class="flex flex-col gap-3">
			<p class="break-words" role="alert">
				{strings.nameTaken(taken.name)}
				<!-- A rename says what a move does: this device goes, the old name and its messages stay. -->
				<span class="text-haze"
					>{initial ? strings.renameTakenAsk(taken.name, initial) : strings.nameTakenAsk}</span
				>
			</p>
			{#if error}
				<p id="{id}-error" class="text-sm text-ember" role="alert">{error.text}</p>
			{/if}
			<!-- Both answers quiet: the lamp would push the one that makes this browser someone else. -->
			<button
				type="button"
				onclick={join}
				disabled={saving || reading}
				class="btn btn-quiet break-words py-2"
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
		{#if shown?.spoken}
			<!-- The count changes on every key, so it stays out of the alert: the alert's words never
			change while typing, and a screen reader reads it once. -->
			<p id="{id}-error" class="sr-only" role="alert">{shown.spoken}</p>
			<p class="text-sm break-words text-ember" aria-hidden="true">{shown.text}</p>
		{:else if shown}
			<p id="{id}-error" class="text-sm break-words text-ember" role="alert">{shown.text}</p>
		{:else if hint}
			<p id="{id}-hint" class="-mt-2 text-sm text-haze">{hint}</p>
		{/if}
		<div class="flex gap-2">
			<!-- A rename's Save stays off until the name changes: a form at rest shows no lit lamp. -->
			<button
				type="submit"
				disabled={saving || trimmed === '' || trimmed === initial || over > 0}
				class="btn btn-primary flex-1"
			>
				{slow ? strings.saving : action}
			</button>
			{#if oncancel}
				<button type="button" onclick={oncancel} disabled={saving} class="btn btn-quiet flex-1">
					{strings.cancel}
				</button>
			{/if}
		</div>
	{/if}
</form>
