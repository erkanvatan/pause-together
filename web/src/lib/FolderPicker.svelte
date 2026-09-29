<script lang="ts">
	import { listFolders } from '$lib/admin';
	import Icon from '$lib/Icon.svelte';
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

<div class="overflow-hidden rounded-panel border border-line bg-dusk">
	<nav class="flex flex-wrap items-center gap-0.5 border-b border-line px-2 py-1 text-sm">
		<button type="button" onclick={() => up(0)} class="btn btn-small px-2 font-normal text-haze hover:text-moonlight">
			{strings.mediaFolder}
		</button>
		{#each parts as part, i (i)}
			<span class="text-haze" aria-hidden="true">/</span>
			<button
				type="button"
				onclick={() => up(i + 1)}
				class="btn btn-small px-2 font-normal break-all {i === parts.length - 1
					? 'text-moonlight'
					: 'text-haze hover:text-moonlight'}"
			>
				{part}
			</button>
		{/each}
	</nav>
	<ul class="max-h-64 overflow-y-auto p-1">
		{#if failed}
			<li class="px-3 py-2 text-sm text-ember">{strings.foldersFailed}</li>
		{:else}
			{#each folders as folder (folder)}
				<li>
					<button type="button" onclick={() => open(folder)} class="row">
						<span class="min-w-0 flex-1 truncate">{folder}</span>
						<Icon name="chevron" class="size-5 shrink-0 text-haze" />
					</button>
				</li>
			{:else}
				<li class="px-3 py-2 text-sm text-haze">{strings.noFolders}</li>
			{/each}
		{/if}
	</ul>
</div>
