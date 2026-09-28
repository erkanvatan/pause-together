<script lang="ts">
	// The room's player: one <video> for the page's life, "Tap to join", our own controls, and the loop
	// that keeps the video with the room. <video> events are status only; only the controls send intents.
	import { onMount, untrack } from 'svelte';
	import type { Room } from '$lib/api';
	import { codecName, whyUnplayable } from '$lib/picker';
	import { loadPlayerPrefs, savePlayerPrefs } from '$lib/prefs';
	import type { Prepare, RoomState } from '$lib/protocol';
	import type { RoomSocket } from '$lib/socket';
	import { strings } from '$lib/strings';
	import { follow, type Step } from '$lib/sync/drift';
	import { local, running, target, type Intent } from '$lib/sync/state';
	import { reportDue, statusOf, type Report } from '$lib/sync/status';
	import { FOLLOW_EVERY_MS, MEDIA_RETRY_MS } from '$lib/sync/timing';
	import { formatTime } from '$lib/time';

	let {
		room,
		prepare,
		playState = $bindable(),
		socket,
		online,
		note
	}: {
		room: Room;
		prepare: Prepare | null;
		playState: RoomState | null; // the server's, or ours applied on top until the next one arrives
		socket: RoomSocket | null;
		online: boolean; // the socket said hello and hasn't dropped since
		note: string; // "Alice paused"
	} = $props();

	let video: HTMLVideoElement;
	const src = $derived(prepare?.state === 'ready' ? `/stream/${prepare.key}/video.mp4` : '');

	const prefs = loadPlayerPrefs(() => localStorage);
	let volume = $state(prefs.volume);
	let muted = $state(prefs.muted);
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

	$effect(() => savePlayerPrefs(() => localStorage, { volume, muted }));

	onMount(() => {
		const timer = setInterval(tick, FOLLOW_EVERY_MS);
		const visibility = () => {
			hidden = document.hidden;
			tick();
		};
		document.addEventListener('visibilitychange', visibility);
		return () => {
			clearInterval(timer);
			clearTimeout(retryTimer);
			document.removeEventListener('visibilitychange', visibility);
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

<div class="flex flex-col overflow-hidden rounded-md bg-black">
	<div class="relative">
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
			class="aspect-video w-full"
		></video>

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

		<div class="pointer-events-none absolute top-2 left-2 flex flex-col items-start gap-1 text-sm">
			{#if note}
				<p class="rounded-md bg-black/70 px-2 py-1 break-words">{note}</p>
			{/if}
			{#each playState?.behind ?? [] as b (b.userId)}
				<p class="rounded-md bg-black/70 px-2 py-1 break-words">{strings.behind(b.name, b.ms)}</p>
			{/each}
		</div>
	</div>

	<div class="flex items-center gap-3 bg-neutral-900 px-3 py-2 text-sm">
		<button
			onclick={() => intent({ type: playState?.playing ? 'pause' : 'play' })}
			disabled={!online || !playState}
			class="w-14 rounded-md border border-neutral-700 py-1 hover:bg-neutral-800 disabled:opacity-50"
		>
			{playState?.playing ? strings.pause : strings.play}
		</button>
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
	</div>
</div>
