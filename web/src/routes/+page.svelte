<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import Picker from '$lib/Picker.svelte';
	import { deleteRoom } from '$lib/admin';
	import { createRoom, listRooms, setArchived, type Pick, type Result, type RoomCard } from '$lib/api';
	import { me } from '$lib/me.svelte';
	import { videoName } from '$lib/picker';
	import { roomTitle, splitArchived } from '$lib/rooms';
	import { strings } from '$lib/strings';

	// How often the room list is asked again while the tab is visible: others make, archive and
	// delete rooms.
	const pollMs = 5000;

	let rooms = $state<RoomCard[] | null>(null); // null until the first load
	let loadFailed = $state(false);
	let actionError = $state(''); // why the last action failed
	let visible = $state(true);
	let picking = $state(false);
	let confirming = $state<number | null>(null); // room whose Delete waits for a yes

	const parts = $derived(splitArchived(rooms ?? []));

	let refreshSeq = 0; // only the latest ask may show its answer

	// refresh runs from the poll, a visibility change and after each action, so asks can overlap.
	async function refresh() {
		const seq = ++refreshSeq;
		const r = await listRooms();
		if (seq !== refreshSeq) return; // an older answer, landing after a newer ask
		loadFailed = !r.ok;
		if (r.ok) rooms = r.value;
	}

	onMount(refresh);

	function visibilityChanged() {
		visible = document.visibilityState === 'visible';
		if (visible) refresh();
	}

	// The next poll waits for the last answer, so polls can't pile up.
	$effect(() => {
		if (!visible) return;
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		const tick = async () => {
			await refresh();
			if (!stopped) timer = setTimeout(tick, pollMs);
		};
		timer = setTimeout(tick, pollMs);
		return () => {
			stopped = true;
			clearTimeout(timer);
		};
	});

	async function act(result: Promise<Result<unknown>>) {
		const r = await result;
		actionError = r.ok ? '' : (strings.roomErrors[r.error] ?? strings.actionFailed);
		await refresh();
	}

	async function create(p: Pick) {
		picking = false;
		const r = await createRoom(p);
		actionError = r.ok ? '' : strings.actionFailed;
		if (r.ok) goto(`/rooms/${r.value.id}`);
	}
</script>

<svelte:document onvisibilitychange={visibilityChanged} />

<svelte:head>
	<title>{strings.appName}</title>
</svelte:head>

{#snippet card(r: RoomCard)}
	<li class="flex flex-col gap-2 rounded-md border border-neutral-800 p-3">
		<div class="flex flex-wrap items-center justify-between gap-2">
			<a href="/rooms/{r.id}" class="min-w-0 flex-1 hover:underline">
				<span class="block font-medium break-words">{roomTitle(r)}</span>
				{#if r.name}
					<span class="block text-sm break-words text-neutral-400">{videoName(r.video)}</span>
				{/if}
				{#if r.video.missing}
					<span class="block text-sm text-amber-400">{strings.videoMissing}</span>
				{/if}
				{#if r.watching.length > 0}
					<span class="block text-sm break-words text-emerald-400">
						{strings.watchingList(r.watching.map((w) => w.name).join(', '))}
					</span>
				{/if}
			</a>
			<div class="flex gap-2 text-sm">
				<!-- Archive only an empty room. The server checks too: someone may just have joined. -->
				{#if r.archived || r.watching.length === 0}
					<button
						onclick={() => act(setArchived(r.id, !r.archived))}
						class="rounded-md border border-neutral-700 px-2.5 py-1 hover:bg-neutral-800"
					>
						{r.archived ? strings.unarchive : strings.archive}
					</button>
				{/if}
				{#if me.isAdmin}
					<button
						onclick={() => (confirming = r.id)}
						class="rounded-md border border-neutral-700 px-2.5 py-1 hover:bg-neutral-800"
					>
						{strings.deleteRoom}
					</button>
				{/if}
			</div>
		</div>
		{#if confirming === r.id}
			<div class="flex flex-wrap items-center gap-2 text-sm">
				<span class="text-neutral-300">{strings.deleteRoomConfirm}</span>
				<button
					onclick={() => {
						confirming = null;
						act(deleteRoom(r.id));
					}}
					class="rounded-md bg-red-600 px-2.5 py-1 font-medium text-white"
				>
					{strings.deleteRoom}
				</button>
				<button
					onclick={() => (confirming = null)}
					class="rounded-md border border-neutral-700 px-2.5 py-1"
				>
					{strings.cancel}
				</button>
			</div>
		{/if}
	</li>
{/snippet}

<main class="mx-auto flex max-w-3xl flex-col gap-10 p-4 pb-16">
	<div class="flex flex-col items-center gap-2 pt-12">
		<h1 class="text-4xl font-bold">{strings.appName}</h1>
		<p class="text-neutral-400">{strings.tagline}</p>
		<button
			onclick={() => (picking = true)}
			class="mt-6 rounded-md bg-neutral-100 px-4 py-2 font-medium text-neutral-950"
		>
			{strings.watchSomething}
		</button>
	</div>

	{#if page.state.roomDeleted}
		<p class="text-sm text-neutral-300" role="status">{strings.roomDeleted}</p>
	{/if}

	{#if loadFailed || actionError}
		<p class="text-sm text-red-400" role="alert">
			{loadFailed ? strings.loadFailed : actionError}
		</p>
	{/if}

	{#if rooms}
		<section class="flex flex-col gap-3">
			<h2 class="text-lg font-semibold">{strings.rooms}</h2>
			{#if parts.active.length === 0}
				<p class="text-neutral-400">{strings.noRooms}</p>
			{:else}
				<ul class="flex flex-col gap-2">
					{#each parts.active as r (r.id)}
						{@render card(r)}
					{/each}
				</ul>
			{/if}
		</section>

		{#if parts.archived.length > 0}
			<section class="flex flex-col gap-3">
				<h2 class="text-lg font-semibold">{strings.archivedRooms}</h2>
				<ul class="flex flex-col gap-2">
					{#each parts.archived as r (r.id)}
						{@render card(r)}
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</main>

{#if picking}
	<Picker onpick={create} onclose={() => (picking = false)} />
{/if}
