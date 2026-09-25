<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import NameForm from '$lib/NameForm.svelte';
	import { loadMe, me } from '$lib/me.svelte';
	import { strings } from '$lib/strings';

	let { children } = $props();

	let loadFailed = $state(false);
	let menuOpen = $state(false);
	let renaming = $state(false);

	// The page loaded, so the server was just up: a failure is likely brief (a restart, a busy
	// database). Keep trying instead of leaving the tab stuck.
	const loadRetryMs = 2000;

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

<div class="min-h-screen bg-neutral-950 text-neutral-100">
	{#if loadFailed}
		<main class="flex min-h-screen items-center justify-center p-4">
			<p class="text-neutral-400">{strings.loadFailed}</p>
		</main>
	{:else if !me.loaded}
		<!-- Loading: show nothing rather than a flash of the name picker. -->
	{:else if me.name === null}
		<main class="flex min-h-screen items-center justify-center p-4">
			<NameForm />
		</main>
	{:else if renaming}
		<main class="flex min-h-screen items-center justify-center p-4">
			<NameForm initial={me.name} ondone={closeRename} oncancel={closeRename} />
		</main>
	{:else}
		<header class="flex items-center justify-between p-3">
			<a href="/" class="px-1 font-semibold">{strings.appName}</a>
			<div class="relative">
				<button
					onclick={() => (menuOpen = !menuOpen)}
					aria-expanded={menuOpen}
					aria-haspopup="menu"
					class="rounded-md px-3 py-1.5 text-neutral-300 hover:bg-neutral-800"
				>
					{me.name} ▾
				</button>
				{#if menuOpen}
					<div
						role="menu"
						class="absolute right-0 z-10 mt-1 min-w-40 rounded-md border border-neutral-800 bg-neutral-900 py-1"
					>
						<button
							role="menuitem"
							onclick={() => {
								menuOpen = false;
								renaming = true;
							}}
							class="w-full px-3 py-2 text-left hover:bg-neutral-800"
						>
							{strings.rename}
						</button>
						{#if me.isAdmin}
							<a
								role="menuitem"
								href="/admin"
								onclick={() => (menuOpen = false)}
								class="block w-full px-3 py-2 text-left hover:bg-neutral-800"
							>
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
