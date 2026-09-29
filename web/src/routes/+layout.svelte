<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import Icon from '$lib/Icon.svelte';
	import NameForm from '$lib/NameForm.svelte';
	import { loadMe, me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let { children } = $props();

	// The room page spans the whole window, so its video can use a big screen. The header lines up with it.
	const wide = $derived(page.route.id === '/rooms/[id]');

	let loadFailed = $state(false);
	let menuOpen = $state(false);
	let renaming = $state(false);

	// The page loaded, so the server was just up: a failure is likely brief (a restart, a busy
	// database). Keep trying instead of leaving the tab stuck.
	const loadRetryMs = 2000;

	// The phone's browser bar takes the page's color, read from the tokens.
	const themeColor = getComputedStyle(document.documentElement)
		.getPropertyValue('--color-midnight')
		.trim();

	async function load() {
		try {
			await loadMe();
			loadFailed = false;
		} catch {
			loadFailed = true;
			setTimeout(load, loadRetryMs);
		}
	}

	onMount(load);

	function closeRename() {
		renaming = false;
	}
</script>

<svelte:head>
	<meta name="theme-color" content={themeColor} />
</svelte:head>

<div class="min-h-screen">
	{#if loadFailed}
		<main class="flex min-h-screen items-center justify-center p-4">
			<p class="text-haze">{strings.loadFailed}</p>
		</main>
	{:else if !me.loaded}
		<!-- Loading: show nothing rather than a flash of the name picker. -->
	{:else if me.name === null}
		<main class="flex min-h-dvh items-center justify-center p-4">
			<NameForm />
		</main>
	{:else if renaming}
		<main class="flex min-h-dvh items-center justify-center p-4">
			<NameForm initial={me.name} ondone={closeRename} oncancel={closeRename} />
		</main>
	{:else}
		<header class="mx-auto flex w-full {wide ? '' : 'max-w-6xl'} items-center justify-between gap-4 px-4 py-3">
			<a href="/" class="-mx-1 flex min-h-11 items-center gap-1.5 rounded-control px-1">
				<Icon name="pause" class="size-6 text-lamp" />
				<span class="font-display text-xl font-bold">{strings.appName}</span>
			</a>
			<div class="relative min-w-0">
				<button
					onclick={() => (menuOpen = !menuOpen)}
					aria-expanded={menuOpen}
					aria-haspopup="menu"
					class="btn max-w-full px-3 font-normal text-haze hover:text-moonlight"
				>
					<span class="truncate">{me.name}</span>
					<Icon name="chevron" class="size-4 shrink-0 rotate-90" />
				</button>
				{#if menuOpen}
					<div
						role="menu"
						class="absolute right-0 z-20 mt-1 min-w-48 rounded-control border border-line bg-dusk p-1"
					>
						<button
							role="menuitem"
							onclick={() => {
								menuOpen = false;
								renaming = true;
							}}
							class="row"
						>
							{strings.rename}
						</button>
						{#if me.isAdmin}
							<a role="menuitem" href="/admin" onclick={() => (menuOpen = false)} class="row">
								{strings.admin}
							</a>
						{/if}
					</div>
				{/if}
			</div>
		</header>
		{@render children()}
	{/if}
</div>
