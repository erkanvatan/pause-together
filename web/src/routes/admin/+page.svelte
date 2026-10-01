<script lang="ts">
	import { onMount } from 'svelte';
	import FolderPicker from '$lib/FolderPicker.svelte';
	import { getLanguages, type LibraryType } from '$lib/api';
	import {
		addLibrary,
		clearCache,
		formatBytes,
		getCache,
		jobText,
		listJobs,
		listLibraries,
		listProblems,
		maxUnusedDays,
		minUnusedDays,
		problemText,
		removeLibrary,
		rescanLibrary,
		setCache,
		setLanguages,
		type Jobs,
		type Library,
		type Problem,
		type Problems
	} from '$lib/admin';
	import { me } from '$lib/me.svelte';
	import { langName } from '$lib/picker';
	import { strings } from '$lib/strings';

	// How often the page asks again while a scan is queued or running, or the last load failed.
	const busyPollMs = 1000;
	// How often it asks otherwise, while the tab is visible: the file watcher and the timed rescan
	// change things without the page knowing.
	const idlePollMs = 5000;

	let libraries = $state<Library[]>([]);
	let problems = $state<Problems>({ skipped: [], unplayable: [], appleOnly: [] });
	let jobs = $state<Jobs | null>(null);
	let loaded = $state(false); // the libraries loaded at least once
	let loadFailed = $state(false);
	let actionFailed = $state(false);
	let visible = $state(true);

	let path = $state('');
	let type = $state<LibraryType>('movies');
	let adding = $state(false);
	let addError = $state('');

	let confirming = $state<number | null>(null); // library whose Remove waits for a yes
	let confirmingClear = $state(false); // Clear cache waits for a yes

	// Language defaults, as typed. Loaded once, not polled, so a poll never overwrites typing.
	let audioLang = $state('');
	let subtitleLangs = $state('');
	let langsLoaded = $state(false);
	let savingLangs = $state(false);
	let langsSaved = $state(false);
	let langsChanged = $state(false); // Save lights up only when there's something to save
	// Edits so far, so a save that returns doesn't mark later typing as saved.
	let langsEdits = 0;
	let langsError = $state('');
	const subtitleCodes = $derived(subtitleLangs.split(/[\s,]+/).filter(Boolean));

	// The cache clean-up setting, as typed. Loaded once, like the language defaults.
	let unusedDays = $state(0);
	let cacheLoaded = $state(false);
	let savingCache = $state(false);
	let cacheSaved = $state(false);
	let cacheChanged = $state(false);
	let cacheEdits = 0;
	let cacheError = $state('');

	const scanning = $derived(libraries.some((l) => l.scan.state !== ''));
	const libraryPath = $derived(new Map(libraries.map((l) => [l.id, l.path])));
	const problemCount = $derived(problems.skipped.length + problems.unplayable.length);

	// Libraries first, then problems: once the libraries say a scan is done, the problems read after
	// already include what it found.
	async function refresh() {
		const libs = await listLibraries();
		const probs = await listProblems();
		const js = await listJobs();
		const langs = langsLoaded ? null : await getLanguages();
		const cache = cacheLoaded ? null : await getCache();
		loadFailed = !libs.ok || !probs.ok || !js.ok || langs?.ok === false || cache?.ok === false;
		if (cache?.ok) {
			unusedDays = cache.value.unusedDays;
			cacheLoaded = true;
		}
		if (langs?.ok) {
			showLanguages(langs.value.audio, langs.value.subtitles);
			langsLoaded = true;
		}
		if (libs.ok) {
			libraries = libs.value;
			loaded = true;
		}
		if (probs.ok) problems = probs.value;
		if (js.ok) jobs = js.value;
	}

	onMount(() => {
		if (me.isAdmin) refresh();
	});

	function visibilityChanged() {
		visible = document.visibilityState === 'visible';
		if (visible && me.isAdmin) refresh();
	}

	// The next ask waits for the last answer, so answers can't pile up or land out of order.
	$effect(() => {
		if (!me.isAdmin || !visible) return;
		const pollMs = scanning || loadFailed ? busyPollMs : idlePollMs;
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		const tick = async () => {
			await refresh();
			if (!stopped) timer = setTimeout(tick, pollMs);
		};
		timer = setTimeout(tick, pollMs);
		return () => {
			stopped = true;
			clearTimeout(timer);
		};
	});

	async function act(result: Promise<{ ok: boolean }>) {
		actionFailed = !(await result).ok;
		await refresh();
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		adding = true;
		const r = await addLibrary(path, type);
		adding = false;
		addError = r.ok ? '' : (strings.addErrors[r.error] ?? strings.addErrors.failed);
		if (r.ok) path = '';
		await refresh();
	}

	function showLanguages(audio: string, subtitles: string[]) {
		audioLang = audio;
		subtitleLangs = subtitles.join(', ');
	}

	async function saveLanguages(e: SubmitEvent) {
		e.preventDefault();
		savingLangs = true;
		const edits = langsEdits;
		const r = await setLanguages({ audio: audioLang.trim(), subtitles: subtitleCodes });
		savingLangs = false;
		const current = langsEdits === edits; // nothing typed while it saved
		langsSaved = r.ok && current;
		langsChanged = !langsSaved;
		langsError = r.ok ? '' : (strings.langErrors[r.error] ?? strings.langErrors.failed);
		if (r.ok && current) showLanguages(r.value.audio, r.value.subtitles);
	}

	async function saveCache(e: SubmitEvent) {
		e.preventDefault();
		savingCache = true;
		const edits = cacheEdits;
		const r = await setCache({ unusedDays });
		savingCache = false;
		cacheSaved = r.ok && cacheEdits === edits;
		cacheChanged = !cacheSaved;
		cacheError = r.ok ? '' : (strings.cacheErrors[r.error] ?? strings.cacheErrors.failed);
	}

	function fullPath(p: Problem) {
		return `${libraryPath.get(p.libraryId) ?? '?'}/${p.path}`;
	}
