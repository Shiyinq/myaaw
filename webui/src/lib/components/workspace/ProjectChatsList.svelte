<script lang="ts">
	import { untrack } from 'svelte';
	import { Loader2, MessageSquarePlus } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatConversationTimestamp, groupConversationsByDay } from './projectDateGroups';
	import type { ConversationSummary } from '$lib/types';

	const PAGE_SIZE = 25;

	interface Props {
		agent: string;
		workingDir: string;
		activeId?: string;
		workingDirExists?: boolean;
		refreshKey?: number;
		onSelectConversation?: (id: string) => void;
		onNewChatInProject?: () => void;
	}

	let {
		agent,
		workingDir,
		activeId = '',
		workingDirExists = true,
		refreshKey = 0,
		onSelectConversation,
		onNewChatInProject
	}: Props = $props();

	const { t, locale } = useTranslation();

	let items = $state<ConversationSummary[]>([]);
	let hasMore = $state(false);
	let loading = $state(false);
	let loadingMore = $state(false);
	let refreshing = false;
	let error = $state('');
	let loadedKey = '';
	let seenHistoryRevision = untrack(() => conversationHistory.revision);

	let scrollRoot = $state<HTMLElement | null>(null);
	let sentinel = $state<HTMLElement | null>(null);

	// Conversations whose run is still streaming in the background. The open one
	// is skipped: its own timeline already shows the progress.
	const streaming = $derived(
		new Set(
			chatRuns
				.all()
				.filter((run) => run.streaming && run.agent === agent && run.conversationId !== '')
				.map((run) => run.conversationId)
		)
	);

	const dayGroups = $derived(
		groupConversationsByDay(
			items,
			locale.value,
			t('myaaw.focus.history_today'),
			t('myaaw.focus.history_yesterday')
		)
	);

	$effect(() => {
		const key = `${agent}:${workingDir}:${refreshKey}`;
		if (!agent || !workingDir) {
			items = [];
			hasMore = false;
			loadedKey = '';
			return;
		}
		if (key === loadedKey) return;
		loadedKey = key;
		seenHistoryRevision = conversationHistory.revision;
		untrack(() => {
			void loadPage(true);
		});
	});

	// Stay in sync silently when a background run finishes, renames, or updates message counts.
	$effect(() => {
		const rev = conversationHistory.revision;
		if (rev === seenHistoryRevision) return;
		seenHistoryRevision = rev;
		if (!agent || !workingDir || !loadedKey) return;
		untrack(() => {
			void loadPage(true, true);
		});
	});

	$effect(() => {
		const el = sentinel;
		const root = scrollRoot;
		if (!el || !hasMore) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					void loadPage(false);
				}
			},
			{ root, rootMargin: '140px' }
		);
		observer.observe(el);
		return () => observer.disconnect();
	});

	async function loadPage(reset: boolean, silent = false) {
		if (!agent || !workingDir) return;
		if (reset) {
			if (silent) {
				if (refreshing || loading) return;
				refreshing = true;
			} else {
				if (loading) return;
				loading = true;
				error = '';
			}
		} else {
			if (loading || loadingMore || !hasMore) return;
			loadingMore = true;
		}
		const unreadRev = unreadChats.revision();
		try {
			const offset = reset ? 0 : items.length;
			const res = await conversationsStore.fetchList(agent, {
				working_dir: workingDir,
				limit: PAGE_SIZE,
				offset
			});
			const incoming = res.conversations ?? [];
			unreadChats.observe(agent, incoming, unreadRev);
			if (reset) {
				if (silent && items.length > PAGE_SIZE) {
					const incomingIds = new Set(incoming.map((chat) => chat.id));
					const tail = items.filter((chat) => !incomingIds.has(chat.id));
					items = [...incoming, ...tail];
					hasMore = Boolean(res.has_more) || hasMore;
				} else {
					items = incoming;
					hasMore = Boolean(res.has_more);
				}
			} else {
				const seen = new Set(items.map((chat) => chat.id));
				const appended = incoming.filter((chat) => !seen.has(chat.id));
				items = [...items, ...appended];
				hasMore = appended.length > 0 && Boolean(res.has_more);
			}
		} catch (err) {
			if (!silent) {
				error = getErrorMessage(err);
			}
		} finally {
			if (silent) {
				refreshing = false;
			} else {
				loading = false;
				loadingMore = false;
			}
		}
	}

	function onScroll() {
		const el = scrollRoot;
		if (!el || !hasMore || loading || loadingMore) return;
		if (el.scrollHeight - el.scrollTop - el.clientHeight < 140) {
			void loadPage(false);
		}
	}
