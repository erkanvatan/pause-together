<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import Icon from '$lib/Icon.svelte';
	import Picker from '$lib/Picker.svelte';
	import { deleteRoom } from '$lib/admin';
	import { createRoom, listRooms, setArchived, type Pick, type Result, type RoomCard } from '$lib/api';
	import { me } from '$lib/me.svelte';
	import { roomSubtitle, roomTitle, splitArchived } from '$lib/rooms';
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
	// Archive and Delete show only while managing, so a guest sees rooms to join, not chores.
	let managing = $state(false);

	const parts = $derived(splitArchived(rooms ?? []));
	// Someone is watching: joining them is the page's main action, not starting something new.
	const busy = $derived(parts.active[0]?.watching.length ? parts.active[0] : null);

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
	{@const subtitle = roomSubtitle(r)}
	<li class="relative flex flex-col gap-3 py-3 pl-5 sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-4">
		{#if r.watching.length > 0}
			<!-- The lamp is on: someone is in this room. -->
			<span class="absolute inset-y-3 left-0 w-1 rounded-full bg-lamp" aria-hidden="true"></span>
		{/if}
		<a href="/rooms/{r.id}" class="group min-h-11 min-w-0 flex-1 rounded-control">
			<span
				class="block font-display text-xl font-bold break-words decoration-lamp decoration-2 underline-offset-4 group-hover:underline {r.archived
					? 'text-haze'
					: ''}"
			>
				{roomTitle(r)}
			</span>
			{#if subtitle}
				<span class="mt-1 block break-words text-haze">{subtitle}</span>
			{/if}
			{#if r.watching.length > 0}
				<span class="mt-1 flex items-center gap-2 text-sm">
					<span class="size-2 shrink-0 rounded-full bg-lamp" aria-hidden="true"></span>
					<span class="min-w-0 break-words">
						{strings.watchingList(r.watching.map((w) => w.name).join(', '))}
					</span>
				</span>
			{/if}
		</a>
		{#if managing}
			<div class="flex flex-wrap gap-2">
				<!-- Archive only an empty room. The server checks too: someone may just have joined. -->
				{#if r.archived || r.watching.length === 0}
					<button onclick={() => act(setArchived(r.id, !r.archived))} class="btn btn-quiet btn-small">
						{r.archived ? strings.unarchive : strings.archive}
					</button>
				{/if}
				{#if me.isAdmin}
					<button onclick={() => (confirming = r.id)} class="btn btn-quiet btn-small">
						{strings.deleteRoom}
					</button>
				{/if}
			</div>
		{:else if r.watching.length > 0 && !r.archived}
			<a
				href="/rooms/{r.id}"
				aria-label={strings.joinRoom(roomTitle(r))}
				class="btn self-start sm:self-center {r === busy ? 'btn-primary' : 'btn-quiet'}"
			>
				{strings.join}
			</a>
		{/if}
		{#if managing && confirming === r.id}
			<div class="flex basis-full flex-wrap items-center gap-2 text-sm">
				<span>{strings.deleteRoomConfirm}</span>
				<button
					onclick={() => {
						confirming = null;
						act(deleteRoom(r.id));
					}}
					class="btn btn-danger btn-small"
				>
					{strings.deleteRoom}
				</button>
				<button onclick={() => (confirming = null)} class="btn btn-quiet btn-small">
					{strings.cancel}
				</button>
			</div>
		{/if}
	</li>
{/snippet}

<main
	class="mx-auto grid w-full max-w-6xl grid-cols-1 gap-10 px-4 pt-8 pb-16 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] lg:gap-16 lg:pt-16"
>
	<div class="flex flex-col items-start gap-6 lg:sticky lg:top-8 lg:self-start">
		<h1 class="font-display text-3xl font-extrabold text-balance">{strings.tagline}</h1>
		<button onclick={() => (picking = true)} class="btn {busy ? 'btn-quiet' : 'btn-primary'}">
			<Icon name="play" class="size-5" />
			{strings.watchSomething}
		</button>
		{#if page.state.roomDeleted}
			<p class="text-haze" role="status">{strings.roomDeleted}</p>
		{/if}
		{#if loadFailed || actionError}
			<p class="text-ember" role="alert">
				{loadFailed ? strings.loadFailed : actionError}
			</p>
		{/if}
	</div>

	{#if rooms}
		<div class="flex flex-col gap-10">
			<section class="flex flex-col gap-2">
				<div class="flex items-center justify-between gap-4">
					<h2 class="font-display text-xl font-bold text-haze">{strings.rooms}</h2>
					{#if rooms.length > 0}
						<button
							onclick={() => {
								managing = !managing;
								confirming = null;
							}}
							aria-pressed={managing}
							class="btn btn-quiet btn-small"
						>
							{managing ? strings.doneManaging : strings.manageRooms}
						</button>
					{/if}
				</div>
				{#if parts.active.length === 0}
					<p class="text-haze">{strings.noRooms}</p>
				{:else}
					<ul class="flex flex-col gap-2">
						{#each parts.active as r (r.id)}
							{@render card(r)}
						{/each}
					</ul>
				{/if}
			</section>

			{#if parts.archived.length > 0}
				<section class="flex flex-col gap-2">
					<h2 class="font-display text-xl font-bold text-haze">{strings.archivedRooms}</h2>
					<ul class="flex flex-col gap-2">
						{#each parts.archived as r (r.id)}
							{@render card(r)}
						{/each}
					</ul>
				</section>
			{/if}
		</div>
	{/if}
</main>

{#if picking}
	<Picker onpick={create} onclose={() => (picking = false)} />
{/if}
