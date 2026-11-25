<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		Loader2,
		MessageSquarePlus,
		MessagesSquare,
		Play,
		Plus,
		RefreshCw,
		Search,
		ShieldAlert,
		SlidersHorizontal,
		X
	} from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, toastStore } from '$lib/stores';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { botPreviews, streamingReplyFromItems } from '$lib/stores/botPreviews.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { AgentInfo } from '$lib/types';

	const { t, locale } = useTranslation();

	let query = $state('');
	let busyBot = $state('');
	let refreshing = $state(false);

	function activeStream(name: string) {
		return chatRuns.all().find((run) => run.agent === name && run.streaming);
	}

	function isStreaming(name: string): boolean {
		return Boolean(activeStream(name));
	}

	function approvalCount(name: string): number {
		return approvalQueue.list(name).length;
	}

	function unreadCount(name: string): number {
		return unreadChats.count(name);
	}

	function activityTimestamp(name: string): number {
		const stream = activeStream(name);
		if (stream) return stream.startedAt || Date.now();
		const preview = botPreviews.get(name);
		const raw = preview?.activityAt || preview?.updatedAt;
		if (!raw) return 0;
		const parsed = Date.parse(raw);
		return Number.isFinite(parsed) ? parsed : 0;
	}

	function compareBots(a: AgentInfo, b: AgentInfo): number {
		const approvalDiff = Number(approvalCount(b.name) > 0) - Number(approvalCount(a.name) > 0);
		if (approvalDiff !== 0) return approvalDiff;
		const streamDiff = Number(isStreaming(b.name)) - Number(isStreaming(a.name));
		if (streamDiff !== 0) return streamDiff;
		const timeDiff = activityTimestamp(b.name) - activityTimestamp(a.name);
		if (timeDiff !== 0) return timeDiff;
		const defaultDiff = Number(Boolean(b.default)) - Number(Boolean(a.default));
		if (defaultDiff !== 0) return defaultDiff;
		return a.name.localeCompare(b.name);
	}

	const searchTerm = $derived(query.trim().toLowerCase());
	const bots = $derived(
		[...agentSelection.agents]
			.filter((bot) => !searchTerm || bot.name.toLowerCase().includes(searchTerm))
			.sort(compareBots)
	);

	const runningCount = $derived(agentSelection.agents.filter((bot) => bot.running).length);
	const attentionCount = $derived(
		agentSelection.agents.reduce(
			(sum, bot) => sum + unreadCount(bot.name) + approvalCount(bot.name),
			0
		)
	);

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
		return formatChatTimestamp(updated, locale.value, t('myaaw.chat.yesterday'));
	}

	function readLastConversation(name: string): string {
		if (typeof localStorage === 'undefined') return '';
		try {
			const raw = localStorage.getItem('myaaw_last_conversation');
			if (!raw) return '';
			const parsed: unknown = JSON.parse(raw);
			if (parsed && typeof parsed === 'object') {
				return (parsed as Record<string, string>)[name] ?? '';
			}
		} catch {
			// Ignore malformed localStorage state.
		}
		return '';
	}

	function clearLastConversation(name: string) {
		if (typeof localStorage === 'undefined') return;
		try {
			const raw = localStorage.getItem('myaaw_last_conversation');
			if (!raw) return;
			const parsed: unknown = JSON.parse(raw);
			if (parsed && typeof parsed === 'object' && name in parsed) {
				delete (parsed as Record<string, string>)[name];
				localStorage.setItem('myaaw_last_conversation', JSON.stringify(parsed));
			}
		} catch {
			// Ignore malformed localStorage state.
		}
	}

	function openBotChat(bot: AgentInfo) {
		const targetId =
			readLastConversation(bot.name) || botPreviews.get(bot.name)?.conversationId || '';
		agentSelection.select(bot.name);
		void goto(targetId ? `/app/chat/${encodeURIComponent(targetId)}` : '/app/chat');
	}

	async function startNewBotChat(bot: AgentInfo) {
		clearLastConversation(bot.name);
		agentSelection.select(bot.name);
		await tick();
		await goto('/app/chat?new=1');
	}

	async function startBotServer(bot: AgentInfo) {
		if (busyBot) return;
		busyBot = bot.name;
		agentSelection.setRunning(bot.name, true);
		try {
			await serverStore.start(bot.name);
			agentSelection.setRunning(bot.name, true);
			toastStore.success(t('myaaw.server.start_done'));
		} catch (err) {
			agentSelection.clearOptimisticRunning(bot.name);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			busyBot = '';
		}
	}

	async function refresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			await agentSelection.load(true);
		} finally {
			refreshing = false;
		}
	}
