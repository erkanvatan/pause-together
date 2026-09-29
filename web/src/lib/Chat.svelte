<script lang="ts">
	// A room's chat: the messages, and a line to write one. Text is shown as text, never as HTML.
	import { onMount, tick, untrack } from 'svelte';
	import type { ChatMessage } from '$lib/api';
	import { canSend, MAX_MESSAGE_CHARS, messageLength } from '$lib/chat';
	import { videoName } from '$lib/picker';
	import { strings } from '$lib/strings';
	import { formatTime } from '$lib/time';

	let {
		messages,
		more,
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
	let loading = false;
	let olderFailed = $state(false);
	let atBottom = true; // new messages keep the list at the bottom only when it's there

	const length = $derived(messageLength(draft));

	onMount(() => {
		lastId = messages.at(-1)?.id;
		list.scrollTop = list.scrollHeight;
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
	}

	function reply(m: ChatMessage) {
		selected = null;
		replyTo = m;
	}
</script>

<section class="flex h-full min-h-0 flex-col bg-neutral-950 text-sm">
	<header class="flex items-center justify-between border-b border-neutral-800 px-3 py-2">
		<h2 class="font-semibold">{strings.chat}</h2>
		{#if onclose}
			<button
				onclick={onclose}
				aria-label={strings.closeChat}
				class="rounded-md px-2 py-0.5 text-neutral-400 hover:bg-neutral-800"
			>
				✕
			</button>
		{/if}
	</header>

	<ol bind:this={list} onscroll={scrolled} class="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto p-2">
		{#if olderFailed}
			<li class="px-1 text-red-400">{strings.olderFailed}</li>
		{/if}
		{#if messages.length === 0}
			<li class="m-auto text-neutral-500">{strings.noMessages}</li>
		{/if}
		{#each messages as m (m.id)}
			<li class="rounded-md {selected === m.id ? 'bg-neutral-900' : ''}">
				<button
					onclick={() => (selected = selected === m.id ? null : m.id)}
					title={strings.sentAt(m.sentAt)}
					class="flex w-full flex-col gap-0.5 rounded-md px-2 py-1 text-left hover:bg-neutral-900"
				>
					<span class="flex flex-wrap items-baseline gap-x-2">
						<span class="font-semibold break-all">{m.from.name}</span>
						<span class="text-xs text-neutral-500 tabular-nums">
							{formatTime(m.positionMs)}{m.video.id === videoId ? '' : ` · ${videoName(m.video)}`}
						</span>
					</span>
					{#if m.replyTo}
						<span class="line-clamp-2 border-l-2 border-neutral-600 pl-2 break-words text-neutral-400">
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
					<div class="flex flex-wrap items-center gap-2 px-2 pb-1 text-xs">
						<span class="text-neutral-400">{strings.sentAt(m.sentAt)}</span>
						{#if !readOnly}
							<button
								onclick={() => reply(m)}
								class="rounded-md border border-neutral-700 px-2 py-0.5 hover:bg-neutral-800"
							>
								{strings.reply}
							</button>
							{#if m.from.userId === userId}
								<button
									onclick={() => ondelete(m.id)}
									disabled={!online}
									class="rounded-md border border-neutral-700 px-2 py-0.5 text-red-300 hover:bg-neutral-800 disabled:opacity-50"
								>
									{strings.deleteMessage}
								</button>
							{/if}
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ol>

	{#if readOnly}
		<p class="border-t border-neutral-800 px-3 py-2 text-neutral-400">{strings.chatReadOnly}</p>
	{:else}
		<form onsubmit={submit} class="flex flex-col gap-1 border-t border-neutral-800 p-2">
			{#if replyTo}
				<div class="flex items-start gap-2">
					<p class="line-clamp-2 min-w-0 flex-1 border-l-2 border-neutral-600 pl-2 break-words text-neutral-400">
						<span class="font-semibold">{strings.replyingTo(replyTo.from.name)}</span>
						{#if replyTo.text}{replyTo.text}{:else}<i>{strings.deletedMessage}</i>{/if}
					</p>
					<button
						type="button"
						onclick={() => (replyTo = null)}
						aria-label={strings.cancelReply}
						class="rounded-md px-2 py-0.5 text-neutral-400 hover:bg-neutral-800"
					>
						✕
					</button>
				</div>
			{/if}
			<div class="flex gap-2">
				<!-- No maxlength: it counts UTF-16 units, not characters. -->
				<input
					bind:this={input}
					bind:value={draft}
					placeholder={strings.messagePlaceholder}
					aria-label={strings.messagePlaceholder}
					class="min-w-0 flex-1 rounded-md border border-neutral-700 bg-neutral-900 px-3 py-1.5 outline-none focus:border-neutral-400"
				/>
				<button
					type="submit"
					disabled={!online || !canSend(draft)}
					class="rounded-md bg-neutral-100 px-3 py-1.5 font-medium text-neutral-950 disabled:opacity-50"
				>
					{strings.send}
				</button>
			</div>
			{#if length > MAX_MESSAGE_CHARS - 100}
				<p class="text-xs tabular-nums {length > MAX_MESSAGE_CHARS ? 'text-red-400' : 'text-neutral-400'}">
					{length} / {MAX_MESSAGE_CHARS}
				</p>
			{/if}
		</form>
	{/if}
</section>
