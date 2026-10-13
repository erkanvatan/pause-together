<script lang="ts">
	// A first visit: what the app is, which room a room's link leads to, then the name.
	import { listRooms, type RoomCard } from '$lib/api';
	import Brand from '$lib/Brand.svelte';
	import NameForm from '$lib/NameForm.svelte';
	import RoomHeading from '$lib/RoomHeading.svelte';
	import { untrack } from 'svelte';
	import { roomJoinable, roomTitle } from '$lib/rooms';
	import { strings } from '$lib/strings';

	let { roomId }: { roomId?: number } = $props();

	// A room's link waits this long for its room before the form shows without it. Shown at once, the
	// room arriving late would push the field down while the guest types. Not asked again: a guest is
	// here for a few seconds, and a card that changes would move the field too.
	const ROOM_WAIT_MS = 1500;

	let room = $state<RoomCard | null>(null); // the linked room, once known
	let now = $state(0); // when it loaded, for "3 days ago"
	let ready = $state(false); // the form shows: the room is known, or not coming in time
	const id = $props.id();

	// The room list, not the room: opening a room would start preparing its video. Without it the form
	// still works, so a failure just leaves the room out.
	$effect(() => {
		const linked = roomId;
		room = null;
		if (linked === undefined || !Number.isInteger(linked)) {
			ready = true; // /rooms/abc: no such room
			return;
		}
		// Only the first room waits for the form. Back and Forward change the page anyway, so the next
		// room may move it.
		const first = !untrack(() => ready);
		let current = true;
		const waited = first ? setTimeout(() => (ready = true), ROOM_WAIT_MS) : undefined;
		listRooms().then((r) => {
			if (!current || (first && ready)) return; // late: the form is up without it
			clearTimeout(waited);
			room = (r.ok && r.value.find((c) => c.id === linked)) || null;
			now = Date.now();
			ready = true;
		});
		return () => {
			current = false;
			clearTimeout(waited);
		};
	});
</script>

<svelte:head>
	<title>{room ? strings.pageTitle(strings.joinRoom(roomTitle(room))) : strings.appName}</title>
</svelte:head>

<div class="flex w-full max-w-80 flex-col gap-4">
	<div class="mb-4 flex flex-col gap-1">
		<p class="flex items-center gap-1.5">
			<Brand />
		</p>
		<!-- The tagline is the brand's voice; the line under it says plainly what the app is. -->
		<p class="text-lg">{strings.tagline}</p>
		<p id="{id}-about" class="mt-1 text-haze">{strings.about}</p>
	</div>
	{#if room}
		<!-- The room as the homepage shows it, so the guest sees who's waiting for them. -->
		<div class="relative mb-4 {roomJoinable(room) ? 'pl-5' : ''}">
			{#if roomJoinable(room)}
				<!-- The lamp is on: someone is in this room. -->
				<span class="absolute inset-y-0 left-0 w-1 rounded-full bg-lamp" aria-hidden="true"></span>
			{/if}
			<p id="{id}-room" class="sr-only">{strings.joining(roomTitle(room))}</p>
			<RoomHeading {room} {now} metaId="{id}-meta" />
		</div>
	{/if}
	{#if ready}
		<!-- The field takes the focus at once: the lines above are read out with it, not skipped. -->
		<NameForm
			label={strings.namePrompt}
			heading="h1"
			action={strings.continue}
			hint={strings.nameWhy}
			describedby={room ? `${id}-about ${id}-room ${id}-meta` : `${id}-about`}
		/>
	{/if}
</div>
