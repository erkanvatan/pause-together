<script lang="ts">
	// A first visit: what the app is, which room a room's link leads to, then the name.
	import { listRooms } from '$lib/api';
	import Brand from '$lib/Brand.svelte';
	import NameForm from '$lib/NameForm.svelte';
	import { roomTitle } from '$lib/rooms';
	import { strings } from '$lib/strings';

	let { roomId }: { roomId?: number } = $props();

	let room = $state(''); // the linked room's title, once known

	// The room list, not the room: opening a room would start preparing its video. Without it the form
	// still works, so a failure just leaves the line out. Back and Forward can change the room while
	// this shows: a late answer for the one before is dropped.
	$effect(() => {
		const id = roomId;
		room = '';
		if (id === undefined || !Number.isInteger(id)) return; // /rooms/abc: no such room
		let current = true;
		listRooms().then((r) => {
			const found = r.ok ? r.value.find((c) => c.id === id) : undefined;
			if (current && found) room = roomTitle(found);
		});
		return () => (current = false);
	});
</script>

<div class="flex w-full max-w-80 flex-col gap-4">
	<div class="mb-4 flex flex-col gap-1">
		<p class="flex items-center gap-1.5">
			<Brand />
		</p>
		<!-- The tagline is the brand's voice; the line under it says plainly what the app is. -->
		<p class="text-lg">{strings.tagline}</p>
		<p class="mt-1 text-haze">{strings.about}</p>
	</div>
	{#if room}
		<p class="break-words">{strings.joining(room)}</p>
	{/if}
	<NameForm label={strings.namePrompt} action={strings.continue} hint={strings.nameWhy} />
</div>
