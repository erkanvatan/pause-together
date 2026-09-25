<script lang="ts">
	import { listFolders } from '$lib/admin';
	import { strings } from '$lib/strings';

	// path is the folder the picker is in, relative to the media folder ('' is the media folder).
	// That folder is the one picked.
	let { path = $bindable('') }: { path?: string } = $props();

	let folders = $state<string[]>([]);
	let failed = $state(false);

	const parts = $derived(path === '' ? [] : path.split('/'));

	$effect(() => {
		load(path);
	});

	async function load(p: string) {
		const r = await listFolders(p);
		if (p !== path) return; // the picker moved on meanwhile
		failed = !r.ok;
		folders = r.ok ? r.value : [];
	}

	function up(depth: number) {
		path = parts.slice(0, depth).join('/');
	}

	function open(name: string) {
		path = path === '' ? name : `${path}/${name}`;
	}
</script>

<div class="rounded-md border border-neutral-800">
	<nav class="flex flex-wrap items-center gap-1 border-b border-neutral-800 px-2 py-1.5 text-sm">
		<button type="button" onclick={() => up(0)} class="rounded px-1.5 py-0.5 hover:bg-neutral-800">
			{strings.mediaFolder}
		</button>
		{#each parts as part, i (i)}
			<span class="text-neutral-600">/</span>
			<button
				type="button"
				onclick={() => up(i + 1)}
				class="rounded px-1.5 py-0.5 hover:bg-neutral-800"
			>
				{part}
			</button>
		{/each}
	</nav>
	<ul class="max-h-64 overflow-y-auto py-1">
		{#if failed}
			<li class="px-3 py-2 text-sm text-red-400">{strings.foldersFailed}</li>
		{:else}
			{#each folders as folder (folder)}
				<li>
					<button
						type="button"
						onclick={() => open(folder)}
						class="flex w-full items-center justify-between px-3 py-1.5 text-left hover:bg-neutral-800"
					>
						<span class="truncate">{folder}</span>
						<span class="text-neutral-500">›</span>
					</button>
				</li>
			{:else}
				<li class="px-3 py-2 text-sm text-neutral-500">{strings.noFolders}</li>
			{/each}
		{/if}
	</ul>
</div>
