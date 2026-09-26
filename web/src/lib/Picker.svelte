<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getLanguages,
		getVideo,
		listVideos,
		type Languages,
		type LibraryType,
		type VideoDetail,
		type VideoSummary
	} from '$lib/api';
	import {
		audioLabel,
		defaultAudio,
		defaultSubtitle,
		episodeCode,
		search,
		searchIndex,
		shelves,
		subtitleLabel,
		subtitleOptions,
		videoName,
		whyUnplayable,
		withYear,
		type Pick,
		type Sort
	} from '$lib/picker';
	import { strings } from '$lib/strings';

	let { onpick, onclose }: { onpick: (p: Pick) => void; onclose: () => void } = $props();

	// How soon a failed load tries again.
	const loadRetryMs = 2000;

	// canPlayType answers the same for the life of the page, so each type is asked once.
	const probe = document.createElement('video');
	const answers = new Map<string, string>();
	function canPlayType(type: string): string {
		let a = answers.get(type);
		if (a === undefined) {
			a = probe.canPlayType(type);
			answers.set(type, a);
		}
		return a;
	}

	let videos = $state<VideoSummary[] | null>(null);
	let langs: Languages = { audio: '', subtitles: [] };
	let loadFailed = $state(false);
	let pickFailed = $state(false);

	let query = $state('');
	let sort = $state<Sort>('title');
	let tab = $state<LibraryType | null>(null);
	let showKey = $state<string | null>(null); // the open show
	let folderName = $state<string | null>(null); // the open folder

	let picked = $state<VideoDetail | null>(null);
	let audio = $state<number | null>(null); // stream
	let subtitle = $state(''); // a SubtitleOption key; '' = off
	let subtitleTouched = false; // the user chose a subtitle, so an audio change leaves it alone
	let pickSeq = 0; // only the latest video asked for may open

	const all = $derived(videos ? shelves(videos, sort) : []);
	const shelf = $derived(all.find((s) => s.type === tab) ?? all[0]);
	const index = $derived(videos ? searchIndex(videos) : []);
	const results = $derived(query.trim() ? search(index, query) : null);
	const show = $derived(
		shelf?.type === 'tv' ? shelf.shows.find((s) => s.key === showKey) : undefined
	);
	const folder = $derived(
		shelf?.type === 'other' ? shelf.folders.find((f) => f.name === folderName) : undefined
	);
	const options = $derived(picked ? subtitleOptions(picked) : []);

	onMount(() => {
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		const load = async () => {
			const [v, l] = await Promise.all([listVideos(), getLanguages()]);
			if (stopped) return;
			// Both or neither: a pick made without the host's defaults would quietly ignore them.
			loadFailed = !v.ok || !l.ok;
			if (v.ok && l.ok) {
				videos = v.value;
				langs = l.value;
			} else {
				timer = setTimeout(load, loadRetryMs);
			}
		};
		load();
		return () => {
			stopped = true;
			clearTimeout(timer);
		};
	});

	function openTab(t: LibraryType) {
		tab = t;
		showKey = null;
		folderName = null;
	}

	async function choose(v: VideoSummary) {
		pickFailed = false;
		const seq = ++pickSeq;
		const r = await getVideo(v.id);
		if (seq !== pickSeq) return; // another video was tapped, or Back, while this one loaded
		if (!r.ok) {
			pickFailed = true; // gone since the list loaded, or no connection
			return;
		}
		const d = r.value;
		const a = defaultAudio(d.audio, langs);
		picked = d;
		audio = a?.stream ?? null;
		subtitle = defaultSubtitle(subtitleOptions(d), a?.lang ?? '', langs)?.key ?? '';
		subtitleTouched = false;
	}

	// audioChanged picks the default subtitle again, since it can hang on the audio's language. It
	// reads the stream from the event: bind:value may not have updated `audio` yet.
	function audioChanged(e: Event & { currentTarget: HTMLSelectElement }) {
		if (subtitleTouched || !picked) return;
		const stream = Number(e.currentTarget.value);
		const lang = picked.audio.find((t) => t.stream === stream)?.lang ?? '';
		subtitle = defaultSubtitle(options, lang, langs)?.key ?? '';
	}

	function back() {
		pickFailed = false;
		pickSeq++;
		if (picked) picked = null;
		else if (showKey !== null) showKey = null;
		else folderName = null;
	}

	function start() {
		if (!picked) return;
		onpick({
			videoId: picked.id,
			audio,
			subtitle: options.find((o) => o.key === subtitle)?.choice ?? null
		});
	}

	const canGoBack = $derived(picked !== null || (!results && (show || folder)));

	// Escape steps back one level: out of a video, a search, a show or folder, then the picker.
	function escape(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (picked) back();
		else if (query) query = '';
		else if (canGoBack) back();
		else onclose();
	}
