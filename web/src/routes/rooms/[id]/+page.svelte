<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { fade } from 'svelte/transition';
	import Chat from '$lib/Chat.svelte';
	import {
		CHAT_PAGE_SIZE,
		TOAST_MS,
		withDeleted,
		withHistory,
		withMessage,
		withOlder,
		withToast
	} from '$lib/chat';
	import Picker from '$lib/Picker.svelte';
	import Player from '$lib/Player.svelte';
	import {
		getVideo,
		listVideos,
		olderMessages,
		openRoom,
		renameRoom,
		setArchived,
		switchVideo,
		type ChatMessage,
		type Pick,
		type Result,
		type Room,
		type VideoDetail,
		type VideoSummary,
		type Who
	} from '$lib/api';
	import { nextEpisode, videoName } from '$lib/picker';
	import type { Prepare, RoomState, ServerMessage } from '$lib/protocol';
	import { needsConfirm, roomTitle } from '$lib/rooms';
	import { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import { target } from '$lib/sync/state';
	import { PAUSED_NOTE_MS } from '$lib/sync/timing';
	import { formatTime } from '$lib/time';

	// How soon a failed load tries again.
	const loadRetryMs = 2000;

	// Toasts leave at once for people who asked for less motion.
	const toastFadeMs = matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 400;

	const id = $derived(Number(page.params.id));

	let room = $state<Room | null>(null);
	let notFound = $state(false);
	let loadFailed = $state(false);
	let error = $state(''); // why the last change failed
	let picking = $state(false);
	let pickStart = $state<VideoSummary | undefined>(); // the picker opens on this video: "Next episode"
	let confirming = $state<{ pick: Pick; text: string } | null>(null); // a switch waiting for a yes
	let detail = $state<VideoDetail | null>(null); // the room's video with its tracks
	let next = $state<VideoSummary | null>(null); // its next episode
	let renaming = $state(false);
	let name = $state('');
	let watching = $state<Who[]>([]);
	let prepare = $state<Prepare | null>(null);
	let socket = $state<RoomSocket | null>(null);
	let playState = $state<RoomState | null>(null);
	let userId = $state(0); // this user's, from hello
	let online = $state(false); // said hello, not dropped since
	let offline = $state(false); // dropped; the first connect doesn't count
	let note = $state(''); // "Alice paused"
	let noteTimer: ReturnType<typeof setTimeout>;
	let messages = $state<ChatMessage[]>([]); // the chat, oldest first
	let more = $state(false); // older messages may be on the server
	let replyTo = $state<ChatMessage | null>(null);
	let draft = $state('');
	// The chat starts open beside the video on wide screens, and closed as a sheet on portrait phones.
	let chatOpen = $state(matchMedia('(orientation: landscape)').matches);
	let toasts = $state<ChatMessage[]>([]);

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
		confirming = null;
		renaming = false;
		watching = [];
		prepare = null;
		socket = null;
		playState = null;
		online = offline = false;
		note = '';
		messages = [];
		more = false;
		replyTo = null;
		draft = '';
		toasts = [];
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

	const videoId = $derived(room?.video.id);

	// The room's video, with its subtitles, and its next episode. Again whenever the video changes.
	$effect(() => {
		const vid = videoId;
		detail = null;
		next = null;
		if (vid === undefined) return;
		const v = untrack(() => room!.video);
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		const load = async () => {
			const [d, list] = await Promise.all([
				getVideo(vid),
				v.type === 'tv' ? listVideos() : Promise.resolve(null)
			]);
			if (stopped) return;
			if (d.ok) detail = d.value;
			if (list?.ok) next = nextEpisode(v, list.value);
			if ((!d.ok && d.error !== 'not-found') || (list && !list.ok)) timer = setTimeout(load, loadRetryMs);
		};
		load();
		return () => {
			stopped = true;
			clearTimeout(timer);
		};
	});

	// A room whose video is gone, with no prepared copy to play, offers another pick in its place. The
	// server says so with no job (state ''); until its prepare message comes, the room isn't counted as
	// missing, so a switch still asks first.
	const missing = $derived(room !== null && room.video.missing && prepare?.state === '');

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
				break;
			case 'prepare':
				prepare = m.prepare;
				break;
			case 'deleted':
				goto('/', { state: { roomDeleted: true } });
				break;
			case 'chatHistory': {
				const h = withHistory(messages, m.messages);
				messages = h.list;
				if (!h.keptOlder) more = m.messages.length === CHAT_PAGE_SIZE;
				break;
			}
			case 'chat':
				messages = withMessage(messages, m.message);
				if (!chatOpen && m.message.from.userId !== userId) toast(m.message);
				break;
			case 'chatDeleted':
				messages = withDeleted(messages, m.id);
				toasts = toasts.filter((t) => t.id !== m.id);
				// The reply may still go out; it quotes "deleted message" then, and so does the reply bar.
				if (replyTo?.id === m.id) replyTo = { ...replyTo, text: '' };
				break;
		}
	}

	function toast(m: ChatMessage) {
		toasts = withToast(toasts, m);
		setTimeout(() => (toasts = toasts.filter((t) => t.id !== m.id)), TOAST_MS);
	}

	// Tapping a toast answers it.
	function replyFromToast(m: ChatMessage) {
		toasts = [];
		replyTo = m;
		chatOpen = true;
	}

	// sendChat keeps the draft when the socket can't take it.
	function sendChat(text: string) {
		if (!socket?.send({ type: 'chat', text, replyTo: replyTo?.id ?? null })) return;
		replyTo = null;
		draft = '';
	}

	// olderChat loads the page of messages before the oldest shown.
	async function olderChat(): Promise<boolean> {
		const roomId = id;
		const first = messages[0];
		if (!first) return true;
		const r = await olderMessages(roomId, first.id);
		// Another room, or a reconnect's history replaced the list: this page no longer fits in front.
		if (roomId !== id || messages[0]?.id !== first.id) return true;
		if (r.ok) {
			messages = withOlder(messages, r.value);
			more = r.value.length === CHAT_PAGE_SIZE;
		}
		return r.ok;
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

	function openPicker(start?: VideoSummary) {
		pickStart = start;
		picking = true;
	}

	// switchTo asks first when the switch would lose the room's place.
	function switchTo(p: Pick, name: string) {
		picking = false;
		if (!room) return;
		const s = playState;
		const now = socket?.clock.serverNow(performance.now()) ?? null;
		const at = s ? target(s, now ?? s.atMs) : room.positionMs;
		if (needsConfirm(at, s?.durationMs ?? room.video.durationMs, missing)) {
			confirming = { pick: p, text: strings.switchConfirm(formatTime(at), name) };
		} else {
			doSwitch(p);
		}
	}

	async function doSwitch(p: Pick) {
		confirming = null;
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

{#snippet chatPanel(readOnly: boolean)}
	<Chat
		{messages}
		{more}
		{watching}
		{userId}
		videoId={room?.video.id ?? 0}
		{readOnly}
		{online}
		bind:replyTo
		bind:draft
		onsend={sendChat}
		ondelete={(chatId) => socket?.send({ type: 'deleteChat', id: chatId })}
		onolder={olderChat}
		onclose={readOnly ? undefined : () => (chatOpen = false)}
	/>
{/snippet}

{#snippet chatToasts()}
	{#each toasts as t (t.id)}
		<button
			onclick={() => replyFromToast(t)}
			out:fade={{ duration: toastFadeMs }}
			class="pill pointer-events-auto flex max-w-[28em] text-left pointer-coarse:min-h-11 pointer-coarse:items-center"
		>
			<span class="line-clamp-2 break-words">
				<span class="font-semibold">{t.from.name}</span>
				{t.text}
			</span>
		</button>
	{/each}
{/snippet}

<svelte:head>
	<title>{room ? `${roomTitle(room)} · ` : ''}{strings.appName}</title>
</svelte:head>

<!-- A portrait chat sheet covers the page's lower part: room to scroll the rest above it. -->
<main
	class="flex w-full flex-col gap-4 px-4 pt-2 pb-16 {chatOpen && room && !room.archived
		? 'portrait:pb-[50dvh]'
		: ''}"
>
	{#if offline}
		<p class="rounded-control bg-lamp/15 px-3 py-2 text-lamp" role="status">
			{strings.hostOffline}
		</p>
	{/if}
	{#if notFound}
		<div class="flex flex-col items-start gap-6 pt-16">
			<p class="font-display text-2xl font-bold">{strings.roomNotFound}</p>
			<a href="/" class="btn btn-quiet">{strings.backHome}</a>
		</div>
	{:else if room}
		{#if renaming}
			<form onsubmit={saveName} class="flex max-w-md flex-col gap-2">
				<label for="room-name" class="text-sm text-haze">{strings.roomName}</label>
				<!-- No maxlength: it counts UTF-16 units, not runes. The server decides. -->
				<!-- svelte-ignore a11y_autofocus -->
				<input
					id="room-name"
					bind:value={name}
					placeholder={videoName(room.video)}
					autofocus
					class="field"
				/>
				<p class="text-sm text-haze">{strings.roomNameHint}</p>
				<div class="flex gap-2">
					<button type="submit" class="btn btn-primary">{strings.save}</button>
					<button type="button" onclick={() => (renaming = false)} class="btn btn-quiet">
						{strings.cancel}
					</button>
				</div>
			</form>
		{:else}
			<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
				<h1 class="min-w-0 font-display text-2xl font-bold break-words">{roomTitle(room)}</h1>
				<div class="flex flex-wrap gap-2">
					<button onclick={startRename} class="btn btn-quiet btn-small">
						{strings.renameRoom}
					</button>
					{#if !room.archived}
						<button onclick={() => openPicker()} class="btn btn-quiet btn-small">
							{missing ? strings.pickAnother : strings.switchVideo}
						</button>
						{#if next && !missing}
							<button onclick={() => openPicker(next ?? undefined)} class="btn btn-quiet btn-small">
								{strings.nextEpisode}
							</button>
						{/if}
					{/if}
				</div>
			</div>
		{/if}
		{#if room.name && !renaming}
			<p class="-mt-3 break-words text-haze">{videoName(room.video)}</p>
		{/if}
		{#if missing}
			<p class="text-ember">{strings.videoMissing}</p>
		{/if}

		{#if error}
			<p class="text-ember" role="alert">{error}</p>
		{/if}

		{#if !room.archived}
			<Player
				{room}
				{detail}
				{prepare}
				bind:playState
				{socket}
				{online}
				{note}
				{next}
				onnext={() => openPicker(next ?? undefined)}
				bind:chatOpen
			>
				{#snippet side()}
					{@render chatPanel(false)}
				{/snippet}
				{#snippet overlay()}
					{@render chatToasts()}
				{/snippet}
			</Player>
		{/if}

		{#if room.archived}
			<p>{strings.roomArchived}</p>
			<button onclick={unarchive} class="btn btn-primary self-start">
				{strings.unarchive}
			</button>
			<div class="h-96 overflow-hidden rounded-panel border border-line">
				{@render chatPanel(true)}
			</div>
		{/if}
	{:else if loadFailed}
		<p class="pt-16 text-haze">{strings.loadFailed}</p>
	{/if}
</main>

{#if picking}
	<Picker open={pickStart} onpick={switchTo} onclose={() => (picking = false)} />
{/if}

{#if confirming}
	{@const c = confirming}
	<div
		class="fixed inset-0 z-20 flex items-center justify-center bg-midnight/80 p-4"
		role="dialog"
		aria-modal="true"
	>
		<div class="flex max-w-md flex-col gap-5 rounded-panel border border-line bg-dusk p-5">
			<p class="text-lg break-words">{c.text}</p>
			<div class="flex gap-2">
				<button onclick={() => doSwitch(c.pick)} class="btn btn-primary">
					{strings.switchAction}
				</button>
				<button onclick={() => (confirming = null)} class="btn btn-quiet">
					{strings.cancel}
				</button>
			</div>
		</div>
	</div>
{/if}