</script>

<div class="flex min-h-0 flex-1 flex-col">
	{#if onNewChatInProject}
		<div class="shrink-0 border-b px-3 py-2">
			<Button
				variant="outline"
				size="sm"
				class="h-8 w-full justify-start gap-2 text-xs"
				disabled={!workingDirExists}
				onclick={onNewChatInProject}
			>
				<MessageSquarePlus class="h-3.5 w-3.5 text-primary" />
				<span>{t('myaaw.workspace.new_chat_in_project')}</span>
			</Button>
		</div>
	{/if}

	<div bind:this={scrollRoot} onscroll={onScroll} class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
		{#if error}
			<div
				class="rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive"
			>
				{error}
			</div>
		{:else if loading && items.length === 0}
			<div class="flex items-center justify-center gap-2 py-8 text-xs text-muted-foreground">
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if items.length === 0}
			<div class="px-3 py-8 text-center text-xs text-muted-foreground">
				{t('myaaw.workspace.project_chats_empty')}
			</div>
		{:else}
			<div class="space-y-3">
				{#each dayGroups as group (group.key)}
					<div class="space-y-0.5">
						{#if group.label}
							<p
								class="px-2.5 pt-1 pb-0.5 text-[10px] font-semibold tracking-wider text-muted-foreground/75 uppercase"
							>
								{group.label}
							</p>
						{/if}
						{#each group.items as chat (chat.id)}
							{@const selected = chat.id === activeId}
							{@const isStreaming = streaming.has(chat.id) && !selected}
							{@const isUnread =
								(unreadChats.has(agent, chat.id) || approvalQueue.has(agent, chat.id)) && !selected}
							<button
								type="button"
								class={[
									'flex w-full flex-col rounded-lg px-2.5 py-2 text-left text-xs transition-colors',
									selected
										? 'bg-primary/12 font-medium text-foreground'
										: 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
								]}
								onclick={() => onSelectConversation?.(chat.id)}
							>
								<span class="flex w-full min-w-0 items-center gap-1.5">
									{#if isStreaming}
										<Loader2
											class="h-3 w-3 shrink-0 animate-spin text-muted-foreground"
											aria-label={t('myaaw.chat.streaming')}
											data-streaming="true"
										/>
									{/if}
									{#if isUnread}
										<span
											class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary"
											role="img"
											aria-label={t('myaaw.chat.unread')}
										></span>
									{/if}
									<span
										class={[
											'min-w-0 flex-1 truncate text-xs text-foreground',
											isUnread || selected ? 'font-medium' : ''
										]}
									>
										{chat.title || t('myaaw.chat.new')}
									</span>
								</span>
								<span
									class="mt-0.5 block w-full truncate text-[10px] text-muted-foreground tabular-nums"
								>
									{formatConversationTimestamp(chat.updated_at, locale.value)}
									{#if chat.message_count > 0}
										· {t('myaaw.conversations.messages', { count: chat.message_count })}
									{/if}
								</span>
							</button>
						{/each}
					</div>
				{/each}
			</div>

			{#if hasMore}
				<div
					bind:this={sentinel}
					class="flex items-center justify-center py-3 text-[11px] text-muted-foreground"
				>
					{#if loadingMore}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>