</script>

<svelte:window onkeydown={escape} />

{#snippet videoRow(v: VideoSummary, label: string)}
	{@const why = whyUnplayable(v, canPlayType)}
	<li>
		<button
			disabled={why !== ''}
			onclick={() => choose(v)}
			class="w-full rounded-md px-3 py-2 text-left hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
		>
			<span class="block break-words">{label}</span>
			{#if why}
				<span class="block text-sm text-neutral-400">{why}</span>
			{:else if v.appleOnly}
				<span class="block text-sm text-amber-400">{strings.appleOnlyPick}</span>
			{/if}
		</button>
	</li>
{/snippet}

{#snippet folderRow(label: string, detail: string, open: () => void)}
	<li>
		<button onclick={open} class="w-full rounded-md px-3 py-2 text-left hover:bg-neutral-800">
			<span class="block break-words">{label}</span>
			{#if detail}
				<span class="block text-sm text-neutral-400">{detail}</span>
			{/if}
		</button>
	</li>
{/snippet}

<div
	class="fixed inset-0 z-20 flex items-stretch justify-center bg-black/70 sm:items-center sm:p-6"
	role="dialog"
	aria-modal="true"
	aria-label={strings.pickVideo}
>
	<div
		class="flex w-full flex-col bg-neutral-900 sm:h-[85vh] sm:max-w-2xl sm:rounded-lg sm:border sm:border-neutral-800"
	>
		<header class="flex items-center gap-2 border-b border-neutral-800 p-3">
			{#if canGoBack}
				<button onclick={back} class="rounded-md px-2 py-1 text-neutral-300 hover:bg-neutral-800">
					← {strings.back}
				</button>
			{/if}
			<h2 class="min-w-0 flex-1 truncate font-semibold">
				{#if picked}
					{videoName(picked)}
				{:else if show && !results}
					{withYear(show.title, show.year)}
				{:else if folder && !results}
					{folder.name}
				{:else}
					{strings.pickVideo}
				{/if}
			</h2>
			<button
				onclick={onclose}
				aria-label={strings.close}
				class="rounded-md px-2.5 py-1 text-neutral-300 hover:bg-neutral-800"
			>
				✕
			</button>
		</header>

		{#if pickFailed}
			<p class="px-4 pt-3 text-sm text-red-400" role="alert">{strings.actionFailed}</p>
		{/if}

		{#if picked}
			<div class="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
				<label class="flex flex-col gap-1">
					<span class="text-sm text-neutral-400">{strings.audio}</span>
					{#if picked.audio.length === 0}
						<span>{strings.noAudio}</span>
					{:else}
						<select
							bind:value={audio}
							onchange={audioChanged}
							class="rounded-md border border-neutral-700 bg-neutral-900 px-2 py-2"
						>
							{#each picked.audio as t (t.stream)}
								<option value={t.stream}>{audioLabel(t)}</option>
							{/each}
						</select>
					{/if}
				</label>
				<label class="flex flex-col gap-1">
					<span class="text-sm text-neutral-400">{strings.subtitle}</span>
					<select
						bind:value={subtitle}
						onchange={() => (subtitleTouched = true)}
						class="rounded-md border border-neutral-700 bg-neutral-900 px-2 py-2"
					>
						<option value="">{strings.subtitleOff}</option>
						{#each options as o (o.key)}
							{@const why = (strings.subtitleUnavailable as Record<string, string>)[o.unavailable]}
							<option value={o.key} disabled={o.unavailable !== ''}>
								{subtitleLabel(o)}{o.unavailable ? ` — ${why ?? o.unavailable}` : ''}
							</option>
						{/each}
					</select>
				</label>
				<button
					onclick={start}
					class="self-start rounded-md bg-neutral-100 px-4 py-2 font-medium text-neutral-950"
				>
					{strings.start}
				</button>
			</div>
		{:else}
			<div class="flex flex-wrap items-center gap-2 p-3">
				<input
					type="search"
					bind:value={query}
					placeholder={strings.search}
					aria-label={strings.search}
					class="min-w-0 flex-1 rounded-md border border-neutral-700 bg-neutral-950 px-3 py-2"
				/>
				<select
					bind:value={sort}
					class="rounded-md border border-neutral-700 bg-neutral-900 px-2 py-2 text-sm"
				>
					<option value="title">{strings.sortTitle}</option>
					<option value="recent">{strings.sortRecent}</option>
				</select>
			</div>

			{#if !results && all.length > 1}
				<nav class="flex gap-1 px-3 pb-2">
					{#each all as s (s.type)}
						<button
							onclick={() => openTab(s.type)}
							aria-pressed={s === shelf}
							class="rounded-md px-3 py-1.5 text-sm {s === shelf
								? 'bg-neutral-100 text-neutral-950'
								: 'text-neutral-300 hover:bg-neutral-800'}"
						>
							{strings.libraryTypes[s.type]}
						</button>
					{/each}
				</nav>
			{/if}

			<div class="flex-1 overflow-y-auto px-1 pb-4">
				{#if videos === null}
					{#if loadFailed}
						<p class="p-3 text-neutral-400">{strings.loadFailed}</p>
					{/if}
				{:else if results?.length === 0}
					<p class="p-3 text-neutral-400">{strings.noMatches}</p>
				{:else if results}
					<ul>
						{#each results as v (v.id)}
							{@render videoRow(v, videoName(v))}
						{/each}
					</ul>
				{:else if !shelf}
					<p class="p-3 text-neutral-400">{strings.noVideos}</p>
				{:else if shelf.type === 'movies'}
					<ul>
						{#each shelf.movies as v (v.id)}
							{@render videoRow(v, videoName(v))}
						{/each}
					</ul>
				{:else if shelf.type === 'tv'}
					{#if show}
						{#each show.seasons as season (season.number)}
							<h3 class="px-3 pt-3 pb-1 text-sm font-semibold text-neutral-400">
								{strings.season(season.number)}
							</h3>
							<ul>
								{#each season.episodes as v (v.id)}
									{@render videoRow(
										v,
										[episodeCode(v), v.episodeTitle].filter(Boolean).join(' · ')
									)}
								{/each}
							</ul>
						{/each}
					{:else}
						<ul>
							{#each shelf.shows as s (s.key)}
								{@render folderRow(withYear(s.title, s.year), strings.seasonCount(s.seasons.length), () => (showKey = s.key))}
							{/each}
						</ul>
					{/if}
				{:else if folder}
					<ul>
						{#each folder.videos as v (v.id)}
							{@render videoRow(v, v.title)}
						{/each}
					</ul>
				{:else}
					<ul>
						{#each shelf.folders as f (f.name)}
							{#if f.name === ''}
								{#each f.videos as v (v.id)}
									{@render videoRow(v, v.title)}
								{/each}
							{:else}
								{@render folderRow(f.name, strings.videoCount(f.videos.length), () => (folderName = f.name))}
							{/if}
						{/each}
					</ul>
				{/if}
			</div>
		{/if}
	</div>
</div>
