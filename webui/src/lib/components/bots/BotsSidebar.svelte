<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Clock, Layers, Plus, Puzzle, Search, X } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import { Button } from '$lib/components/ui/button';
	import { colorOf } from '$lib/bloub';
	import { agentSelection } from '$lib/stores';
	import {
		botPreviews,
		lastAssistantFromMessages,
		streamingReplyFromItems
	} from '$lib/stores/botPreviews.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { chatUpdates } from '$lib/stores/conversationEvents.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref } from '$lib/utils/nav';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { AgentInfo } from '$lib/types';

	const { t, locale } = useTranslation();

	interface Props {
		/** Icon-only rail: avatars stay visible, labels and search fold away. */
		compact?: boolean;
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, onClose }: Props = $props();

	let query = $state('');
	const fetchedAgents = new Set<string>();

	// Delay swapping between compact rail and expanded list until the 200ms
	// sidebar width transition settles, preventing clipped text mid-slide.
	let contentCompact = $state(untrack(() => compact));
	$effect(() => {
		const target = compact;
		if (target === contentCompact) return;
		const timer = setTimeout(() => (contentCompact = target), 200);
		return () => clearTimeout(timer);
	});

	async function hydrateConversationPreview(
		name: string,
		conversationId: string,
		fallbackTitle: string,
		fallbackUpdatedAt: string,
		force: boolean
	) {
		try {
			const detail = await conversationsStore.fetchDetail(name, conversationId, {
				limit: 12,
				offset: -1
			});
			const last = lastAssistantFromMessages(detail.messages ?? []);
			botPreviews.seed(
				name,
				{
					conversationId,
					text: last.text,
					title: detail.title || fallbackTitle,
					updatedAt: last.updatedAt || detail.updated_at || fallbackUpdatedAt
				},
				force
			);
		} catch {
			// Fall back to the summary title when the detail fetch is unavailable.
			botPreviews.seed(
				name,
				{
					conversationId,
					text: '',
					title: fallbackTitle,
					updatedAt: fallbackUpdatedAt
				},
				force
			);
		}
	}

	async function refreshLatest(name: string, force = false) {
		try {
			const res = await conversationsStore.fetchList(name, { limit: 1 });
			const rows = res.conversations ?? [];
			if (rows.length > 0) {
				unreadChats.observe(name, rows, unreadChats.revision());
			}
			const top = rows[0];
			if (!top) {
				botPreviews.seed(name, { conversationId: '', text: '', title: '', updatedAt: '' }, force);
				return;
			}
			await hydrateConversationPreview(name, top.id, top.title ?? '', top.updated_at ?? '', force);
		} catch {
			// Offline or stopped agents simply fall back to their status line.
		}
	}

	// Fetch the latest assistant response once for each bot so every row shows
	// a meaningful preview line and timestamp like a messaging inbox.
	$effect(() => {
		const list = agentSelection.agents;
		if (list.length === 0) return;
		untrack(() => {
			for (const bot of list) {
				if (fetchedAgents.has(bot.name)) continue;
				fetchedAgents.add(bot.name);
				void refreshLatest(bot.name, false);
			}
		});
	});

	// Keep background/scheduled turns fresh for bots whose chat is not currently open.
	$effect(() => {
		return chatUpdates.subscribe((event) => {
			if (!event.agent || !event.conversation_id) return;
			const openOnScreen = unreadChats.isOpen(event.agent, event.conversation_id);
			if (openOnScreen) return;
			void hydrateConversationPreview(event.agent, event.conversation_id, '', event.at ?? '', true);
		});
	});

	const searchTerm = $derived(query.trim().toLowerCase());
	const filteredAgents = $derived(
		searchTerm
			? agentSelection.agents.filter((bot) => bot.name.toLowerCase().includes(searchTerm))
			: agentSelection.agents
	);

	const onChatRoute = $derived(
		page.url.pathname === '/app/chat' || page.url.pathname.startsWith('/app/chat/')
	);

	const utilityLinks = $derived([
		{
			href: '/app/artifacts',
			label: t('myaaw.nav.artifacts'),
			icon: Layers,
			active: isActiveHref(page.url.pathname, '/app/artifacts')
		},
		{
			href: '/app/schedule',
			label: t('myaaw.nav.schedule'),
			icon: Clock,
			active: isActiveHref(page.url.pathname, '/app/schedule')
		},
		{
			href: '/app/plugins',
			label: t('myaaw.plugins.title'),
			icon: Puzzle,
			active: isActiveHref(page.url.pathname, '/app/plugins')
		}
	]);

	function selectBot(bot: AgentInfo) {
		const switched = agentSelection.selected !== bot.name;
		agentSelection.select(bot.name);
		if (!onChatRoute || switched) {
			void goto('/app/chat');
		}
		onClose();
	}

	function createBot() {
		void goto('/app/new');
		onClose();
	}

	function activeStream(name: string) {
		return chatRuns.all().find((run) => run.agent === name && run.streaming);
	}

	function isStreaming(name: string): boolean {
		return Boolean(activeStream(name));
	}

	function badgeCount(name: string): number {
		return unreadChats.count(name) + approvalQueue.list(name).length;
	}

	function previewLine(bot: AgentInfo): string {
		const run = activeStream(bot.name);
		if (run) {
			const liveReply = streamingReplyFromItems(run.items);
			return liveReply || t('myaaw.bots.thinking');
		}
		const preview = botPreviews.get(bot.name);
		if (preview?.text) return preview.text;
		if (preview?.title) return preview.title;
		return bot.running ? t('myaaw.bots.ready') : t('myaaw.bots.stopped');
	}

	function previewTime(bot: AgentInfo): string {
		const updated = botPreviews.get(bot.name)?.updatedAt;
		if (!updated) return '';
		const formatted = formatChatTimestamp(updated, locale.value, t('myaaw.chat.yesterday'));
		// Keep the compact time or date portion so it fits cleanly beside the name.
		const parts = formatted.split(' · ');
		return parts[0] ?? formatted;
	}
