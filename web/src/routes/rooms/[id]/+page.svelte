<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { fade } from 'svelte/transition';
	import { MediaQuery } from 'svelte/reactivity';
	import Chat from '$lib/Chat.svelte';
	import Dialog from '$lib/Dialog.svelte';
	import Icon from '$lib/Icon.svelte';
	import GuestLink from '$lib/GuestLink.svelte';
	import { forgetMe, loadMe, me } from '$lib/me.svelte';
	import {
		CHAT_PAGE_SIZE,
		nameColor,
		type NameColors,
		TOAST_MS,
		withDeleted,
		withHistory,
		withMessage,
		withName,
		withOlder,
		withPeople,
		withToast
	} from '$lib/chat';
	import Picker from '$lib/Picker.svelte';
	import Menu from '$lib/Menu.svelte';
	import Player from '$lib/Player.svelte';
	import {
		getLanguages,
		getVideo,
		isNameReason,
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
	import { carryOver, nextEpisode, videoName, whyUnplayable } from '$lib/picker';
	import type { Did, Prepare, RoomState, ServerMessage } from '$lib/protocol';
	import { didNote, needsConfirm, roomSubtitle, roomTitle } from '$lib/rooms';
	import { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import { target } from '$lib/sync/state';
	import { DID_NOTE_MS, OFFLINE_NOTE_MS } from '$lib/sync/timing';
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
	let confirming = $state<{ pick: Pick; text: string } | null>(null); // a switch waiting for a yes
	let detail = $state<VideoDetail | null>(null); // the room's video with its tracks
	let next = $state<VideoSummary | null | undefined>(null); // its next episode; undefined while looked up
	let renaming = $state(false);
	let name = $state('');
	let renameError = $state(''); // the rename dialog's own, so the page's stays as it was
	let renameInvalid = $state(false); // the error is about the name: the field turns ember until it's edited
	let savingName = $state(false);
	let watching = $state<Who[]>([]);
	let prepare = $state<Prepare | null>(null);
	let socket = $state<RoomSocket | null>(null);
	let playState = $state<RoomState | null>(null);
	let userId = $state(0); // this user's, from hello
	let online = $state(false); // said hello, not dropped since
	// Dropped for OFFLINE_NOTE_MS, so a blip that reconnects at once never says so. The first connect
	// doesn't count.
	let offline = $state(false);
	let offlineTimer: ReturnType<typeof setTimeout> | undefined;
	// backOnline forgets a drop: a new room, or the socket said hello again.
	function backOnline() {
		offline = false;
		clearTimeout(offlineTimer);
		offlineTimer = undefined;
	}
	let note = $state<{ did: Did; text: string } | null>(null); // what someone else did: "Alice paused"
	let noteTimer: ReturnType<typeof setTimeout>;
	let messages = $state<ChatMessage[]>([]); // the chat, oldest first
	let more = $state(false); // older messages may be on the server
	let replyTo = $state<ChatMessage | null>(null);
	let draft = $state('');
	// Beside the video in landscape, under it in portrait. Closing it holds only in landscape: upright, it
	// always shows, as the screen under the video has nothing else to hold. It starts open, except on a
	// phone: a touch screen whose short side is under 30rem (app.css's `fit` height). Held sideways, its
	// video needs all of its width. A short desktop window isn't one.
	let chatOpen = $state(
		!matchMedia('(pointer: coarse) and (width < 30rem), (pointer: coarse) and (height < 30rem)').matches
	);
	const portrait = new MediaQuery('(orientation: portrait)');
	const chatShown = $derived(chatOpen || portrait.current);
	let toasts = $state<ChatMessage[]>([]);
	// A shown chat holds every message, so its toasts go, and don't come back when it hides again.
	$effect(() => {
		if (chatShown) toasts = [];
	});
	let heard = $state(''); // the newest message from someone else, for screen readers
	// Each person's name color. meet adds the new people, so older messages loading in never recolor
	// anyone; the room change below starts it over.
	let colors = $state.raw<NameColors>(new Map());

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
		online = false;
		backOnline();
		note = null;
		messages = [];
		more = false;
		replyTo = null;
		draft = '';
		toasts = [];
		heard = '';
		colors = new Map();
		const load = async () => {
			const r = await openRoom(roomId);
			if (stopped) return;
			loadFailed = !r.ok && r.error !== 'not-found';
			if (r.ok) {
				room = r.value;
				socket = new RoomSocket(roomId, received, () => {
					online = false;
					offlineTimer ??= setTimeout(() => (offline = true), OFFLINE_NOTE_MS);
				});
			} else if (r.error === 'not-found') notFound = true;
			else timer = setTimeout(load, loadRetryMs);
		};
		load();
		return () => {
			stopped = true;
			clearTimeout(timer);
			clearTimeout(noteTimer);
			clearTimeout(offlineTimer);
			socket?.close();
		};
	});

	// A rename to another person's name makes this browser that person. Join again as them, so the room
	// hears it and the page gets the new user id. The page stays, so the player plays on with no new tap.
	// A rename of the person needs none of that: the server renames its sockets. joinedAs is who the
	// socket joined as, from its hello.
	let joinedAs = me.id;
	$effect(() => {
		if (me.id === joinedAs) return;
		joinedAs = me.id;
		untrack(() => {
			if (!socket) return;
			online = false; // hello sets it back, and the player reports its status to the new connection
			socket.rejoin();
		});
	});

	const videoId = $derived(room?.video.id);

	// The room's video, with its subtitles, and its next episode. Again whenever the video changes.
	$effect(() => {
		const vid = videoId;
		detail = null;
		next = null;
		if (vid === undefined) return;
		const v = untrack(() => room!.video);
		if (v.type === 'tv') next = undefined;
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
				userId = joinedAs = m.userId;
				// A rename may have come while the socket was down, or made this browser another person:
				// ask who it is now.
				loadMe().catch(() => {});
				online = true;
				backOnline();
				break;
			case 'state':
				playState = m.state;
				break;
			case 'did': {
				if (m.by.userId === userId || !room) break;
				const text = didNote(m.action, m.by.name, playState, room, detail);
				if (!text) break;
				note = { did: m.action, text };
				clearTimeout(noteTimer);
				noteTimer = setTimeout(() => (note = null), DID_NOTE_MS);
				break;
			}
			case 'room':
				room = m.room;
				break;
			case 'presence':
				watching = m.watching;
				meet();
				break;
			case 'renamed':
				messages = withName(messages, m.who);
				toasts = withName(toasts, m.who);
				if (replyTo?.from.userId === m.who.userId) replyTo = { ...replyTo, from: m.who };
				// This socket's person, or this browser's: ask the server who this browser is now, so the
				// header shows it. The answer is newer than the rename.
				if (m.who.userId === userId || m.who.userId === me.id) loadMe().catch(() => {});
				break;

			case 'loggedOut':
				// On another page. With no name, the layout asks for one in place of this page.
				socket?.close();
				forgetMe();
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
				meet();
				break;
			}
			case 'chat':
				messages = withMessage(messages, m.message);
				meet();
				if (m.message.from.userId !== userId) {
					heard = strings.said(m.message.from.name, m.message.text);
					if (!chatShown) toast(m.message);
				}
				break;
			case 'chatDeleted':
				messages = withDeleted(messages, m.id);
				toasts = toasts.filter((t) => t.id !== m.id);
				// The reply may still go out; it quotes "deleted message" then, and so does the reply bar.
				if (replyTo?.id === m.id) replyTo = { ...replyTo, text: '' };
				break;
		}
	}

	// meet gives a name color to everyone new in the chat or the watching list.
	function meet() {
		colors = withPeople(colors, messages, watching, userId);
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
			meet();
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

	// playNext switches to the next episode with the room's audio and subtitle, without the picker:
	// a series night shouldn't stop to ask the same thing every episode. False when it failed, and
	// error says why.
	let nexting = $state(false); // until the switch is done: a double tap mustn't switch twice
	async function playNext(): Promise<boolean> {
		const v = next;
		const was = room;
		if (!v || !was || nexting) return true;
		// The picker would grey it out: it would switch everyone to a video this device can't show.
		const why = whyUnplayable(v, (t) => document.createElement('video').canPlayType(t));
		if (why) {
			error = why;
			return false;
		}
		nexting = true;
		try {
			const [to, l] = await Promise.all([getVideo(v.id), getLanguages()]);
			// Another room, or someone switched this one meanwhile: the tap was for what's gone.
			if (room?.id !== was.id || room.video.id !== was.video.id) return true;
			if (!to.ok || to.value.missing || !l.ok) {
				error = strings.actionFailed;
				return false;
			}
			return await switchTo(carryOver(detail, room.audio, room.subtitle, to.value, l.value), videoName(v));
		} finally {
			nexting = false;
		}
	}

	// switchTo asks first when the switch would lose the room's place. False when the switch failed.
	async function switchTo(p: Pick, name: string): Promise<boolean> {
		picking = false;
		if (!room) return true;
		const s = playState;
		const now = socket?.clock.serverNow(performance.now()) ?? null;
		const at = s ? target(s, now ?? s.atMs) : room.positionMs;
		if (needsConfirm(at, s?.durationMs ?? room.video.durationMs, missing)) {
			confirming = { pick: p, text: strings.switchConfirm(formatTime(at), name) };
			return true;
		}
		return doSwitch(p);
	}

	async function doSwitch(p: Pick): Promise<boolean> {
		confirming = null;
		return apply(await switchVideo(id, p));
	}

	function startRename() {
		name = room?.name ?? '';
		renameError = '';
		renameInvalid = false;
		renaming = true;
	}

	async function saveName(e: SubmitEvent) {
		e.preventDefault();
		savingName = true;
		const r = await renameRoom(id, name);
		savingName = false;
		if (r.ok) room = r.value;
		else if (r.error === 'not-found') notFound = true;
		else {
			renameInvalid = isNameReason(r.error);
			renameError = renameInvalid
				? strings.nameRefused(r.error, r.symbol ?? '')
				: (strings.roomErrors[r.error] ?? strings.actionFailed);
			return;
		}
		renaming = false;
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
		{colors}
		videoId={room?.video.id ?? 0}
		{readOnly}
		{online}
		{offline}
		bind:replyTo
		bind:draft
		onsend={sendChat}
		ondelete={(chatId) => socket?.send({ type: 'deleteChat', id: chatId })}
		onolder={olderChat}
		onclose={readOnly || portrait.current ? undefined : () => (chatOpen = false)}
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
				<span class="font-semibold {nameColor(t.from.userId, userId, colors)}">{t.from.name}</span>
				{t.text}
			</span>
		</button>
	{/each}
{/snippet}

<svelte:head>
	<title>{room ? strings.pageTitle(roomTitle(room)) : strings.appName}</title>
</svelte:head>

<!-- In portrait, the player with its chat fills the screen down to the bottom edge. -->
<main
	class="flex w-full flex-1 flex-col gap-4 px-4 pt-2 pb-6 {room && !room.archived
		? 'portrait:pb-0'
		: ''}"
>
	<!-- An open room says it over the video, where fullscreen still shows it. -->
	{#if offline && room?.archived}
		<p class="text-haze" role="status">{strings.hostOffline}</p>
	{/if}
	<p aria-live="polite" class="sr-only">{heard}</p>
	{#if notFound}
		<div class="flex flex-col items-start gap-6 pt-16">
			<p class="font-display text-2xl font-bold">{strings.roomNotFound}</p>
			<a href="/" class="btn btn-quiet">{strings.backHome}</a>
		</div>
	{:else if room}
		<!-- The title with its video name under it, and the room's few actions beside it, so the video
		starts high on a phone. -->
		<div class="flex items-start gap-3">
			<div class="min-w-0 flex-1">
				<h1 class="font-display text-xl font-bold break-words sm:text-2xl">{roomTitle(room)}</h1>
				{#if roomSubtitle(room)}
					<p class="break-words text-haze">{roomSubtitle(room)}</p>
				{/if}
			</div>
			<div class="flex shrink-0 items-center gap-2 pt-1">
				<!-- A missing video's pick sits in the player, in its place. -->
				{#if !room.archived && !missing}
					<button onclick={() => (picking = true)} class="btn btn-quiet btn-small">
						{strings.switchVideo}
					</button>
				{/if}
				<Menu label={strings.roomActions} buttonClass="btn btn-quiet btn-small px-2 pointer-coarse:min-w-11">
					{#snippet button()}
						<Icon name="more" class="size-5" />
					{/snippet}
					{#snippet items(close)}
						{#if next && !room?.archived && !missing}
							<button
								onclick={() => {
									close();
									playNext();
								}}
								class="row"
							>
								{strings.nextEpisode}
							</button>
						{/if}
						<button
							onclick={() => {
								close();
								startRename();
							}}
							class="row"
						>
							{strings.renameRoom}
						</button>
					{/snippet}
				</Menu>
			</div>
		</div>
		<!-- Only the admin port sends a guest link: guests never see this. -->
		{#if me.guestUrl && !room.archived}
			<div class="-mt-2"><GuestLink path="/rooms/{room.id}" /></div>
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
				{userId}
				{online}
				{offline}
				{note}
				{missing}
				onpick={() => (picking = true)}
				{next}
				{nexting}
				onnext={playNext}
				bind:chatOpen
				{chatShown}
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
			<!-- A reading width: across a wide screen, the lines get too long to follow. -->
			<div class="h-96 w-full max-w-xl overflow-hidden rounded-panel border border-line">
				{@render chatPanel(true)}
			</div>
		{/if}
	{:else if loadFailed}
		<p class="pt-16 text-haze">{strings.loadFailed}</p>
	{/if}
</main>

{#if picking}
	<Picker onpick={switchTo} onclose={() => (picking = false)} />
{/if}

{#if renaming && room}
	{@const current = room.name}
	<Dialog label={strings.renameRoom} onclose={() => (renaming = false)} class="items-center justify-center p-4">
		<form
			onsubmit={saveName}
			class="flex w-full max-w-sm flex-col gap-4 rounded-panel border border-line bg-dusk p-5"
		>
			<label for="room-name" class="font-display text-2xl font-bold">{strings.roomName}</label>
			<!-- No maxlength: it counts UTF-16 units, not runes. The server decides. -->
			<!-- svelte-ignore a11y_autofocus -->
			<input
				id="room-name"
				bind:value={name}
				oninput={() => (renameInvalid = false)}
				aria-invalid={renameInvalid || undefined}
				aria-describedby={renameError ? 'room-name-error' : 'room-name-hint'}
				placeholder={videoName(room.video)}
				autofocus
				class="field"
			/>
			{#if renameError}
				<p id="room-name-error" class="-mt-2 text-sm break-words text-ember" role="alert">{renameError}</p>
			{:else}
				<p id="room-name-hint" class="-mt-2 text-sm text-haze">{strings.roomNameHint}</p>
			{/if}
			<div class="flex gap-2">
				<!-- Off until the name changes: a form at rest shows no lit lamp. -->
				<button
					type="submit"
					disabled={savingName || name.trim() === current}
					class="btn btn-primary flex-1"
				>
					{strings.save}
				</button>
				<button type="button" onclick={() => (renaming = false)} class="btn btn-quiet flex-1">
					{strings.cancel}
				</button>
			</div>
		</form>
	</Dialog>
{/if}

{#if confirming}
	{@const c = confirming}
	<Dialog label={strings.switchVideo} onclose={() => (confirming = null)} class="items-center justify-center p-4">
		<div class="flex max-w-md flex-col gap-5 rounded-panel border border-line bg-dusk p-5">
			<p class="text-lg break-words">{c.text}</p>
			<div class="flex gap-2">
				<button onclick={() => doSwitch(c.pick)} class="btn btn-primary">
					{strings.switchAction}
				</button>
				<!-- Focus starts on Cancel: a stray Enter mustn't change the video for everyone. -->
				<!-- svelte-ignore a11y_autofocus -->
				<button onclick={() => (confirming = null)} autofocus class="btn btn-quiet">
					{strings.cancel}
				</button>
			</div>
		</div>
	</Dialog>
{/if}
