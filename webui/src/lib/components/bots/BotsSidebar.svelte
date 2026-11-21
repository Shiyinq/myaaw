<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Bot,
		Check,
		Clock,
		Layers,
		ListFilter,
		Loader2,
		MoreHorizontal,
		Plus,
		Puzzle,
		Search,
		Settings2,
		ShieldAlert,
		Trash2,
		Users,
		X
	} from 'lucide-svelte';
	import AgentMoreMenuItems from '$lib/components/app/AgentMoreMenuItems.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, toastStore } from '$lib/stores';
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
	import { isAuthenticated } from '$lib/stores/authStatus.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { isOverlayOpen, isTypingTarget, matchesShortcut } from '$lib/utils/keys';
	import { isActiveHref } from '$lib/utils/nav';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { AgentInfo, GroupSummary } from '$lib/types';

	type BotFilter = 'all' | 'running' | 'unread';

	const { t, locale } = useTranslation();

	interface Props {
		/** Icon-only rail: avatars stay visible, labels and search fold away. */
		compact?: boolean;
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, onClose }: Props = $props();

	let query = $state('');
	let filter = $state<BotFilter>('all');
	let busyMap = $state<Record<string, boolean>>({});
	let openMenuBot = $state('');
	let openMenuGroup = $state('');
	const fetchedAgents = new Set<string>();

	// Load groups list whenever authenticated.
	$effect(() => {
		if (!isAuthenticated.value) return;
		untrack(() => void groupsStore.load());
	});

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

	function needsAttention(name: string): boolean {
		return approvalCount(name) > 0 || unreadCount(name) > 0;
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
		// Pending approvals first, then live streams, then newest chat activity.
		// Never sort by daemon running/stopped or empty New Chat state so a bot stays
		// in place until a conversation actually starts or receives new messages.
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

	const runningTotal = $derived(agentSelection.agents.filter((bot) => bot.running).length);
	const attentionTotal = $derived(
		agentSelection.agents.reduce(
			(sum, bot) => sum + unreadCount(bot.name) + approvalCount(bot.name),
			0
		)
	);
	const attentionBotCount = $derived(
		agentSelection.agents.filter((bot) => needsAttention(bot.name)).length
	);

	const visibleAgents = $derived.by(() => {
		const base = agentSelection.agents.filter((bot) => {
			if (searchTerm && !bot.name.toLowerCase().includes(searchTerm)) return false;
			if (filter === 'running' && !bot.running) return false;
			if (filter === 'unread' && !needsAttention(bot.name)) return false;
			return true;
		});
		return [...base].sort(compareBots);
	});

	const attentionBots = $derived(visibleAgents.filter((bot) => needsAttention(bot.name)));
	const regularBots = $derived(visibleAgents.filter((bot) => !needsAttention(bot.name)));
	const splitSections = $derived(
		filter === 'all' && !searchTerm && attentionBots.length > 0 && regularBots.length > 0
	);
	// Canonical display order used by both the compact rail and keyboard shortcuts.
	const orderedBots = $derived(splitSections ? [...attentionBots, ...regularBots] : visibleAgents);

	const visibleGroups = $derived.by(() => {
		return groupsStore.groups.filter((grp) => {
			if (searchTerm) {
				const nameHit = grp.name.toLowerCase().includes(searchTerm);
				const memberHit = grp.members.some((m) => m.name.toLowerCase().includes(searchTerm));
				if (!nameHit && !memberHit) return false;
			}
			if (filter === 'running') {
				return grp.running || grp.members.some((m) => m.running);
			}
			if (filter === 'unread') {
				return grp.running;
			}
			return true;
		});
	});

	const onChatRoute = $derived(
		page.url.pathname === '/app/chat' || page.url.pathname.startsWith('/app/chat/')
	);

	function isGroupActive(id: string): boolean {
		const path = page.url.pathname;
		return path === `/app/groups/${id}` || path === `/app/groups/${encodeURIComponent(id)}`;
	}

	function selectGroup(grp: GroupSummary) {
		void goto(`/app/groups/${encodeURIComponent(grp.id)}`);
		onClose();
	}

	function openGroupSettings(grp: GroupSummary) {
		openMenuGroup = '';
		void goto(`/app/groups/${encodeURIComponent(grp.id)}?inspector=settings`);
		onClose();
	}

	async function deleteGroupFromSidebar(grp: GroupSummary) {
		openMenuGroup = '';
		const confirmed = await askConfirm({
			title: t('myaaw.groups.delete_title'),
			message: t('myaaw.groups.delete_confirm', { name: grp.name }),
			confirmText: t('myaaw.groups.delete_action'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		const wasActive = isGroupActive(grp.id);
		try {
			await groupsStore.remove(grp.id);
			toastStore.success(t('myaaw.groups.deleted', { name: grp.name }));
			if (wasActive) {
				await goto('/app/chat');
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function groupPreviewLine(grp: GroupSummary): string {
		if (grp.running && grp.current_agent) {
			return t('myaaw.groups.agent_thinking', { name: grp.current_agent });
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
		const formatted = formatChatTimestamp(raw, locale.value, t('myaaw.chat.yesterday'));
		const parts = formatted.split(' · ');
		return parts[0] ?? formatted;
	}

	function openCreateGroup() {
		groupsStore.openCreateDialog();
		onClose();
	}

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

	function selectBot(bot: AgentInfo) {
		const switched = agentSelection.selected !== bot.name;
		const targetId =
			readLastConversation(bot.name) || botPreviews.get(bot.name)?.conversationId || '';
		const targetHref = targetId ? `/app/chat/${encodeURIComponent(targetId)}` : '/app/chat';
		agentSelection.select(bot.name);
		if (!onChatRoute || switched || page.url.pathname !== targetHref) {
			void goto(targetHref);
		}
		onClose();
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

	async function newChatForBot(bot: AgentInfo) {
		openMenuBot = '';
		clearLastConversation(bot.name);
		agentSelection.select(bot.name);
		onClose();
		await tick();
		await goto('/app/chat?new=1');
	}

	function createBot() {
		void goto('/app/new');
		onClose();
	}

	// Whenever a stopped bot transitions to running, fetch its latest conversation
	// preview once its daemon port is ready.
	$effect(() => {
		const runningNames = agentSelection.agents.filter((bot) => bot.running).map((bot) => bot.name);
		untrack(() => {
			for (const name of runningNames) {
				if (botPreviews.get(name)?.conversationId) continue;
				setTimeout(() => void refreshLatest(name, false), 1200);
			}
		});
	});

	// Quick keyboard switching between bots (Alt+Shift+Up/Down and Alt+1..9).
	// Scoped strictly to Bots mode because this component only mounts there.
	$effect(() => {
		if (typeof window === 'undefined') return;
		const onKey = (event: KeyboardEvent) => {
			if (event.defaultPrevented || isOverlayOpen()) return;
			if (isTypingTarget(event.target)) return;
			const list = orderedBots;
			if (list.length === 0) return;

			if (matchesShortcut(event, 'alt+shift+arrowup')) {
				event.preventDefault();
				const idx = list.findIndex((bot) => bot.name === agentSelection.selected);
				const prev = idx <= 0 ? list[list.length - 1] : list[idx - 1];
				if (prev) selectBot(prev);
				return;
			}
			if (matchesShortcut(event, 'alt+shift+arrowdown')) {
				event.preventDefault();
				const idx = list.findIndex((bot) => bot.name === agentSelection.selected);
				const next = idx < 0 || idx >= list.length - 1 ? list[0] : list[idx + 1];
				if (next) selectBot(next);
				return;
			}
			if (
				!event.repeat &&
				event.altKey &&
				!event.ctrlKey &&
				!event.metaKey &&
				!event.shiftKey &&
				/^Digit[1-9]$/.test(event.code)
			) {
				const index = Number(event.code.slice(5)) - 1;
				const target = list[index];
				if (!target) return;
				event.preventDefault();
				selectBot(target);
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

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

{#snippet botRow(bot: AgentInfo)}
	{@const active = onChatRoute && agentSelection.selected === bot.name}
	{@const approvals = approvalCount(bot.name)}
	{@const unreads = unreadCount(bot.name)}
	{@const streaming = isStreaming(bot.name)}
	{@const timeLabel = previewTime(bot)}
	{@const busy = Boolean(busyMap[bot.name])}
	{@const menuOpen = openMenuBot === bot.name}
	<div class="group relative">
		<button
			type="button"
			onclick={() => selectBot(bot)}
			aria-current={active ? 'page' : undefined}
			class={[
				'flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors list-row',
				active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/70'
			]}
		>
			<span
				class="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-muted/60 ring-1 ring-border/40"
			>
				<Bloub mood="attentif" size={30} tight color={colorOf(bot.agent_color)} />
				{#if busy}
					<span
						aria-hidden="true"
						class="absolute -right-1 -bottom-1 grid h-3.5 w-3.5 place-items-center rounded-full bg-card ring-1 ring-border"
					>
						<Loader2 class="h-2.5 w-2.5 animate-spin text-muted-foreground" />
					</span>
				{:else}
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
				{/if}
			</span>

			<span class="min-w-0 flex-1">
				<span class="flex items-center justify-between gap-1.5">
					<span class="truncate text-sm font-semibold text-foreground">{bot.name}</span>
					{#if timeLabel}
						<span
							class={[
								'shrink-0 text-[10px] text-muted-foreground tabular-nums transition-opacity',
								menuOpen ? 'opacity-0' : 'group-focus-within:opacity-0 group-hover:opacity-0'
							]}
						>
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
					{#if approvals > 0 || unreads > 0}
						<span class="flex shrink-0 items-center gap-1">
							{#if approvals > 0}
								<span
									title={t('myaaw.chat.need_approval')}
									class="inline-flex h-4 min-w-4 items-center justify-center gap-0.5 rounded-full bg-amber-500 px-1 text-[10px] leading-none font-semibold text-white tabular-nums"
								>
									<ShieldAlert class="h-2.5 w-2.5 shrink-0" />
									{approvals > 9 ? '9+' : approvals}
								</span>
							{/if}
							{#if unreads > 0}
								<span
									aria-label={t('myaaw.chat.unread')}
									class="grid h-4 min-w-4 place-items-center rounded-full bg-primary px-1 text-[10px] leading-none font-semibold text-primary-foreground tabular-nums"
								>
									{unreads > 99 ? '99+' : unreads}
								</span>
							{/if}
						</span>
					{/if}
				</span>
			</span>
		</button>

		<div
			class={[
				'absolute top-1.5 right-1.5 transition-opacity',
				menuOpen
					? 'opacity-100'
					: 'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100'
			]}
		>
			<DropdownMenu.Root
				open={menuOpen}
				onOpenChange={(open) => {
					openMenuBot = open ? bot.name : openMenuBot === bot.name ? '' : openMenuBot;
				}}
			>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							type="button"
							onclick={(event) => {
								event.stopPropagation();
								(props.onclick as ((e: MouseEvent) => void) | undefined)?.(event);
							}}
							aria-label={t('myaaw.bots.actions')}
							title={t('myaaw.bots.actions')}
							class="grid h-6 w-6 place-items-center rounded-md bg-card/90 text-muted-foreground shadow-xs ring-1 ring-border/60 transition-colors hover:bg-muted hover:text-foreground"
						>
							<MoreHorizontal class="h-3.5 w-3.5" />
						</button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-44">
					<AgentMoreMenuItems
						agent={bot.name}
						running={bot.running}
						{busy}
						onBusyChange={(next) => {
							busyMap = { ...busyMap, [bot.name]: next };
						}}
						includeChatActions
						includeServerControls
						passAgentParam
						onNewChat={() => void newChatForBot(bot)}
						onStarted={(startedName) => {
							setTimeout(() => void refreshLatest(startedName, false), 1500);
						}}
						onNavigate={() => {
							openMenuBot = '';
							onClose();
						}}
					/>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
{/snippet}

{#snippet groupRow(grp: GroupSummary)}
	{@const active = isGroupActive(grp.id)}
	{@const timeLabel = groupPreviewTime(grp)}
	{@const menuOpen = openMenuGroup === grp.id}
	<div class="group relative">
		<button
			type="button"
			onclick={() => selectGroup(grp)}
			aria-current={active ? 'page' : undefined}
			class={[
				'flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors list-row',
				active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/70'
			]}
		>
			<span
				class="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-muted/60 ring-1 ring-border/40"
			>
				{#if grp.members.length > 0}
					<span class="grid grid-cols-2 gap-0.5 p-1.5">
						{#each grp.members.slice(0, 4) as member (member.name)}
							<span
								aria-hidden="true"
								class={['h-2.5 w-2.5 rounded-full', groupAccent(member.agent_color).dot]}
							></span>
						{/each}
					</span>
				{:else}
					<Users class="h-4 w-4 text-muted-foreground" />
				{/if}
				{#if grp.running}
					<span
						aria-hidden="true"
						class="absolute -right-0.5 -bottom-0.5 h-2.5 w-2.5 animate-pulse rounded-full bg-primary ring-2 ring-card"
					></span>
				{/if}
			</span>

			<span class="min-w-0 flex-1">
				<span class="flex items-center justify-between gap-1.5">
					<span class="truncate text-sm font-semibold text-foreground">{grp.name}</span>
					{#if timeLabel}
						<span
							class={[
								'shrink-0 text-[10px] text-muted-foreground tabular-nums transition-opacity',
								menuOpen ? 'opacity-0' : 'group-focus-within:opacity-0 group-hover:opacity-0'
							]}
						>
							{timeLabel}
						</span>
					{/if}
				</span>
				<span class="mt-0.5 flex items-center justify-between gap-1.5">
					<span
						class={[
							'truncate text-xs',
							grp.running ? 'font-medium text-primary' : 'text-muted-foreground'
						]}
					>
						{groupPreviewLine(grp)}
					</span>
				</span>
			</span>
		</button>

		<div
			class={[
				'absolute top-1.5 right-1.5 transition-opacity',
				menuOpen
					? 'opacity-100'
					: 'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100'
			]}
		>
			<DropdownMenu.Root
				open={menuOpen}
				onOpenChange={(open) => {
					openMenuGroup = open ? grp.id : openMenuGroup === grp.id ? '' : openMenuGroup;
				}}
			>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							type="button"
							onclick={(event) => {
								event.stopPropagation();
								(props.onclick as ((e: MouseEvent) => void) | undefined)?.(event);
							}}
							aria-label={t('myaaw.groups.actions')}
							title={t('myaaw.groups.actions')}
							class="grid h-6 w-6 place-items-center rounded-md bg-card/90 text-muted-foreground shadow-xs ring-1 ring-border/60 transition-colors hover:bg-muted hover:text-foreground"
						>
							<MoreHorizontal class="h-3.5 w-3.5" />
						</button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-44">
					<DropdownMenu.Item onclick={() => openGroupSettings(grp)} class="gap-2">
						<Settings2 class="h-3.5 w-3.5" />
						{t('myaaw.groups.open_settings')}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => void deleteGroupFromSidebar(grp)}
						class="gap-2 text-destructive focus:text-destructive"
					>
						<Trash2 class="h-3.5 w-3.5" />
						{t('myaaw.groups.delete_action')}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
{/snippet}

<!--
	Bots mode turns the left sidebar into a direct bot roster (inspired by Grok
	Bots / OpenBot): search, filter, and add a bot at the top, pick any agent to
	jump into its chat, manage daemons via quick actions, and keep utility links
	above the account menu.
-->
<SidebarBrand {compact} {onClose} />

{#if contentCompact}
	<div class="flex shrink-0 flex-col items-center gap-1 px-2 py-1.5">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						title={t('myaaw.bots.new')}
						aria-label={t('myaaw.bots.new')}
						class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					>
						<Plus class="h-4 w-4" />
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content side="right" align="start" class="w-44">
				<DropdownMenu.Item onclick={openCreateGroup} class="gap-2">
					<Users class="h-3.5 w-3.5" />
					{t('myaaw.groups.create_group')}
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={createBot} class="gap-2">
					<Bot class="h-3.5 w-3.5" />
					{t('myaaw.groups.new_agent')}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
	<div class="mx-auto my-1 h-px w-6 bg-border"></div>
	<nav class="flex-1 space-y-1.5 overflow-y-auto px-2 py-1" aria-label={t('myaaw.bots.list_label')}>
		{#if groupsStore.groups.length > 0}
			{#each groupsStore.groups as grp (grp.id)}
				{@const active = isGroupActive(grp.id)}
				<button
					type="button"
					onclick={() => selectGroup(grp)}
					title={grp.name}
					aria-label={grp.name}
					aria-current={active ? 'page' : undefined}
					class={[
						'relative flex h-10 w-10 items-center justify-center rounded-xl transition-colors',
						active
							? 'bg-accent text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{#if active}
						<span class="absolute inset-y-1.5 left-0 w-0.5 rounded-full bg-primary"></span>
					{/if}
					<Users class="h-4 w-4" />
					{#if grp.running}
						<span
							aria-hidden="true"
							class="absolute right-1 bottom-1 h-2 w-2 animate-pulse rounded-full bg-primary ring-2 ring-card"
						></span>
					{/if}
				</button>
			{/each}
			<div class="mx-auto my-1.5 h-px w-6 bg-border"></div>
		{/if}
		{#each orderedBots as bot (bot.name)}
			{@const active = onChatRoute && agentSelection.selected === bot.name}
			{@const approvals = approvalCount(bot.name)}
			{@const unreads = unreadCount(bot.name)}
			{@const total = approvals + unreads}
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
				{#if approvals > 0}
					<span
						class="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-amber-500 px-1 text-[9px] leading-4 font-semibold text-white tabular-nums"
					>
						{total > 9 ? '9+' : total}
					</span>
				{:else if unreads > 0}
					<span
						class="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[9px] leading-4 font-semibold text-primary-foreground tabular-nums"
					>
						{unreads > 9 ? '9+' : unreads}
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
	<div class="flex shrink-0 items-center gap-1.5 px-3 py-2">
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

		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						title={t('myaaw.bots.filter_label')}
						aria-label={t('myaaw.bots.filter_label')}
						aria-pressed={filter !== 'all'}
						class={[
							'relative flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border transition-colors focus-visible:ring-2 focus-visible:ring-ring/60',
							filter !== 'all'
								? 'border-primary/40 bg-accent text-foreground'
								: 'border-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
						]}
					>
						<ListFilter class="h-4 w-4" />
						{#if filter !== 'all'}
							<span
								aria-hidden="true"
								class="absolute top-1.5 right-1.5 h-1.5 w-1.5 rounded-full bg-primary"
							></span>
						{/if}
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="end" class="w-44">
				<DropdownMenu.Label class="text-[11px] font-medium text-muted-foreground">
					{t('myaaw.bots.filter_label')}
				</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={() => (filter = 'all')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'all' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_all')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">
						{agentSelection.agents.length}
					</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'running')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'running' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_running')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">{runningTotal}</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'unread')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'unread' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_unread')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">{attentionBotCount}</span>
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>

		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						title={t('myaaw.bots.new')}
						aria-label={t('myaaw.bots.new')}
						class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-dashed text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
					>
						<Plus class="h-4 w-4" />
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="end" class="w-44">
				<DropdownMenu.Item onclick={openCreateGroup} class="gap-2">
					<Users class="h-3.5 w-3.5" />
					{t('myaaw.groups.create_group')}
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={createBot} class="gap-2">
					<Bot class="h-3.5 w-3.5" />
					{t('myaaw.groups.new_agent')}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>

	<nav
		class="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 py-1"
		aria-label={t('myaaw.bots.list_label')}
	>
		{#if visibleGroups.length > 0}
			<div class="flex items-center justify-between px-2 pt-1 pb-1">
				<span class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
					{t('myaaw.groups.section_groups')}
				</span>
				<span class="text-[10px] font-medium text-muted-foreground tabular-nums">
					{visibleGroups.length}
				</span>
			</div>
			{#each visibleGroups as grp (grp.id)}
				{@render groupRow(grp)}
			{/each}
		{/if}

		{#if agentSelection.loading && agentSelection.agents.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
		{:else if visibleAgents.length === 0 && visibleGroups.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">
				{searchTerm
					? t('myaaw.bots.no_matches')
					: filter !== 'all'
						? t('myaaw.bots.empty_filter')
						: t('myaaw.agent.none')}
			</p>
		{:else if splitSections}
			<div
				class={[
					'flex items-center justify-between px-2 pb-1',
					visibleGroups.length > 0 ? 'pt-3' : 'pt-1'
				]}
			>
				<span class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
					{t('myaaw.bots.section_unread')}
				</span>
				<span class="text-[10px] font-medium text-muted-foreground tabular-nums">
					{attentionTotal}
				</span>
			</div>
			{#each attentionBots as bot (bot.name)}
				{@render botRow(bot)}
			{/each}

			<div class="flex items-center justify-between px-2 pt-3 pb-1">
				<span class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
					{t('myaaw.bots.section_all')}
				</span>
				<span class="text-[10px] font-medium text-muted-foreground tabular-nums">
					{regularBots.length}
				</span>
			</div>
			{#each regularBots as bot (bot.name)}
				{@render botRow(bot)}
			{/each}
		{:else if visibleAgents.length > 0}
			{#if visibleGroups.length > 0}
				<div class="flex items-center justify-between px-2 pt-3 pb-1">
					<span class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
						{t('myaaw.groups.section_bots')}
					</span>
					<span class="text-[10px] font-medium text-muted-foreground tabular-nums">
						{visibleAgents.length}
					</span>
				</div>
			{/if}
			{#each visibleAgents as bot (bot.name)}
				{@render botRow(bot)}
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