</script>

<!--
	Bots mode turns the left sidebar into a direct bot roster (inspired by Grok
	Bots / OpenBot): search and add a bot at the top, pick any agent to jump into
	its chat, and keep quick utility shortcuts above the account menu.
-->
<SidebarBrand {compact} {onClose} />

{#if contentCompact}
	<div class="flex shrink-0 flex-col items-center gap-1 px-2 py-1.5">
		<Button
			variant="ghost"
			size="icon-sm"
			class="rounded-lg text-muted-foreground hover:text-foreground"
			onclick={createBot}
			title={t('myaaw.bots.new')}
			aria-label={t('myaaw.bots.new')}
		>
			<Plus class="h-4 w-4" />
		</Button>
	</div>
	<div class="mx-auto my-1 h-px w-6 bg-border"></div>
	<nav class="flex-1 space-y-1.5 overflow-y-auto px-2 py-1" aria-label={t('myaaw.bots.list_label')}>
		{#each agentSelection.agents as bot (bot.name)}
			{@const active = onChatRoute && agentSelection.selected === bot.name}
			{@const count = badgeCount(bot.name)}
			{@const streaming = isStreaming(bot.name)}
			<button
				type="button"
				onclick={() => selectBot(bot)}
				title={bot.name}
				aria-label={bot.name}
				aria-current={active ? 'page' : undefined}
				class={[
					'relative flex h-10 w-10 items-center justify-center rounded-xl transition-colors',
					active ? 'bg-accent' : 'hover:bg-muted'
				]}
			>
				{#if active}
					<span class="absolute inset-y-1.5 left-0 w-0.5 rounded-full bg-primary"></span>
				{/if}
				<Bloub mood="attentif" size={28} tight color={colorOf(bot.agent_color)} />
				{#if count > 0}
					<span
						class="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[9px] leading-4 font-semibold text-primary-foreground tabular-nums"
					>
						{count > 9 ? '9+' : count}
					</span>
				{:else}
					<span
						aria-hidden="true"
						class={[
							'absolute right-1 bottom-1 h-2 w-2 rounded-full ring-2 ring-card',
							streaming
								? 'animate-pulse bg-primary'
								: bot.running
									? 'bg-emerald-500'
									: 'bg-muted-foreground/40'
						]}
					></span>
				{/if}
			</button>
		{/each}
	</nav>
	<div class="mx-auto my-1 h-px w-6 bg-border"></div>
	<div class="flex shrink-0 flex-col items-center gap-1 px-2 py-1">
		{#each utilityLinks as item (item.href)}
			{@const Icon = item.icon}
			<a
				href={item.href}
				title={item.label}
				aria-label={item.label}
				aria-current={item.active ? 'page' : undefined}
				class={[
					'relative flex h-9 w-10 items-center justify-center rounded-md transition-colors',
					item.active
						? 'bg-accent text-accent-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				{#if item.active}
					<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
				{/if}
				<Icon class="h-4 w-4 shrink-0" />
			</a>
		{/each}
	</div>
{:else}
	<div class="flex shrink-0 items-center gap-2 px-3 py-2">
		<div class="relative min-w-0 flex-1">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<input
				type="search"
				bind:value={query}
				placeholder={t('myaaw.bots.search')}
				aria-label={t('myaaw.bots.search')}
				class="h-8 w-full rounded-lg bg-muted/60 pr-7 pl-8 text-xs text-foreground placeholder:text-muted-foreground outline-none transition-colors focus:bg-muted focus:ring-1 focus:ring-ring/40"
			/>
			{#if query}
				<button
					type="button"
					onclick={() => (query = '')}
					aria-label={t('myaaw.conversations.clear_search')}
					class="absolute top-1/2 right-1.5 grid h-5 w-5 -translate-y-1/2 place-items-center rounded-full text-muted-foreground hover:bg-background/80 hover:text-foreground"
				>
					<X class="h-3 w-3" />
				</button>
			{/if}
		</div>
		<button
			type="button"
			onclick={createBot}
			title={t('myaaw.bots.new')}
			aria-label={t('myaaw.bots.new')}
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-dashed text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
		>
			<Plus class="h-4 w-4" />
		</button>
	</div>

	<nav
		class="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 py-1"
		aria-label={t('myaaw.bots.list_label')}
	>
		{#if agentSelection.loading && agentSelection.agents.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
		{:else if filteredAgents.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">
				{searchTerm ? t('myaaw.bots.no_matches') : t('myaaw.agent.none')}
			</p>
		{:else}
			{#each filteredAgents as bot (bot.name)}
				{@const active = onChatRoute && agentSelection.selected === bot.name}
				{@const count = badgeCount(bot.name)}
				{@const streaming = isStreaming(bot.name)}
				{@const timeLabel = previewTime(bot)}
				<button
					type="button"
					onclick={() => selectBot(bot)}
					aria-current={active ? 'page' : undefined}
					class={[
						'group flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors list-row',
						active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/70'
					]}
				>
					<span
						class="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-muted/60 ring-1 ring-border/40"
					>
						<Bloub mood="attentif" size={30} tight color={colorOf(bot.agent_color)} />
						<span
							aria-hidden="true"
							class={[
								'absolute -right-0.5 -bottom-0.5 h-2.5 w-2.5 rounded-full ring-2 ring-card',
								streaming
									? 'animate-pulse bg-primary'
									: bot.running
										? 'bg-emerald-500'
										: 'bg-muted-foreground/40'
							]}
						></span>
					</span>

					<span class="min-w-0 flex-1">
						<span class="flex items-center justify-between gap-1.5">
							<span class="truncate text-sm font-semibold text-foreground">{bot.name}</span>
							{#if timeLabel}
								<span class="shrink-0 text-[10px] text-muted-foreground tabular-nums">
									{timeLabel}
								</span>
							{/if}
						</span>
						<span class="mt-0.5 flex items-center justify-between gap-1.5">
							<span
								class={[
									'truncate text-xs',
									streaming ? 'font-medium text-primary' : 'text-muted-foreground'
								]}
							>
								{previewLine(bot)}
							</span>
							{#if count > 0}
								<span
									class="grid min-w-4 shrink-0 place-items-center rounded-full bg-primary px-1 text-[10px] leading-4 font-semibold text-primary-foreground tabular-nums"
								>
									{count > 99 ? '99+' : count}
								</span>
							{/if}
						</span>
					</span>
				</button>
			{/each}
		{/if}
	</nav>

	<div class="shrink-0 space-y-0.5 px-2 pt-2 pb-1">
		{#each utilityLinks as item (item.href)}
			{@const Icon = item.icon}
			<a
				href={item.href}
				aria-current={item.active ? 'page' : undefined}
				class={[
					'relative flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors list-row',
					item.active
						? 'bg-accent font-medium text-accent-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				<Icon class="h-4 w-4 shrink-0" />
				<span class="truncate">{item.label}</span>
			</a>
		{/each}
	</div>
{/if}

<SidebarAccount {compact} />
