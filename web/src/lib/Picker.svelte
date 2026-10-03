<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		getLanguages,
		getVideo,
		listVideos,
		type Languages,
		type LibraryType,
		type Pick,
		type RoomCard,
		type VideoDetail,
		type VideoSummary
	} from '$lib/api';
	import Dialog from '$lib/Dialog.svelte';
	import Icon from '$lib/Icon.svelte';
	import { me } from '$lib/me.svelte';
	import RoomMeta from '$lib/RoomMeta.svelte';
	import { roomProgress, roomsByVideo } from '$lib/rooms';
	import {
		audioLabel,
		defaultAudio,
		defaultSubtitle,
		episodeCode,
		search,
		searchIndex,
		shelves,
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
		onclose,
		rooms = [],
		now = 0
	}: {
		onpick: (p: Pick, name: string) => void; // name: the video's, as videoName writes it
		onclose: () => void;
		rooms?: RoomCard[]; // making a room: a pick of a video that has one offers to join it first
		now?: number; // when rooms loaded, for "3 days ago"
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

	const byVideo = $derived(roomsByVideo(rooms));
	let offered = $state<VideoDetail | null>(null); // a video with rooms: join one, or make another
	const offeredRooms = $derived((offered && byVideo.get(offered.id)) || []);
	let picked = $state<VideoDetail | null>(null);
	let audio = $state<number | null>(null); // stream
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
			pickSeq++; // closed while a video loaded: a one-track video would still pick itself
		};
	});

	// Moving on (a search, another tab, a show or folder) drops a video still loading, for the same reason.
	$effect(() => {
		void [query, tab, showKey, folderName];
		pickSeq++;
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
		if (byVideo.has(r.value.id)) offered = r.value;
		else startNew(r.value);
	}

	function startNew(d: VideoDetail) {
		const a = defaultAudio(d.audio, langs);
		// Each audio track is its own prepared copy, so only a choice of them is asked. The subtitle
		// takes the default and can be changed in the player.
		if (d.audio.length < 2) {
			pick(d, a?.stream ?? null);
			return;
		}
		picked = d;
		audio = a?.stream ?? null;
	}

	// The rooms are polled: the last one offered may switch away, archive or go. Then the tap stands
	// as a plain pick again, back in the list.
	$effect(() => {
		if (offered && !picked && offeredRooms.length === 0) offered = null;
	});

	function back() {
		pickFailed = false;
		pickSeq++;
		if (picked) picked = null;
		else if (offered) offered = null;
		else if (showKey !== null) showKey = null;
		else folderName = null;
	}

	function pick(d: VideoDetail, stream: number | null) {
		const lang = d.audio.find((t) => t.stream === stream)?.lang ?? '';
		const subtitle = defaultSubtitle(subtitleOptions(d), lang, langs)?.choice ?? null;
		onpick({ videoId: d.id, audio: stream, subtitle }, videoName(d));
	}

	// A step that replaces the focused row (a video, show or folder opened, or Back) would drop focus
	// out of the dialog. Put it on the new step's first row or menu instead: not the search box, which
	// would open a phone's keyboard.
	let panel: HTMLDivElement;
	$effect(() => {
		void [picked, offered, showKey, folderName];
		tick().then(() => {
			if (panel && !panel.contains(document.activeElement)) {
				(panel.querySelector<HTMLElement>('.row:not(:disabled)') ?? panel.querySelector('select'))?.focus();
			}
		});
	});

	const canGoBack = $derived(picked !== null || offered !== null || (!results && (show || folder)));

	// Escape steps back one level: out of a video, a search, a show or folder, then the picker. Handled
	// here, so the dialog doesn't close at once.
	function escape(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		if (picked || offered) back();
		else if (query) query = '';
		else if (canGoBack) back();
		else onclose();
	}
</script>

<svelte:window onkeydown={escape} />

{#snippet videoRow(v: VideoSummary, label: string, code = '')}
	{@const why = whyUnplayable(v, canPlayType)}
	{@const vRooms = (!why && byVideo.get(v.id)) || []}
	<li>
		<button disabled={why !== ''} onclick={() => choose(v)} class="row">
			<span class="min-w-0 flex-1">
				<span class="block break-words">
					{#if code}<span class="mr-2 text-haze tabular-nums">{code}</span>{/if}{label}
				</span>
				<!-- Release tags ("1080p.BrRip.x264") are noise in a title: a quiet line of their own. -->
				{#if v.version}
					<span class="block text-sm break-words text-haze">{v.version}</span>
				{/if}
				<!-- Screen readers hear the reason in the button's name; the line that shows it is below. -->
				{#if why}
					<span class="sr-only">{why}</span>
				{:else if v.appleOnly}
					<span class="block text-sm text-haze">{strings.appleOnlyPick}</span>
				{/if}
				{#if vRooms.length > 0}
					<span class="block text-sm text-haze tabular-nums">
						{vRooms.length === 1 ? strings.inRoom(roomProgress(vRooms[0])) : strings.inRooms(vRooms.length)}
					</span>
				{/if}
			</span>
		</button>
		<!-- Outside the button, so the greying leaves the reason readable: it's what the guest needs. -->
		{#if why}
			<p aria-hidden="true" class="-mt-2 px-3 pb-2 text-sm text-haze">{why}</p>
		{/if}
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
	<!-- The list keeps one tall frame, so it doesn't jump while searching; the short room and track
	steps fit their content. -->
	<div
		bind:this={panel}
		class="flex w-full flex-col bg-dusk sm:max-w-2xl sm:rounded-panel sm:border sm:border-line {picked ||
		offered
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
				{:else if offered}
					{videoTitle(offered)}
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
					<select bind:value={audio} class="field">
						{#each picked.audio as t (t.stream)}
							<option value={t.stream}>{audioLabel(t)}</option>
						{/each}
					</select>
				</label>
				<button onclick={() => picked && pick(picked, audio)} class="btn btn-primary self-start">
					<Icon name="play" class="size-5" />
					{strings.start}
				</button>
			</div>
		{:else if offered}
			{@const tracks = offered.audio.length > 1 ? offered.audio : []}
			<div class="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
				{#if offered.version}
					<p class="-mt-1 break-words text-haze">{offered.version}</p>
				{/if}
				<p>{strings.videoHasRooms(offeredRooms.length)}</p>
				<ul class="-mx-2">
					{#each offeredRooms as r (r.id)}
						<!-- A room's audio track never changes, so with a choice of them, say which it plays. -->
						{@const track = tracks.find((t) => t.stream === r.audio)}
						<li>
							<a href="/rooms/{r.id}" class="row">
								<span class="min-w-0 flex-1">
									{#if r.name}
										<span class="block break-words">{r.name}</span>
									{/if}
									{#if track}
										<span class="block break-words">{audioLabel(track)}</span>
									{/if}
									<RoomMeta room={r} {now} />
								</span>
								<Icon name="chevron" class="size-5 shrink-0 text-haze" />
							</a>
						</li>
					{/each}
				</ul>
				<button onclick={() => offered && startNew(offered)} class="btn btn-quiet self-start">
					{strings.startNewRoom}
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
							{@render videoRow(v, videoTitle(v))}
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
							{@render videoRow(v, videoTitle(v))}
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
