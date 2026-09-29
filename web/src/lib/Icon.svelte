<script lang="ts">
	// Icons drawn inline: no icon library. Decorative only: the button or text around each one
	// carries its meaning.
	type Name =
		| 'play'
		| 'pause'
		| 'volume'
		| 'muted'
		| 'captions'
		| 'chat'
		| 'expand'
		| 'shrink'
		| 'close'
		| 'back'
		| 'chevron'
		| 'skip-back'
		| 'skip-forward';

	let {
		name,
		class: cls = 'size-6',
		step = 10
	}: {
		name: Name;
		class?: string;
		step?: number; // skip icons: the seconds inside the arrow
	} = $props();
</script>

<svg
	viewBox="0 0 24 24"
	class={cls}
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	{#if name === 'play'}
		<path d="M7.5 4.8v14.4a.8.8 0 0 0 1.2.7l11.6-7.2a.8.8 0 0 0 0-1.4L8.7 4.1a.8.8 0 0 0-1.2.7z" fill="currentColor" stroke="none" />
	{:else if name === 'pause'}
		<rect x="5.5" y="4" width="4.5" height="16" rx="1.2" fill="currentColor" stroke="none" />
		<rect x="14" y="4" width="4.5" height="16" rx="1.2" fill="currentColor" stroke="none" />
	{:else if name === 'volume' || name === 'muted'}
		<path d="M4 9.5h3.5L12 5.5v13l-4.5-4H4z" fill="currentColor" stroke="none" />
		{#if name === 'volume'}
			<path d="M15.5 9a4 4 0 0 1 0 6M18.5 6.5a7.5 7.5 0 0 1 0 11" />
		{:else}
			<path d="M16 9.5l5 5M21 9.5l-5 5" />
		{/if}
	{:else if name === 'captions'}
		<rect x="3" y="5.5" width="18" height="13" rx="2.5" />
		<path d="M10.5 10.3a2.2 2.2 0 1 0 0 3.4M17 10.3a2.2 2.2 0 1 0 0 3.4" />
	{:else if name === 'chat'}
		<path d="M4 5.5h16v10.5h-9.5L6 19.5V16H4z" />
	{:else if name === 'expand'}
		<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
	{:else if name === 'shrink'}
		<path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5" />
	{:else if name === 'close'}
		<path d="M6 6l12 12M18 6L6 18" />
	{:else if name === 'back'}
		<path d="M15 5l-7 7 7 7" />
	{:else if name === 'chevron'}
		<path d="M9 5l7 7-7 7" />
	{:else if name === 'skip-back' || name === 'skip-forward'}
		<!-- A circling arrow around the skip step. -->
		{#if name === 'skip-back'}
			<path d="M12 5a8 8 0 1 1-8 8M14.5 2.5 12 5l2.5 2.5" />
		{:else}
			<path d="M12 5a8 8 0 1 0 8 8M9.5 2.5 12 5l-2.5 2.5" />
		{/if}
		<text x="12" y="15.8" text-anchor="middle" font-size="7.5" font-weight="700" fill="currentColor" stroke="none">{step}</text>
	{/if}
</svg>
