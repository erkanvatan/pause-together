<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		getLanguages,
		getVideo,
		listVideos,
		type Languages,
		type LibraryType,
		type Pick,
		type VideoDetail,
		type VideoSummary
	} from '$lib/api';
	import Dialog from '$lib/Dialog.svelte';
	import Icon from '$lib/Icon.svelte';
	import { me } from '$lib/me.svelte';
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
		videoTitle,
		whyUnplayable,
		withYear,
		type Sort
	} from '$lib/picker';
	import { strings } from '$lib/strings';

	let {
		onpick,
		onclose
	}: {
		onpick: (p: Pick, name: string) => void; // name: the video's, as videoName writes it
		onclose: () => void;
	} = $props();

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
		if (!r.ok || r.value.missing) {
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
		onpick(
			{
				videoId: picked.id,
				audio,
				subtitle: options.find((o) => o.key === subtitle)?.choice ?? null
			},
			videoName(picked)
		);
	}

	// A step that replaces the focused row (a video, show or folder opened, or Back) would drop focus
	// out of the dialog. Put it on the new step's first row or menu instead: not the search box, which
	// would open a phone's keyboard.
	let panel: HTMLDivElement;
	$effect(() => {
		void [picked, showKey, folderName];
		tick().then(() => {
			if (panel && !panel.contains(document.activeElement)) {
				(panel.querySelector<HTMLElement>('.row:not(:disabled)') ?? panel.querySelector('select'))?.focus();
			}
		});
	});

	const canGoBack = $derived(picked !== null || (!results && (show || folder)));

	// Escape steps back one level: out of a video, a search, a show or folder, then the picker. Handled
	// here, so the dialog doesn't close at once.
	function escape(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		if (picked) back();
		else if (query) query = '';
		else if (canGoBack) back();
		else onclose();
	}
</script>

<svelte:window onkeydown={escape} />

