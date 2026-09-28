<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Picker from '$lib/Picker.svelte';
	import Player from '$lib/Player.svelte';
	import {
		openRoom,
		renameRoom,
		setArchived,
		switchVideo,
		type Pick,
		type Result,
		type Room,
		type Who
	} from '$lib/api';
	import { videoName } from '$lib/picker';
	import type { Prepare, RoomState, ServerMessage } from '$lib/protocol';
	import { roomTitle } from '$lib/rooms';
	import { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import { PAUSED_NOTE_MS } from '$lib/sync/timing';

	// How soon a failed load tries again.
	const loadRetryMs = 2000;

	const id = $derived(Number(page.params.id));

	let room = $state<Room | null>(null);
	let notFound = $state(false);
	let loadFailed = $state(false);
	let error = $state(''); // why the last change failed
	let picking = $state(false);
	let renaming = $state(false);
	let name = $state('');
	let watching = $state<Who[]>([]);
	let wasHere = $state<Who[]>([]);
	let prepare = $state<Prepare | null>(null);
	let socket = $state<RoomSocket | null>(null);
	let playState = $state<RoomState | null>(null);
	let userId = 0; // this user's, from hello
	let online = $state(false); // said hello, not dropped since
	let offline = $state(false); // dropped; the first connect doesn't count
	let note = $state(''); // "Alice paused"
	let noteTimer: ReturnType<typeof setTimeout>;

	// Opening the room starts preparing its video, then the socket joins it. It runs again when a link
	// leads to another room, since that reuses this page.
	$effect(() => {
		const roomId = id;
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		room = null;
		notFound = false;
		error = '';
		picking = false;
		renaming = false;
		watching = [];
		wasHere = [];
		prepare = null;
		socket = null;
		playState = null;
		online = offline = false;
		note = '';
		const load = async () => {
			const r = await openRoom(roomId);
			if (stopped) return;
			loadFailed = !r.ok && r.error !== 'not-found';
			if (r.ok) {
				room = r.value;
				socket = new RoomSocket(roomId, received, () => {
					online = false;
					offline = true;
				});
			} else if (r.error === 'not-found') notFound = true;
			else timer = setTimeout(load, loadRetryMs);
		};
		load();
		return () => {
			stopped = true;
			clearTimeout(timer);
			clearTimeout(noteTimer);
			socket?.close();
		};
	});

	// received applies a message from the room.
	function received(m: ServerMessage) {
		switch (m.type) {
			case 'hello':
				userId = m.userId;
				online = true;
				offline = false;
				break;
			case 'state':
				playState = m.state;
				break;
			case 'paused':
				if (m.by.userId === userId) break;
				note = strings.pausedBy(m.by.name);
				clearTimeout(noteTimer);
				noteTimer = setTimeout(() => (note = ''), PAUSED_NOTE_MS);
				break;
			case 'room':
				room = m.room;
				break;
			case 'presence':
				watching = m.watching;
				wasHere = m.wasHere;
				break;
			case 'prepare':
				prepare = m.prepare;
				break;
			case 'deleted':
				goto('/', { state: { roomDeleted: true } });
				break;
		}
	}

	// prepareText says where the room's prepared copy stands; '' when there's nothing to say, or the
	// player shows it: ready.
	function prepareText(p: Prepare | null): string {
		switch (p?.state) {
			case 'running':
				return strings.jobRunning(Math.round(p.progress * 100));
			case 'queued':
				return strings.jobQueued(p.place);
			case 'failed':
				return strings.jobErrors[p.error] ?? strings.jobErrors.failed;
			default:
				return '';
		}
	}

	// apply shows a changed room, or why the change failed.
	function apply(r: Result<Room>) {
		if (r.ok) {
			room = r.value;
			error = '';
		} else if (r.error === 'not-found') {
			notFound = true;
		} else {
			error = strings.roomErrors[r.error] ?? strings.actionFailed;
		}
		return r.ok;
	}

	async function switchTo(p: Pick) {
		picking = false;
		apply(await switchVideo(id, p));
	}

	function startRename() {
		name = room?.name ?? '';
		renaming = true;
	}

	async function saveName(e: SubmitEvent) {
		e.preventDefault();
		if (apply(await renameRoom(id, name))) renaming = false;
	}

	// unarchive opens the room again afterwards: this page is open, so its video is needed.
	async function unarchive() {
		if (apply(await setArchived(id, false))) apply(await openRoom(id));
	}
</script>

<svelte:head>
	<title>{room ? `${roomTitle(room)} · ` : ''}{strings.appName}</title>
</svelte:head>

<main class="mx-auto flex max-w-5xl flex-col gap-4 p-4 pb-16">
	{#if offline}
		<p class="rounded-md bg-amber-900/60 px-3 py-2 text-amber-200" role="status">
			{strings.hostOffline}
		</p>
	{/if}
	{#if notFound}
		<div class="flex flex-col items-center gap-3 pt-24">
			<p class="text-neutral-400">{strings.roomNotFound}</p>
			<a href="/" class="underline">{strings.backHome}</a>
		</div>
	{:else if room}
		{#if renaming}
			<form onsubmit={saveName} class="flex max-w-md flex-col gap-2">
				<label for="room-name" class="text-sm text-neutral-400">{strings.roomName}</label>
				<!-- No maxlength: it counts UTF-16 units, not runes. The server decides. -->
				<!-- svelte-ignore a11y_autofocus -->
				<input
					id="room-name"
					bind:value={name}
					placeholder={videoName(room.video)}
					autofocus
					class="rounded-md border border-neutral-700 bg-neutral-900 px-3 py-2 outline-none focus:border-neutral-400"
				/>
				<p class="text-sm text-neutral-400">{strings.roomNameHint}</p>
				<div class="flex gap-2">
					<button
						type="submit"
						class="rounded-md bg-neutral-100 px-3 py-1.5 font-medium text-neutral-950"
					>
						{strings.save}
					</button>
					<button
						type="button"
						onclick={() => (renaming = false)}
						class="rounded-md border border-neutral-700 px-3 py-1.5 text-neutral-300"
					>
						{strings.cancel}
					</button>
				</div>
			</form>
		{:else}
			<div class="flex flex-wrap items-baseline gap-3">
				<h1 class="min-w-0 text-2xl font-bold break-words">{roomTitle(room)}</h1>
				<button
					onclick={startRename}
					class="rounded-md border border-neutral-700 px-2.5 py-1 text-sm hover:bg-neutral-800"
				>
					{strings.renameRoom}
				</button>
			</div>
		{/if}
		{#if room.name}
			<p class="break-words text-neutral-400">{videoName(room.video)}</p>
		{/if}
		{#if room.video.missing}
			<p class="text-amber-400">{strings.videoMissing}</p>
		{:else if !room.archived && prepareText(prepare)}
			<p class={prepare?.state === 'failed' ? 'text-red-400' : 'text-neutral-300'}>
				{prepareText(prepare)}
			</p>
		{/if}

		{#if error}
			<p class="text-sm text-red-400" role="alert">{error}</p>
		{/if}

		{#if !room.archived}
			<Player {room} {prepare} bind:playState {socket} {online} {note} />
		{/if}

		{#if room.archived}
			<p class="text-neutral-300">{strings.roomArchived}</p>
			<button
				onclick={unarchive}
				class="self-start rounded-md bg-neutral-100 px-4 py-2 font-medium text-neutral-950"
			>
				{strings.unarchive}
			</button>
		{:else}
			<button
				onclick={() => (picking = true)}
				class="self-start rounded-md border border-neutral-700 px-4 py-2 hover:bg-neutral-800"
			>
				{strings.switchVideo}
			</button>
		{/if}

		{#each [{ title: strings.watchingNow, people: watching }, { title: strings.wasHere, people: wasHere }] as list (list.title)}
			{#if list.people.length > 0}
				<section class="flex flex-col gap-1">
					<h2 class="text-sm text-neutral-400">{list.title}</h2>
					<ul class="flex flex-wrap gap-2">
						{#each list.people as p (p.userId)}
							<li class="rounded-full bg-neutral-800 px-3 py-1 text-sm break-all">{p.name}</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/each}
	{:else if loadFailed}
		<p class="pt-24 text-center text-neutral-400">{strings.loadFailed}</p>
	{/if}
</main>

{#if picking}
	<Picker onpick={switchTo} onclose={() => (picking = false)} />
{/if}
