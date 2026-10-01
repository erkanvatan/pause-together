<script lang="ts">
	// A room's chat: the messages, and a line to write one. Text is shown as text, never as HTML.
	import { onMount, tick, untrack } from 'svelte';
	import type { ChatMessage, Who } from '$lib/api';
	import { canSend, MAX_MESSAGE_CHARS, messageLength } from '$lib/chat';
	import Icon from '$lib/Icon.svelte';
	import { videoName } from '$lib/picker';
	import { strings } from '$lib/strings';
	import { formatTime } from '$lib/time';

	let {
		messages,
		more,
		watching,
		userId,
		videoId,
		readOnly,
		online,
		replyTo = $bindable(),
		draft = $bindable(),
		onsend,
		ondelete,
		onolder,
		onclose
	}: {
		messages: ChatMessage[]; // oldest first
		more: boolean; // older ones may be on the server
		watching: Who[];
		userId: number; // this user's
		videoId: number; // the room's video now; other videos are named on their messages
		readOnly: boolean; // an archived room
		online: boolean;
		replyTo: ChatMessage | null;
		draft: string;
		onsend: (text: string) => void;
		ondelete: (id: number) => void;
		onolder: () => Promise<boolean>; // false: loading failed
		onclose?: () => void;
	} = $props();

	// How close to an edge counts as at it, in pixels.
	const edgePx = 48;

	let list: HTMLOListElement;
	let input = $state<HTMLInputElement>();
	let selected = $state<number | null>(null); // the tapped message, showing its time and actions
	let confirming = $state(false); // the selected message's Delete was pressed: ask before deleting
	// The one message in the tab order; the arrow keys move it. null: the newest.
	let current = $state<number | null>(null);
	const focusable = $derived(
		messages.some((m) => m.id === current) ? current : (messages.at(-1)?.id ?? null)
	);
	let loading = false;
	let olderFailed = $state(false);
	let atBottom = true; // new messages keep the list at the bottom only when it's there

	const length = $derived(messageLength(draft));

	onMount(() => {
		lastId = messages.at(-1)?.id;
		list.scrollTop = list.scrollHeight;
		// A shorter list (leaving fullscreen, a rotated phone) keeps its scroll spot, which hides the
		// last messages. Stay at the bottom when it was there.
		const resized = new ResizeObserver(() => {
			if (atBottom) list.scrollTop = list.scrollHeight;
		});
		resized.observe(list);
		return () => resized.disconnect();
	});

	// A new message at the end: follow it when at the bottom, or when it's our own. Older pages added in
	// front leave the last one as it was.
	let lastId: number | undefined;
	$effect.pre(() => {
		const last = messages.at(-1);
		untrack(() => {
			if (!list || !last || last.id === lastId) return;
			lastId = last.id;
			const follow = atBottom || last.from.userId === userId;
			tick().then(() => {
				if (follow) list.scrollTop = list.scrollHeight;
			});
		});
	});

	// Replying starts writing.
	$effect(() => {
		if (replyTo) untrack(() => input?.focus());
	});

	async function scrolled() {
		atBottom = list.scrollHeight - list.scrollTop - list.clientHeight < edgePx;
		if (list.scrollTop > edgePx * 4 || !more || loading) return;
		loading = true;
		const height = list.scrollHeight;
		const ok = await onolder();
		olderFailed = !ok;
		await tick();
		if (!list) return; // the chat was closed meanwhile
		// Keep the messages on screen where they were, above the ones just added.
		list.scrollTop += list.scrollHeight - height;
		loading = false;
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!online || !canSend(draft)) return;
		onsend(draft.trim());
		// On touch screens, close the keyboard so the video shows again.
		if (matchMedia('(pointer: coarse)').matches) input?.blur();
	}

	// The messages are one stop for Tab, like a list box: ↑ and ↓ step through them, Home and End jump
	// to the ends. Moving focus scrolls the message into view.
	function listKey(e: KeyboardEvent) {
		const step = { ArrowUp: -1, ArrowDown: 1, Home: -Infinity, End: Infinity }[e.key];
		const at = messages.findIndex((m) => m.id === focusable);
		if (step === undefined || at < 0 || !(e.target as HTMLElement).dataset.message) return;
		e.preventDefault();
		const to = messages[Math.min(Math.max(at + step, 0), messages.length - 1)];
		current = to.id;
		tick().then(() => list.querySelector<HTMLElement>(`[data-message="${to.id}"]`)?.focus());
	}

	function select(id: number) {
		selected = selected === id ? null : id;
		confirming = false;
	}

	function reply(m: ChatMessage) {
		selected = null;
		replyTo = m;
	}
</script>

