<script lang="ts">
	import { untrack } from 'svelte';
	import { Check, Loader2, MessageSquarePlus, Pencil, Search, Trash2, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatConversationTimestamp, groupConversationsByDay } from './projectDateGroups';
	import ProjectSidebarSkeleton from './ProjectSidebarSkeleton.svelte';
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
		onDeleted?: (id: string) => void;
		onRenamed?: (id: string, title: string) => void;
	}

	let {
		agent,
		workingDir,
		activeId = '',
		workingDirExists = true,
		refreshKey = 0,
		onSelectConversation,
		onNewChatInProject,
		onDeleted,
		onRenamed
	}: Props = $props();

	const { t, locale } = useTranslation();

	let items = $state<ConversationSummary[]>([]);
	let hasMore = $state(false);
	let loading = $state(false);
	let settled = $state(false);
	let requestVersion = 0;
	let loadedScope = '';
	let loadingMore = $state(false);
	let refreshing = false;
	let error = $state('');
	let query = $state('');
	let debouncedQuery = $state('');
	let loadedKey = '';
	let seenHistoryRevision = untrack(() => conversationHistory.revision);

	let editingId = $state('');
	let editingTitle = $state('');
	let renameInput = $state<HTMLInputElement | null>(null);

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
		const raw = query.trim();
		const timer = setTimeout(() => {
			debouncedQuery = raw;
		}, 180);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		if (editingId && renameInput) {
			renameInput.focus();
			renameInput.select();
		}
	});

	$effect(() => {
		const scope = agent && workingDir ? JSON.stringify([agent, workingDir]) : '';
		if (scope !== loadedScope) {
			loadedScope = scope;
			requestVersion += 1;
			items = [];
			hasMore = false;
			settled = false;
			error = '';
			loading = false;
			loadingMore = false;
			refreshing = false;
		}
		const key = scope ? JSON.stringify([scope, refreshKey, debouncedQuery]) : '';
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
				if (refreshing || loading || loadingMore) return;
				refreshing = true;
			} else {
				refreshing = false;
				loadingMore = false;
				loading = true;
				error = '';
			}
		} else {
			if (refreshing || loading || loadingMore || !hasMore) return;
			loadingMore = true;
		}
		const request = ++requestVersion;
		const unreadRev = unreadChats.revision();
		try {
			const offset = reset ? 0 : items.length;
			const res = await conversationsStore.fetchList(agent, {
				working_dir: workingDir,
				search: debouncedQuery || undefined,
				limit: PAGE_SIZE,
				offset
			});
			if (request !== requestVersion) return;
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
			if (request === requestVersion && !silent) error = getErrorMessage(err);
		} finally {
			if (request === requestVersion) {
				if (reset) settled = true;
				if (silent) {
					refreshing = false;
				} else {
					loading = false;
					loadingMore = false;
				}
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

	function startRename(chat: ConversationSummary) {
		editingId = chat.id;
		editingTitle = chat.title || '';
	}

	async function saveRename() {
		if (!editingId) return;
		const id = editingId;
		const title = editingTitle.trim();
		if (!title) return;
		if (await conversationHistory.rename(id, title)) {
			editingId = '';
			items = items.map((item) => (item.id === id ? { ...item, title } : item));
			onRenamed?.(id, title);
		}
	}

	async function removeChat(chat: ConversationSummary) {
		if (
			!(await askConfirm({
				message: t('myaaw.conversations.delete_confirm'),
				variant: 'destructive'
			}))
		) {
			return;
		}
		if (await conversationHistory.remove(chat.id)) {
			if (editingId === chat.id) editingId = '';
			items = items.filter((item) => item.id !== chat.id);
			onDeleted?.(chat.id);
		}
	}
</script>

<div class="flex min-h-0 flex-1 flex-col">
	<div class="shrink-0 space-y-2 border-b px-3 py-2">
		{#if onNewChatInProject}
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
		{/if}
		<div class="relative">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:value={query}
				placeholder={t('myaaw.conversations.search')}
				aria-label={t('myaaw.conversations.search')}
				class="h-8 pr-7 pl-8 text-xs"
			/>
			{#if query}
				<button
					type="button"
					class="absolute top-1/2 right-1.5 flex h-5 w-5 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={() => {
						query = '';
						debouncedQuery = '';
					}}
					aria-label={t('myaaw.conversations.clear_search')}
				>
					<X class="h-3 w-3" />
				</button>
			{/if}
		</div>
	</div>

	<div bind:this={scrollRoot} onscroll={onScroll} class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
		{#if error}
			<div
				class="rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive"
			>
				{error}
			</div>
		{/if}
		{#if agent && workingDir && !settled}
			<ProjectSidebarSkeleton variant="chats" />
		{:else if items.length === 0 && !error}
			<div class="px-3 py-8 text-center text-xs text-muted-foreground">
				{t('myaaw.workspace.project_chats_empty')}
			</div>
		{:else if items.length > 0}
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
							<div
								class={[
									'group/entry relative rounded-lg transition-colors',
									selected
										? 'bg-primary/12 text-foreground'
										: 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
								]}
							>
								{#if editingId === chat.id}
									<div class="flex items-center gap-1 p-1.5">
										<Input
											bind:value={editingTitle}
											bind:ref={renameInput}
											class="h-7 text-xs"
											onkeydown={(event) => {
												if (event.key === 'Enter') void saveRename();
												if (event.key === 'Escape') editingId = '';
											}}
										/>
										<Button
											size="icon-xs"
											variant="secondary"
											onclick={() => void saveRename()}
											aria-label={t('common.save')}
										>
											<Check class="h-3 w-3" />
										</Button>
										<Button
											size="icon-xs"
											variant="ghost"
											onclick={() => (editingId = '')}
											aria-label={t('common.cancel')}
										>
											<X class="h-3 w-3" />
										</Button>
									</div>
								{:else}
									<button
										type="button"
										class="flex w-full flex-col rounded-lg px-2.5 py-2 text-left text-xs"
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
									<div
										class="pointer-events-none absolute top-1.5 right-1 flex items-center gap-0.5 rounded-md bg-muted/95 pl-0.5 opacity-0 transition-opacity group-focus-within/entry:pointer-events-auto group-focus-within/entry:opacity-100 group-hover/entry:pointer-events-auto group-hover/entry:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
									>
										<Button
											variant="ghost"
											size="icon-xs"
											onclick={() => startRename(chat)}
											aria-label={t('myaaw.chat.rename')}
											title={t('myaaw.chat.rename')}
										>
											<Pencil class="h-3 w-3" />
										</Button>
										<Button
											variant="ghost"
											size="icon-xs"
											onclick={() => void removeChat(chat)}
											aria-label={t('common.delete')}
											title={t('common.delete')}
										>
											<Trash2 class="h-3 w-3 text-destructive" />
										</Button>
									</div>
								{/if}
							</div>
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
