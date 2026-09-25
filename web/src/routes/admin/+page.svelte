<script lang="ts">
	import { onMount } from 'svelte';
	import FolderPicker from '$lib/FolderPicker.svelte';
	import {
		addLibrary,
		listLibraries,
		listProblems,
		problemText,
		removeLibrary,
		rescanLibrary,
		type Library,
		type LibraryType,
		type Problem,
		type Problems
	} from '$lib/admin';
	import { me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	// How often the page asks again while a scan is queued or running, or the last load failed.
	const pollMs = 1000;

	let libraries = $state<Library[]>([]);
	let problems = $state<Problems>({ skipped: [], unplayable: [], appleOnly: [] });
	let loaded = $state(false); // the libraries loaded at least once
	let loadFailed = $state(false);
	let actionFailed = $state(false);

	let path = $state('');
	let type = $state<LibraryType>('movies');
	let adding = $state(false);
	let addError = $state('');

	let confirming = $state<number | null>(null); // library whose Remove waits for a yes

	const scanning = $derived(libraries.some((l) => l.scan.state !== ''));
	const libraryPath = $derived(new Map(libraries.map((l) => [l.id, l.path])));
	const problemCount = $derived(problems.skipped.length + problems.unplayable.length);

	// Libraries first, then problems: once the libraries say a scan is done, the problems read after
	// already include what it found.
	async function refresh() {
		const libs = await listLibraries();
		const probs = await listProblems();
		loadFailed = !libs.ok || !probs.ok;
		if (libs.ok) {
			libraries = libs.value;
			loaded = true;
		}
		if (probs.ok) problems = probs.value;
	}

	onMount(() => {
		if (me.isAdmin) refresh();
	});

	// The next ask waits for the last answer, so answers can't pile up or land out of order.
	$effect(() => {
		if (!scanning && !loadFailed) return;
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

	function fullPath(p: Problem) {
		return `${libraryPath.get(p.libraryId) ?? '?'}/${p.path}`;
	}
</script>

<svelte:head>
	<title>{strings.admin} · {strings.appName}</title>
</svelte:head>

{#if !me.isAdmin}
	<main class="flex flex-col items-center gap-3 pt-24">
		<p class="text-neutral-400">{strings.hostOnly}</p>
		<a href="/" class="underline">{strings.backHome}</a>
	</main>
{:else}
	<main class="mx-auto flex max-w-3xl flex-col gap-10 p-4 pb-16">
		<h1 class="text-2xl font-bold">{strings.admin}</h1>

		{#if loadFailed || actionFailed}
			<p class="text-sm text-red-400" role="alert">
				{loadFailed ? strings.loadFailed : strings.actionFailed}
			</p>
		{/if}

		<section class="flex flex-col gap-3">
			<h2 class="text-lg font-semibold">{strings.libraries}</h2>
			{#each libraries as lib (lib.id)}
				<div class="flex flex-col gap-2 rounded-md border border-neutral-800 p-3">
					<div class="flex flex-wrap items-baseline justify-between gap-2">
						<div class="min-w-0">
							<span class="font-medium break-all">{lib.path}</span>
							<span class="ml-2 text-sm text-neutral-400">
								{strings.libraryTypes[lib.type]} · {strings.videoCount(lib.videos)}
							</span>
						</div>
						<div class="flex gap-2 text-sm">
							<button
								onclick={() => act(rescanLibrary(lib.id))}
								disabled={lib.scan.state !== ''}
								class="rounded-md border border-neutral-700 px-2.5 py-1 hover:bg-neutral-800 disabled:opacity-40"
							>
								{strings.rescan}
							</button>
							<button
								onclick={() => (confirming = lib.id)}
								class="rounded-md border border-neutral-700 px-2.5 py-1 hover:bg-neutral-800"
							>
								{strings.remove}
							</button>
						</div>
					</div>
					{#if lib.scan.state === 'queued'}
						<p class="text-sm text-neutral-400">{strings.scanQueued}</p>
					{:else if lib.scan.state === 'scanning'}
						<p class="text-sm text-neutral-400">
							{lib.scan.total > 0
								? strings.scanProgress(lib.scan.done, lib.scan.total)
								: strings.scanLooking}
						</p>
					{/if}
					{#if lib.scan.error}
						<p class="text-sm text-red-400">
							{strings.scanFailed} <span class="font-mono break-all">{lib.scan.error}</span>
						</p>
					{/if}
					{#if confirming === lib.id}
						<div class="flex flex-wrap items-center gap-2 text-sm">
							<span class="text-neutral-300">{strings.removeConfirm}</span>
							<button
								onclick={() => {
									confirming = null;
									act(removeLibrary(lib.id));
								}}
								class="rounded-md bg-red-600 px-2.5 py-1 font-medium text-white"
							>
								{strings.remove}
							</button>
							<button
								onclick={() => (confirming = null)}
								class="rounded-md border border-neutral-700 px-2.5 py-1"
							>
								{strings.cancel}
							</button>
						</div>
					{/if}
				</div>
			{:else}
				{#if loaded}
					<p class="text-neutral-400">{strings.noLibraries}</p>
				{/if}
			{/each}
		</section>

		<section class="flex flex-col gap-3">
			<h2 class="text-lg font-semibold">{strings.addLibrary}</h2>
			<form onsubmit={add} class="flex flex-col gap-3">
				<FolderPicker bind:path />
				<div class="flex flex-wrap items-center gap-3">
					<label class="flex items-center gap-2">
						<span class="text-neutral-400">{strings.libraryType}</span>
						<select
							bind:value={type}
							class="rounded-md border border-neutral-700 bg-neutral-900 px-2 py-1.5"
						>
							{#each Object.entries(strings.libraryTypes) as [value, label] (value)}
								<option {value}>{label}</option>
							{/each}
						</select>
					</label>
					<button
						type="submit"
						disabled={adding || path === ''}
						class="rounded-md bg-neutral-100 px-3 py-1.5 font-medium text-neutral-950 disabled:opacity-40"
					>
						{strings.add}
					</button>
					<span class="text-sm break-all text-neutral-400">/{path}</span>
				</div>
				{#if addError}
					<p class="text-sm text-red-400" role="alert">{addError}</p>
				{/if}
			</form>
		</section>

		{#if libraries.length > 0}
			<section class="flex flex-col gap-3">
				<h2 class="text-lg font-semibold">{strings.cantUse}</h2>
				{#if problemCount === 0}
					<p class="text-neutral-400">{strings.allUsable}</p>
				{:else}
					<ul class="flex flex-col gap-2">
						{#each [...problems.skipped, ...problems.unplayable] as p (`${p.libraryId}/${p.path}`)}
							<li class="flex flex-col">
								<span class="break-all">{fullPath(p)}</span>
								<span class="text-sm text-neutral-400">{problemText(p)}</span>
								{#if p.probeError}
									<span class="font-mono text-xs break-all text-neutral-500">{p.probeError}</span>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}

		{#if problems.appleOnly.length > 0}
			<section class="flex flex-col gap-3">
				<h2 class="text-lg font-semibold">{strings.appleOnly}</h2>
				<p class="text-sm text-neutral-400">{strings.appleOnlyNote}</p>
				<ul class="flex flex-col gap-1">
					{#each problems.appleOnly as p (`${p.libraryId}/${p.path}`)}
						<li class="break-all">{fullPath(p)}</li>
					{/each}
				</ul>
			</section>
		{/if}
	</main>
{/if}
