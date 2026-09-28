<script lang="ts">
	// The room's subtitle, drawn over the video by us, not as native captions: the offset is a plain
	// time shift, and we choose where the text sits. Cue text is shown as text; only <i> and <b> are
	// real elements.
	import type { SubtitleSize } from '$lib/prefs';
	import { strings } from '$lib/strings';
	import { cuesAt, parseVtt, type Cue } from '$lib/subtitles';
	import { MEDIA_RETRY_MS } from '$lib/sync/timing';

	let {
		url,
		offsetMs,
		size,
		videoMs
	}: {
		url: string; // '' = none
		offsetMs: number; // positive shows the text later
		size: SubtitleSize;
		videoMs: () => number; // where the video is now
	} = $props();

	// Relative to the player's width, so fullscreen makes the text bigger; never below a readable size.
	const sizes: Record<SubtitleSize, string> = {
		small: 'text-[length:max(0.8rem,2.4cqi)]',
		medium: 'text-[length:max(0.9rem,3.2cqi)]',
		large: 'text-[length:max(1rem,4.2cqi)]'
	};

	let shown = $state.raw<Cue[]>([]);
	// Why nothing shows: 'gone' (the server has no such file), 'retrying' (no connection), or ''.
	let failed = $state<'' | 'gone' | 'retrying'>('');

	// Loads the subtitle, then checks every frame which cues show, so each shows when its time comes.
	// The DOM changes only when the cues on screen do. No subtitle, no frame loop.
	$effect(() => {
		const u = url;
		shown = [];
		failed = '';
		if (!u) return;
		const abort = new AbortController();
		let timer: ReturnType<typeof setTimeout>;
		let frame = 0;
		const draw = (cues: Cue[]) => {
			const now = cuesAt(cues, videoMs(), offsetMs);
			if (now.length !== shown.length || now.some((c, k) => c !== shown[k])) shown = now;
			frame = requestAnimationFrame(() => draw(cues));
		};
		const load = () =>
			fetch(u, { signal: abort.signal })
				.then(async (r) => {
					if (r.status === 404) {
						failed = 'gone';
						return;
					}
					if (!r.ok) throw new Error(`${r.status}`);
					const cues = parseVtt(await r.text());
					failed = '';
					draw(cues);
				})
				.catch(() => {
					if (abort.signal.aborted) return;
					failed = 'retrying';
					timer = setTimeout(load, MEDIA_RETRY_MS);
				});
		load();
		return () => {
			abort.abort();
			clearTimeout(timer);
			cancelAnimationFrame(frame);
		};
	});
</script>

{#if failed}
	<p class="pointer-events-none absolute top-2 right-2 rounded-md bg-black/70 px-2 py-1 text-sm">
		{failed === 'gone' ? strings.subtitleFailed : strings.subtitleRetrying}
	</p>
{/if}

<div
	class="pointer-events-none absolute inset-x-0 bottom-[6%] flex flex-col items-center gap-0.5 px-[5%] text-center leading-snug {sizes[
		size
	]}"
>
	{#each shown as cue (cue)}
		{#each cue.lines as line, n (n)}
			<p class="rounded bg-black/65 px-[0.4em] break-words whitespace-pre-wrap text-white">
				{#each line as s, k (k)}{#if s.i && s.b}<i><b>{s.text}</b></i>{:else if s.i}<i>{s.text}</i
						>{:else if s.b}<b>{s.text}</b>{:else}{s.text}{/if}{/each}
			</p>
		{/each}
	{/each}
</div>
