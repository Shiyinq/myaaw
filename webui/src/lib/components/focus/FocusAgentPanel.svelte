<script lang="ts">
	import { untrack } from 'svelte';
	import {
		Bot,
		Brain,
		CalendarClock,
		ChevronDown,
		ChevronRight,
		FileText,
		Fingerprint,
		HeartPulse,
		History,
		Loader2,
		MessagesSquare,
		Play,
		RotateCw,
		ScrollText,
		Server,
		ShieldAlert,
		ShieldCheck,
		Sparkles,
		Square,
		UserRound,
		Wrench,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { context as contextApi } from '$lib/apis/context';
	import { server as serverApi } from '$lib/apis/server';
	import { agentSelection, toastStore } from '$lib/stores';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ContextFile, ConversationSummary } from '$lib/types';

	type Tab = 'history' | 'context' | 'control';
	type Status = 'connected' | 'stopped' | 'unknown';

	interface Props {
		/** The focus agent this panel describes. */
		agent: string;
		/** The open conversation, highlighted in the history list. */
		activeId?: string;
		/** Tab the panel opens on; the chat header can return it to history. */
		tab?: Tab;
		onOpen: (id: string) => void;
		onClose: () => void;
		/** Opens the full-page editor for one context file. */
		onOpenContext: (path: string) => void;
	}

	let {
		agent,
		activeId = '',
		tab = $bindable<Tab>('history'),
		onOpen,
		onClose,
		onOpenContext
	}: Props = $props();

	const { t, locale } = useTranslation();

	/** Conversations per page in the history tab. */
	const PAGE = 20;
	const CONTEXT_SOUL = 'SOUL.md';
	const CONTEXT_MEMORY = 'MEMORY.md';
	/** Dated memory notes, one Markdown file per day. */
	const DATED_MEMORY = /^\d{4}-\d{2}-\d{2}\.md$/;

	let status = $state<Status>('unknown');

	let items = $state<ConversationSummary[]>([]);
	let loading = $state(false);
	let loaded = $state(false);
	let hasMore = $state(false);
	let offset = 0;
	let generation = 0;
	/** The scroll container of the tab body, used as the observer root. */
	let scroller = $state<HTMLElement | null>(null);
	let sentinelVisible = $state(false);

	let files = $state<Record<string, ContextFile>>({});

	const soul = $derived(files[CONTEXT_SOUL]);
	const memory = $derived(files[CONTEXT_MEMORY]);
	/** Everything else the Context page shows, minus the two hero cards and the daily notes. */
	const contextFiles = $derived(
		Object.values(files)
			.filter(
				(file) =>
					file.path !== CONTEXT_SOUL &&
					file.path !== CONTEXT_MEMORY &&
					!(file.folder === 'memory' && DATED_MEMORY.test(file.name))
			)
			.sort((left, right) => {
				// Root files first, then anything a subfolder holds.
				const depth = Number(left.folder !== 'root') - Number(right.folder !== 'root');
				return depth !== 0 ? depth : left.path.localeCompare(right.path);
			})
	);
	/** One note per day, newest first; they can be many, so the list folds. */
	const datedMemory = $derived(
		Object.values(files)
			.filter((file) => file.folder === 'memory' && DATED_MEMORY.test(file.name))
			.sort((left, right) => right.name.localeCompare(left.name))
	);
	/** Daily notes hidden until the reader asks for them. */
	let memoryOpen = $state(false);
	/** A start/restart/stop request is in flight. */
	let controlling = $state(false);
	const initial = $derived((agent || '?').slice(0, 1).toUpperCase());
	/** Chats of this agent whose run is waiting for approval. */
	const approvals = $derived(approvalQueue.list(agent));
	/** The palette key this agent was saved with; ink for agents without one. */
	const agentColor = $derived(
		colorOf(agentSelection.agents.find((entry) => entry.name === agent)?.agent_color)
	);

	const statusLabel = $derived(
		status === 'connected'
			? t('myaaw.focus.status_connected')
			: status === 'stopped'
				? t('myaaw.focus.status_stopped')
				: t('myaaw.focus.status_unknown')
	);

	/** History rows in the order they arrived, split by local day. */
	const groups = $derived.by(() => {
		const out: { key: string; label: string; items: ConversationSummary[] }[] = [];
		for (const item of items) {
			const date = new Date(item.updated_at);
			const key = Number.isNaN(date.getTime())
				? 'unknown'
				: `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
			const last = out[out.length - 1];
			if (last && last.key === key) {
				last.items.push(item);
			} else {
				out.push({ key, label: dayLabel(date), items: [item] });
			}
		}
		return out;
	});

	// The panel describes one agent at a time; a new agent starts from scratch.
	$effect(() => {
		const name = agent;
		if (!name) return;
		untrack(() => {
			generation += 1;
			items = [];
			offset = 0;
			hasMore = false;
			loaded = false;
			files = {};
			void loadHistory(name, true);
			void loadFiles(name);
		});
	});

	// The shell refreshes the shared list when a run ends or a background turn
	// lands. Mirror that here, so a chat created a moment ago picks up its generated
	// title and message count instead of staying "New Chat".
	let lastRevision = -1;
	$effect(() => {
		const revision = conversationHistory.revision;
		if (revision === lastRevision) return;
		lastRevision = revision;
		if (!loaded || !agent) return;
		untrack(() => void reconcile(agent));
	});

	// History keeps plain infinite scroll: while the end of the list is on screen,
	// the next page loads. The sentinel stays mounted so the list never jumps.
	$effect(() => {
		if (!sentinelVisible) return;
		if (!loaded || !hasMore || loading) return;
		untrack(() => void loadHistory(agent, false));
	});

	function sentinel(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				sentinelVisible = entries.some((entry) => entry.isIntersecting);
			},
			{ root: scroller, rootMargin: '160px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
				sentinelVisible = false;
			}
		};
	}

	// Status is cheap to poll and matters while the panel is on screen.
	async function refreshStatus(name: string) {
		try {
			const result = await serverApi.status(name);
			status = result.running ? 'connected' : 'stopped';
		} catch {
			status = 'unknown';
		}
	}

	/** Starts, stops, or restarts the agent from the panel, then re-reads its state. */
	async function control(action: 'start' | 'stop' | 'restart') {
		const name = agent;
		if (!name || controlling) return;
		controlling = true;
		try {
			await serverApi[action](name);
			const doneKey =
				action === 'start'
					? 'myaaw.server.start_done'
					: action === 'stop'
						? 'myaaw.server.stop_done'
						: 'myaaw.server.restart_done';
			toastStore.success(t(doneKey));
			status = action === 'stop' ? 'stopped' : 'connected';
			// The process needs a moment before the port answers.
			setTimeout(() => void refreshStatus(name), 1500);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			void refreshStatus(name);
		} finally {
			controlling = false;
		}
	}

	$effect(() => {
		const name = agent;
		if (!name) return;
		void refreshStatus(name);
		const timer = setInterval(() => void refreshStatus(name), 20_000);
		return () => clearInterval(timer);
	});

	async function loadHistory(name: string, reset: boolean) {
		if (!name || loading) return;
		if (!reset && (!loaded || !hasMore)) return;
		const current = generation;
		const pageOffset = reset ? 0 : offset;
		const revision = unreadChats.revision();
		loading = true;
		try {
			const result = await conversationsApi.list(name, { limit: PAGE, offset: pageOffset });
			if (current !== generation) return;
			const page = result.conversations ?? [];
			items = reset
				? page
				: [...items, ...page.filter((item) => !items.some((known) => known.id === item.id))];
			offset = pageOffset + page.length;
			hasMore = page.length > 0 && result.has_more;
			loaded = true;
			unreadChats.observe(name, page, revision);
		} catch (err) {
			if (current === generation) toastStore.error(getErrorMessage(err));
		} finally {
			if (current === generation) loading = false;
		}
	}

	/**
	 * Re-reads the first page in place: a run that finished elsewhere changed a
	 * title, a message count, or the order. Older pages stay where they are.
	 */
	async function reconcile(name: string) {
		if (!name || loading) return;
		const current = generation;
		const revision = unreadChats.revision();
		try {
			const result = await conversationsApi.list(name, { limit: PAGE, offset: 0 });
			if (current !== generation) return;
			const page = result.conversations ?? [];
			if (page.length === 0) return;
			const fresh = new Set(page.map((item) => item.id));
			items = [...page, ...items.filter((item) => !fresh.has(item.id))];
			offset = Math.max(offset, page.length);
			hasMore = hasMore || result.has_more;
			loaded = true;
			unreadChats.observe(name, page, revision);
		} catch {
			// Keep what is on screen; the next refresh tries again.
		}
	}

	async function loadFiles(name: string) {
		try {
			const result = await contextApi.list(name);
			const next: Record<string, ContextFile> = {};
			for (const file of result.files ?? []) next[file.path] = file;
			files = next;
		} catch {
			files = {};
		}
	}

	function dayLabel(date: Date) {
		if (Number.isNaN(date.getTime())) return '';
		const startOfDay = (value: Date) =>
			new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime();
		const diff = Math.round((startOfDay(new Date()) - startOfDay(date)) / 86_400_000);
		if (diff === 0) return t('myaaw.focus.history_today');
		if (diff === 1) return t('myaaw.focus.history_yesterday');
		return date.toLocaleDateString(locale.value || undefined, { month: 'short', day: 'numeric' });
	}

	function formatTime(value: string) {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleTimeString(locale.value || undefined, {
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function conversationTitle(id: string) {
		return (
			items.find((item) => item.id === id)?.title ||
			approvalQueue.titleOf(agent, id) ||
			`${id.slice(0, 12)}…`
		);
	}

	function formatSize(bytes: number) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function fileMeta(file: ContextFile | undefined) {
		if (!file) return t('myaaw.focus.context_empty');
		return formatSize(file.size);
	}

	/** The icon a context file gets, by its known name; anything else is a plain file. */
	function iconFor(file: ContextFile) {
		switch (file.name.toUpperCase()) {
			case 'SOUL.MD':
				return Sparkles;
			case 'MEMORY.MD':
				return Brain;
			case 'AGENTS.MD':
				return Bot;
			case 'HEARTBEAT.MD':
				return HeartPulse;
			case 'IDENTITY.MD':
				return Fingerprint;
			case 'SECURITY.MD':
				return ShieldCheck;
			case 'TOOLS.MD':
				return Wrench;
			case 'USER.MD':
				return UserRound;
			case 'RULES.MD':
				return ScrollText;
			default:
				return FileText;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="relative flex shrink-0 flex-col items-center gap-1 px-4 pt-4 pb-2 lg:pt-16">
		<Button
			variant="ghost"
			size="icon-sm"
			class="absolute top-2 right-2 lg:hidden"
			onclick={onClose}
			aria-label={t('myaaw.focus.panel_close')}
			title={t('myaaw.focus.panel_close')}
		>
			<X class="h-4 w-4" />
		</Button>

		<Bloub
			mood="attentif"
			size={64}
			tight
			color={agentColor}
			class="text-lg font-semibold uppercase"
		>
			{#snippet fallback()}
				{initial}
			{/snippet}
		</Bloub>
		<p class="max-w-full truncate text-sm font-semibold">{agent}</p>
		<p class="flex items-center gap-1.5 text-xs text-muted-foreground">
			<span
				aria-hidden="true"
				class={[
					'h-2 w-2 rounded-full',
					status === 'connected' ? 'bg-emerald-500' : 'bg-muted-foreground/40'
				]}
			></span>
			{statusLabel}
		</p>
	</div>

	<div role="tablist" class="mx-3 mb-2 grid shrink-0 grid-cols-3 gap-1 rounded-lg border p-0.5">
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'history'}
			class={[
				'flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
				tab === 'history'
					? 'bg-primary text-primary-foreground'
					: 'text-muted-foreground hover:bg-muted hover:text-foreground'
			]}
			onclick={() => (tab = 'history')}
		>
			<History class="h-3.5 w-3.5" />
			{t('myaaw.focus.tab_history')}
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'context'}
			class={[
				'flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
				tab === 'context'
					? 'bg-primary text-primary-foreground'
					: 'text-muted-foreground hover:bg-muted hover:text-foreground'
			]}
			onclick={() => (tab = 'context')}
		>
			<Brain class="h-3.5 w-3.5" />
			{t('myaaw.focus.tab_context')}
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'control'}
			class={[
				'flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
				tab === 'control'
					? 'bg-primary text-primary-foreground'
					: 'text-muted-foreground hover:bg-muted hover:text-foreground'
			]}
			onclick={() => (tab = 'control')}
		>
			<Server class="h-3.5 w-3.5" />
			{t('myaaw.focus.tab_control')}
		</button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto px-2 pb-3" bind:this={scroller}>
		{#if tab === 'history'}
			{#if loading && items.length === 0}
				<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
			{:else if items.length === 0 && approvals.length === 0}
				<div class="flex flex-col items-center gap-2 px-2 py-4 text-center">
					<Bloub mood="timide" size={40} tight color={agentColor} />
					<p class="text-xs text-muted-foreground">{t('myaaw.focus.history_empty')}</p>
				</div>
			{/if}

			{#if approvals.length > 0}
				<p
					class="flex items-center gap-1 px-2 pt-2 pb-1 text-[10px] font-semibold tracking-widest text-amber-600 uppercase dark:text-amber-400"
				>
					<ShieldAlert class="h-3 w-3 shrink-0" />
					{t('myaaw.chat.need_approval')}
				</p>
				{#each approvals as entry (entry.approvalId)}
					<button
						type="button"
						class="mb-1 flex w-full items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-2 text-left transition-colors hover:bg-amber-500/15"
						onclick={() => onOpen(entry.conversationId)}
					>
						<span
							class="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-amber-500/20 text-amber-700 dark:text-amber-300"
						>
							<ShieldAlert class="h-3.5 w-3.5" />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-xs font-medium">
								{conversationTitle(entry.conversationId)}
							</span>
							<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
								{entry.toolName}{entry.subAgent ? ` · ${entry.subAgent}` : ''}
							</span>
						</span>
					</button>
				{/each}
			{/if}

			{#each groups as group (group.key)}
				<p
					class="px-2 pt-3 pb-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
				>
					{group.label}
				</p>
				{#each group.items as item (item.id)}
					<button
						type="button"
						data-conversation-id={item.id}
						class={[
							'flex w-full items-start gap-2 rounded-lg p-2 text-left transition-colors list-row',
							item.id === activeId ? 'bg-accent' : 'hover:bg-muted'
						]}
						onclick={() => onOpen(item.id)}
					>
						<span
							class="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
						>
							<MessagesSquare class="h-3.5 w-3.5" />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-xs font-medium">
								{item.title || item.id.slice(0, 12)}
							</span>
							<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
								{formatTime(item.updated_at)} ·
								{t('myaaw.conversations.messages', { count: item.message_count })}
							</span>
						</span>
						{#if (unreadChats.has(agent, item.id) || approvalQueue.has(agent, item.id)) && item.id !== activeId}
							<span
								class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-primary"
								aria-label={t('myaaw.chat.unread')}
							></span>
						{/if}
					</button>
				{/each}
			{/each}

			{#if hasMore}
				<div use:sentinel class="flex h-8 items-center justify-center">
					{#if loading}
						<Loader2 class="h-3.5 w-3.5 animate-spin text-muted-foreground" />
					{/if}
				</div>
			{/if}
		{:else if tab === 'context'}
			<div class="space-y-2 pt-1">
				<button
					type="button"
					class="flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-muted"
					onclick={() => onOpenContext(CONTEXT_SOUL)}
				>
					<span
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
					>
						<Sparkles class="h-5 w-5" />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium">{t('myaaw.focus.context_soul')}</span>
						<span class="block truncate text-xs text-muted-foreground">
							{CONTEXT_SOUL} · {fileMeta(soul)}
						</span>
					</span>
					<ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground" />
				</button>

				<button
					type="button"
					class="flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-muted"
					onclick={() => onOpenContext(CONTEXT_MEMORY)}
				>
					<span
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
					>
						<Brain class="h-5 w-5" />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium">{t('myaaw.focus.context_memory')}</span>
						<span class="block truncate text-xs text-muted-foreground">
							{CONTEXT_MEMORY} · {fileMeta(memory)}
						</span>
					</span>
					<ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground" />
				</button>

				{#if contextFiles.length > 0}
					{#each contextFiles as file (file.path)}
						{@const Icon = iconFor(file)}
						<button
							type="button"
							class="flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-muted"
							onclick={() => onOpenContext(file.path)}
						>
							<span
								class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
							>
								<Icon class="h-5 w-5" />
							</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium">
									{file.name.replace(/\.md$/i, '')}
								</span>
								<span class="block truncate text-xs text-muted-foreground">
									{file.path} · {formatSize(file.size)}
								</span>
							</span>
							<ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground" />
						</button>
					{/each}
				{/if}

				{#if datedMemory.length > 0}
					<!-- One note per day: folded by default so the section never crowds the
					     files above it, with the count as the hint that they exist. -->
					<button
						type="button"
						class="flex w-full items-center gap-1 rounded-lg px-2 py-1.5 text-left text-[10px] font-semibold tracking-widest text-muted-foreground uppercase transition-colors hover:text-foreground"
						aria-expanded={memoryOpen}
						onclick={() => (memoryOpen = !memoryOpen)}
					>
						{#if memoryOpen}
							<ChevronDown class="h-3 w-3 shrink-0" />
						{:else}
							<ChevronRight class="h-3 w-3 shrink-0" />
						{/if}
						{t('myaaw.focus.memory_history')}
						<span
							class="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-[9px] tracking-normal text-muted-foreground"
						>
							{datedMemory.length}
						</span>
					</button>
					{#if memoryOpen}
						{#each datedMemory as file (file.path)}
							<button
								type="button"
								class="flex w-full items-center gap-2 rounded-lg p-2 text-left transition-colors hover:bg-muted"
								onclick={() => onOpenContext(file.path)}
							>
								<CalendarClock class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								<span class="min-w-0 flex-1 truncate font-mono text-[11px]">
									{file.name.replace('.md', '')}
								</span>
								<span class="shrink-0 text-[10px] text-muted-foreground">
									{formatSize(file.size)}
								</span>
								<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
							</button>
						{/each}
					{/if}
				{/if}
			</div>
		{:else}
			<div class="space-y-2 pt-1">
				<div class="rounded-xl border p-3">
					<p class="flex items-center gap-2 text-sm font-medium">
						<span
							aria-hidden="true"
							class={[
								'h-2 w-2 rounded-full',
								status === 'connected' ? 'bg-emerald-500' : 'bg-muted-foreground/40'
							]}
						></span>
						{statusLabel}
					</p>
					<p class="mt-1 text-xs text-muted-foreground">{t('myaaw.focus.control_hint')}</p>
					<div class={status === 'connected' ? 'mt-3 grid grid-cols-2 gap-2' : 'mt-3'}>
						{#if status === 'connected'}
							<Button
								size="sm"
								variant="outline"
								class="w-full"
								disabled={controlling}
								onclick={() => control('restart')}
							>
								<RotateCw class="mr-1.5 h-3.5 w-3.5" />
								{t('myaaw.server.restart')}
							</Button>
							<Button
								size="sm"
								variant="ghost"
								class="w-full text-destructive hover:text-destructive"
								disabled={controlling}
								onclick={() => control('stop')}
							>
								<Square class="mr-1.5 h-3.5 w-3.5" />
								{t('myaaw.server.stop')}
							</Button>
						{:else}
							<Button
								size="sm"
								class="w-full"
								disabled={controlling}
								onclick={() => control('start')}
							>
								<Play class="mr-1.5 h-3.5 w-3.5" />
								{t('myaaw.server.start')}
							</Button>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>
</div>