</script>

<svelte:document onvisibilitychange={visibilityChanged} />

<svelte:head>
	<title>{strings.admin} · {strings.appName}</title>
</svelte:head>

{#snippet heading(text: string)}
	<h2 class="font-display text-xl font-bold">{text}</h2>
{/snippet}

{#if !me.isAdmin}
	<main class="mx-auto flex w-full max-w-6xl flex-col items-start gap-6 px-4 pt-16">
		<p class="font-display text-2xl font-bold">{strings.hostOnly}</p>
		<a href="/" class="btn btn-quiet">{strings.backHome}</a>
	</main>
{:else}
	<main class="mx-auto w-full max-w-6xl px-4 pt-2 pb-16">
		<div class="flex max-w-3xl flex-col gap-12">
			<h1 class="font-display text-2xl font-bold">{strings.admin}</h1>

			{#if loadFailed || actionFailed}
				<p class="text-ember" role="alert">
					{loadFailed ? strings.loadFailed : strings.actionFailed}
				</p>
			{/if}

			<section class="flex flex-col gap-3">
				{@render heading(strings.libraries)}
				{#each libraries as lib (lib.id)}
					<div class="flex flex-col gap-2 py-1">
						<div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
							<div class="min-w-0">
								<p class="font-semibold break-all">{lib.path}</p>
								<p class="text-sm text-haze">
									{strings.libraryTypes[lib.type]}, {strings.videoCount(lib.videos)}
								</p>
							</div>
							<div class="flex gap-2">
								<button
									onclick={() => act(rescanLibrary(lib.id))}
									disabled={lib.scan.state !== ''}
									class="btn btn-quiet btn-small"
								>
									{strings.rescan}
								</button>
								<button onclick={() => (confirming = lib.id)} class="btn btn-quiet btn-small">
									{strings.remove}
								</button>
							</div>
						</div>
						{#if lib.scan.state === 'queued'}
							<p class="text-sm text-haze">{strings.scanQueued}</p>
						{:else if lib.scan.state === 'scanning'}
							<div class="flex flex-col gap-1.5">
								<p class="text-sm text-haze">
									{lib.scan.total > 0
										? strings.scanProgress(lib.scan.done, lib.scan.total)
										: strings.scanLooking}
								</p>
								{#if lib.scan.total > 0}
									<div class="h-1 max-w-80 overflow-hidden rounded-full bg-dusk">
										<div
											class="h-full bg-lamp"
											style:width="{(lib.scan.done / lib.scan.total) * 100}%"
										></div>
									</div>
								{/if}
							</div>
						{/if}
						{#if lib.scan.gone}
							<p class="text-sm text-ember">{strings.folderGone}</p>
						{:else if lib.scan.error}
							<p class="text-sm text-ember">
								{strings.scanFailed} <span class="font-mono break-all">{lib.scan.error}</span>
							</p>
						{/if}
						{#if confirming === lib.id}
							<div class="flex flex-wrap items-center gap-2 text-sm">
								<span>{strings.removeConfirm}</span>
								<button
									onclick={() => {
										confirming = null;
										act(removeLibrary(lib.id));
									}}
									class="btn btn-danger btn-small"
								>
									{strings.remove}
								</button>
								<button onclick={() => (confirming = null)} class="btn btn-quiet btn-small">
									{strings.cancel}
								</button>
							</div>
						{/if}
					</div>
				{:else}
					{#if loaded}
						<p class="text-haze">{strings.noLibraries}</p>
					{/if}
				{/each}
			</section>

			<section class="flex flex-col gap-3">
				{@render heading(strings.addLibrary)}
				<form onsubmit={add} class="flex flex-col gap-3">
					<FolderPicker bind:path />
					<div class="flex flex-wrap items-center gap-3">
						<label class="flex items-center gap-2">
							<span class="text-haze">{strings.libraryType}</span>
							<select bind:value={type} class="field">
								{#each Object.entries(strings.libraryTypes) as [value, label] (value)}
									<option {value}>{label}</option>
								{/each}
							</select>
						</label>
						<button type="submit" disabled={adding || path === ''} class="btn btn-primary">
							{strings.add}
						</button>
						<span class="text-sm break-all text-haze">
							{path === '' ? strings.openFolderHint : `/${path}`}
						</span>
					</div>
					{#if addError}
						<p class="text-sm text-ember" role="alert">{addError}</p>
					{/if}
				</form>
			</section>

			{#if libraries.length > 0}
				<section class="flex flex-col gap-3">
					{@render heading(strings.cantUse)}
					{#if problemCount === 0}
						<p class="text-haze">{strings.allUsable}</p>
					{:else}
						<ul class="flex flex-col gap-3">
							{#each [...problems.skipped, ...problems.unplayable] as p (`${p.libraryId}/${p.path}`)}
								<li class="flex flex-col">
									<span class="break-all">{fullPath(p)}</span>
									<span class="text-sm text-haze">{problemText(p)}</span>
									{#if p.probeError}
										<span class="font-mono text-sm break-all text-haze">{p.probeError}</span>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</section>
			{/if}

			{#if problems.appleOnly.length > 0}
				<section class="flex flex-col gap-3">
					{@render heading(strings.appleOnly)}
					<p class="text-sm text-haze">{strings.appleOnlyNote}</p>
					<ul class="flex flex-col gap-1">
						{#each problems.appleOnly as p (`${p.libraryId}/${p.path}`)}
							<li class="break-all">{fullPath(p)}</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if jobs}
				<section class="flex flex-col gap-3">
					{@render heading(strings.jobs)}
					{#if jobs.jobs.length === 0}
						<p class="text-haze">{strings.noJobs}</p>
					{:else}
						<ul class="flex flex-col gap-3">
							{#each jobs.jobs as j (j.key)}
								<li class="flex flex-col">
									<span class="break-all">{j.name}</span>
									<span class="text-sm {j.state === 'failed' ? 'text-ember' : 'text-haze'}">
										{jobText(j)}
									</span>
									{#if j.detail}
										<span class="font-mono text-sm break-all whitespace-pre-wrap text-haze">
											{j.detail}
										</span>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</section>
			{/if}

			<div class="grid gap-12 md:grid-cols-2">
				{#if langsLoaded}
					<section class="flex flex-col gap-3">
						{@render heading(strings.languageDefaults)}
						<p class="text-sm text-haze">{strings.languageDefaultsNote}</p>
						<form
							onsubmit={saveLanguages}
							oninput={() => {
								langsSaved = false;
								langsChanged = true;
								langsEdits++;
							}}
							class="flex flex-col gap-4"
						>
							<label class="flex flex-col gap-1.5">
								<span>{strings.audioLanguage}</span>
								<input bind:value={audioLang} placeholder={strings.original} class="field max-w-xs" />
								<span class="text-sm text-haze">{strings.audioLanguageHint}</span>
								<span class="text-sm">
									{strings.readsAs(audioLang.trim() ? langName(audioLang.trim()) : strings.original)}
								</span>
							</label>
							<label class="flex flex-col gap-1.5">
								<span>{strings.subtitleLanguages}</span>
								<input bind:value={subtitleLangs} placeholder="tr, en" class="field max-w-xs" />
								<span class="text-sm text-haze">{strings.subtitleLanguagesHint}</span>
								<span class="text-sm">
									{strings.readsAs(
										subtitleCodes.length > 0 ? subtitleCodes.map(langName).join(', ') : strings.none
									)}
								</span>
							</label>
							<div class="flex items-center gap-3">
								<button type="submit" disabled={savingLangs || !langsChanged} class="btn btn-primary">
									{strings.save}
								</button>
								{#if langsSaved}
									<span class="text-sm text-haze">{strings.saved}</span>
								{/if}
							</div>
							{#if langsError}
								<p class="text-sm text-ember" role="alert">{langsError}</p>
							{/if}
						</form>
					</section>
				{/if}

				{#if cacheLoaded}
					<section class="flex flex-col gap-3">
						{@render heading(strings.cacheCleanup)}
						{#if jobs}
							<div class="flex flex-col items-start gap-3">
								<p class="text-sm text-haze">
									{strings.diskUsage(formatBytes(jobs.cacheBytes), formatBytes(jobs.freeBytes))}
								</p>
								{#if !confirmingClear}
									<!-- Quiet until asked: ember belongs to the confirm step. -->
									<button onclick={() => (confirmingClear = true)} class="btn btn-quiet btn-small">
										{strings.clearCache}
									</button>
								{/if}
							</div>
							{#if confirmingClear}
								<div class="flex flex-wrap items-center gap-2 text-sm" role="alert">
									<span>{strings.clearCacheConfirm}</span>
									<button
										onclick={() => {
											confirmingClear = false;
											act(clearCache());
										}}
										class="btn btn-danger btn-small"
									>
										{strings.clearCache}
									</button>
									<button onclick={() => (confirmingClear = false)} class="btn btn-quiet btn-small">
										{strings.cancel}
									</button>
								</div>
							{/if}
						{/if}
						<form
							onsubmit={saveCache}
							oninput={() => {
								cacheSaved = false;
								cacheChanged = true;
								cacheEdits++;
							}}
							class="flex flex-col gap-4"
						>
							<label class="flex flex-col gap-1.5">
								<span>{strings.unusedDays}</span>
								<input
									type="number"
									bind:value={unusedDays}
									min={minUnusedDays}
									max={maxUnusedDays}
									step="1"
									required
									class="field w-24"
								/>
								<span class="text-sm text-haze">{strings.unusedDaysHint}</span>
							</label>
							<div class="flex items-center gap-3">
								<button type="submit" disabled={savingCache || !cacheChanged} class="btn btn-primary">
									{strings.save}
								</button>
								{#if cacheSaved}
									<span class="text-sm text-haze">{strings.saved}</span>
								{/if}
							</div>
							{#if cacheError}
								<p class="text-sm text-ember" role="alert">{cacheError}</p>
							{/if}
						</form>
					</section>
				{/if}
			</div>
		</div>
	</main>
{/if}
