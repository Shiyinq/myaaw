<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { chat as chatApi } from '$lib/apis/chat';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import {
		AlertTriangle,
		ArrowDownToLine,
		Brain,
		Check,
		Loader2,
		MessageSquarePlus,
		Minimize2,
		PanelRightClose,
		PanelRightOpen,
		Pencil,
		RotateCcw,
		Search,
		Send,
		ShieldAlert,
		Square,
		Trash2,
		Wrench,
		X
	} from 'lucide-svelte';
	import type {
		ApprovalDecision,
		ApprovalPrompt,
		Attachment,
		ChatStreamEvent,
		ConversationSummary,
		TimelineItem,
		Usage
	} from '$lib/types';

	const { t } = useTranslation();

	interface Item extends TimelineItem {
		localId?: string;
		resolved?: string;
	}

	let agent = $derived(agentSelection.selected);
	let conversationId = $state('');
	let items = $state<Item[]>([]);
	let input = $state('');
	let streaming = $state(false);
	let loading = $state(false);
	let pendingApproval = $state<ApprovalPrompt | null>(null);
	let usage = $state<Usage | null>(null);
	let attachments = $state<Attachment[]>([]);
	let error = $state('');
	let scroller = $state<HTMLDivElement | null>(null);
	let composer = $state<HTMLTextAreaElement | null>(null);

	let controller: AbortController | null = null;
	let counter = 0;
	let loadKey = '';

	// Conversation history sidebar
	const HISTORY_LIMIT = 20;
	const HISTORY_KEY = 'myaaw_chat_history_open';
	const LAST_CONVERSATION_KEY = 'myaaw_last_conversation';

	let historyOpen = $state(
		browser
			? (localStorage.getItem(HISTORY_KEY) ?? String(window.innerWidth >= 1024)) === 'true'
			: false
	);
	let history = $state<ConversationSummary[]>([]);
	let historyOffset = $state(0);
	let historyHasMore = $state(false);
	let historyLoading = $state(false);
	let historySearch = $state('');
	let historyQuery = $state('');
	let historyKey = '';
	let chatAgentKey = '';
	let editingId = $state('');
	let editingTitle = $state('');

	function scrollToBottom() {
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	$effect(() => {
		void items.length;
		scrollToBottom();
	});

	async function loadConversation(currentAgent: string, id: string) {
		if (!id) {
			items = [];
			usage = null;
			attachments = [];
			return;
		}
		loading = true;
		error = '';
		try {
			const detail = await conversationsApi.get(currentAgent, id, { limit: 0 });
			items = detail.messages
				.filter((message) => message.role === 'user' || message.role === 'assistant')
				.map((message, index) => ({
					kind: message.role === 'user' ? 'user' : 'text',
					text: message.content,
					localId: `history-${index}`
				}));
			usage = null;
			attachments = [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
			scrollToBottom();
		}
	}

	function readLastConversations(): Record<string, string> {
		if (!browser) return {};
		try {
			const raw = localStorage.getItem(LAST_CONVERSATION_KEY);
			if (!raw) return {};
			const parsed: unknown = JSON.parse(raw);
			return parsed && typeof parsed === 'object' ? (parsed as Record<string, string>) : {};
		} catch {
			return {};
		}
	}

	function rememberConversation(id: string) {
		if (!browser || !agent) return;
		const map = readLastConversations();
		if (id) map[agent] = id;
		else delete map[agent];
		localStorage.setItem(LAST_CONVERSATION_KEY, JSON.stringify(map));
	}

	function lastConversationFor(agentName: string) {
		return readLastConversations()[agentName] ?? '';
	}

	// Switching the agent must not keep showing the previous agent's transcript.
	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent) return;
		const previous = chatAgentKey;
		chatAgentKey = currentAgent;
		if (!previous || previous === currentAgent) return;

		const openConversation = page.params.conversation ?? '';
		// Do not load a conversation that belongs to the previous agent.
		if (openConversation) loadKey = `${currentAgent}:${openConversation}`;
		conversationId = '';
		items = [];
		usage = null;
		attachments = [];
		error = '';
		pendingApproval = null;
		if (page.url.pathname !== '/app/chat') {
			void goto('/app/chat', { replaceState: true, noScroll: true });
		}
	});

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const currentConversation = page.params.conversation ?? '';
		const key = `${currentAgent}:${currentConversation}`;
		if (!currentAgent || key === loadKey) return;

		// The sidebar links to /app/chat, so restore the last opened conversation
		// instead of dropping the user into a brand new chat.
		if (!currentConversation) {
			const last = lastConversationFor(currentAgent);
			if (last) {
				void goto(`/app/chat/${last}`, { replaceState: true, noScroll: true });
				return;
			}
		}

		loadKey = key;
		conversationId = currentConversation;
		rememberConversation(currentConversation);
		void loadConversation(currentAgent, currentConversation);
	});

	function push(item: Item) {
		items = [...items, { ...item, localId: item.localId ?? `item-${++counter}` }];
	}

	function mergeLast(kind: string, text: string) {
		const last = items[items.length - 1];
		if (last && last.kind === kind) {
			items = [...items.slice(0, -1), { ...last, text: (last.text ?? '') + text }];
		} else {
			push({ kind, text });
		}
	}

	function setToolResult(id: string, result: string, isError?: boolean) {
		items = items.map((item) =>
			item.kind === 'tool' && item.id === id ? { ...item, result, is_error: isError } : item
		);
	}

	async function send() {
		const text = input.trim();
		if (!text || !agent || streaming) return;

		push({ kind: 'user', text });
		input = '';
		if (composer) composer.style.height = '';
		streaming = true;
		error = '';
		pendingApproval = null;
		usage = null;
		attachments = [];
		controller = new AbortController();

		try {
			await chatApi.stream(
				agent,
				{ text, conversation_id: conversationId || undefined },
				(event) => {
					const data = event.data as Record<string, any>;
					switch (event.event as ChatStreamEvent) {
						case 'thought':
							mergeLast('thought', data?.text ?? '');
							break;
						case 'chunk':
							mergeLast('text', data?.text ?? '');
							break;
						case 'tool':
							push({
								kind: 'tool',
								id: data?.id,
								name: data?.name,
								arguments: data?.arguments
							});
							break;
						case 'tool_result':
							setToolResult(data?.id, data?.result ?? '', data?.is_error);
							break;
						case 'retry':
							push({ kind: 'retry', attempt: data?.attempt });
							break;
						case 'compaction':
							push({
								kind: 'compaction',
								tokens_before: data?.tokens_before,
								tokens_after: data?.tokens_after,
								dropped_messages: data?.dropped_messages
							});
							break;
						case 'approval_required': {
							const prompt: ApprovalPrompt = {
								approval_id: data?.approval_id,
								tool_name: data?.tool_name,
								action: data?.action,
								args_summary: data?.args_summary,
								subagent: data?.subagent
							};
							pendingApproval = prompt;
							push({
								kind: 'approval',
								id: prompt.approval_id,
								name: prompt.tool_name,
								arguments: prompt.args_summary,
								subagent: prompt.subagent
							});
							break;
						}
						case 'approval_resolved': {
							pendingApproval = null;
							items = items.map((item) =>
								item.kind === 'approval' && item.id === data?.approval_id
									? { ...item, resolved: data?.decision ?? data?.reason ?? 'resolved' }
									: item
							);
							break;
						}
						case 'done': {
							const nextConversation = data?.conversation_id;
							if (nextConversation && nextConversation !== conversationId) {
								conversationId = nextConversation;
								loadKey = `${agent}:${nextConversation}`;
								rememberConversation(nextConversation);
								void loadHistory(true);
								void goto(`/app/chat/${nextConversation}`, {
									replaceState: true,
									noScroll: true
								});
							}
							if (data?.text) {
								const hasText = [...items].reverse().some((item) => item.kind === 'text');
								if (!hasText) push({ kind: 'text', text: data.text });
							}
							usage = data?.usage ?? null;
							attachments = data?.attachments ?? [];
							break;
						}
						case 'error': {
							error = data?.error ?? 'Stream error';
							push({ kind: 'error', text: error });
							break;
						}
					}
				},
				controller.signal
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				error = getErrorMessage(err);
				toastStore.error(error);
			}
		} finally {
			streaming = false;
			controller = null;
		}
	}

	async function decide(decision: ApprovalDecision) {
		const prompt = pendingApproval;
		if (!prompt || !agent) return;
		pendingApproval = null;
		try {
			await chatApi.decide(agent, prompt.approval_id, decision);
			items = items.map((item) =>
				item.kind === 'approval' && item.id === prompt.approval_id
					? { ...item, resolved: decision }
					: item
			);
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		}
	}

	function stop() {
		controller?.abort();
		controller = null;
		streaming = false;
	}

	function newChat() {
		resetChat();
		if (page.url.pathname !== '/app/chat') {
			void goto('/app/chat');
		}
	}

	function resetChat() {
		controller?.abort();
		controller = null;
		streaming = false;
		conversationId = '';
		items = [];
		input = '';
		if (composer) composer.style.height = '';
		usage = null;
		attachments = [];
		error = '';
		pendingApproval = null;
		loadKey = `${agent}:`;
		rememberConversation('');
	}

	function toggleHistory() {
		historyOpen = !historyOpen;
		if (browser) localStorage.setItem(HISTORY_KEY, String(historyOpen));
	}

	async function loadHistory(reset = false) {
		if (!agent || historyLoading) return;
		historyLoading = true;
		try {
			const offset = reset ? 0 : historyOffset;
			const result = await conversationsApi.list(agent, {
				search: historyQuery || undefined,
				limit: HISTORY_LIMIT,
				offset
			});
			const pageItems = result.conversations ?? [];
			if (reset) {
				history = pageItems;
			} else {
				const known = new Set(history.map((item) => item.id));
				history = [...history, ...pageItems.filter((item) => !known.has(item.id))];
			}
			historyOffset = offset + pageItems.length;
			historyHasMore = pageItems.length > 0 && result.has_more;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			historyLoading = false;
		}
	}

	function loadMoreHistory() {
		if (!historyHasMore || historyLoading) return;
		void loadHistory(false);
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent) return;
		const key = `${currentAgent}:${historyQuery}`;
		if (historyKey === key) return;
		historyKey = key;
		history = [];
		historyOffset = 0;
		historyHasMore = false;
		void loadHistory(true);
	});

	// Debounce the search box so typing does not hit the API on every keystroke.
	$effect(() => {
		const value = historySearch.trim();
		if (value === historyQuery) return;
		const timer = setTimeout(() => (historyQuery = value), 300);
		return () => clearTimeout(timer);
	});

	function infinite(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) loadMoreHistory();
			},
			{ rootMargin: '160px' }
		);
		observer.observe(node);
		return { destroy: () => observer.disconnect() };
	}

	function startRename(item: ConversationSummary) {
		editingId = item.id;
		editingTitle = item.title;
	}

	async function saveRename() {
		if (!agent || !editingId) return;
		const title = editingTitle.trim();
		if (!title) return;
		try {
			await conversationsApi.rename(agent, editingId, title);
			history = history.map((item) => (item.id === editingId ? { ...item, title } : item));
			editingId = '';
			toastStore.success(t('myaaw.conversations.renamed'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeHistory(item: ConversationSummary) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.conversations.delete_confirm'))) return;
		try {
			await conversationsApi.remove(agent, item.id);
			history = history.filter((entry) => entry.id !== item.id);
			historyOffset = Math.max(0, historyOffset - 1);
			toastStore.success(t('myaaw.conversations.deleted'));
			if (item.id === conversationId) {
				resetChat();
				await goto('/app/chat');
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function openConversation(id: string) {
		if (browser && window.innerWidth < 1024) toggleHistory();
		if (id !== conversationId) void goto(`/app/chat/${id}`);
	}

	function formatWhen(value?: string) {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
			event.preventDefault();
			void send();
		}
	}

	function autosize(node: HTMLTextAreaElement) {
		const resize = () => {
			node.style.height = 'auto';
			node.style.height = `${Math.min(node.scrollHeight, 160)}px`;
		};
		resize();
		node.addEventListener('input', resize);
		return {
			destroy: () => node.removeEventListener('input', resize)
		};
	}

	function attachmentUrl(attachment: Attachment) {
		return `data:${attachment.mime_type || 'application/octet-stream'};base64,${attachment.data_base64}`;
	}

	function isImage(attachment: Attachment) {
		return (attachment.mime_type ?? '').startsWith('image/');
	}
</script>

<TopBarActions id="chat-actions">
	{#snippet children()}
		<TopBarButton label={t('myaaw.chat.new')} icon={MessageSquarePlus} onclick={newChat} />
		<Button
			variant="outline"
			size="icon-sm"
			onclick={toggleHistory}
			title={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
			aria-label={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
		>
			{#if historyOpen}
				<PanelRightClose class="h-4 w-4" />
			{:else}
				<PanelRightOpen class="h-4 w-4" />
			{/if}
		</Button>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0">
	<div class="flex min-w-0 flex-1 flex-col">
		{#if !agent}
			<div class="flex flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</div>
		{:else}
			<div bind:this={scroller} class="min-h-0 flex-1 overflow-y-auto">
				<div class="mx-auto w-full max-w-5xl space-y-5 px-4 py-6">
					{#if loading}
						<p class="text-sm text-muted-foreground">{t('myaaw.chat.loading')}</p>
					{:else if items.length === 0}
						<p class="py-20 text-center text-sm text-muted-foreground">
							{t('myaaw.chat.empty')}
						</p>
					{/if}

					{#each items as item, index (item.localId ?? `${item.kind}-${index}`)}
						{#if item.kind === 'user'}
							<div class="flex justify-end">
								<div
									class="max-w-[80%] whitespace-pre-wrap rounded-3xl bg-secondary px-4 py-2.5 text-sm text-secondary-foreground"
								>
									{item.text}
								</div>
							</div>
						{:else if item.kind === 'text'}
							<div class="text-sm">
								<Markdown content={item.text ?? ''} />
							</div>
						{:else if item.kind === 'thought'}
							<div
								class="rounded-xl border border-border/60 bg-muted/30 px-3 py-2 text-xs text-muted-foreground"
							>
								<div class="mb-1 flex items-center gap-2 font-medium not-italic">
									<Brain class="h-3.5 w-3.5" />
									{t('myaaw.chat.thinking')}
								</div>
								<span class="whitespace-pre-wrap italic">{item.text}</span>
							</div>
						{:else if item.kind === 'tool'}
							<div class="rounded-xl border bg-card/60 px-3 py-2 text-xs">
								<div class="flex items-center gap-2 font-medium">
									<Wrench class="h-3.5 w-3.5" />
									<span class="font-mono">{item.name}</span>
								</div>
								{#if item.arguments}
									<pre
										class="mt-1 overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{item.arguments}</pre>
								{/if}
								{#if item.result}
									<div
										class={item.is_error ? 'mt-1 text-destructive' : 'mt-1 text-muted-foreground'}
									>
										<div class="mb-0.5 flex items-center gap-1 font-medium">
											<ArrowDownToLine class="h-3 w-3" />
											{t('myaaw.chat.result')}
										</div>
										<pre class="overflow-x-auto whitespace-pre-wrap break-all">{item.result}</pre>
									</div>
								{/if}
							</div>
						{:else if item.kind === 'retry'}
							<div class="flex items-center gap-1 text-[11px] text-muted-foreground">
								<RotateCcw class="h-3 w-3" />
								{t('myaaw.chat.retry', { attempt: item.attempt ?? 2 })}
							</div>
						{:else if item.kind === 'compaction'}
							<div class="flex items-center gap-1 text-[11px] text-muted-foreground">
								<Minimize2 class="h-3 w-3" />
								{t('myaaw.chat.compaction', {
									before: item.tokens_before ?? 0,
									after: item.tokens_after ?? 0
								})}
							</div>
						{:else if item.kind === 'approval'}
							<div class="rounded-xl border border-amber-500/40 bg-amber-500/10 p-3 text-xs">
								<div class="flex items-center gap-2 font-medium text-amber-700 dark:text-amber-300">
									<ShieldAlert class="h-4 w-4" />
									{t('myaaw.chat.waiting')}
								</div>
								<div class="mt-1 font-mono text-foreground">{item.name}</div>
								{#if item.arguments}
									<pre
										class="mt-1 overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{item.arguments}</pre>
								{/if}
								{#if item.resolved}
									<Badge variant="outline" class="mt-2">{item.resolved}</Badge>
								{:else if pendingApproval && pendingApproval.approval_id === item.id}
									<div class="mt-2 flex flex-wrap gap-2">
										<Button size="xs" onclick={() => decide('allow_once')}>
											{t('myaaw.chat.allow_once')}
										</Button>
										<Button size="xs" variant="secondary" onclick={() => decide('allow_session')}>
											{t('myaaw.chat.allow_session')}
										</Button>
										<Button size="xs" variant="secondary" onclick={() => decide('allow_always')}>
											{t('myaaw.chat.allow_always')}
										</Button>
										<Button size="xs" variant="destructive" onclick={() => decide('deny')}>
											{t('myaaw.chat.deny')}
										</Button>
									</div>
								{/if}
							</div>
						{:else if item.kind === 'error'}
							<div class="flex items-start gap-2 text-xs text-destructive">
								<AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
								<span class="whitespace-pre-wrap">{item.text}</span>
							</div>
						{/if}
					{/each}

					{#if attachments.length > 0}
						<div class="space-y-2">
							{#each attachments as attachment (attachment.file_name)}
								{#if isImage(attachment)}
									<img
										src={attachmentUrl(attachment)}
										alt={attachment.file_name}
										class="max-h-72 rounded-xl border"
									/>
								{:else}
									<a
										href={attachmentUrl(attachment)}
										download={attachment.file_name}
										class="inline-flex items-center gap-2 text-xs text-primary underline-offset-4 hover:underline"
									>
										<ArrowDownToLine class="h-3.5 w-3.5" />
										{attachment.file_name}
									</a>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			</div>

			{#if error}
				<div class="shrink-0 px-4">
					<div
						class="mx-auto max-w-5xl rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive"
					>
						{error}
					</div>
				</div>
			{/if}

			<div class="shrink-0 border-t bg-background/80 px-4 py-3 backdrop-blur">
				<div class="mx-auto w-full max-w-5xl">
					<div
						class="rounded-2xl border bg-card shadow-sm transition-colors focus-within:border-ring/60"
					>
						<textarea
							bind:this={composer}
							bind:value={input}
							use:autosize
							onkeydown={onKeydown}
							rows="1"
							placeholder={t('myaaw.chat.placeholder')}
							class="max-h-40 min-h-[3.25rem] w-full resize-none rounded-t-2xl bg-transparent px-4 py-3 text-sm outline-none placeholder:text-muted-foreground"
						></textarea>
						<div class="flex items-center justify-between px-3 pb-2">
							<span class="text-[10px] text-muted-foreground">
								{#if usage}
									{t('myaaw.chat.tokens', { count: usage.total_tokens })} ·
								{/if}
								{t('myaaw.chat.hint')}
							</span>
							{#if streaming}
								<Button
									size="icon"
									variant="destructive"
									class="rounded-full"
									onclick={stop}
									aria-label={t('myaaw.chat.stop')}
								>
									<Square class="h-4 w-4" />
								</Button>
							{:else}
								<Button
									size="icon"
									class="rounded-full"
									onclick={send}
									disabled={!input.trim()}
									aria-label={t('myaaw.chat.send')}
								>
									<Send class="h-4 w-4" />
								</Button>
							{/if}
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if agent && historyOpen}
		<!-- Drawer on small screens, a static third column from lg up. -->
		<button
			type="button"
			class="fixed inset-0 z-30 bg-black/40 lg:hidden"
			aria-label={t('myaaw.chat.history_hide')}
			onclick={toggleHistory}
		></button>
		<aside
			class="fixed inset-y-0 right-0 z-40 flex w-72 max-w-[85vw] flex-col border-l bg-card lg:static lg:z-auto lg:h-full lg:max-w-none"
		>
			<div class="shrink-0 border-b p-2">
				<div class="relative">
					<Search
						class="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						bind:value={historySearch}
						placeholder={t('myaaw.conversations.search')}
						class="h-8 pl-7 text-xs"
						aria-label={t('myaaw.conversations.search')}
					/>
				</div>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto p-2">
				{#if historyLoading && history.length === 0}
					<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
				{:else if history.length === 0}
					<p class="px-2 py-3 text-xs text-muted-foreground">
						{historyQuery ? t('myaaw.chat.history_no_results') : t('myaaw.chat.history_empty')}
					</p>
				{/if}

				{#each history as entry (entry.id)}
					<div
						class={[
							'rounded-md transition-colors',
							entry.id === conversationId ? 'bg-accent' : 'hover:bg-muted'
						]}
					>
						{#if editingId === entry.id}
							<div class="flex items-center gap-1 p-1">
								<Input
									bind:value={editingTitle}
									class="h-7 text-xs"
									onkeydown={(event) => {
										if (event.key === 'Enter') void saveRename();
										if (event.key === 'Escape') editingId = '';
									}}
								/>
								<Button
									size="icon-xs"
									variant="secondary"
									onclick={saveRename}
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
							<div class="flex items-start gap-1 p-1">
								<button
									class="min-w-0 flex-1 rounded px-1 py-0.5 text-left"
									onclick={() => openConversation(entry.id)}
								>
									<span class="block truncate text-xs font-medium">
										{entry.title || entry.id.slice(0, 12)}
									</span>
									<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
										{formatWhen(entry.updated_at)} · {t('myaaw.conversations.messages', {
											count: entry.message_count
										})}
									</span>
								</button>
								<div class="flex shrink-0">
									<Button
										variant="ghost"
										size="icon-xs"
										onclick={() => startRename(entry)}
										aria-label={t('myaaw.chat.rename')}
									>
										<Pencil class="h-3 w-3" />
									</Button>
									<Button
										variant="ghost"
										size="icon-xs"
										onclick={() => removeHistory(entry)}
										aria-label={t('common.delete')}
									>
										<Trash2 class="h-3 w-3 text-destructive" />
									</Button>
								</div>
							</div>
						{/if}
					</div>
				{/each}

				{#if historyHasMore}
					<div use:infinite class="flex justify-center py-3">
						{#if historyLoading}
							<Loader2 class="h-4 w-4 animate-spin text-muted-foreground" />
						{/if}
					</div>
				{/if}
			</div>
		</aside>
	{/if}
</div>
