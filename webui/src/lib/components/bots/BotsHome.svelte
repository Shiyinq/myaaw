<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		ArrowUpRight,
		Bot,
		Ellipsis,
		LayoutGrid,
		List,
		Loader2,
		MessageSquarePlus,
		Play,
		Plus,
		RefreshCw,
		RotateCw,
		Search,
		ShieldAlert,
		SlidersHorizontal,
		Square,
		Users,
		X
	} from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { groupAccent } from '$lib/components/groups/accent';
	import { colorOf, type BloubCycle, type BloubMood } from '$lib/bloub';
	import { agentSelection, groupsStore, toastStore } from '$lib/stores';
	import { createAgentDialog } from '$lib/stores/createAgentDialog.svelte';
	import { fleetBulk, type FleetBulkKind } from '$lib/stores/fleetBulk.svelte';
	import {
		BOTS_SECTION_AGENTS,
		BOTS_SECTION_GROUPS,
		botsSidebarStore
	} from '$lib/stores/botsSidebar.svelte';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { botPreviews, streamingReplyFromItems } from '$lib/stores/botPreviews.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { isAuthenticated } from '$lib/stores/authStatus.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { AgentInfo, GroupSummary } from '$lib/types';

	type LoungeTab = 'all' | 'running' | 'stopped' | 'groups' | `sec:${string}`;
	type ViewMode = 'grid' | 'list';
	type AttentionItem = { kind: 'group'; group: GroupSummary } | { kind: 'bot'; bot: AgentInfo };

	const { t, locale } = useTranslation();

	const VIEW_STORAGE_KEY = 'myaaw_bots_lounge_view';

	function loadViewMode(): ViewMode {
		if (typeof localStorage === 'undefined') return 'grid';
		return localStorage.getItem(VIEW_STORAGE_KEY) === 'list' ? 'list' : 'grid';
	}

	let query = $state('');
	let activeTab = $state<LoungeTab>('all');
	let viewMode = $state<ViewMode>(loadViewMode());
	let hoveredBot = $state('');
	let busyBot = $state('');
	let busyBotAction = $state<'' | 'start' | 'stop' | 'restart'>('');
	let refreshing = $state(false);

	$effect(() => {
		if (!isAuthenticated.value) return;
		untrack(() => void groupsStore.load());
	});

	function setViewMode(mode: ViewMode) {
		viewMode = mode;
		if (typeof localStorage !== 'undefined') {
			try {
				localStorage.setItem(VIEW_STORAGE_KEY, mode);
			} catch {
				// Ignore storage errors.
			}
		}
	}

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

	function botNeedsAttention(name: string): boolean {
		return approvalCount(name) > 0 || unreadCount(name) > 0;
	}

	function groupApprovals(grp: GroupSummary): number {
		return Math.max(approvalQueue.groupCount(grp.id), grp.pending_approval_count ?? 0);
	}

	function groupUnreads(grp: GroupSummary): number {
		return groupsStore.unreadCount(grp.id);
	}

	function groupNeedsAttention(grp: GroupSummary): boolean {
		return groupApprovals(grp) > 0 || groupUnreads(grp) > 0;
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

	function groupActivityTimestamp(grp: GroupSummary): number {
		if (grp.running) return Date.now();
		const raw = grp.last_message?.created_at || grp.updated_at;
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

	function compareGroups(a: GroupSummary, b: GroupSummary): number {
		const approvalDiff = Number(groupApprovals(b) > 0) - Number(groupApprovals(a) > 0);
		if (approvalDiff !== 0) return approvalDiff;
		const runDiff = Number(Boolean(b.running)) - Number(Boolean(a.running));
		if (runDiff !== 0) return runDiff;
		const timeDiff = groupActivityTimestamp(b) - groupActivityTimestamp(a);
		if (timeDiff !== 0) return timeDiff;
		return a.name.localeCompare(b.name);
	}

	function runtimeShortLabel(bot: AgentInfo): string {
		if (!bot.runtime_type || bot.runtime_type === 'myaaw') {
			return t('myaaw.onboard.runtime_builtin');
		}
		const raw = (bot.runtime_preset || bot.runtime_type).toLowerCase();
		switch (raw) {
			case 'antigravity':
				return 'Antigravity';
			case 'claude_code':
				return 'Claude Code';
			case 'codex':
				return 'Codex';
			case 'opencode':
				return 'OpenCode';
			case 'deepseek_harness':
				return 'DeepSeek';
			case 'pi':
				return 'Pi';
			case 'omp':
				return 'Oh My Pi';
			default:
				return 'ACP';
		}
	}

	function botBloubState(
		bot: AgentInfo,
		hovered: boolean
	): { mood: BloubMood; cycle?: BloubCycle } {
		if (isStreaming(bot.name)) return { mood: 'attentif', cycle: 'loading' };
		if (approvalCount(bot.name) > 0) return { mood: 'surpris' };
		if (unreadCount(bot.name) > 0) return { mood: 'excite' };
		if (!bot.running) return { mood: 'somnolent' };
		return { mood: hovered ? 'heureux' : 'attentif' };
	}

	function customSectionBadge(kind: 'agent' | 'group', id: string): string {
		const homeId = botsSidebarStore.homeSectionOf(kind, id);
		if (homeId === BOTS_SECTION_AGENTS || homeId === BOTS_SECTION_GROUPS) return '';
		return botsSidebarStore.getSection(homeId)?.name ?? '';
	}

	const searchTerm = $derived(query.trim().toLowerCase());
	const customSections = $derived(botsSidebarStore.sections);

	// Reset activeTab back to 'all' if the selected custom section was deleted.
	$effect(() => {
		if (activeTab.startsWith('sec:')) {
			const id = activeTab.slice(4);
			if (!customSections.some((s) => s.id === id)) {
				activeTab = 'all';
			}
		}
	});

	const runningCount = $derived(agentSelection.agents.filter((bot) => bot.running).length);
	const stoppedCount = $derived(agentSelection.agents.filter((bot) => !bot.running).length);
	const onlineAgents = $derived(
		[...agentSelection.agents].filter((bot) => bot.running).sort(compareBots)
	);

	function matchesBotSearch(bot: AgentInfo): boolean {
		if (!searchTerm) return true;
		const nameMatch = bot.name.toLowerCase().includes(searchTerm);
		const runtimeMatch = runtimeShortLabel(bot).toLowerCase().includes(searchTerm);
		const secMatch = customSectionBadge('agent', bot.name).toLowerCase().includes(searchTerm);
		return nameMatch || runtimeMatch || secMatch;
	}

	function matchesGroupSearch(grp: GroupSummary): boolean {
		if (!searchTerm) return true;
		const nameMatch = grp.name.toLowerCase().includes(searchTerm);
		const memberMatch = grp.members.some((m) => m.name.toLowerCase().includes(searchTerm));
		return nameMatch || memberMatch;
	}

	function compareAttentionItems(a: AttentionItem, b: AttentionItem): number {
		const aApprovals = a.kind === 'group' ? groupApprovals(a.group) : approvalCount(a.bot.name);
		const bApprovals = b.kind === 'group' ? groupApprovals(b.group) : approvalCount(b.bot.name);
		const approvalDiff = Number(bApprovals > 0) - Number(aApprovals > 0);
		if (approvalDiff !== 0) return approvalDiff;

		const aLive = a.kind === 'group' ? Boolean(a.group.running) : isStreaming(a.bot.name);
		const bLive = b.kind === 'group' ? Boolean(b.group.running) : isStreaming(b.bot.name);
		const liveDiff = Number(bLive) - Number(aLive);
		if (liveDiff !== 0) return liveDiff;

		const aTime =
			a.kind === 'group' ? groupActivityTimestamp(a.group) : activityTimestamp(a.bot.name);
		const bTime =
			b.kind === 'group' ? groupActivityTimestamp(b.group) : activityTimestamp(b.bot.name);
		if (bTime !== aTime) return bTime - aTime;

		const aName = a.kind === 'group' ? a.group.name : a.bot.name;
		const bName = b.kind === 'group' ? b.group.name : b.bot.name;
		return aName.localeCompare(bName);
	}

	const attentionAgents = $derived(
		[...agentSelection.agents].filter((bot) => botNeedsAttention(bot.name)).sort(compareBots)
	);
	const attentionGroups = $derived(
		[...groupsStore.groups].filter((grp) => groupNeedsAttention(grp)).sort(compareGroups)
	);
	const attentionAgentNames = $derived(new Set(attentionAgents.map((bot) => bot.name)));
	const attentionGroupIds = $derived(new Set(attentionGroups.map((grp) => grp.id)));

	const visibleAttentionItems = $derived.by(() => {
		const items: AttentionItem[] = [
			...attentionAgents
				.filter((bot) => matchesBotSearch(bot))
				.map((bot): AttentionItem => ({ kind: 'bot', bot })),
			...attentionGroups
				.filter((grp) => matchesGroupSearch(grp))
				.map((group): AttentionItem => ({ kind: 'group', group }))
		];
		return items.sort(compareAttentionItems);
	});
	const visibleAttentionCount = $derived(visibleAttentionItems.length);
	const hasPendingApprovals = $derived(
		visibleAttentionItems.some((item) =>
			item.kind === 'group' ? groupApprovals(item.group) > 0 : approvalCount(item.bot.name) > 0
		)
	);

	// Items temporarily lifted into "Needs your attention" are excluded from the
	// regular sections below so each bot/group card appears in exactly one place.
	const regularBots = $derived(
		agentSelection.agents.filter((bot) => !attentionAgentNames.has(bot.name))
	);
	const regularGroups = $derived(
		groupsStore.groups.filter((grp) => !attentionGroupIds.has(grp.id))
	);
	const tabAllCount = $derived(regularBots.length + regularGroups.length);
	const tabRunningCount = $derived(regularBots.filter((bot) => bot.running).length);
	const tabStoppedCount = $derived(regularBots.filter((bot) => !bot.running).length);
	const tabGroupsCount = $derived(regularGroups.length);

	function tabSectionCount(sectionId: string): number {
		let count = 0;
		for (const bot of regularBots) {
			if (botsSidebarStore.homeSectionOf('agent', bot.name) === sectionId) count += 1;
		}
		for (const grp of regularGroups) {
			if (botsSidebarStore.homeSectionOf('group', grp.id) === sectionId) count += 1;
		}
		return count;
	}

	const selectedCustomSectionId = $derived(activeTab.startsWith('sec:') ? activeTab.slice(4) : '');

	const filteredBots = $derived.by(() => {
		if (activeTab === 'groups') return [];
		return [...regularBots]
			.filter((bot) => {
				if (!matchesBotSearch(bot)) return false;
				if (activeTab === 'running' && !bot.running) return false;
				if (activeTab === 'stopped' && bot.running) return false;
				if (
					selectedCustomSectionId &&
					botsSidebarStore.homeSectionOf('agent', bot.name) !== selectedCustomSectionId
				) {
					return false;
				}
				return true;
			})
			.sort(compareBots);
	});

	const filteredGroups = $derived.by(() => {
		if (activeTab === 'running' || activeTab === 'stopped') return [];
		return [...regularGroups]
			.filter((grp) => {
				if (!matchesGroupSearch(grp)) return false;
				if (
					selectedCustomSectionId &&
					botsSidebarStore.homeSectionOf('group', grp.id) !== selectedCustomSectionId
				) {
					return false;
				}
				return true;
			})
			.sort(compareGroups);
	});

	const showEmptyState = $derived(
		filteredGroups.length === 0 &&
			filteredBots.length === 0 &&
			(visibleAttentionCount === 0 || activeTab !== 'all')
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

	function groupPreviewLine(grp: GroupSummary): string {
		if (grp.running) {
			return grp.current_agent
				? t('myaaw.groups.agent_thinking', { name: grp.current_agent })
				: t('myaaw.bots.thinking');
		}
		if (grp.last_message) {
			const who =
				grp.last_message.sender_type === 'user'
					? t('myaaw.groups.you')
					: grp.last_message.sender_name;
			const snippet = grp.last_message.content.replace(/\s+/g, ' ').trim();
			return who ? `${who}: ${snippet}` : snippet;
		}
		return t('myaaw.groups.members_count', { count: grp.member_count || grp.members.length });
	}

	function groupPreviewTime(grp: GroupSummary): string {
		const raw = grp.last_message?.created_at || grp.updated_at;
		if (!raw) return '';
		return formatChatTimestamp(raw, locale.value, t('myaaw.chat.yesterday'));
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

	function openGroupRoom(grp: GroupSummary) {
		void goto(`/app/groups/${encodeURIComponent(grp.id)}`);
	}

	async function startNewBotChat(bot: AgentInfo) {
		clearLastConversation(bot.name);
		agentSelection.select(bot.name);
		await tick();
		await goto('/app/chat?new=1');
	}

	async function runBotServerAction(bot: AgentInfo, kind: 'start' | 'stop' | 'restart') {
		if (busyBot || fleetBulk.busy) return;
		busyBot = bot.name;
		busyBotAction = kind;
		const nextRunning = kind !== 'stop';
		agentSelection.setRunning(bot.name, nextRunning);
		try {
			if (kind === 'start') await serverStore.start(bot.name);
			else if (kind === 'stop') await serverStore.stop(bot.name);
			else await serverStore.restart(bot.name);
			agentSelection.setRunning(bot.name, nextRunning);
			toastStore.success(
				t(
					kind === 'start'
						? 'myaaw.server.start_done'
						: kind === 'stop'
							? 'myaaw.server.stop_done'
							: 'myaaw.server.restart_done'
				)
			);
		} catch (err) {
			agentSelection.clearOptimisticRunning(bot.name);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			busyBot = '';
			busyBotAction = '';
		}
	}

	async function runFleetAction(kind: FleetBulkKind) {
		if (busyBot || fleetBulk.busy) return;
		await fleetBulk.run(kind, t);
	}

	function openNewAgentModal() {
		createAgentDialog.openCreate({
			onSuccess: () => {
				void goto('/app/chat');
			}
		});
	}

	async function refresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			await Promise.all([agentSelection.load(true), groupsStore.load(true)]);
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

<div class="mx-auto flex w-full max-w-6xl flex-col gap-5 pb-8">
	<!-- Living Lounge Hero Banner -->
	<section
		class="relative overflow-hidden rounded-3xl border bg-gradient-to-br from-card via-card to-muted/35 p-5 sm:p-6"
	>
		<div
			aria-hidden="true"
			class="pointer-events-none absolute -top-20 -right-16 h-56 w-56 rounded-full bg-primary/6 blur-3xl"
		></div>

		<div class="relative flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
			<div class="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
				<!-- Overlapping live mascot parade -->
				{#if onlineAgents.length > 0}
					<div class="flex shrink-0 items-center -space-x-2.5">
						{#each onlineAgents.slice(0, 5) as bot (bot.name)}
							{@const accent = groupAccent(bot.agent_color)}
							<button
								type="button"
								onclick={() => openBotChat(bot)}
								title={bot.name}
								aria-label={bot.name}
								class={[
									'relative grid h-11 w-11 place-items-center rounded-2xl border border-card shadow-xs ring-2 ring-card transition-transform hover:z-20 hover:-translate-y-1 hover:scale-105',
									accent.softBg
								]}
							>
								<Bloub
									mood={isStreaming(bot.name) ? 'attentif' : 'heureux'}
									cycle={isStreaming(bot.name) ? 'loading' : undefined}
									size={34}
									tight
									color={colorOf(bot.agent_color)}
								/>
							</button>
						{/each}
						{#if onlineAgents.length > 5}
							<span
								class="grid h-11 min-w-11 place-items-center rounded-2xl border border-border/60 bg-muted px-2 text-xs font-semibold text-muted-foreground ring-2 ring-card tabular-nums"
							>
								+{onlineAgents.length - 5}
							</span>
						{/if}
					</div>
				{:else}
					<div
						class="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-muted/70 ring-1 ring-border/50"
					>
						<Bloub mood="somnolent" size={38} tight />
					</div>
				{/if}

				<div class="min-w-0 space-y-1">
					<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
						<span class="inline-flex items-center gap-1.5 text-sm font-semibold text-foreground">
							<span class="h-2 w-2 rounded-full bg-emerald-500"></span>
							<span class="tabular-nums">{runningCount}</span>
							{t('myaaw.bots.filter_running')}
						</span>
						{#if stoppedCount > 0}
							<span class="text-xs text-muted-foreground">
								· <span class="font-medium text-foreground/80 tabular-nums">{stoppedCount}</span>
								{t('myaaw.bots.filter_stopped')}
							</span>
						{/if}
						{#if groupsStore.groups.length > 0}
							<span class="text-xs text-muted-foreground">
								· <span class="font-medium text-foreground/80 tabular-nums">
									{groupsStore.groups.length}
								</span>
								{t('myaaw.groups.section_groups')}
							</span>
						{/if}
					</div>
					<p class="text-xs text-muted-foreground sm:text-sm">
						{t('myaaw.home.bots_subtitle')}
					</p>
				</div>
			</div>

			<div class="flex shrink-0 flex-wrap items-center gap-2">
				<Button size="sm" onclick={openNewAgentModal} class="gap-1.5 rounded-xl">
					<Bot class="h-3.5 w-3.5" />
					{t('myaaw.groups.new_agent')}
				</Button>
				<Button
					size="sm"
					variant="outline"
					onclick={() => groupsStore.openCreateDialog()}
					class="gap-1.5 rounded-xl bg-background/70"
				>
					<Users class="h-3.5 w-3.5" />
					{t('myaaw.groups.create_group')}
				</Button>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button
								{...props}
								variant="outline"
								size="icon-sm"
								aria-label={t('myaaw.home.fleet_actions')}
								title={t('myaaw.home.fleet_actions')}
								class="rounded-xl bg-background/70"
							>
								{#if fleetBulk.busy}
									<Loader2 class="h-4 w-4 animate-spin" />
								{:else}
									<Ellipsis class="h-4 w-4" />
								{/if}
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end" class="w-60">
						<DropdownMenu.Item
							disabled={fleetBulk.busy !== '' ||
								Boolean(busyBot) ||
								runningCount === agentSelection.agents.length}
							onclick={() => void runFleetAction('start')}
						>
							{#if fleetBulk.busy === 'start'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<Play class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.start_all')}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							disabled={fleetBulk.busy !== '' || Boolean(busyBot) || runningCount === 0}
							onclick={() => void runFleetAction('restart')}
						>
							{#if fleetBulk.busy === 'restart'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<RotateCw class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.restart_all')}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							disabled={fleetBulk.busy !== '' || Boolean(busyBot) || runningCount === 0}
							onclick={() => void runFleetAction('stop')}
						>
							{#if fleetBulk.busy === 'stop'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<Square class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.stop_all')}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</div>
		</div>
	</section>

	<!-- Pinned Attention Section: unread & pending-approval cards move here temporarily -->
	{#if visibleAttentionCount > 0}
		<section
			aria-label={t('myaaw.bots.lounge_waiting')}
			class={[
				'space-y-3 rounded-3xl border p-4',
				hasPendingApprovals
					? 'border-amber-500/35 bg-amber-500/6'
					: 'border-primary/25 bg-primary/4'
			]}
		>
			<div class="flex items-center justify-between gap-2 px-0.5">
				<div class="flex items-center gap-2">
					{#if hasPendingApprovals}
						<ShieldAlert class="h-4 w-4 text-amber-600 dark:text-amber-400" />
					{:else}
						<span class="h-2 w-2 animate-pulse rounded-full bg-primary"></span>
					{/if}
					<h2 class="text-[11px] font-semibold tracking-widest text-foreground uppercase">
						{t('myaaw.bots.lounge_waiting')}
					</h2>
					<span
						class={[
							'grid h-5 min-w-5 place-items-center rounded-full px-1.5 text-[10px] font-semibold tabular-nums',
							hasPendingApprovals ? 'bg-amber-500 text-white' : 'bg-primary text-primary-foreground'
						]}
					>
						{visibleAttentionCount}
					</span>
				</div>
			</div>

			{#if viewMode === 'grid'}
				<div class="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
					{#each visibleAttentionItems as item (item.kind === 'group' ? `g:${item.group.id}` : `b:${item.bot.name}`)}
						{#if item.kind === 'group'}
							{@render groupCard(item.group, true)}
						{:else}
							{@render botGridCard(item.bot)}
						{/if}
					{/each}
				</div>
			{:else}
				<div class="divide-y overflow-hidden rounded-2xl border bg-card">
					{#each visibleAttentionItems as item (item.kind === 'group' ? `g:${item.group.id}` : `b:${item.bot.name}`)}
						{#if item.kind === 'group'}
							{@render groupListRow(item.group)}
						{:else}
							{@render botListRow(item.bot)}
						{/if}
					{/each}
				</div>
			{/if}
		</section>
	{/if}

	<!-- Filter Pills + Search + View Switcher -->
	<div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
		<div
			role="tablist"
			aria-label={t('myaaw.bots.filter_label')}
			class="flex flex-nowrap items-center gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
		>
			<button
				type="button"
				role="tab"
				aria-selected={activeTab === 'all'}
				onclick={() => (activeTab = 'all')}
				class={[
					'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
					activeTab === 'all'
						? 'border-foreground bg-foreground text-background'
						: 'border-border/80 bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				{t('myaaw.bots.filter_all')}
				<span class="text-[11px] opacity-75 tabular-nums">{tabAllCount}</span>
			</button>

			<button
				type="button"
				role="tab"
				aria-selected={activeTab === 'running'}
				onclick={() => (activeTab = 'running')}
				class={[
					'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
					activeTab === 'running'
						? 'border-foreground bg-foreground text-background'
						: 'border-border/80 bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				<span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
				{t('myaaw.bots.filter_running')}
				<span class="text-[11px] opacity-75 tabular-nums">{tabRunningCount}</span>
			</button>

			{#if tabStoppedCount > 0 || activeTab === 'stopped'}
				<button
					type="button"
					role="tab"
					aria-selected={activeTab === 'stopped'}
					onclick={() => (activeTab = 'stopped')}
					class={[
						'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
						activeTab === 'stopped'
							? 'border-foreground bg-foreground text-background'
							: 'border-border/80 bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{t('myaaw.bots.filter_stopped')}
					<span class="text-[11px] opacity-75 tabular-nums">{tabStoppedCount}</span>
				</button>
			{/if}

			{#if tabGroupsCount > 0 || activeTab === 'groups'}
				<button
					type="button"
					role="tab"
					aria-selected={activeTab === 'groups'}
					onclick={() => (activeTab = 'groups')}
					class={[
						'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
						activeTab === 'groups'
							? 'border-foreground bg-foreground text-background'
							: 'border-border/80 bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{t('myaaw.groups.section_groups')}
					<span class="text-[11px] opacity-75 tabular-nums">{tabGroupsCount}</span>
				</button>
			{/if}

			{#each customSections as sec (sec.id)}
				{@const tabKey = `sec:${sec.id}` as LoungeTab}
				{@const secCount = tabSectionCount(sec.id)}
				<button
					type="button"
					role="tab"
					aria-selected={activeTab === tabKey}
					onclick={() => (activeTab = tabKey)}
					class={[
						'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
						activeTab === tabKey
							? 'border-foreground bg-foreground text-background'
							: 'border-border/80 bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					<span class="max-w-28 truncate">{sec.name}</span>
					<span class="text-[11px] opacity-75 tabular-nums">{secCount}</span>
				</button>
			{/each}
		</div>

		<div class="flex shrink-0 items-center gap-2">
			<div class="relative min-w-0 flex-1 md:w-56 md:flex-initial">
				<Search
					class="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
				/>
				<input
					type="text"
					inputmode="search"
					role="searchbox"
					bind:value={query}
					placeholder={t('myaaw.bots.search')}
					aria-label={t('myaaw.bots.search')}
					class="h-8.5 w-full rounded-xl border bg-card pr-8 pl-8.5 text-xs text-foreground placeholder:text-muted-foreground outline-none transition-colors focus:border-ring focus:ring-2 focus:ring-ring/30"
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

			<div class="inline-flex shrink-0 items-center rounded-xl border bg-card p-0.5">
				<button
					type="button"
					onclick={() => setViewMode('grid')}
					aria-pressed={viewMode === 'grid'}
					title={t('myaaw.bots.lounge_view_grid')}
					aria-label={t('myaaw.bots.lounge_view_grid')}
					class={[
						'grid h-7 w-7 place-items-center rounded-lg transition-colors',
						viewMode === 'grid'
							? 'bg-muted text-foreground'
							: 'text-muted-foreground hover:text-foreground'
					]}
				>
					<LayoutGrid class="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					onclick={() => setViewMode('list')}
					aria-pressed={viewMode === 'list'}
					title={t('myaaw.bots.lounge_view_list')}
					aria-label={t('myaaw.bots.lounge_view_list')}
					class={[
						'grid h-7 w-7 place-items-center rounded-lg transition-colors',
						viewMode === 'list'
							? 'bg-muted text-foreground'
							: 'text-muted-foreground hover:text-foreground'
					]}
				>
					<List class="h-3.5 w-3.5" />
				</button>
			</div>
		</div>
	</div>

	{#if agentSelection.error}
		<div
			role="alert"
			class="rounded-2xl border border-destructive/30 bg-destructive/10 p-3.5 text-sm text-destructive"
		>
			{agentSelection.error}
		</div>
	{/if}

	<!-- Empty search/filter state -->
	{#if showEmptyState}
		<div
			class="flex flex-col items-center justify-center gap-3 rounded-3xl border border-dashed bg-card/40 px-6 py-14 text-center"
		>
			<Bloub mood="curieux" size={64} tight />
			<div class="space-y-1">
				<p class="text-sm font-semibold text-foreground">
					{searchTerm ? t('myaaw.bots.no_matches') : t('myaaw.bots.empty_filter')}
				</p>
				<p class="text-xs text-muted-foreground">{t('myaaw.home.new_agent_desc')}</p>
			</div>
			{#if searchTerm || activeTab !== 'all'}
				<Button
					size="sm"
					variant="outline"
					onclick={() => {
						query = '';
						activeTab = 'all';
					}}
				>
					{t('myaaw.bots.filter_all')}
				</Button>
			{:else}
				<Button size="sm" onclick={openNewAgentModal}>
					<Plus class="mr-1.5 h-3.5 w-3.5" />
					{t('myaaw.groups.new_agent')}
				</Button>
			{/if}
		</div>
	{:else}
		<!-- Group Rooms Showcase -->
		{#if filteredGroups.length > 0}
			<section class="space-y-2.5">
				<div class="flex items-center justify-between px-0.5">
					<h2 class="text-[11px] font-semibold tracking-widest text-muted-foreground uppercase">
						{t('myaaw.bots.lounge_rooms')}
					</h2>
					<span class="text-[11px] font-medium text-muted-foreground tabular-nums">
						{filteredGroups.length}
					</span>
				</div>

				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each filteredGroups as grp (grp.id)}
						{@render groupCard(grp)}
					{/each}
				</div>
			</section>
		{/if}

		<!-- Agents Gallery or Compact List -->
		{#if filteredBots.length > 0}
			<section class="space-y-2.5">
				<div class="flex items-center justify-between px-0.5">
					<h2 class="text-[11px] font-semibold tracking-widest text-muted-foreground uppercase">
						{t('myaaw.bots.section_all')}
					</h2>
					<span class="text-[11px] font-medium text-muted-foreground tabular-nums">
						{filteredBots.length}
					</span>
				</div>

				{#if viewMode === 'grid'}
					<div class="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
						{#each filteredBots as bot (bot.name)}
							{@render botGridCard(bot)}
						{/each}
					</div>
				{:else}
					<!-- Sleek Dense List View -->
					<div class="divide-y overflow-hidden rounded-2xl border bg-card">
						{#each filteredBots as bot (bot.name)}
							{@render botListRow(bot)}
						{/each}
					</div>
				{/if}
			</section>
		{/if}
	{/if}
</div>

{#snippet groupCard(grp: GroupSummary, featured = false)}
	{@const approvals = groupApprovals(grp)}
	{@const unreads = groupUnreads(grp)}
	{@const timeLabel = groupPreviewTime(grp)}
	{@const secBadge = customSectionBadge('group', grp.id)}
	{@const accent = groupAccent(grp.members[0]?.agent_color)}
	{#if featured}
		<button
			type="button"
			onclick={() => openGroupRoom(grp)}
			class="group relative flex flex-col justify-between overflow-hidden rounded-2xl border bg-card text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-md"
		>
			<!-- Subtle color-matched top wash (mirrors single agent card) -->
			<div
				aria-hidden="true"
				class={['pointer-events-none h-1 w-full opacity-75', accent.dot]}
			></div>

			<div class="flex flex-1 flex-col gap-3.5 p-4">
				<div class="flex items-start justify-between gap-3">
					<div class="flex min-w-0 items-center gap-3">
						{#if grp.members.length === 1}
							<span
								class={[
									'relative grid h-13 w-13 shrink-0 place-items-center rounded-2xl ring-1 transition-transform duration-200 group-hover:scale-105',
									accent.softBg,
									accent.ring
								]}
							>
								<Bloub
									mood={approvals > 0 ? 'surpris' : unreads > 0 ? 'excite' : 'attentif'}
									size={44}
									tight
									color={colorOf(grp.members[0]?.agent_color)}
								/>
							</span>
						{:else}
							<div class="flex h-13 shrink-0 items-center -space-x-2">
								{#if grp.members.length > 0}
									{#each grp.members.slice(0, 3) as member (member.name)}
										<span
											class={[
												'grid h-9 w-9 place-items-center rounded-full ring-2 ring-card',
												groupAccent(member.agent_color).softBg
											]}
										>
											<Bloub mood="attentif" size={24} tight color={colorOf(member.agent_color)} />
										</span>
									{/each}
									{#if grp.members.length > 3}
										<span
											class="grid h-9 min-w-9 place-items-center rounded-full bg-muted px-1.5 text-[10px] font-semibold text-muted-foreground ring-2 ring-card tabular-nums"
										>
											+{grp.members.length - 3}
										</span>
									{/if}
								{:else}
									<span class="grid h-13 w-13 place-items-center rounded-2xl bg-muted">
										<Users class="h-5 w-5 text-muted-foreground" />
									</span>
								{/if}
							</div>
						{/if}

						<div class="min-w-0">
							<h3 class="truncate text-base font-bold tracking-tight text-foreground">
								{grp.name}
							</h3>
							<p class="mt-0.5 truncate text-[11px] text-muted-foreground">
								{t('myaaw.groups.members_count', {
									count: grp.member_count || grp.members.length
								})}
								{#if secBadge}
									· {secBadge}
								{/if}
							</p>
						</div>
					</div>

					<div class="flex shrink-0 items-center gap-1">
						{#if approvals > 0}
							<span
								title={t('myaaw.chat.need_approval')}
								class="inline-flex h-5 items-center gap-0.5 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
							>
								<ShieldAlert class="h-2.5 w-2.5" />
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
						<ArrowUpRight
							class="h-4 w-4 text-muted-foreground/40 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
						/>
					</div>
				</div>

				<!-- Accent-bordered activity quote (matches single agent card) -->
				<div class={['min-h-10 border-l-2 pl-3', accent.border]}>
					<p
						class={[
							'line-clamp-2 text-xs leading-relaxed',
							grp.running ? 'font-medium text-primary' : 'text-foreground/85'
						]}
					>
						{groupPreviewLine(grp)}
					</p>
				</div>
			</div>

			<!-- Quiet footer with timestamp only -->
			<div
				class="flex min-h-[43px] items-center justify-between gap-2 border-t border-border/50 bg-muted/20 px-3.5 py-2"
			>
				<span class="truncate text-[10px] text-muted-foreground tabular-nums">
					{timeLabel ||
						t('myaaw.groups.members_count', { count: grp.member_count || grp.members.length })}
				</span>
			</div>
		</button>
	{:else}
		<button
			type="button"
			onclick={() => openGroupRoom(grp)}
			class="group relative flex flex-col justify-between gap-3 rounded-2xl border bg-card p-4 text-left transition-all hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-md"
		>
			<div class="flex items-start justify-between gap-3">
				<div class="flex min-w-0 items-center gap-3">
					<!-- Overlapping mini Bloubs of room members -->
					<div class="flex shrink-0 items-center -space-x-2">
						{#if grp.members.length > 0}
							{#each grp.members.slice(0, 3) as member (member.name)}
								<span
									class={[
										'grid h-8 w-8 place-items-center rounded-full ring-2 ring-card',
										groupAccent(member.agent_color).softBg
									]}
								>
									<Bloub mood="attentif" size={22} tight color={colorOf(member.agent_color)} />
								</span>
							{/each}
							{#if grp.members.length > 3}
								<span
									class="grid h-8 min-w-8 place-items-center rounded-full bg-muted px-1.5 text-[10px] font-semibold text-muted-foreground ring-2 ring-card tabular-nums"
								>
									+{grp.members.length - 3}
								</span>
							{/if}
						{:else}
							<span class="grid h-8 w-8 place-items-center rounded-full bg-muted">
								<Users class="h-4 w-4 text-muted-foreground" />
							</span>
						{/if}
					</div>

					<div class="min-w-0">
						<h3 class="truncate text-sm font-bold text-foreground">{grp.name}</h3>
						<p class="text-[11px] text-muted-foreground">
							{t('myaaw.groups.members_count', {
								count: grp.member_count || grp.members.length
							})}
							{#if secBadge}
								· {secBadge}
							{/if}
						</p>
					</div>
				</div>

				<div class="flex shrink-0 items-center gap-1">
					{#if approvals > 0}
						<span
							class="inline-flex h-5 items-center gap-0.5 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
						>
							<ShieldAlert class="h-2.5 w-2.5" />
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
					<ArrowUpRight
						class="h-4 w-4 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
					/>
				</div>
			</div>

			<div class="flex items-end justify-between gap-2 border-l-2 border-border pl-2.5">
				<p
					class={[
						'line-clamp-1 min-w-0 flex-1 text-xs',
						grp.running ? 'font-medium text-primary' : 'text-muted-foreground'
					]}
				>
					{groupPreviewLine(grp)}
				</p>
				{#if timeLabel}
					<span class="shrink-0 text-[10px] text-muted-foreground/80 tabular-nums">
						{timeLabel}
					</span>
				{/if}
			</div>
		</button>
	{/if}
{/snippet}

{#snippet groupListRow(grp: GroupSummary)}
	{@const approvals = groupApprovals(grp)}
	{@const unreads = groupUnreads(grp)}
	{@const timeLabel = groupPreviewTime(grp)}
	{@const secBadge = customSectionBadge('group', grp.id)}
	<button
		type="button"
		onclick={() => openGroupRoom(grp)}
		class="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/45"
	>
		<span class="flex shrink-0 items-center -space-x-2">
			{#if grp.members.length > 0}
				{#each grp.members.slice(0, 2) as member (member.name)}
					<span
						class={[
							'grid h-8 w-8 place-items-center rounded-full ring-2 ring-card',
							groupAccent(member.agent_color).softBg
						]}
					>
						<Bloub mood="attentif" size={22} tight color={colorOf(member.agent_color)} />
					</span>
				{/each}
			{:else}
				<span class="grid h-8 w-8 place-items-center rounded-full bg-muted">
					<Users class="h-4 w-4 text-muted-foreground" />
				</span>
			{/if}
		</span>

		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2">
				<span class="truncate text-sm font-semibold text-foreground">{grp.name}</span>
				<span class="hidden text-[11px] text-muted-foreground sm:inline">
					{t('myaaw.groups.members_count', { count: grp.member_count || grp.members.length })}
				</span>
				{#if secBadge}
					<span
						class="hidden rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground md:inline"
					>
						{secBadge}
					</span>
				{/if}
			</div>
			<p
				class={[
					'mt-0.5 truncate text-xs',
					grp.running ? 'font-medium text-primary' : 'text-muted-foreground'
				]}
			>
				{groupPreviewLine(grp)}
			</p>
		</div>

		<div class="flex shrink-0 items-center gap-1.5">
			{#if timeLabel}
				<span class="hidden text-[11px] text-muted-foreground tabular-nums sm:inline">
					{timeLabel}
				</span>
			{/if}
			{#if approvals > 0}
				<span
					class="inline-flex h-5 items-center gap-0.5 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
				>
					<ShieldAlert class="h-2.5 w-2.5" />
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
		</div>
	</button>
{/snippet}

{#snippet botGridCard(bot: AgentInfo)}
	{@const streaming = isStreaming(bot.name)}
	{@const approvals = approvalCount(bot.name)}
	{@const unreads = unreadCount(bot.name)}
	{@const timeLabel = previewTime(bot)}
	{@const busy = busyBot === bot.name}
	{@const accent = groupAccent(bot.agent_color)}
	{@const bloub = botBloubState(bot, hoveredBot === bot.name)}
	{@const secBadge = customSectionBadge('agent', bot.name)}
	{@const rtLabel = runtimeShortLabel(bot)}
	<div
		role="article"
		onmouseenter={() => (hoveredBot = bot.name)}
		onmouseleave={() => {
			if (hoveredBot === bot.name) hoveredBot = '';
		}}
		class={[
			'group relative flex flex-col justify-between overflow-hidden rounded-2xl border bg-card transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-md',
			!bot.running ? 'opacity-85 hover:opacity-100' : ''
		]}
	>
		<!-- Subtle color-matched top wash -->
		<div aria-hidden="true" class={['pointer-events-none h-1 w-full opacity-75', accent.dot]}></div>

		<!-- Main clickable area to enter chat -->
		<button
			type="button"
			onclick={() => openBotChat(bot)}
			class="flex flex-1 flex-col gap-3.5 p-4 text-left"
		>
			<div class="flex items-start justify-between gap-3">
				<div class="flex min-w-0 items-center gap-3">
					<span
						class={[
							'relative grid h-13 w-13 shrink-0 place-items-center rounded-2xl ring-1 transition-transform duration-200 group-hover:scale-105',
							accent.softBg,
							accent.ring
						]}
					>
						<Bloub
							mood={bloub.mood}
							cycle={bloub.cycle}
							size={44}
							tight
							color={colorOf(bot.agent_color)}
						/>
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

					<div class="min-w-0">
						<div class="flex items-center gap-1.5">
							<h3 class="truncate text-base font-bold tracking-tight text-foreground">
								{bot.name}
							</h3>
							{#if bot.default}
								<span
									class="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
								>
									{t('myaaw.agent.default')}
								</span>
							{/if}
						</div>
						<div class="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px]">
							{#if streaming}
								<span class="font-medium text-primary">
									{t('myaaw.bots.thinking')}
								</span>
							{:else if bot.running}
								<span class="text-muted-foreground">{rtLabel}</span>
							{:else}
								<span class="text-muted-foreground">{t('myaaw.bots.stopped')}</span>
							{/if}
							{#if secBadge}
								<span class="text-muted-foreground/50">·</span>
								<span class="truncate text-muted-foreground">{secBadge}</span>
							{/if}
						</div>
					</div>
				</div>

				<div class="flex shrink-0 items-center gap-1">
					{#if approvals > 0}
						<span
							title={t('myaaw.chat.need_approval')}
							class="inline-flex h-5 items-center gap-0.5 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
						>
							<ShieldAlert class="h-2.5 w-2.5" />
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
					<ArrowUpRight
						class="h-4 w-4 text-muted-foreground/40 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
					/>
				</div>
			</div>

			<!-- Accent-bordered activity quote -->
			<div class={['min-h-10 border-l-2 pl-3', accent.border]}>
				<p class="line-clamp-2 text-xs leading-relaxed text-foreground/85">
					{previewLine(bot)}
				</p>
			</div>
		</button>

		<!-- Subtle quiet footer toolbar -->
		<div
			class="flex items-center justify-between gap-2 border-t border-border/50 bg-muted/20 px-3.5 py-2"
		>
			<span class="truncate text-[10px] text-muted-foreground tabular-nums">
				{timeLabel || (bot.running ? t('myaaw.bots.ready') : t('myaaw.bots.stopped'))}
			</span>

			<div class="flex shrink-0 items-center gap-1">
				{#if bot.running}
					<button
						type="button"
						disabled={busy || Boolean(fleetBulk.busy)}
						onclick={() => void runBotServerAction(bot, 'restart')}
						title={t('myaaw.server.restart')}
						aria-label={t('myaaw.server.restart')}
						class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
					>
						{#if busy && busyBotAction === 'restart'}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<RotateCw class="h-3.5 w-3.5" />
						{/if}
					</button>
					<button
						type="button"
						disabled={busy || Boolean(fleetBulk.busy)}
						onclick={() => void runBotServerAction(bot, 'stop')}
						title={t('myaaw.server.stop')}
						aria-label={t('myaaw.server.stop')}
						class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/12 hover:text-destructive disabled:opacity-50"
					>
						{#if busy && busyBotAction === 'stop'}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Square class="h-3 w-3" />
						{/if}
					</button>
				{:else}
					<button
						type="button"
						disabled={busy || Boolean(fleetBulk.busy)}
						onclick={() => void runBotServerAction(bot, 'start')}
						class="inline-flex h-6.5 items-center gap-1 rounded-lg bg-emerald-500/12 px-2 text-[11px] font-medium text-emerald-700 transition-colors hover:bg-emerald-500/20 disabled:opacity-50 dark:text-emerald-300"
					>
						{#if busy && busyBotAction === 'start'}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else}
							<Play class="h-3 w-3" />
						{/if}
						{t('myaaw.server.start')}
					</button>
				{/if}
				<span aria-hidden="true" class="mx-0.5 h-3.5 w-px bg-border/70"></span>
				<button
					type="button"
					onclick={() => void startNewBotChat(bot)}
					title={t('myaaw.chat.new')}
					aria-label={t('myaaw.chat.new')}
					class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
				>
					<MessageSquarePlus class="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					onclick={() => createAgentDialog.openReconfigure(bot.name)}
					title={t('myaaw.bots.reconfigure')}
					aria-label={t('myaaw.bots.reconfigure')}
					class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
				>
					<SlidersHorizontal class="h-3.5 w-3.5" />
				</button>
			</div>
		</div>
	</div>
{/snippet}

{#snippet botListRow(bot: AgentInfo)}
	{@const streaming = isStreaming(bot.name)}
	{@const approvals = approvalCount(bot.name)}
	{@const unreads = unreadCount(bot.name)}
	{@const timeLabel = previewTime(bot)}
	{@const busy = busyBot === bot.name}
	{@const accent = groupAccent(bot.agent_color)}
	{@const bloub = botBloubState(bot, hoveredBot === bot.name)}
	{@const secBadge = customSectionBadge('agent', bot.name)}
	{@const rtLabel = runtimeShortLabel(bot)}
	<div
		role="article"
		onmouseenter={() => (hoveredBot = bot.name)}
		onmouseleave={() => {
			if (hoveredBot === bot.name) hoveredBot = '';
		}}
		class="group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/45"
	>
		<button
			type="button"
			onclick={() => openBotChat(bot)}
			class="flex min-w-0 flex-1 items-center gap-3 text-left"
		>
			<span
				class={[
					'relative grid h-10 w-10 shrink-0 place-items-center rounded-xl ring-1',
					accent.softBg,
					accent.ring
				]}
			>
				<Bloub
					mood={bloub.mood}
					cycle={bloub.cycle}
					size={32}
					tight
					color={colorOf(bot.agent_color)}
				/>
				<RuntimeBadge runtimeType={bot.runtime_type} runtimePreset={bot.runtime_preset} />
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

			<div class="min-w-0 flex-1">
				<div class="flex items-center gap-2">
					<span class="truncate text-sm font-semibold text-foreground">
						{bot.name}
					</span>
					<span class="hidden text-[11px] text-muted-foreground sm:inline">
						{rtLabel}
					</span>
					{#if secBadge}
						<span
							class="hidden rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground md:inline"
						>
							{secBadge}
						</span>
					{/if}
				</div>
				<p
					class={[
						'mt-0.5 truncate text-xs',
						streaming ? 'font-medium text-primary' : 'text-muted-foreground'
					]}
				>
					{previewLine(bot)}
				</p>
			</div>
		</button>

		<div class="flex shrink-0 items-center gap-1.5">
			{#if timeLabel}
				<span class="hidden text-[11px] text-muted-foreground tabular-nums sm:inline">
					{timeLabel}
				</span>
			{/if}
			{#if approvals > 0}
				<span
					class="inline-flex h-5 items-center gap-0.5 rounded-full bg-amber-500 px-1.5 text-[10px] font-semibold text-white tabular-nums"
				>
					<ShieldAlert class="h-2.5 w-2.5" />
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
			{#if bot.running}
				<button
					type="button"
					disabled={busy || Boolean(fleetBulk.busy)}
					onclick={() => void runBotServerAction(bot, 'restart')}
					title={t('myaaw.server.restart')}
					aria-label={t('myaaw.server.restart')}
					class="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
				>
					{#if busy && busyBotAction === 'restart'}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<RotateCw class="h-3.5 w-3.5" />
					{/if}
				</button>
				<button
					type="button"
					disabled={busy || Boolean(fleetBulk.busy)}
					onclick={() => void runBotServerAction(bot, 'stop')}
					title={t('myaaw.server.stop')}
					aria-label={t('myaaw.server.stop')}
					class="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/12 hover:text-destructive disabled:opacity-50"
				>
					{#if busy && busyBotAction === 'stop'}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<Square class="h-3 w-3" />
					{/if}
				</button>
			{:else}
				<button
					type="button"
					disabled={busy || Boolean(fleetBulk.busy)}
					onclick={() => void runBotServerAction(bot, 'start')}
					class="inline-flex h-7 items-center gap-1 rounded-lg bg-emerald-500/12 px-2 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-500/20 disabled:opacity-50 dark:text-emerald-300"
				>
					{#if busy && busyBotAction === 'start'}
						<Loader2 class="h-3 w-3 animate-spin" />
					{:else}
						<Play class="h-3 w-3" />
					{/if}
					{t('myaaw.server.start')}
				</button>
			{/if}
			<button
				type="button"
				onclick={() => void startNewBotChat(bot)}
				title={t('myaaw.chat.new')}
				aria-label={t('myaaw.chat.new')}
				class="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
			>
				<MessageSquarePlus class="h-3.5 w-3.5" />
			</button>
			<button
				type="button"
				onclick={() => createAgentDialog.openReconfigure(bot.name)}
				title={t('myaaw.bots.reconfigure')}
				aria-label={t('myaaw.bots.reconfigure')}
				class="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
			>
				<SlidersHorizontal class="h-3.5 w-3.5" />
			</button>
		</div>
	</div>
{/snippet}
