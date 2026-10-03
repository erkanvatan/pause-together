<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import Brand from '$lib/Brand.svelte';
	import Dialog from '$lib/Dialog.svelte';
	import Icon from '$lib/Icon.svelte';
	import Menu from '$lib/Menu.svelte';
	import NameForm from '$lib/NameForm.svelte';
	import { loadMe, me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let { children } = $props();

	// The room page spans the whole window, so its video can use a big screen. The header lines up with it,
	// and the page fills the window's height, so a portrait chat reaches the bottom edge.
	const wide = $derived(page.route.id === '/rooms/[id]');

	let loadFailed = $state(false);
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

<div class="min-h-dvh {wide ? 'flex flex-col' : ''}">
	{#if loadFailed}
		<main class="flex min-h-screen items-center justify-center p-4">
			<p class="text-haze">{strings.loadFailed}</p>
		</main>
	{:else if !me.loaded}
		<!-- Loading: show nothing rather than a flash of the name picker. -->
	{:else if me.name === null}
		<main class="flex min-h-dvh items-center justify-center p-4">
			<NameForm roomId={wide ? Number(page.params.id) : undefined} />
		</main>
	{:else}
		<header class="mx-auto flex w-full {wide ? '' : 'max-w-6xl'} items-center justify-between gap-4 px-4 py-3">
			<a href="/" class="-mx-1 flex min-h-11 items-center gap-1.5 rounded-control px-1">
				<Brand />
			</a>
			<div class="flex min-w-0 items-center gap-1">
				<!-- In plain sight, not only in the name menu: a host who just set this up looks for it first. A
				narrow window keeps it in the menu, so the name has room. -->
				{#if me.isAdmin}
					<a
						href="/admin"
						aria-current={page.route.id === '/admin' ? 'page' : undefined}
						class="btn hidden shrink-0 px-3 font-normal text-haze hover:text-moonlight aria-[current=page]:text-moonlight sm:inline-flex"
					>
						{strings.admin}
					</a>
				{/if}
				<Menu buttonClass="btn max-w-full px-3 font-normal text-haze hover:text-moonlight">
					{#snippet button()}
						<span class="truncate">{me.name}</span>
						<Icon name="chevron" class="size-4 shrink-0 rotate-90" />
					{/snippet}
					{#snippet items(close)}
						<button
							onclick={() => {
								close();
								renaming = true;
							}}
							class="row"
						>
							{strings.rename}
						</button>
						{#if me.isAdmin}
							<a href="/admin" onclick={close} class="row sm:hidden">{strings.admin}</a>
						{/if}
					{/snippet}
				</Menu>
			</div>
		</header>
		{@render children()}
		<!-- A dialog over the page, not a page of its own: a room page stays, so a rename mid-movie never
		leaves the room. -->
		{#if renaming}
			<Dialog label={strings.rename} onclose={closeRename} class="items-center justify-center p-4">
				<div class="flex w-full max-w-sm justify-center rounded-panel border border-line bg-dusk p-5">
					<NameForm initial={me.name} ondone={closeRename} oncancel={closeRename} />
				</div>
			</Dialog>
		{/if}
	{/if}
</div>
