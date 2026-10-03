<script lang="ts">
	import type { RoomCard } from '$lib/api';
	import { roomJoinable, roomProgress, usedAgo } from '$lib/rooms';
	import { strings } from '$lib/strings';

	// A room's meta line, on the homepage's cards and in the picker's rooms to join: where the room is,
	// so a guest sees where they left off before opening it, and who's there.
	let { room: r, now }: { room: RoomCard; now: number } = $props(); // now: for "3 days ago"

	const joinable = $derived(roomJoinable(r));
	// Tells rooms with the same video apart, and says which one was on last.
	const ago = $derived(joinable ? '' : usedAgo(r.usedAt, now));
</script>

<span class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
	{#if r.gone}
		<span class="text-ember">{strings.videoMissing}</span>
	{:else}
		<span class="text-haze tabular-nums">{roomProgress(r)}</span>
	{/if}
	{#if ago}
		<span class="text-haze">{ago}</span>
	{/if}
	{#if joinable}
		<span class="flex min-w-0 items-center gap-2">
			<span class="size-2 shrink-0 rounded-full bg-lamp" aria-hidden="true"></span>
			<span class="min-w-0 break-words">
				{strings.watchingList(r.watching.map((w) => w.name).join(', '))}
			</span>
		</span>
	{/if}
</span>