</script>

<TopBarActions id="bots-home-actions">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={refresh}
			loading={refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto flex w-full max-w-6xl flex-col gap-5">
	<!-- Summary & search header -->
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		<div class="space-y-1">
			<p class="text-sm text-muted-foreground">{t('myaaw.home.bots_subtitle')}</p>
			<div class="flex flex-wrap items-center gap-2 pt-0.5 text-xs text-muted-foreground">
				<span class="inline-flex items-center gap-1.5 rounded-full border bg-card px-2.5 py-0.5">
					<span class="font-semibold text-foreground tabular-nums">
						{agentSelection.agents.length}
					</span>
					{t('myaaw.bots.list_label')}
				</span>
				<span class="inline-flex items-center gap-1.5 rounded-full border bg-card px-2.5 py-0.5">
					<span class="h-2 w-2 rounded-full bg-emerald-500"></span>
					<span class="font-semibold text-foreground tabular-nums">{runningCount}</span>
					{t('myaaw.bots.filter_running')}
				</span>
				{#if attentionCount > 0}
					<span
						class="inline-flex items-center gap-1.5 rounded-full border border-primary/30 bg-accent px-2.5 py-0.5 text-foreground"
					>
						<span class="font-semibold tabular-nums">{attentionCount}</span>
						{t('myaaw.bots.filter_unread')}
					</span>
				{/if}
			</div>
		</div>

		<div class="relative w-full sm:w-64">
			<Search
				class="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<input
				type="search"
				bind:value={query}
				placeholder={t('myaaw.bots.search')}
				aria-label={t('myaaw.bots.search')}
				class="h-9 w-full rounded-xl border bg-card pr-8 pl-9 text-xs text-foreground placeholder:text-muted-foreground outline-none transition-colors focus:ring-2 focus:ring-ring/40"
			/>
			{#if query}
				<button
					type="button"
					onclick={() => (query = '')}
					aria-label={t('myaaw.conversations.clear_search')}
					class="absolute top-1/2 right-2 grid h-5 w-5 -translate-y-1/2 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
				>
					<X class="h-3 w-3" />
				</button>
			{/if}
		</div>
	</div>

	{#if agentSelection.error}
		<div
			role="alert"
			class="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{agentSelection.error}
		</div>
	{/if}

	<!-- Character cards grid -->
	<div class="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
		{#each bots as bot (bot.name)}
			{@const streaming = isStreaming(bot.name)}
			{@const approvals = approvalCount(bot.name)}
			{@const unreads = unreadCount(bot.name)}
			{@const timeLabel = previewTime(bot)}
			{@const busy = busyBot === bot.name}
			<div
				class="group flex flex-col justify-between rounded-2xl border bg-card p-4 transition-colors hover:border-foreground/20"
			>
				<div class="space-y-3">
					<div class="flex items-start justify-between gap-3">
						<button
							type="button"
							onclick={() => openBotChat(bot)}
							class="flex min-w-0 flex-1 items-center gap-3 text-left"
						>
							<span
								class="relative grid h-13 w-13 shrink-0 place-items-center rounded-2xl bg-muted/60 ring-1 ring-border/50"
							>
								<Bloub mood="attentif" size={44} tight color={colorOf(bot.agent_color)} />
								<RuntimeBadge
									runtimeType={bot.runtime_type}
									runtimePreset={bot.runtime_preset}
									size="md"
								/>
								<span
									aria-hidden="true"
									class={[
										'absolute -right-0.5 -bottom-0.5 h-3 w-3 rounded-full ring-2 ring-card',
										streaming
											? 'animate-pulse bg-primary'
											: bot.running
												? 'bg-emerald-500'
												: 'bg-muted-foreground/40'
									]}
								></span>
							</span>
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<h3 class="truncate text-base font-bold text-foreground">{bot.name}</h3>
									{#if bot.default}
										<Badge variant="secondary" class="px-1.5 py-0 text-[10px]">
											{t('myaaw.agent.default')}
										</Badge>
									{/if}
								</div>
								<p class="mt-0.5 text-xs text-muted-foreground">
									{#if streaming}
										<span class="font-medium text-primary">{t('myaaw.bots.thinking')}</span>
									{:else if bot.running}
										{t('myaaw.bots.ready')}
									{:else}
										{t('myaaw.bots.stopped')}
									{/if}
								</p>
							</div>
						</button>

						<div class="flex shrink-0 items-center gap-1">
							{#if approvals > 0}
								<span
									title={t('myaaw.chat.need_approval')}
									class="inline-flex h-5 items-center gap-1 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
								>
									<ShieldAlert class="h-3 w-3" />
									{approvals}
								</span>
							{/if}
							{#if unreads > 0}
								<span
									class="grid h-5 min-w-5 place-items-center rounded-full bg-primary px-1.5 text-[10px] font-semibold text-primary-foreground tabular-nums"
								>
									{unreads}
								</span>
							{/if}
							<Button
								variant="ghost"
								size="icon-sm"
								onclick={() => void goto(`/app/new?agent=${encodeURIComponent(bot.name)}`)}
								title={t('myaaw.bots.reconfigure')}
								aria-label={t('myaaw.bots.reconfigure')}
							>
								<SlidersHorizontal class="h-3.5 w-3.5 text-muted-foreground" />
							</Button>
						</div>
					</div>

					<!-- Latest response bubble -->
					<button
						type="button"
						onclick={() => openBotChat(bot)}
						class="flex w-full flex-col gap-1 rounded-xl bg-muted/50 px-3 py-2.5 text-left transition-colors hover:bg-muted"
					>
						<p class="line-clamp-2 text-xs text-foreground/90">
							{previewLine(bot)}
						</p>
						{#if timeLabel}
							<span class="text-[10px] text-muted-foreground tabular-nums">{timeLabel}</span>
						{/if}
					</button>
				</div>

				<div class="mt-3.5 flex items-center gap-2 pt-1">
					<Button size="sm" class="flex-1" onclick={() => openBotChat(bot)}>
						<MessagesSquare class="mr-1.5 h-3.5 w-3.5" />
						{t('myaaw.nav.chat')}
					</Button>
					<Button
						size="sm"
						variant="outline"
						onclick={() => void startNewBotChat(bot)}
						title={t('myaaw.chat.new')}
						aria-label={t('myaaw.chat.new')}
					>
						<MessageSquarePlus class="h-3.5 w-3.5" />
					</Button>
					{#if !bot.running}
						<Button
							size="sm"
							variant="secondary"
							disabled={busy}
							onclick={() => void startBotServer(bot)}
						>
							{#if busy}
								<Loader2 class="mr-1 h-3.5 w-3.5 animate-spin" />
							{:else}
								<Play class="mr-1 h-3.5 w-3.5" />
							{/if}
							{t('myaaw.server.start')}
						</Button>
					{/if}
				</div>
			</div>
		{/each}

		<!-- Create new bot card -->
		<button
			type="button"
			onclick={() => void goto('/app/new')}
			class="flex min-h-44 flex-col items-center justify-center gap-2 rounded-2xl border border-dashed bg-card/40 p-6 text-center text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/40 hover:text-foreground"
		>
			<span class="grid h-11 w-11 place-items-center rounded-2xl border border-dashed">
				<Plus class="h-5 w-5" />
			</span>
			<span class="text-sm font-semibold text-foreground">{t('myaaw.bots.new')}</span>
			<span class="max-w-52 text-xs text-muted-foreground">{t('myaaw.home.new_agent_desc')}</span>
		</button>
	</div>
</div>