<section class="flex h-full min-h-0 flex-col bg-dusk">
	<header class="flex items-center justify-between py-1 pr-1 pl-4">
		<h2 class="font-display text-lg font-bold">{strings.chat}</h2>
		{#if onclose}
			<button onclick={onclose} aria-label={strings.closeChat} class="icon-btn text-haze">
				<Icon name="close" class="size-5" />
			</button>
		{/if}
	</header>

	<!-- Who's here, like a chat app's online list. Two lines at most, then it scrolls, so it never
	pushes the messages off a phone. -->
	{#if watching.length > 0}
		<section aria-label={strings.watchingNow} class="shrink-0 border-b border-line px-4 pb-2 text-sm">
			<!-- The label flows with the names, so they wrap under it at full width. -->
			<!-- Focusable, so a keyboard can scroll it. -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<ul tabindex="0" class="flex max-h-[calc(2lh+0.25rem)] flex-wrap gap-x-1.5 gap-y-1 overflow-y-auto [scrollbar-color:var(--color-line)_transparent] [scrollbar-width:thin]">
				<li class="mr-1.5 flex items-center gap-2 text-haze" aria-hidden="true">
					<span class="size-2 rounded-full bg-lamp"></span>
					{strings.watchingNow}
				</li>
				<!-- Each name on its own chip: names can hold spaces, so a plain gap can't tell two apart. -->
				{#each watching as w (w.userId)}
					<li title={w.name} class="max-w-full truncate rounded-control bg-midnight px-2 {w.userId === userId ? 'text-lamp' : ''}">
						{w.name}
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- svelte-ignore a11y_no_noninteractive_element_interactions (the keys move between its buttons) -->
	<ol bind:this={list} onscroll={scrolled} onkeydown={listKey} class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 pb-2">
		{#if olderFailed}
			<li class="px-2 text-sm text-ember">{strings.olderFailed}</li>
		{/if}
		{#if messages.length === 0}
			<li class="m-auto text-haze">{strings.noMessages}</li>
		{/if}
		{#each messages as m, i (m.id)}
			<li class="rounded-control {selected === m.id ? 'bg-midnight/60' : ''}">
				<button
					data-message={m.id}
					tabindex={m.id === focusable ? 0 : -1}
					onfocus={() => (current = m.id)}
					onclick={() => select(m.id)}
					title={strings.sentAt(m.sentAt)}
					class="group flex w-full flex-col gap-0.5 rounded-control px-2 py-1.5 text-left hover:bg-midnight/40"
				>
					<span class="flex flex-wrap items-baseline gap-x-2 text-sm">
						<span class="font-bold break-all {m.from.userId === userId ? 'text-lamp' : ''}">
							{m.from.name}
						</span>
						<!-- Not before the video starts: every such message would say 0:00. -->
						{#if m.positionMs >= 1000}
							<span class="text-haze tabular-nums">{formatTime(m.positionMs)}</span>
						{/if}
						{#if m.video.id !== videoId}
							<span class="min-w-0 break-words text-haze">{videoName(m.video)}</span>
						{/if}
						<!-- A hint that a message opens Reply: on hover and focus, and on a touch screen's newest
						message, where nothing hovers. -->
						{#if !readOnly && selected !== m.id}
							<Icon
								name="reply"
								class="ml-auto size-4 shrink-0 self-center text-haze opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 {i ===
								messages.length - 1
									? 'pointer-coarse:opacity-100'
									: ''}"
							/>
						{/if}
					</span>
					{#if m.replyTo}
						<span class="line-clamp-2 border-l-2 border-line pl-2 text-sm break-words text-haze">
							{#if m.replyTo.deleted}
								<i>{strings.deletedMessage}</i>
							{:else}
								<span class="font-semibold">{m.replyTo.from.name}</span> {m.replyTo.text}
							{/if}
						</span>
					{/if}
					<span class="break-words whitespace-pre-wrap">{m.text}</span>
				</button>
				{#if selected === m.id}
					<div class="flex flex-wrap items-center gap-2 px-2 pb-2 text-sm">
						<span class="text-haze">{strings.sentAt(m.sentAt)}</span>
						{#if !readOnly}
							<button onclick={() => reply(m)} class="btn btn-quiet btn-small">
								{strings.reply}
							</button>
							{#if m.from.userId === userId}
								{#if confirming}
									<span class="flex basis-full flex-wrap items-center gap-2">
										<span>{strings.deleteMessageConfirm}</span>
										<button
											onclick={() => ondelete(m.id)}
											disabled={!online}
											class="btn btn-danger btn-small"
										>
											{strings.deleteMessage}
										</button>
										<button onclick={() => (confirming = false)} class="btn btn-quiet btn-small">
											{strings.cancel}
										</button>
									</span>
								{:else}
									<button
										onclick={() => (confirming = true)}
										disabled={!online}
										class="btn btn-quiet btn-small text-ember"
									>
										{strings.deleteMessage}
									</button>
								{/if}
							{/if}
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ol>

	{#if readOnly}
		<p class="border-t border-line px-4 py-3 text-sm text-haze">{strings.chatReadOnly}</p>
	{:else}
		<form onsubmit={submit} class="flex flex-col gap-2 border-t border-line p-2">
			{#if replyTo}
				<div class="flex items-start gap-2">
					<p class="line-clamp-2 min-w-0 flex-1 border-l-2 border-lamp pl-2 text-sm break-words text-haze">
						<span class="font-semibold text-moonlight">{strings.replyingTo(replyTo.from.name)}</span>
						{#if replyTo.text}{replyTo.text}{:else}<i>{strings.deletedMessage}</i>{/if}
					</p>
					<button
						type="button"
						onclick={() => (replyTo = null)}
						aria-label={strings.cancelReply}
						class="icon-btn -my-2 text-haze"
					>
						<Icon name="close" class="size-4" />
					</button>
				</div>
			{/if}
			<div class="flex gap-2">
				<!-- No maxlength: it counts UTF-16 units, not characters. -->
				<input
					bind:this={input}
					bind:value={draft}
					placeholder={strings.messagePlaceholder}
					enterkeyhint="send"
					aria-label={strings.messagePlaceholder}
					class="field min-w-0 flex-1 bg-midnight"
				/>
				<button type="submit" disabled={!online || !canSend(draft)} class="btn btn-primary">
					{strings.send}
				</button>
			</div>
			{#if length > MAX_MESSAGE_CHARS - 100}
				<p class="text-sm tabular-nums {length > MAX_MESSAGE_CHARS ? 'text-ember' : 'text-haze'}">
					{length} / {MAX_MESSAGE_CHARS}
				</p>
			{/if}
		</form>
	{/if}
</section>
