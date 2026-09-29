<script lang="ts">
	// The room's player: one <video> for the page's life, "Tap to join", our own controls, and the loop
	// that keeps the video with the room. <video> events are status only; only the controls send intents.
	// Subtitles, fullscreen, "Next episode" at the end and the chat live here too, so they work in
	// fullscreen.
	import { onMount, untrack, type Snippet } from 'svelte';
	import type { Room, SubtitleChoice, VideoDetail, VideoSummary } from '$lib/api';
	import {
		codecName,
		episodeCode,
		subtitleLabel,
		subtitleOptions,
		whyUnplayable
	} from '$lib/picker';
	import { loadPlayerPrefs, savePlayerPrefs, SUBTITLE_SIZES } from '$lib/prefs';
	import type { Prepare, RoomState } from '$lib/protocol';
	import type { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import Subtitles from '$lib/Subtitles.svelte';
	import { sameSubtitle, subtitleUrl } from '$lib/subtitles';
	import { follow, type Step } from '$lib/sync/drift';
	import { local, running, target, type Intent } from '$lib/sync/state';
	import { reportDue, statusOf, type Report } from '$lib/sync/status';
	import { FOLLOW_EVERY_MS, MEDIA_RETRY_MS } from '$lib/sync/timing';
	import { formatTime } from '$lib/time';

	let {
		room,
		detail,
		prepare,
		playState = $bindable(),
		socket,
		online,
		note,
		next,
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
		online: boolean; // the socket said hello and hasn't dropped since
		note: string; // "Alice paused"
		next: VideoSummary | null; // the next episode, offered at the end
		onnext: () => void;
		chatOpen: boolean;
		// The chat panel: beside the video in landscape, a bottom sheet in portrait.
		side: Snippet;
		overlay: Snippet; // new chat messages over the video, while the chat is closed
	} = $props();

	// One press of the subtitle timing buttons.
	const offsetStepMs = 250;

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
	let blocked = $state(false); // the browser refused play(): needs a fresh tap
	let hidden = $state(document.hidden);
	let decodeFailed = $state(false); // the video's error event says this device can't decode it
	let retrying = false; // after a network error, until the video loads again
	let retryTimer: ReturnType<typeof setTimeout> | undefined;
	let last: Report | null = null; // the last status sent on this connection
	let shownMs = $state(0); // the room's position, for the seek bar
	let dragMs = $state<number | null>(null); // the seek bar's thumb while it's held

	// Why this device can't play the video: the codec check, or the video's own error event.
	const unplayable = $derived(
		whyUnplayable(room.video, (t) => document.createElement('video').canPlayType(t)) ||
			(decodeFailed ? strings.cantPlayHere(codecName(room.video.codecString)) : '')
	);
	const cantPlay = $derived(unplayable !== '');
	const durationMs = $derived(playState?.durationMs ?? 0);

	let subtitlesOpen = $state(false); // the subtitle panel
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

	// At the end the room pauses; a TV episode then offers the next one.
	const atEnd = $derived(
		playState !== null && !playState.playing && durationMs > 0 && playState.positionMs >= durationMs
	);

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
			document.removeEventListener('visibilitychange', visibility);
			document.removeEventListener('fullscreenchange', fullscreen);
		};
	});

	// tick brings the video to where the room is, and reports this player's status when due.
	function tick() {
		if (!video) return;
		const now = performance.now();
		const server = socket?.clock.serverNow(now) ?? null;
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

	function toggleFullscreen() {
		if (full) exitFullscreen();
		else if (document.fullscreenEnabled) wrapper.requestFullscreen().catch(() => (filled = true));
		else filled = true;
	}

	function exitFullscreen() {
		filled = false;
		if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
	}

	// The picker opens outside the wrapper, where fullscreen would hide it.
	function nextEpisode() {
		exitFullscreen();
		onnext();
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

<svelte:window onkeydown={(e) => e.key === 'Escape' && filled && (filled = false)} />

<div
	bind:this={wrapper}
	class="flex overflow-hidden bg-black {full ? 'fixed inset-0 z-30 h-dvh' : 'rounded-md'}"
>
	<!-- In portrait fullscreen, the video and controls move up out of the chat sheet's way. -->
	<div class="flex min-w-0 flex-1 flex-col {full && chatOpen ? 'portrait:pb-[50dvh]' : ''}">
		<!-- The container for the subtitles' cqi sizes. Not the wrapper: a container is the box its fixed
		children position in, which would hold the chat's bottom sheet inside the player. -->
		<div class="@container relative {full ? 'min-h-0 flex-1' : ''}">
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
				/>
			{/if}

			<div class="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
				{#if cantPlay}
					<p class="rounded-md bg-black/70 px-3 py-2">
						{unplayable}
					</p>
				{:else if src && (!joined || blocked)}
					<button
						onclick={join}
						class="rounded-full bg-neutral-100 px-6 py-3 text-lg font-medium text-neutral-950"
					>
						{strings.tapToJoin}
					</button>
				{/if}
				{#if atEnd && next}
					<button
						onclick={nextEpisode}
						class="max-w-full rounded-md bg-neutral-100 px-4 py-2 font-medium break-words text-neutral-950"
					>
						{strings.nextEpisodeNamed([episodeCode(next), next.episodeTitle].filter(Boolean).join(' · '))}
					</button>
				{/if}
				{#if playState && playState.waiting.length > 0}
					<div class="flex flex-col items-center gap-2 rounded-md bg-black/70 px-3 py-2">
						<p class="break-words">{strings.waitingFor(playState.waiting.map((w) => w.name))}</p>
						<button
							onclick={() => socket?.send({ type: 'playAnyway' })}
							disabled={!online}
							class="rounded-md border border-neutral-500 px-3 py-1 text-sm hover:bg-neutral-800 disabled:opacity-50"
						>
							{strings.playAnyway}
						</button>
					</div>
				{/if}
			</div>

			<div
				class="pointer-events-none absolute top-2 left-2 flex max-w-[70%] flex-col items-start gap-1 text-sm"
			>
				{#if note}
					<p class="rounded-md bg-black/70 px-2 py-1 break-words">{note}</p>
				{/if}
				{#each playState?.behind ?? [] as b (b.userId)}
					<p class="rounded-md bg-black/70 px-2 py-1 break-words">{strings.behind(b.name, b.ms)}</p>
				{/each}
				{#if !chatOpen}
					{@render overlay()}
				{/if}
			</div>
		</div>

		{#if subtitlesOpen}
			<div
				class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-neutral-800 bg-neutral-900 px-3 py-2 text-sm"
			>
				<select
					aria-label={strings.subtitle}
					value={subtitleKey}
					disabled={!online || !playState}
					onchange={(e) =>
						setSubtitle(options.find((o) => o.key === e.currentTarget.value)?.choice ?? null)}
					class="max-w-full min-w-0 rounded-md border border-neutral-700 bg-neutral-900 px-2 py-1"
				>
					<option value="">{strings.subtitleOff}</option>
					{#each options as o (o.key)}
						{@const missing =
							'stream' in o.choice &&
							prepare?.state === 'ready' &&
							!prepare.subtitles.includes(o.choice.stream)}
						<option value={o.key} disabled={o.unavailable !== '' || missing}>
							{subtitleLabel(o)}{missing ? ` — ${strings.notInCopy}` : ''}
						</option>
					{/each}
				</select>
				<div class="flex items-center gap-1">
					<span class="text-neutral-400">{strings.subtitleTiming}</span>
					<button
						onclick={() => setOffset(offsetMs - offsetStepMs)}
						disabled={!online || !playState}
						aria-label={strings.subtitleSooner}
						class="w-8 rounded-md border border-neutral-700 py-0.5 hover:bg-neutral-800 disabled:opacity-50"
					>
						−
					</button>
					<span class="w-16 text-center tabular-nums">{strings.subtitleOffset(offsetMs)}</span>
					<button
						onclick={() => setOffset(offsetMs + offsetStepMs)}
						disabled={!online || !playState}
						aria-label={strings.subtitleLater}
						class="w-8 rounded-md border border-neutral-700 py-0.5 hover:bg-neutral-800 disabled:opacity-50"
					>
						+
					</button>
					{#if offsetMs !== 0}
						<button
							onclick={() => setOffset(0)}
							disabled={!online || !playState}
							class="rounded-md px-2 py-0.5 text-neutral-300 hover:bg-neutral-800 disabled:opacity-50"
						>
							{strings.reset}
						</button>
					{/if}
				</div>
				<label class="flex items-center gap-2">
					<span class="text-neutral-400">{strings.subtitleSize}</span>
					<select
						bind:value={subtitleSize}
						class="rounded-md border border-neutral-700 bg-neutral-900 px-2 py-1"
					>
						{#each SUBTITLE_SIZES as size (size)}
							<option value={size}>{strings.subtitleSizes[size]}</option>
						{/each}
					</select>
				</label>
			</div>
		{/if}

		<!-- On a narrow screen the seek bar and time take a row of their own, above the buttons. -->
		<div class="flex flex-wrap items-center gap-x-2 gap-y-2 bg-neutral-900 px-3 py-2 text-sm sm:gap-x-3">
			<button
				onclick={() => intent({ type: playState?.playing ? 'pause' : 'play' })}
				disabled={!online || !playState}
				class="w-14 rounded-md border border-neutral-700 py-1 hover:bg-neutral-800 disabled:opacity-50"
			>
				{playState?.playing ? strings.pause : strings.play}
			</button>
			<div class="flex min-w-0 flex-1 items-center gap-3 max-sm:order-first max-sm:basis-full">
				<input
					type="range"
					aria-label={strings.position}
					min="0"
					max={durationMs}
					step="1000"
					value={dragMs ?? shownMs}
					disabled={!online || durationMs === 0}
					oninput={(e) => (dragMs = Number(e.currentTarget.value))}
					onchange={(e) => {
						dragMs = null;
						intent({ type: 'seek', positionMs: Number(e.currentTarget.value) });
					}}
					class="min-w-0 flex-1 accent-neutral-100"
				/>
				<span class="text-neutral-300 tabular-nums">
					{formatTime(dragMs ?? shownMs)} / {formatTime(durationMs)}
				</span>
			</div>
			<button
				onclick={() => (muted = !muted)}
				class="rounded-md border border-neutral-700 px-2 py-1 hover:bg-neutral-800"
			>
				{muted ? strings.unmute : strings.mute}
			</button>
			{#if volumeWorks}
				<input
					type="range"
					aria-label={strings.volume}
					min="0"
					max="1"
					step="0.05"
					bind:value={volume}
					class="hidden w-20 accent-neutral-100 sm:block"
				/>
			{/if}
			<button
				onclick={() => (subtitlesOpen = !subtitlesOpen)}
				aria-label={strings.subtitle}
				aria-pressed={subtitlesOpen}
				class="rounded-md border px-2 py-1 font-semibold hover:bg-neutral-800 {playState?.subtitle
					? 'border-neutral-300'
					: 'border-neutral-700 text-neutral-400'}"
			>
				{strings.subtitlesButton}
			</button>
			<button
				onclick={() => (chatOpen = !chatOpen)}
				aria-pressed={chatOpen}
				class="rounded-md border px-2 py-1 hover:bg-neutral-800 {chatOpen
					? 'border-neutral-300'
					: 'border-neutral-700 text-neutral-400'}"
			>
				{strings.chat}
			</button>
			<button
				onclick={toggleFullscreen}
				aria-label={full ? strings.exitFullscreen : strings.fullscreen}
				class="rounded-md border border-neutral-700 px-2 py-1 hover:bg-neutral-800"
			>
				<svg viewBox="0 0 16 16" class="size-4" fill="none" stroke="currentColor" stroke-width="1.5">
					{#if full}
						<path d="M6 1v5H1M10 1v5h5M6 15v-5H1M10 15v-5h5" />
					{:else}
						<path d="M1 6V1h5M15 6V1h-5M1 10v5h5M15 10v5h-5" />
					{/if}
				</svg>
			</button>
		</div>
	</div>

	{#if chatOpen}
		<!-- Landscape: the panel takes the row's height; absolute, so its messages never make the row
		taller. Portrait: a sheet over the page's lower part, or over the video in fullscreen. -->
		<aside
			class="z-10 border-neutral-800 landscape:relative landscape:w-80 landscape:shrink-0 landscape:border-l portrait:fixed portrait:inset-x-0 portrait:bottom-0 portrait:h-[50dvh] portrait:overflow-hidden portrait:rounded-t-xl portrait:border-t"
		>
			<div class="h-full landscape:absolute landscape:inset-0">
				{@render side()}
			</div>
		</aside>
	{/if}
</div>
