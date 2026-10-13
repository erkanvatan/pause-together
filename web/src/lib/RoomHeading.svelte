<script lang="ts">
	// A room as a card names it, on the homepage and on a room link's welcome: its title in big type
	// (haze when archived), the quiet line under it, and where it is and who's watching. Spans, so it
	// fits inside a link; a hover on a group parent underlines the title in lamp.
	import type { RoomCard } from '$lib/api';
	import RoomMeta from '$lib/RoomMeta.svelte';
	import { roomSubtitle, roomTitle } from '$lib/rooms';

	let {
		room: r,
		now,
		titleId,
		metaId,
		watching = true
	}: {
		room: RoomCard;
		now: number; // for "3 days ago"
		// So a field can be described by the room's title, where it is and who's in it.
		titleId?: string;
		metaId?: string;
		watching?: boolean; // false where the page says who's watching itself
	} = $props();

	const subtitle = $derived(roomSubtitle(r));
</script>

<span
	id={titleId}
	class="block font-display text-xl font-bold break-words decoration-lamp decoration-2 underline-offset-4 group-hover:underline {r.archived
		? 'text-haze'
		: ''}"
>
	{roomTitle(r)}
</span>
{#if subtitle}
	<span class="mt-1 block break-words text-haze">{subtitle}</span>
{/if}
<span id={metaId} class="mt-1 block"><RoomMeta room={r} {now} {watching} /></span>
