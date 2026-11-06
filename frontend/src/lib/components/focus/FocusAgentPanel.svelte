<script lang="ts">
	import { untrack } from 'svelte';
	import {
		Brain,
		ChevronRight,
		History,
		Loader2,
		MessagesSquare,
		ShieldAlert,
		Sparkles,
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
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ContextFile, ConversationSummary } from '$lib/types';

	interface Props {
		/** The focus agent this panel describes. */
		agent: string;
		/** The open conversation, highlighted in the history list. */
		activeId?: string;
		onOpen: (id: string) => void;
		onClose: () => void;
		/** Opens the full-page editor for one context file. */
		onOpenContext: (path: string) => void;
	}

	let { agent, activeId = '', onOpen, onClose, onOpenContext }: Props = $props();

	const { t, locale } = useTranslation();

	/** Conversations per page in the history tab. */
	const PAGE = 20;
	const CONTEXT_SOUL = 'SOUL.md';
	const CONTEXT_MEMORY = 'MEMORY.md';

	type Tab = 'history' | 'context';
	type Status = 'connected' | 'stopped' | 'unknown';

	let tab = $state<Tab>('history');
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
	$effect(() => {
		const name = agent;
		if (!name) return;
		let alive = true;
		const check = async () => {
			try {
				const result = await serverApi.status(name);
				if (alive) status = result.running ? 'connected' : 'stopped';
			} catch {
				if (alive) status = 'unknown';
			}
		};
		void check();
		const timer = setInterval(() => void check(), 20_000);
		return () => {
			alive = false;
			clearInterval(timer);
		};
	});

	async function loadHistory(name: string, reset: boolean) {
		if (!name || loading) return;
		if (!reset && (!loaded || !hasMore)) return;
		const current = generation;
		const pageOffset = reset ? 0 : offset;
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
		} catch (err) {
			if (current === generation) toastStore.error(getErrorMessage(err));
		} finally {
			if (current === generation) loading = false;
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

	<div role="tablist" class="mx-3 mb-2 grid shrink-0 grid-cols-2 gap-1 rounded-lg border p-0.5">
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
		{:else}
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
			</div>
		{/if}
	</div>
</div>