{#snippet videoRow(v: VideoSummary, label: string, code = '')}
	{@const why = whyUnplayable(v, canPlayType)}
	<li>
		<button disabled={why !== ''} onclick={() => choose(v)} class="row">
			<span class="min-w-0 flex-1">
				<span class="block break-words">
					{#if code}<span class="mr-2 text-haze tabular-nums">{code}</span>{/if}{label}
				</span>
				{#if why}
					<span class="block text-sm text-haze">{why}</span>
				{:else if v.appleOnly}
					<span class="block text-sm text-haze">{strings.appleOnlyPick}</span>
				{/if}
			</span>
		</button>
	</li>
{/snippet}

{#snippet folderRow(label: string, detail: string, open: () => void)}
	<li>
		<button onclick={open} class="row">
			<span class="min-w-0 flex-1">
				<span class="block break-words">{label}</span>
				{#if detail}
					<span class="block text-sm text-haze">{detail}</span>
				{/if}
			</span>
			<Icon name="chevron" class="size-5 shrink-0 text-haze" />
		</button>
	</li>
{/snippet}

<Dialog label={strings.pickVideo} {onclose} class="items-stretch justify-center sm:items-center sm:p-6">
	<!-- The list keeps one tall frame, so it doesn't jump while searching; the short track step fits
	its content. -->
	<div
		bind:this={panel}
		class="flex w-full flex-col bg-dusk sm:max-w-2xl sm:rounded-panel sm:border sm:border-line {picked
			? ''
			: 'sm:h-[85vh]'}"
	>
		<header class="flex items-center gap-1 border-b border-line p-1 pl-2">
			{#if canGoBack}
				<button onclick={back} class="btn gap-1 px-2 font-normal text-haze hover:text-moonlight">
					<Icon name="back" class="size-5" />
					{strings.back}
				</button>
			{/if}
			<h2 class="min-w-0 flex-1 truncate px-2 font-display text-lg font-bold">
				{#if picked}
					{videoTitle(picked)}
				{:else if show && !results}
					{withYear(show.title, show.year)}
				{:else if folder && !results}
					{folder.name}
				{:else}
					{strings.pickVideo}
				{/if}
			</h2>
			<button onclick={onclose} aria-label={strings.close} class="icon-btn text-haze">
				<Icon name="close" class="size-5" />
			</button>
		</header>

		{#if pickFailed}
			<p class="px-4 pt-3 text-ember" role="alert">{strings.actionFailed}</p>
		{/if}

		{#if picked}
			<div class="flex flex-1 flex-col gap-5 overflow-y-auto p-4">
				{#if picked.version}
					<p class="-mt-1 break-words text-haze">{picked.version}</p>
				{/if}
				<label class="flex flex-col gap-1.5">
					<span class="text-sm text-haze">{strings.audio}</span>
					<!-- One track is no choice: say what it is. -->
					{#if picked.audio.length === 0}
						<span>{strings.noAudio}</span>
					{:else if picked.audio.length === 1}
						<span>{audioLabel(picked.audio[0])}</span>
					{:else}
						<select bind:value={audio} onchange={audioChanged} class="field">
							{#each picked.audio as t (t.stream)}
								<option value={t.stream}>{audioLabel(t)}</option>
							{/each}
						</select>
					{/if}
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm text-haze">{strings.subtitle}</span>
					<select bind:value={subtitle} onchange={() => (subtitleTouched = true)} class="field">
						<option value="">{strings.subtitleOff}</option>
						{#each options as o (o.key)}
							{@const why = (strings.subtitleUnavailable as Record<string, string>)[o.unavailable]}
							<option value={o.key} disabled={o.unavailable !== ''}>
								{o.unavailable ? strings.withNote(subtitleLabel(o, options), why ?? o.unavailable) : subtitleLabel(o, options)}
							</option>
						{/each}
					</select>
				</label>
				<button onclick={start} class="btn btn-primary self-start">
					<Icon name="play" class="size-5" />
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
					class="field min-w-0 flex-1 bg-midnight"
				/>
				<select bind:value={sort} aria-label={strings.sort} class="field">
					<option value="title">{strings.sortTitle}</option>
					<option value="recent">{strings.sortRecent}</option>
				</select>
			</div>

			{#if !results && all.length > 1}
				<nav class="flex gap-5 border-b border-line px-4">
					{#each all as s (s.type)}
						<button
							onclick={() => openTab(s.type)}
							aria-pressed={s === shelf}
							class="-mb-px min-h-11 border-b-2 {s === shelf
								? 'border-lamp text-moonlight'
								: 'border-transparent text-haze hover:text-moonlight'}"
						>
							{strings.libraryTypes[s.type]}
						</button>
					{/each}
				</nav>
			{/if}

			<div class="flex-1 overflow-y-auto p-2 pb-4">
				{#if videos === null}
					{#if loadFailed}
						<p class="p-3 text-haze">{strings.loadFailed}</p>
					{/if}
				{:else if results?.length === 0}
					<p class="p-3 text-haze">{strings.noMatches}</p>
				{:else if results}
					<ul>
						{#each results as v (v.id)}
							{@render videoRow(v, videoName(v))}
						{/each}
					</ul>
				{:else if !shelf}
					<!-- The host's first visit lands here: point them at the one place that fixes it. -->
					{#if me.isAdmin}
						<div class="flex flex-col items-start gap-4 p-3">
							<p class="text-haze">{strings.noVideosHost}</p>
							<a href="/admin" class="btn btn-primary">{strings.addVideos}</a>
						</div>
					{:else}
						<p class="p-3 text-haze">{strings.noVideos}</p>
					{/if}
				{:else if shelf.type === 'movies'}
					<ul>
						{#each shelf.movies as v (v.id)}
							{@render videoRow(v, videoName(v))}
						{/each}
					</ul>
				{:else if shelf.type === 'tv'}
					{#if show}
						{#each show.seasons as season (season.number)}
							<h3 class="px-3 pt-4 pb-1 font-display text-lg font-bold text-haze">
								{strings.season(season.number)}
							</h3>
							<ul>
								{#each season.episodes as v (v.id)}
									{@render videoRow(v, v.episodeTitle, episodeCode(v))}
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
</Dialog>
