<script lang="ts">
	// The room's player: one <video> for the page's life, "Tap to join", our own controls, and the loop
	// that keeps the video with the room. <video> events are status only; only the controls send intents.
	// Subtitles, fullscreen, "Next episode" at the end and the chat live here too, so they work in
	// fullscreen.
	import { onMount, untrack, type Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import type { Room, SubtitleChoice, VideoDetail, VideoSummary } from '$lib/api';
	import { jobText } from '$lib/admin';
	import Icon from '$lib/Icon.svelte';
	import {
		codecName,
		episodeCode,
		subtitleLabel,
		subtitleOptions,
		whyUnplayable
	} from '$lib/picker';
	import { loadPlayerPrefs, savePlayerPrefs, SUBTITLE_SIZES } from '$lib/prefs';
	import type { Prepare, RoomState } from '$lib/protocol';
	import { waitText } from '$lib/rooms';
	import type { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import Subtitles from '$lib/Subtitles.svelte';
	import { sameSubtitle, subtitleUrl } from '$lib/subtitles';
	import { follow, type Step } from '$lib/sync/drift';
	import { local, running, target, type Intent } from '$lib/sync/state';
	import { reportDue, statusOf, type Report } from '$lib/sync/status';
	import { FOLLOW_EVERY_MS, MEDIA_RETRY_MS, OFFLINE_NOTE_MS } from '$lib/sync/timing';
	import { formatTime } from '$lib/time';

	let {
		room,
		detail,
		prepare,
		playState = $bindable(),
		socket,
		userId,
		online,
		offline,
		note,
		missing,
		onpick,
		next,
		nexting,
		onnext,
		chatOpen = $bindable(),
		side,
		overlay
	}: {
		room: Room;
		detail: VideoDetail | null; // the room's video with its tracks; null until loaded
		prepare: Prepare | null;
		playState: RoomState | null; // the server's, or ours applied on top until the next one arrives
		socket: RoomSocket | null;
		userId: number; // this user's, from hello
		online: boolean; // the socket said hello and hasn't dropped since
		offline: boolean; // the socket dropped: "Host is offline"; not before the first connect
		note: string; // "Alice paused"
		missing: boolean; // the video is gone, with no prepared copy: offer another pick
		onpick: () => void;
		next: VideoSummary | null | undefined; // the next episode, offered at the end; undefined while looked up
		nexting: boolean; // a "Next episode" switch is under way
		onnext: () => Promise<boolean>; // false: it failed, and the page says why
		chatOpen: boolean;
		// The chat panel: beside the video in landscape, under it in portrait.
		side: Snippet;
		overlay: Snippet; // new chat messages over the video, while the chat is closed
	} = $props();

	// One press of the subtitle timing buttons.
	const offsetStepMs = 250;
	// One press of the skip buttons.
	const skipMs = 10_000;
	// The controls fade after this long without a touch, mouse move or key, while the video plays.
	const controlsHideMs = 3000;

	let wrapper: HTMLDivElement;
	let video: HTMLVideoElement;
	const src = $derived(prepare?.state === 'ready' ? `/stream/${prepare.key}/video.mp4` : '');

	const prefs = loadPlayerPrefs(() => localStorage);
	let volume = $state(prefs.volume);
	let muted = $state(prefs.muted);
	let subtitleSize = $state(prefs.subtitleSize);
	// iPhones ignore volume set from script: the hardware buttons own it. No slider there.
	const volumeWorks = (() => {
		const v = document.createElement('video');
		v.volume = 0.5;
		return v.volume === 0.5;
	})();

	let joined = $state(false); // pressed "Tap to join"
	// A finger taps, a mouse clicks: the join button names the one this device uses.
	const coarse = matchMedia('(pointer: coarse)').matches;
	let blocked = $state(false); // the browser refused play(): needs a fresh tap
	let hidden = $state(document.hidden);
	let decodeFailed = $state(false); // the video's error event says this device can't decode it
	let retrying = false; // after a network error, until the video loads again
	let retryTimer: ReturnType<typeof setTimeout> | undefined;
	let last: Report | null = null; // the last status sent on this connection
	let shownMs = $state(0); // the room's position, for the seek bar
	let serverMs = $state<number | null>(null); // the server's clock at the last tick; null before a pong
	let dragMs = $state<number | null>(null); // the seek bar's thumb while it's held

	// Why this device can't play the video: the codec check, or the video's own error event.
	const unplayable = $derived(
		whyUnplayable(room.video, (t) => document.createElement('video').canPlayType(t)) ||
			(decodeFailed ? strings.cantPlayHere(codecName(room.video.codecString)) : '')
	);
	const cantPlay = $derived(unplayable !== '');
	// Nothing to play yet, or ever: the copy is still being prepared, or failed, or the video is gone.
	// Or this device can't play it. Play and seek would only move the room's clock over a black box. A
	// room that somehow plays can still be paused, but not from a screen that can't see it.
	const stuck = $derived(missing || src === '' || cantPlay);
	const toggleable = $derived(!!playState && (!stuck || (playState.playing && !cantPlay)));
	const durationMs = $derived(playState?.durationMs ?? 0);

	let subtitlesOpen = $state(false); // the subtitle panel
	let panelHeight = $state(0); // its height, so the subtitles sit above it where it lies over the video
	// The page fits the screen: app.css's `fit` variant, which the layout below follows. Change both.
	const fits = new MediaQuery('(orientation: landscape) and (min-height: 30rem)');
	const options = $derived(detail ? subtitleOptions(detail) : []);
	const subtitleKey = $derived(
		options.find((o) => sameSubtitle(o.choice, playState?.subtitle ?? null))?.key ?? ''
	);
	const subUrl = $derived(subtitleUrl(playState?.subtitle ?? null, prepare, detail?.sidecars ?? []));
	const offsetMs = $derived(playState?.subtitleOffsetMs ?? 0);

	// Fullscreen is the wrapper's, so subtitles and controls stay on top. Where the browser can't
	// (iPhone), the wrapper fills the window instead.
	let native = $state(false); // the browser's fullscreen
	let filled = $state(false); // the CSS fill
	const full = $derived(native || filled);
	// The subtitle panel lies over the video's foot only where the page fits the screen: there, taking
	// room would shrink the video. Elsewhere the video is short (a phone), and the panel would hide the
	// subtitles its timing is set by, so it goes under the bar and the page grows.
	const panelOver = $derived(!full && fits.current);

	// The controls step aside while the video plays and nobody touches them: the buttons over the video
	// always, the bar in fullscreen only. Never while paused, the subtitle panel is open, or the seek
	// bar is held.
	let stage: HTMLDivElement; // the layer over the video; a tap on it, not on a button, hides the controls
	let idle = $state(false);
	let idleTimer: ReturnType<typeof setTimeout> | undefined;
	let overBar = $state(false);
	let barHeight = $state(0);
	const canFade = $derived(joined && !!playState?.playing && !subtitlesOpen && dragMs === null);
	const faded = $derived(idle && canFade && !overBar);
	const fade = $derived(faded ? 'invisible opacity-0' : '');

	// Where the room's prepared copy stands, shown in the video box; '' when there's nothing to say, or
	// it's ready.
	const preparing = $derived(prepare ? jobText(prepare) : '');
	// Before the room's first word on the socket the box is black, with nothing to press: say why.
	const connecting = $derived(prepare === null && !offline);
	// Offline long enough to say so: a blip changes nothing on screen.
	let hostDown = $state(false);
	$effect(() => {
		if (!offline) {
			hostDown = false;
			return;
		}
		const t = setTimeout(() => (hostDown = true), OFFLINE_NOTE_MS);
		return () => clearTimeout(t);
	});

	// At the end the room pauses; a TV episode then offers the next one.
	const atEnd = $derived(
		playState !== null && !playState.playing && durationMs > 0 && playState.positionMs >= durationMs
	);

	// This page waits for "Tap to join": the button in the middle, and what it says.
	const needsTap = $derived(src !== '' && (!joined || blocked));
	const waits = $derived(
		playState && playState.waiting.length > 0 ? waitText(playState.waiting, userId, serverMs, needsTap) : null
	);

	// What a screen reader hears: the room's state changes that show only over the video.
	// Who's behind is the server's word: stale while it's offline.
	const behind = $derived(hostDown ? [] : (playState?.behind ?? []));
	const announce = $derived(hostDown ? strings.hostOffline : (waits?.headline ?? note));

	// What holds the video's middle, one thing at a time, the first that applies. "Waiting for …" is the
	// room's, not this screen's: it can sit under any of them. 'transport' is back, play or pause, forward.
	const middle = $derived.by(() => {
		if (missing) return 'missing';
		// A failed prepare first: it's everyone's, and another pick is what helps.
		if (prepare?.state === 'failed') return 'failed';
		if (cantPlay) return 'cantPlay';
		// Offline, what needs the server is dead or stale: say why where the hands go.
		if (hostDown) return 'offline';
		if (src === '') return connecting ? 'connecting' : preparing ? 'preparing' : '';
		if (needsTap) return 'tap';
		// While the next episode is still looked up (undefined), neither card: a film's would flash.
		if (atEnd) return next ? 'next' : next === null ? 'end' : '';
		return playState?.waiting.length ? '' : 'transport';
	});

	// Only src changes, never the element: a new one may need a fresh tap on iOS.
	$effect(() => {
		const s = src;
		untrack(() => {
			decodeFailed = false;
			retrying = false;
			clearTimeout(retryTimer);
			if (s) video.src = s;
			else if (video.hasAttribute('src')) {
				// src="" would load the page itself as video, and fail.
				video.removeAttribute('src');
				video.load();
			}
			tick();
		});
	});

	// The controls get their full time again whenever they start to be allowed to fade: play starts,
	// the subtitle panel closes, the seek bar is let go.
	$effect.pre(() => {
		if (canFade) untrack(wake);
	});

	// The server forgets a socket's status when it drops, so a new connection hears it again.
	$effect(() => {
		if (online) last = null;
	});

	$effect(() => {
		void playState;
		untrack(tick);
	});

	$effect(() => savePlayerPrefs(() => localStorage, { volume, muted, subtitleSize }));

	// The page behind a CSS fill mustn't scroll.
	$effect(() => {
		document.documentElement.style.overflow = filled ? 'hidden' : '';
		return () => (document.documentElement.style.overflow = '');
	});

	onMount(() => {
		const timer = setInterval(tick, FOLLOW_EVERY_MS);
		const visibility = () => {
			hidden = document.hidden;
			tick();
		};
		document.addEventListener('visibilitychange', visibility);
		const fullscreen = () => (native = document.fullscreenElement === wrapper);
		document.addEventListener('fullscreenchange', fullscreen);
		return () => {
			clearInterval(timer);
			clearTimeout(retryTimer);
			clearTimeout(idleTimer);
			document.removeEventListener('visibilitychange', visibility);
			document.removeEventListener('fullscreenchange', fullscreen);
		};
	});

	// tick brings the video to where the room is, and reports this player's status when due.
	function tick() {
		if (!video) return;
		const now = performance.now();
		const server = socket?.clock.serverNow(now) ?? null;
		serverMs = server;
		const s = playState;
		const run = s !== null && running(s);
		const playing = s?.playing ?? false;
		// A running room needs the server's clock; the first pong after a connect brings it.
		if (s && (server !== null || !run)) {
			shownMs = target(s, server ?? 0);
			if (joined && !blocked && !cantPlay && src && !retrying && !video.seeking) {
				// The prepared copy may end a little before the source's duration, which the room goes by.
				const endMs = Number.isFinite(video.duration) ? video.duration * 1000 : Infinity;
				apply(follow(s, server ?? 0, video.currentTime * 1000, endMs));
			}
		}
		report(now, playing, run);
	}

	function apply(step: Step) {
		if (!step.play && !video.paused) video.pause();
		// A hidden tab only pauses: the room waits for it soon, and a phone may refuse play() there.
		// Seeking it along would tell the room it keeps up while nobody watches.
		if (hidden) return;
		if (step.seekTo !== undefined) video.currentTime = step.seekTo / 1000;
		if (video.playbackRate !== step.rate) video.playbackRate = step.rate;
		// Not at the end: play() on an ended video starts over from 0:00.
		if (step.play && video.paused && !video.ended) play();
	}

	function play() {
		video.play().catch((e: unknown) => {
			// AbortError: a pause came first, which is fine.
			if (e instanceof DOMException && e.name === 'NotAllowedError') blocked = true;
		});
	}

	function report(now: number, playing: boolean, run: boolean) {
		if (!online || !socket) return;
		const status = statusOf({
			joined,
			blocked,
			cantPlay,
			hidden,
			seeking: video.seeking,
			loaded: !retrying && video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA,
			shouldPlay: playing
		});
		if (status === null || !reportDue(last, status, now, run)) return;
		socket.send({ type: 'status', status, positionMs: video.currentTime * 1000 });
		last = { status, at: now };
	}

	// join runs in the tap, so play() counts as the user's: that unlocks sound, and iOS playback. The
	// tick right after pauses again if the room is paused.
	function join() {
		joined = true;
		blocked = false;
		play();
		tick();
	}

	// togglePlay is the play button. Pressed before "Tap to join", it joins: it's the same tap. A
	// room that already plays then plays on here, rather than pausing for everyone.
	function togglePlay() {
		if (src && !joined) {
			join();
			if (playState?.playing) return;
		}
		intent({ type: playState?.playing ? 'pause' : 'play' });
	}

	// intent applies one of our controls at once, and sends it. The server's next state replaces ours.
	function intent(i: Intent) {
		if (!playState || !socket) return;
		const now = socket.clock.serverNow(performance.now());
		if (now !== null) playState = local(playState, i, now);
		socket.send(i);
		// Now, not in the playState effect: iOS lets play() start only inside the tap itself.
		tick();
	}

	// setSubtitle and setOffset change room state for everyone. Shown here at once; the server's next
	// state confirms it.
	function setSubtitle(choice: SubtitleChoice | null) {
		if (!playState || !socket) return;
		playState = { ...playState, subtitle: choice };
		socket.send({ type: 'subtitle', subtitle: choice });
	}

	function setOffset(ms: number) {
		if (!playState || !socket) return;
		playState = { ...playState, subtitleOffsetMs: ms };
		socket.send({ type: 'offset', ms });
	}

	// Keys for the player, like other video players: Space or K plays and pauses, ← and → skip, J and L
	// skip twice as far, F for fullscreen, C for the subtitle panel, H for chat, M to mute. Never while typing, in a dialog, or for a key
	// something on the page already used. Space stays with a focused button or link, and the arrows with
	// a focused slider.
	function shortcut(e: KeyboardEvent) {
		// A held key repeats: each repeat would be another intent for everyone, or another toggle.
		if (e.defaultPrevented || e.repeat || e.ctrlKey || e.metaKey || e.altKey) return;
		const t = e.target as HTMLElement;
		const typing = t.closest('input:not([type=range]), textarea, select, [contenteditable]');
		if (typing || document.querySelector('dialog[open]')) return;
		const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
		const ready = online && !!playState && !stuck;
		if ((key === ' ' && !t.closest('button, a, input')) || key === 'k') {
			if (online && toggleable) togglePlay();
		} else if ((key === 'ArrowLeft' || key === 'ArrowRight') && !t.closest('input')) {
			if (ready && durationMs > 0) skip(key === 'ArrowLeft' ? -skipMs : skipMs);
		} else if (key === 'j' || key === 'l') {
			if (ready && durationMs > 0) skip(key === 'j' ? -2 * skipMs : 2 * skipMs);
		} else if (key === 'f') toggleFullscreen();
		else if (key === 'c') subtitlesOpen = !subtitlesOpen;
		else if (key === 'h') chatOpen = !chatOpen;
		else if (key === 'm') muted = !muted;
		else return;
		e.preventDefault();
	}

	function wake() {
		idle = false;
		clearTimeout(idleTimer);
		idleTimer = setTimeout(() => (idle = true), controlsHideMs);
	}

	function rest() {
		clearTimeout(idleTimer);
		idle = true;
	}

	// A finger on the bare video toggles the controls, like a phone's own player; anything else shows them.
	function pointerDown(e: PointerEvent) {
		const t = e.target as Element;
		const bare = stage.contains(t) && !t.closest('button, a, input, select');
		if (e.pointerType !== 'mouse' && bare && !faded) rest();
		else wake();
	}

	function skip(ms: number) {
		intent({ type: 'seek', positionMs: Math.min(Math.max(shownMs + ms, 0), durationMs) });
	}

	function toggleFullscreen() {
		if (full) exitFullscreen();
		else if (document.fullscreenEnabled) wrapper.requestFullscreen().catch(() => (filled = true));
		else filled = true;
	}

	function exitFullscreen() {
		filled = false;
		if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
	}

	// Next keeps fullscreen. Only a failure leaves it: the page says why outside the wrapper.
	async function nextEpisode() {
		if (!(await onnext())) exitFullscreen();
	}

	// The picker opens outside the wrapper, where fullscreen would hide it.
	function pickAnother() {
		exitFullscreen();
		onpick();
	}

	function failed() {
		const code = video.error?.code;
		if (!src || code === undefined) return;
		if (code === MediaError.MEDIA_ERR_DECODE) {
			decodeFailed = true;
			tick();
			return;
		}
		// "Not supported" also comes when the first fetch fails. It's the video's fault only if the
		// server does answer for it.
		if (code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED) {
			const s = src;
			fetch(s, { method: 'HEAD' })
				.then((r) => r.ok)
				.catch(() => false)
				.then((served) => {
					if (s !== src) return;
					if (served) {
						decodeFailed = true;
						tick();
					} else retry();
				});
			return;
		}
		retry();
	}

	// retry loads the video again after a network error: bad Wi-Fi, or the host restarting. The tick
	// seeks back to the room.
	function retry() {
		retrying = true;
		tick();
		clearTimeout(retryTimer);
		retryTimer = setTimeout(() => {
			retrying = false;
			video.load();
			tick();
		}, MEDIA_RETRY_MS);
	}
</script>

<svelte:window
	onkeydown={(e) => {
		// Typing in chat or a search box isn't asking for the controls. A slider is: it's one of them.
		const t = e.target as HTMLElement;
		if (!t.closest('input:not([type=range]), textarea, select, [contenteditable]')) wake();
		if (e.key === 'Escape') filled = false;
		shortcut(e);
	}}
/>

<!-- The subtitle panel: a strip by the bar. -->
{#snippet subtitlePanel()}
	{#if subtitlesOpen}
		<div
			bind:clientHeight={panelHeight}
			class="flex flex-wrap items-center gap-x-8 gap-y-2 border-t border-line bg-dusk px-3 py-2 text-sm {panelOver
				? 'absolute inset-x-0 bottom-full'
				: ''}"
		>
			<!-- Subtitle and timing change the room for everyone; size, only this screen. Said, so a guest
			fixing their own view doesn't move everyone's. -->
			<div
				role="group"
				aria-labelledby="subs-everyone"
				class="flex max-w-full min-w-0 flex-wrap items-center gap-x-5 gap-y-2"
			>
				<span id="subs-everyone" class="font-semibold">{strings.forEveryone}</span>
				<label class="flex max-w-full min-w-0 items-center gap-2">
					<span class="text-haze">{strings.subtitle}</span>
					<select
						value={subtitleKey}
						disabled={!online || !playState}
						onchange={(e) =>
							setSubtitle(options.find((o) => o.key === e.currentTarget.value)?.choice ?? null)}
						class="field max-w-full min-w-0 py-1"
					>
						<option value="">{strings.subtitleOff}</option>
						{#each options as o (o.key)}
							{@const notInCopy =
								'stream' in o.choice &&
								prepare?.state === 'ready' &&
								!prepare.subtitles.includes(o.choice.stream)}
							{@const why = o.unavailable
								? (strings.subtitleUnavailable[o.unavailable] ?? o.unavailable)
								: notInCopy
									? strings.notInCopy
									: ''}
							<option value={o.key} disabled={why !== ''}>
								{why ? strings.withNote(subtitleLabel(o, options), why) : subtitleLabel(o, options)}
							</option>
						{/each}
					</select>
				</label>
				<div class="flex items-center gap-1">
					<span class="mr-1 text-haze">{strings.subtitleTiming}</span>
					<button
						onclick={() => setOffset(offsetMs - offsetStepMs)}
						disabled={!online || !playState}
						aria-label={strings.subtitleSooner}
						class="btn btn-quiet btn-small w-11 px-0"
					>
						−
					</button>
					<span class="w-16 text-center tabular-nums">{strings.subtitleOffset(offsetMs)}</span>
					<button
						onclick={() => setOffset(offsetMs + offsetStepMs)}
						disabled={!online || !playState}
						aria-label={strings.subtitleLater}
						class="btn btn-quiet btn-small w-11 px-0"
					>
						+
					</button>
					{#if offsetMs !== 0}
						<button
							onclick={() => setOffset(0)}
							disabled={!online || !playState}
							class="btn btn-small px-2 font-normal text-haze hover:text-moonlight"
						>
							{strings.reset}
						</button>
					{/if}
				</div>
			</div>
			<div role="group" aria-labelledby="subs-screen" class="flex items-center gap-x-5">
				<span id="subs-screen" class="font-semibold">{strings.onThisScreen}</span>
				<label class="flex items-center gap-2">
					<span class="text-haze">{strings.subtitleSize}</span>
					<select bind:value={subtitleSize} class="field py-1">
						{#each SUBTITLE_SIZES as size (size)}
							<option value={size}>{strings.subtitleSizes[size]}</option>
						{/each}
					</select>
				</label>
			</div>
		</div>
	{/if}
{/snippet}

<!-- A problem that holds the video's middle: what happened, what's left to do, and maybe another pick.
Compact on a phone's small video box, so it never spills out of it. -->
{#snippet notice(headline: string, why: string, error: boolean, pick: boolean)}
	<div
		class="flex max-w-md flex-col items-center gap-4 rounded-panel bg-dusk/90 p-5 @max-md:gap-2 @max-md:p-3"
	>
		<p class="text-lg break-words {error ? 'text-ember' : ''}">{headline}</p>
		<p class="break-words @max-md:text-sm">{why}</p>
		{#if pick}
			<button onclick={pickAnother} disabled={!online} class="btn btn-primary">
				{strings.pickAnother}
			</button>
		{/if}
	</div>
{/snippet}

<div
	bind:this={wrapper}
	class="flex overflow-hidden bg-black portrait:flex-col {full
		? 'fixed inset-0 z-30 h-dvh'
		: `max-sm:-mx-4 min-h-0 fit:flex-1 sm:rounded-panel ${chatOpen ? 'portrait:flex-1' : ''}`}"
>
	<!-- In portrait, the video keeps its own height and the chat takes the rest; in fullscreen the
	video takes the rest. The pointer handlers only show and hide the controls; the buttons inside do the rest. No text selection or iOS
	callout: a long press on the video would select a subtitle or button label. Chat stays selectable. -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		onpointerdown={pointerDown}
		onpointermove={(e) => e.pointerType === 'mouse' && wake()}
		onpointerleave={(e) => e.pointerType === 'mouse' && rest()}
		class="relative flex min-w-0 flex-1 flex-col select-none [-webkit-touch-callout:none] {full ? 'portrait:min-h-0' : 'portrait:flex-none'}"
	>
		<!-- When the page fits the screen, this space is what's left above the control bar, and the video
		box is the largest 16:9 that fits in it, between black bars. The box narrows with it, not just the
		picture, so subtitles size and wrap to the picture. -->
		<div
			class={full
				? 'flex min-h-0 flex-1 flex-col'
				: 'fit:flex fit:min-h-0 fit:flex-1 fit:items-center fit:justify-center fit:[container-type:size]'}
		>
			<!-- The container for the subtitles' and pills' cqi sizes. Not the wrapper: it holds the chat too. -->
			<div
				class="@container relative {full
					? 'min-h-0 flex-1'
					: 'mx-auto w-full fit:w-[min(100cqw,100cqh*16/9)]'} {faded
					? 'cursor-none'
					: ''}"
			>
				<video
					bind:this={video}
					bind:volume
					bind:muted
					playsinline
					preload="metadata"
					disablepictureinpicture
					disableremoteplayback
					onerror={failed}
					onwaiting={tick}
					onplaying={tick}
					onseeking={tick}
					onseeked={tick}
					oncanplay={tick}
					onpause={tick}
					onloadedmetadata={tick}
					class={full ? 'h-full w-full object-contain' : 'aspect-video w-full'}
				></video>

				{#if src && !cantPlay}
					<Subtitles
						url={subUrl}
						{offsetMs}
						size={subtitleSize}
						videoMs={() => video.currentTime * 1000}
						lift={full ? (faded ? 0 : barHeight) : subtitlesOpen && panelOver ? panelHeight : 0}
					/>
				{/if}

				<!-- The end cards sit on black, not on whatever the last frame happens to be. -->
				{#if middle === 'next' || middle === 'end'}
					<div class="fade-out-video absolute inset-0 bg-black"></div>
				{/if}

				<div
					bind:this={stage}
					class="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center @max-md:p-2"
				>
					{#if middle === 'transport'}
						<div
							class="flex items-center gap-6 transition-[opacity,visibility] duration-300 sm:gap-10 {fade}"
						>
							<button
								onclick={() => skip(-skipMs)}
								disabled={!online || durationMs === 0}
								aria-label={strings.skipBack(skipMs / 1000)}
								title={strings.withKey(strings.skipBack(skipMs / 1000), strings.keys.back)}
								class="transport size-12 sm:size-14"
							>
								<Icon name="skip-back" step={skipMs / 1000} class="size-7 sm:size-8" />
							</button>
							<button
								onclick={togglePlay}
								disabled={!online || !playState}
								aria-label={playState?.playing ? strings.pause : strings.play}
								title={strings.withKey(playState?.playing ? strings.pause : strings.play, strings.keys.play)}
								class="transport transport-main size-16 sm:size-20"
							>
								<Icon name={playState?.playing ? 'pause' : 'play'} class="size-8 sm:size-10" />
							</button>
							<button
								onclick={() => skip(skipMs)}
								disabled={!online || durationMs === 0}
								aria-label={strings.skipForward(skipMs / 1000)}
								title={strings.withKey(strings.skipForward(skipMs / 1000), strings.keys.forward)}
								class="transport size-12 sm:size-14"
							>
								<Icon name="skip-forward" step={skipMs / 1000} class="size-7 sm:size-8" />
							</button>
						</div>
					{:else if middle === 'offline'}
						<div class="transition-[opacity,visibility] duration-300 {fade}">
							{@render notice(strings.hostOffline, strings.hostOfflineWhy, false, false)}
						</div>
					{:else if middle === 'cantPlay'}
						<!-- Unplayable for the server is unplayable for everyone: only another pick helps. -->
						{@const everywhere = room.video.unplayable !== ''}
						{@render notice(
							unplayable,
							everywhere
								? strings.cantPlayAnywhere
								: strings.cantPlayHereWhy(codecName(room.video.codecString)),
							false,
							everywhere
						)}
					{:else if middle === 'failed' && prepare}
						{@render notice(
							strings.prepareFailed,
							strings.prepareFailedWhy[prepare.error] ?? strings.prepareFailedWhy.failed,
							true,
							true
						)}
					{:else if middle === 'tap'}
						<button onclick={join} class="btn btn-primary min-h-14 rounded-full px-7 text-lg">
							<Icon name="play" class="size-6" />
							{coarse ? strings.tapToJoin : strings.clickToJoin}
						</button>
					{:else if middle === 'connecting'}
						<!-- Late, so a quick connect never flashes it. -->
						<p class="pill appear-late">{strings.gettingReady}</p>
					{:else if middle === 'preparing'}
						<div class="flex w-full max-w-64 flex-col items-center gap-3 @max-md:gap-1.5">
							<p class="pill whitespace-nowrap tabular-nums">{preparing}</p>
							{#if prepare?.state === 'running'}
								<div class="h-1 w-full overflow-hidden rounded-full bg-dusk">
									<div class="h-full bg-lamp" style:width="{prepare.progress * 100}%"></div>
								</div>
							{/if}
							<p class="text-sm text-balance text-haze">{strings.preparingWhy}</p>
						</div>
					{:else if middle === 'next' && next}
						<!-- An episode's end gets its own title card too: the evening may go on, or stop here. -->
						<div class="flex max-w-full flex-col items-center gap-4 @max-md:gap-2">
							<p class="font-display text-3xl font-bold @max-md:text-2xl">
								{strings.episodeEnded(episodeCode(room.video))}
							</p>
							<div class="flex max-w-full flex-col items-center gap-2">
								<button
									onclick={nextEpisode}
									disabled={!online || nexting}
									class="btn btn-primary max-w-full break-words"
								>
									{strings.nextEpisodeNamed(episodeCode(next), next.episodeTitle)}
								</button>
								<button onclick={pickAnother} disabled={!online} class="btn btn-quiet">
									{strings.watchSomethingElse}
								</button>
							</div>
						</div>
					{:else if middle === 'end'}
						<!-- A film's last frame is the room's last shared moment: a title card, and what's next. -->
						<div class="flex flex-col items-center gap-4 @max-md:gap-2">
							<p class="font-display text-3xl font-bold @max-md:text-2xl">{strings.theEnd}</p>
							<button onclick={pickAnother} disabled={!online} class="btn btn-primary">
								{strings.watchSomethingElse}
							</button>
						</div>
					{:else if middle === 'missing'}
						{@render notice(
							strings.videoMissing,
							strings.videoGone(shownMs >= 1000 ? formatTime(shownMs) : ''),
							true,
							true
						)}
					{/if}
					<!-- Offline, who the room waits for is stale, and "Play anyway" can't reach anyone. -->
					{#if waits && !hostDown}
						<div
							class="flex max-w-md flex-col items-center gap-4 rounded-panel bg-dusk/90 px-5 py-4 @max-md:gap-2 @max-md:p-3"
						>
							<div class="flex flex-col items-center gap-1">
								<p class="text-lg break-words">{waits.headline}</p>
								{#each waits.lines as line, i (i)}
									<p class="text-sm break-words text-haze tabular-nums">{line}</p>
								{/each}
							</div>
							<button
								onclick={() => socket?.send({ type: 'playAnyway' })}
								disabled={!online}
								class="btn btn-quiet max-w-full break-words"
							>
								{waits.action}
							</button>
						</div>
					{/if}
				</div>

				<div
					class="pointer-events-none absolute top-2 left-2 flex max-w-[70%] flex-col items-start gap-1"
				>
					<!-- Unless the middle says it: while the controls show, it does. -->
					{#if hostDown && (middle !== 'offline' || faded)}
						<p class="pill flex animate-[appear_300ms_both] items-center gap-1.5 break-words">
							<Icon name="offline" class="size-[0.9em] shrink-0 text-ember" />{strings.hostOffline}
						</p>
					{/if}
					{#if note}
						<p class="pill flex items-center gap-1.5 break-words">
							<Icon name="pause" class="size-[0.9em] shrink-0 text-lamp" />{note}
						</p>
					{/if}
					{#each behind as b (b.userId)}
						<p class="pill break-words">{strings.behind(b.name, b.ms)}</p>
					{/each}
					{#if !chatOpen}
						{@render overlay()}
					{/if}
				</div>
				<p role="status" class="sr-only">{announce}</p>
			</div>
		</div>

		<!-- In fullscreen the controls lie over the video's foot on a dark fade, so hiding them never
		resizes the video. -->
		<div
			class={full
				? `pointer-events-none absolute inset-x-0 bottom-0 bg-linear-to-t from-black/80 to-transparent pt-12 transition-[opacity,visibility] duration-300 ${fade}`
				: ''}
		>
			<!-- svelte-ignore a11y_no_static_element_interactions (keeps the controls up under the mouse) -->
			<div
				bind:clientHeight={barHeight}
				onpointerenter={() => (overBar = true)}
				onpointerleave={() => (overBar = false)}
				class="pointer-events-auto relative"
			>
				<!-- The subtitle panel sits where it shows, so Tab and screen readers meet it in that order: above
				the bar in fullscreen and over the video's foot, under it where the page grows. -->
				{#if full || panelOver}
					{@render subtitlePanel()}
				{/if}

				<!-- Laid out by the bar's own width, not the screen's: the chat panel takes part of a wide one.
				A narrow bar puts the seek bar and time on a row of their own, above the buttons. -->
				<div class="@container/bar {full ? '' : 'bg-dusk'}">
					<div class="flex flex-wrap items-center gap-x-1 px-1 py-1 @xl/bar:gap-x-2 @xl/bar:px-2">
						<button
							onclick={togglePlay}
							disabled={!online || !toggleable}
							aria-label={playState?.playing ? strings.pause : strings.play}
							title={strings.withKey(playState?.playing ? strings.pause : strings.play, strings.keys.play)}
							class="icon-btn"
						>
							<Icon name={playState?.playing ? 'pause' : 'play'} />
						</button>
						<div class="flex min-w-0 flex-1 items-center gap-3 px-2 @max-xl/bar:order-first @max-xl/bar:basis-full">
							<input
								type="range"
								aria-label={strings.position}
								min="0"
								max={durationMs}
								step="1000"
								value={dragMs ?? shownMs}
								aria-valuetext={strings.positionOf(formatTime(dragMs ?? shownMs), formatTime(durationMs))}
								disabled={!online || durationMs === 0 || stuck}
								oninput={(e) => (dragMs = Number(e.currentTarget.value))}
								onkeydown={(e) => {
									// An arrow skips 10 s, as everywhere else. The slider's own 1 s step would seek the
									// whole room once per press.
									if (e.ctrlKey || e.metaKey || e.altKey) return;
									const back = e.key === 'ArrowLeft' || e.key === 'ArrowDown';
									if (!back && e.key !== 'ArrowRight' && e.key !== 'ArrowUp') return;
									e.preventDefault();
									if (!e.repeat && online && durationMs > 0) skip(back ? -skipMs : skipMs);
								}}
								onchange={(e) => {
									dragMs = null;
									intent({ type: 'seek', positionMs: Number(e.currentTarget.value) });
								}}
								class="h-11 min-w-0 flex-1"
							/>
							<span class="text-sm whitespace-nowrap text-haze tabular-nums">
								{formatTime(dragMs ?? shownMs)} / {formatTime(durationMs)}
							</span>
						</div>
						<button
							onclick={() => (muted = !muted)}
							aria-label={muted ? strings.unmute : strings.mute}
							title={strings.withKey(muted ? strings.unmute : strings.mute, strings.keys.mute)}
							class="icon-btn"
						>
							<Icon name={muted ? 'muted' : 'volume'} />
						</button>
						{#if volumeWorks}
							<input
								type="range"
								aria-label={strings.volume}
								min="0"
								max="1"
								step="0.05"
								bind:value={volume}
								aria-valuetext={strings.percent(volume)}
								class="hidden h-11 w-20 accent-moonlight @xl/bar:block"
							/>
						{/if}
						<button
							onclick={() => (subtitlesOpen = !subtitlesOpen)}
							aria-label={strings.subtitle}
							aria-pressed={subtitlesOpen}
							title={strings.withKey(strings.subtitle, strings.keys.subtitles)}
							class="icon-btn ml-auto {playState?.subtitle ? '' : 'text-haze'}"
						>
							<Icon name="captions" />
						</button>
						<button
							onclick={() => (chatOpen = !chatOpen)}
							aria-label={strings.chat}
							aria-pressed={chatOpen}
							title={strings.withKey(strings.chat, strings.keys.chat)}
							class="icon-btn"
						>
							<Icon name="chat" />
						</button>
						<button
							onclick={toggleFullscreen}
							aria-label={full ? strings.exitFullscreen : strings.fullscreen}
							title={strings.withKey(full ? strings.exitFullscreen : strings.fullscreen, strings.keys.fullscreen)}
							class="icon-btn"
						>
							<Icon name={full ? 'shrink' : 'expand'} />
						</button>
					</div>
				</div>
				{#if !full && !panelOver}
					{@render subtitlePanel()}
				{/if}
			</div>
		</div>
	</div>

	{#if chatOpen}
		<!-- Landscape: the panel takes the row's height. Portrait: the screen's rest under the video, or
		its lower half in fullscreen; it gives up more while the subtitle panel is open. Absolute inside,
		so its messages never make the player taller. -->
		<aside
			class="relative z-10 border-line landscape:w-72 landscape:shrink-0 landscape:border-l lg:landscape:w-80 portrait:border-t {full
				? 'portrait:h-[50dvh] portrait:shrink-0'
				: `portrait:flex-1 ${subtitlesOpen ? 'portrait:min-h-56' : 'portrait:min-h-72'}`}"
		>
			<div class="absolute inset-0">
				{@render side()}
			</div>
		</aside>
	{/if}
</div>
