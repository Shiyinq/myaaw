<script lang="ts">
	import { untrack } from 'svelte';
	import {
		AlertTriangle,
		Check,
		ChevronDown,
		ChevronRight,
		FolderGit2,
		FolderOpen,
		GitBranch,
		Loader2,
		MessageSquarePlus,
		Pencil,
		RefreshCw,
		Search,
		Trash2,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import {
		readExpandedProjectHistory,
		writeExpandedProjectHistory
	} from '$lib/stores/projectWorkspace.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatConversationTimestamp, groupConversationsByDay } from './projectDateGroups';
	import type { ConversationProject, ConversationSummary } from '$lib/types';

	const PROJECT_PAGE_SIZE = 20;
	const CHAT_PAGE_SIZE = 20;

	interface ProjectChatState {
		items: ConversationSummary[];
		hasMore: boolean;
		loading: boolean;
		loadingMore: boolean;
	}

	interface Props {
		agent: string;
		activeId?: string;
		activeWorkingDir?: string;
		refreshKey?: number;
		onOpen: (id: string) => void;
		onNewInProject?: (project: ConversationProject) => void;
		onAttachProject?: () => void;
		onDeleted?: (id: string) => void;
		onRenamed?: (id: string, title: string) => void;
	}

	let {
		agent,
		activeId = '',
		activeWorkingDir = '',
		refreshKey = 0,
		onOpen,
		onNewInProject,
		onAttachProject,
		onDeleted,
		onRenamed
	}: Props = $props();

	const { t, locale } = useTranslation();

	let projects = $state<ConversationProject[]>([]);
	let projectsHasMore = $state(false);
	let loading = $state(false);
	let loadingMoreProjects = $state(false);
	let refreshingProjects = false;
	const refreshingDirs = new Set<string>();
	let error = $state('');
	let query = $state('');
	let debouncedQuery = $state('');
	let expanded = $state<Record<string, boolean>>(untrack(() => readExpandedProjectHistory(agent)));
	let chatsByDir = $state<Record<string, ProjectChatState>>({});
	let loadedKey = '';
	let seenHistoryRevision = untrack(() => conversationHistory.revision);

	let scrollRoot = $state<HTMLElement | null>(null);
	let projectsSentinel = $state<HTMLElement | null>(null);
	let editingId = $state('');
	let editingDir = '';
	let editingTitle = $state('');
	let renameInput = $state<HTMLInputElement | null>(null);

	$effect(() => {
		if (editingId && renameInput) {
			renameInput.focus();
			renameInput.select();
		}
	});

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

	function projectHasStreaming(proj: ConversationProject): boolean {
		const loaded = chatsByDir[proj.working_dir]?.items;
		if (loaded && loaded.some((chat) => streaming.has(chat.id) && chat.id !== activeId)) {
			return true;
		}
		return Boolean(
			proj.latest_conversation_id &&
			streaming.has(proj.latest_conversation_id) &&
			proj.latest_conversation_id !== activeId
		);
	}

	function projectHasUnread(proj: ConversationProject): boolean {
		const isChatUnread = (id: string) =>
			Boolean(id) &&
			id !== activeId &&
			(unreadChats.has(agent, id) || approvalQueue.has(agent, id));
		const loaded = chatsByDir[proj.working_dir]?.items;
		if (loaded && loaded.some((chat) => isChatUnread(chat.id))) {
			return true;
		}
		return isChatUnread(proj.latest_conversation_id);
	}

	// Debounce server-side project search so paging stays accurate.
	$effect(() => {
		const raw = query.trim();
		const timer = setTimeout(() => {
			debouncedQuery = raw;
		}, 180);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		const key = `${agent}:${refreshKey}:${debouncedQuery}`;
		if (!agent) {
			projects = [];
			projectsHasMore = false;
			chatsByDir = {};
			loadedKey = '';
			return;
		}
		if (key === loadedKey) return;
		const agentChanged = !loadedKey.startsWith(`${agent}:`);
		loadedKey = key;
		seenHistoryRevision = conversationHistory.revision;
		untrack(() => {
			if (agentChanged) {
				chatsByDir = {};
				expanded = readExpandedProjectHistory(agent);
			}
			void loadProjectsPage(true);
		});
	});

	// Stay in sync silently when a background run finishes, renames, or updates message counts.
	$effect(() => {
		const rev = conversationHistory.revision;
		if (rev === seenHistoryRevision) return;
		seenHistoryRevision = rev;
		if (!agent || !loadedKey) return;
		untrack(() => {
			void loadProjectsPage(true, true);
		});
	});

	$effect(() => {
		const el = projectsSentinel;
		const root = scrollRoot;
		if (!el || !projectsHasMore) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					void loadProjectsPage(false);
				}
			},
			{ root, rootMargin: '140px' }
		);
		observer.observe(el);
		return () => observer.disconnect();
	});

	async function loadProjectsPage(reset: boolean, silent = false) {
		if (!agent) return;
		if (reset) {
			if (silent) {
				if (refreshingProjects || loading) return;
				refreshingProjects = true;
			} else {
				if (loading) return;
				loading = true;
				error = '';
			}
		} else {
			if (loading || loadingMoreProjects || !projectsHasMore) return;
			loadingMoreProjects = true;
		}
		try {
			const offset = reset ? 0 : projects.length;
			const res = await conversationsStore.fetchProjects(agent, {
				search: debouncedQuery || undefined,
				limit: PROJECT_PAGE_SIZE,
				offset
			});
			const incoming = res.projects ?? [];
			if (reset) {
				if (silent && projects.length > PROJECT_PAGE_SIZE) {
					const incomingDirs = new Set(incoming.map((p) => p.working_dir));
					const tail = projects.filter((p) => !incomingDirs.has(p.working_dir));
					projects = [...incoming, ...tail];
					projectsHasMore = Boolean(res.has_more) || projectsHasMore;
				} else {
					projects = incoming;
					projectsHasMore = Boolean(res.has_more);
				}

				const openDirs = projects.map((p) => p.working_dir).filter((dir) => Boolean(expanded[dir]));
				await Promise.all(openDirs.map((dir) => loadProjectChatsPage(dir, true, silent)));
			} else {
				const seen = new Set(projects.map((p) => p.working_dir));
				const appended = incoming.filter((p) => !seen.has(p.working_dir));
				projects = [...projects, ...appended];
				projectsHasMore = appended.length > 0 && Boolean(res.has_more);
				const openAppended = appended
					.map((p) => p.working_dir)
					.filter((dir) => Boolean(expanded[dir]) && !chatsByDir[dir]);
				if (openAppended.length > 0) {
					await Promise.all(openAppended.map((dir) => loadProjectChatsPage(dir, true, true)));
				}
			}
		} catch (err) {
			if (!silent) {
				error = getErrorMessage(err);
			}
		} finally {
			if (silent) {
				refreshingProjects = false;
			} else {
				loading = false;
				loadingMoreProjects = false;
			}
		}
	}

	async function loadProjectChatsPage(dir: string, reset: boolean, silent = false) {
		if (!agent || !dir) return;
		const prev: ProjectChatState = chatsByDir[dir] ?? {
			items: [],
			hasMore: false,
			loading: false,
			loadingMore: false
		};
		if (reset) {
			if (silent) {
				if (refreshingDirs.has(dir) || prev.loading) return;
				refreshingDirs.add(dir);
			} else {
				if (prev.loading) return;
				chatsByDir = { ...chatsByDir, [dir]: { ...prev, loading: true } };
			}
		} else {
			if (prev.loading || prev.loadingMore || !prev.hasMore) return;
			chatsByDir = { ...chatsByDir, [dir]: { ...prev, loadingMore: true } };
		}
		const unreadRev = unreadChats.revision();
		try {
			const offset = reset ? 0 : prev.items.length;
			const res = await conversationsStore.fetchList(agent, {
				working_dir: dir,
				limit: CHAT_PAGE_SIZE,
				offset
			});
			const incoming = res.conversations ?? [];
			unreadChats.observe(agent, incoming, unreadRev);
			const current = chatsByDir[dir] ?? prev;
			if (reset) {
				if (silent && current.items.length > CHAT_PAGE_SIZE) {
					const incomingIds = new Set(incoming.map((chat) => chat.id));
					const tail = current.items.filter((chat) => !incomingIds.has(chat.id));
					chatsByDir = {
						...chatsByDir,
						[dir]: {
							items: [...incoming, ...tail],
							hasMore: Boolean(res.has_more) || current.hasMore,
							loading: false,
							loadingMore: false
						}
					};
				} else {
					chatsByDir = {
						...chatsByDir,
						[dir]: {
							items: incoming,
							hasMore: Boolean(res.has_more),
							loading: false,
							loadingMore: false
						}
					};
				}
			} else {
				const seen = new Set(current.items.map((chat) => chat.id));
				const appended = incoming.filter((chat) => !seen.has(chat.id));
				chatsByDir = {
					...chatsByDir,
					[dir]: {
						items: [...current.items, ...appended],
						hasMore: appended.length > 0 && Boolean(res.has_more),
						loading: false,
						loadingMore: false
					}
				};
			}
		} catch {
			if (!silent) {
				const current = chatsByDir[dir] ?? prev;
				chatsByDir = {
					...chatsByDir,
					[dir]: { ...current, loading: false, loadingMore: false }
				};
			}
		} finally {
			if (silent) {
				refreshingDirs.delete(dir);
			}
		}
	}

	async function toggleProject(dir: string) {
		const next = !expanded[dir];
		const updated = { ...expanded, [dir]: next };
		expanded = updated;
		writeExpandedProjectHistory(agent, updated);
		if (next && !chatsByDir[dir]) {
			await loadProjectChatsPage(dir, true);
		}
	}

	function onProjectsScroll() {
		const el = scrollRoot;
		if (!el || !projectsHasMore || loading || loadingMoreProjects) return;
		if (el.scrollHeight - el.scrollTop - el.clientHeight < 140) {
			void loadProjectsPage(false);
		}
	}

	function shortenPath(fullPath: string): string {
		const parts = fullPath.replace(/\/+$/, '').split('/').filter(Boolean);
		if (parts.length <= 3) return fullPath;
		return `…/${parts.slice(-3).join('/')}`;
	}

	function startRename(dir: string, chat: ConversationSummary) {
		editingId = chat.id;
		editingDir = dir;
		editingTitle = chat.title || '';
	}

	async function saveRename() {
		if (!editingId) return;
		const id = editingId;
		const dir = editingDir;
		const title = editingTitle.trim();
		if (!title) return;
		if (await conversationHistory.rename(id, title)) {
			editingId = '';
			const state = chatsByDir[dir];
			if (state) {
				chatsByDir = {
					...chatsByDir,
					[dir]: {
						...state,
						items: state.items.map((item) => (item.id === id ? { ...item, title } : item))
					}
				};
			}
			onRenamed?.(id, title);
		}
	}

	async function removeChat(dir: string, chat: ConversationSummary) {
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
			const state = chatsByDir[dir];
			if (state) {
				chatsByDir = {
					...chatsByDir,
					[dir]: {
						...state,
						items: state.items.filter((item) => item.id !== chat.id)
					}
				};
			}
			projects = projects
				.map((proj) =>
					proj.working_dir === dir
						? { ...proj, conversation_count: Math.max(0, proj.conversation_count - 1) }
						: proj
				)
				.filter((proj) => proj.conversation_count > 0);
			onDeleted?.(chat.id);
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="flex shrink-0 items-center gap-1.5 px-3 pb-2">
		<div class="relative flex-1">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:value={query}
				placeholder={t('myaaw.workspace.filter_projects')}
				class="h-8 pl-8 text-xs"
			/>
		</div>
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={() => void loadProjectsPage(true)}
			title={t('common.refresh')}
			aria-label={t('common.refresh')}
		>
			<RefreshCw class={loading ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'} />
		</Button>
	</div>

	<div
		bind:this={scrollRoot}
		onscroll={onProjectsScroll}
		class="min-h-0 flex-1 overflow-y-auto px-2 pb-3"
	>
		{#if error}
			<div
				class="mx-1 rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive"
			>
				{error}
			</div>
		{:else if loading && projects.length === 0}
			<div class="flex items-center justify-center gap-2 py-8 text-xs text-muted-foreground">
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if projects.length === 0}
			<div class="flex flex-col items-center justify-center gap-2.5 px-4 py-8 text-center">
				<span class="grid h-10 w-10 place-items-center rounded-xl bg-muted text-muted-foreground">
					<FolderOpen class="h-5 w-5" />
				</span>
				<div class="space-y-1">
					<p class="text-xs font-medium text-foreground">
						{t('myaaw.workspace.project_history_empty')}
					</p>
					<p class="text-[11px] text-muted-foreground">
						{t('myaaw.workspace.project_history_empty_desc')}
					</p>
				</div>
				{#if onAttachProject && !query.trim()}
					<Button size="sm" variant="outline" onclick={onAttachProject}>
						<FolderOpen class="h-3.5 w-3.5" />
						{t('myaaw.workspace.attach_button')}
					</Button>
				{/if}
			</div>
		{:else}
			<div class="space-y-1.5">
				{#each projects as proj (proj.working_dir)}
					{@const isOpen = Boolean(expanded[proj.working_dir])}
					{@const isActiveProj = activeWorkingDir === proj.working_dir}
					{@const projStreaming = projectHasStreaming(proj)}
					{@const projUnread = projectHasUnread(proj)}
					{@const chatState = chatsByDir[proj.working_dir]}
					{@const items = chatState?.items ?? []}
					{@const groups = groupConversationsByDay(
						items,
						locale.value,
						t('myaaw.focus.history_today'),
						t('myaaw.focus.history_yesterday')
					)}
					<div
						class={[
							'rounded-xl border transition-colors',
							isActiveProj ? 'border-primary/30 bg-primary/[0.03]' : 'border-border/60 bg-card/40'
						]}
					>
						<div class="flex items-center gap-1 px-2 py-1.5">
							<button
								type="button"
								class="flex min-w-0 flex-1 items-center gap-1.5 text-left"
								onclick={() => void toggleProject(proj.working_dir)}
								title={proj.working_dir}
							>
								{#if isOpen}
									<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{:else}
									<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{/if}
								{#if proj.has_git}
									<FolderGit2 class="h-3.5 w-3.5 shrink-0 text-primary" />
								{:else}
									<FolderOpen class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{/if}
								<span class="min-w-0 flex-1">
									<span class="flex items-center gap-1.5">
										{#if projStreaming}
											<Loader2
												class="h-3 w-3 shrink-0 animate-spin text-muted-foreground"
												aria-label={t('myaaw.chat.streaming')}
												data-streaming="true"
											/>
										{/if}
										{#if projUnread}
											<span
												class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary"
												role="img"
												aria-label={t('myaaw.chat.unread')}
											></span>
										{/if}
										<span class="truncate text-xs font-semibold text-foreground">
											{proj.name}
										</span>
										{#if proj.git_branch}
											<span
												class="inline-flex max-w-[45%] min-w-0 shrink items-center gap-0.5 rounded-full bg-muted/80 px-1.5 py-0 font-mono text-[10px] font-normal text-muted-foreground"
												title={proj.git_branch}
											>
												<GitBranch class="h-2.5 w-2.5 shrink-0" />
												<span class="min-w-0 truncate">{proj.git_branch}</span>
											</span>
										{/if}
										<span
											class="shrink-0 rounded-full bg-muted px-1.5 py-0 text-[10px] font-medium text-muted-foreground tabular-nums"
										>
											{proj.conversation_count}
										</span>
										{#if !proj.exists}
											<span
												class="inline-flex shrink-0 items-center gap-0.5 rounded-full bg-destructive/10 px-1.5 py-0 text-[9px] font-medium text-destructive"
												title={t('myaaw.workspace.dir_missing')}
											>
												<AlertTriangle class="h-2.5 w-2.5" />
												{t('myaaw.workspace.missing_badge')}
											</span>
										{/if}
									</span>
									<span class="block truncate font-mono text-[10px] text-muted-foreground/80">
										{shortenPath(proj.working_dir)}
									</span>
								</span>
							</button>

							{#if onNewInProject}
								<Button
									variant="ghost"
									size="icon-sm"
									disabled={!proj.exists}
									onclick={() => onNewInProject(proj)}
									title={proj.exists
										? t('myaaw.workspace.new_chat_in_project')
										: t('myaaw.workspace.dir_missing')}
									aria-label={t('myaaw.workspace.new_chat_in_project')}
								>
									<MessageSquarePlus class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>

						{#if isOpen}
							<div class="border-t border-border/50 px-1.5 py-1.5">
								{#if chatState?.loading && items.length === 0}
									<div
										class="flex items-center gap-1.5 px-2 py-2 text-[11px] text-muted-foreground"
									>
										<Loader2 class="h-3 w-3 animate-spin" />
										{t('common.loading')}
									</div>
								{:else if items.length === 0}
									<p class="px-2 py-1.5 text-[11px] text-muted-foreground">
										{t('myaaw.workspace.project_chats_empty')}
									</p>
								{:else}
									<div class="space-y-2">
										{#each groups as group (group.key)}
											<div class="space-y-0.5">
												{#if group.label}
													<p
														class="px-2 pt-0.5 text-[9.5px] font-semibold tracking-wider text-muted-foreground/75 uppercase"
													>
														{group.label}
													</p>
												{/if}
												{#each group.items as chat (chat.id)}
													{@const selected = chat.id === activeId}
													{@const isStreaming = streaming.has(chat.id) && !selected}
													{@const isUnread =
														(unreadChats.has(agent, chat.id) ||
															approvalQueue.has(agent, chat.id)) &&
														!selected}
													<div
														class={[
															'group/entry relative rounded-lg transition-colors',
															selected
																? 'bg-primary/12 text-foreground'
																: 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
														]}
													>
														{#if editingId === chat.id}
															<div class="flex items-center gap-1 p-1">
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
																class="flex w-full flex-col rounded-lg px-2 py-1.5 text-left text-xs"
																onclick={() => onOpen(chat.id)}
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
																	class="mt-0.5 block w-full truncate text-[10px] text-muted-foreground/75 tabular-nums"
																>
																	{formatConversationTimestamp(chat.updated_at, locale.value)}
																	{#if chat.message_count > 0}
																		· {t('myaaw.conversations.messages', {
																			count: chat.message_count
																		})}
																	{/if}
																</span>
															</button>
															<div
																class="pointer-events-none absolute top-1 right-1 flex items-center gap-0.5 rounded-md bg-muted/95 pl-0.5 opacity-0 transition-opacity group-focus-within/entry:pointer-events-auto group-focus-within/entry:opacity-100 group-hover/entry:pointer-events-auto group-hover/entry:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
															>
																<Button
																	variant="ghost"
																	size="icon-xs"
																	onclick={() => startRename(proj.working_dir, chat)}
																	aria-label={t('myaaw.chat.rename')}
																	title={t('myaaw.chat.rename')}
																>
																	<Pencil class="h-3 w-3" />
																</Button>
																<Button
																	variant="ghost"
																	size="icon-xs"
																	onclick={() => void removeChat(proj.working_dir, chat)}
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

									{#if chatState?.hasMore}
										<div class="flex justify-center pt-1">
											<Button
												variant="ghost"
												size="sm"
												class="h-7 gap-1.5 px-2 text-[11px] font-normal text-muted-foreground"
												disabled={chatState.loadingMore}
												onclick={() => void loadProjectChatsPage(proj.working_dir, false)}
											>
												{#if chatState.loadingMore}
													<Loader2 class="h-3 w-3 animate-spin" />
												{/if}
												{t('myaaw.chat.show_more')}
											</Button>
										</div>
									{/if}
								{/if}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			{#if projectsHasMore}
				<div
					bind:this={projectsSentinel}
					class="flex items-center justify-center py-3 text-[11px] text-muted-foreground"
				>
					{#if loadingMoreProjects}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>
