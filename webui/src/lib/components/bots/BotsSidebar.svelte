<script lang="ts">
	import BotsSidebarSkeleton from './BotsSidebarSkeleton.svelte';
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		ArrowDown,
		ArrowUp,
		Bot,
		Check,
		ChevronDown,
		FolderInput,
		FolderPlus,
		GripVertical,
		ListFilter,
		Loader2,
		MoreHorizontal,
		Pencil,
		Play,
		Plus,
		RotateCw,
		Search,
		Settings2,
		ShieldAlert,
		Square,
		Trash2,
		Users,
		X
	} from 'lucide-svelte';
	import AgentMoreMenuItems from '$lib/components/app/AgentMoreMenuItems.svelte';
	import BotsSectionDialog from '$lib/components/bots/BotsSectionDialog.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, toastStore } from '$lib/stores';
	import { createAgentDialog } from '$lib/stores/createAgentDialog.svelte';
	import { fleetBulk } from '$lib/stores/fleetBulk.svelte';
	import {
		BOTS_SECTION_AGENTS,
		BOTS_SECTION_GROUPS,
		BOTS_SECTION_UNREAD,
		botsSidebarStore
	} from '$lib/stores/botsSidebar.svelte';
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
	import { formatChatTimestamp } from '$lib/utils/time';
	import type {
		AgentInfo,
		BotsSidebarItemKind,
		BotsSidebarItemRef,
		GroupSummary
	} from '$lib/types';

	type BotFilter = 'all' | 'groups' | 'bots' | 'running' | 'stopped' | 'unread';
	type RosterItem = { kind: 'group'; group: GroupSummary } | { kind: 'bot'; bot: AgentInfo };

	interface SidebarSectionView {
		id: string;
		kind: 'groups' | 'agents' | 'custom';
		label: string;
		items: RosterItem[];
	}

	type DragState =
		| { mode: 'item'; kind: BotsSidebarItemKind; id: string; sourceSectionId: string }
		| { mode: 'section'; sectionId: string }
		| null;

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
	let openMenuSection = $state('');

	let sectionDialogOpen = $state(false);
	let editingSectionId = $state<string | null>(null);

	let dragState = $state<DragState>(null);
	let dropItemTarget = $state<string | null>(null);
	let dropSectionTarget = $state<{ sectionId: string; position: 'before' | 'after' } | null>(null);
	let slotWidths = $state<Record<string, number>>({});

	function trackSlotWidth(node: HTMLElement, sectionId: string) {
		const update = () => {
			const w = Math.floor(node.clientWidth);
			if (slotWidths[sectionId] !== w) {
				slotWidths = { ...slotWidths, [sectionId]: w };
			}
		};
		update();
		const observer = new ResizeObserver(update);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}

	/** Computes how many 20px overlapping (-space-x-1.5 -> 14px step) avatars fit inside the inline header slot. */
	function inlineAvatarSlice(
		sectionId: string,
		total: number
	): { count: number; overflow: number } {
		if (total <= 0) return { count: 0, overflow: 0 };
		const w = slotWidths[sectionId] ?? 80;
		if (20 + (total - 1) * 14 + 12 <= w) {
			return { count: total, overflow: 0 };
		}
		const count = Math.max(0, Math.min(total - 1, Math.floor((w - 32) / 14)));
		return { count, overflow: total - count };
	}

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

	function itemApprovals(item: RosterItem): number {
		return item.kind === 'group' ? groupApprovals(item.group) : approvalCount(item.bot.name);
	}

	function itemUnreads(item: RosterItem): number {
		return item.kind === 'group' ? groupUnreads(item.group) : unreadCount(item.bot.name);
	}

	function itemRunning(item: RosterItem): boolean {
		return item.kind === 'group' ? Boolean(item.group.running) : isStreaming(item.bot.name);
	}

	function itemActivityTimestamp(item: RosterItem): number {
		return item.kind === 'group'
			? groupActivityTimestamp(item.group)
			: activityTimestamp(item.bot.name);
	}

	function itemLabel(item: RosterItem): string {
		return item.kind === 'group' ? item.group.name : item.bot.name;
	}

	function itemKey(kind: BotsSidebarItemKind, id: string): string {
		return `${kind}\0${id}`;
	}

	function compareRosterItems(a: RosterItem, b: RosterItem): number {
		const approvalDiff = Number(itemApprovals(b) > 0) - Number(itemApprovals(a) > 0);
		if (approvalDiff !== 0) return approvalDiff;
		const runningDiff = Number(itemRunning(b)) - Number(itemRunning(a));
		if (runningDiff !== 0) return runningDiff;
		const timeDiff = itemActivityTimestamp(b) - itemActivityTimestamp(a);
		if (timeDiff !== 0) return timeDiff;
		const defaultA = a.kind === 'bot' && Boolean(a.bot.default);
		const defaultB = b.kind === 'bot' && Boolean(b.bot.default);
		const defaultDiff = Number(defaultB) - Number(defaultA);
		if (defaultDiff !== 0) return defaultDiff;
		return itemLabel(a).localeCompare(itemLabel(b));
	}

	const searchTerm = $derived(query.trim().toLowerCase());
	const isFiltering = $derived(filter !== 'all' || searchTerm.length > 0);

	const runningTotal = $derived(agentSelection.agents.filter((bot) => bot.running).length);
	const stoppedTotal = $derived(agentSelection.agents.filter((bot) => !bot.running).length);
	const attentionFilterCount = $derived(
		agentSelection.agents.filter((bot) => needsAttention(bot.name)).length +
			groupsStore.groups.filter((grp) => groupNeedsAttention(grp)).length
	);

	const includesGroups = $derived(filter === 'all' || filter === 'groups' || filter === 'unread');
	const listError = $derived(
		(filter !== 'groups' && agentSelection.error) || (includesGroups && groupsStore.error)
	);
	const listLoading = $derived(
		(filter !== 'groups' && (!agentSelection.loaded || agentSelection.loading)) ||
			(includesGroups && groupsStore.loading)
	);

	const visibleAgents = $derived.by(() => {
		if (filter === 'groups') return [];
		return agentSelection.agents.filter((bot) => {
			if (searchTerm && !bot.name.toLowerCase().includes(searchTerm)) return false;
			if (filter === 'running' && !bot.running) return false;
			if (filter === 'stopped' && bot.running) return false;
			if (filter === 'unread' && !needsAttention(bot.name)) return false;
			return true;
		});
	});

	const visibleGroups = $derived.by(() => {
		if (!includesGroups) return [];
		return groupsStore.groups.filter((grp) => {
			if (filter === 'unread' && !groupNeedsAttention(grp)) return false;
			if (searchTerm) {
				const nameHit = grp.name.toLowerCase().includes(searchTerm);
				const memberHit = grp.members.some((m) => m.name.toLowerCase().includes(searchTerm));
				if (!nameHit && !memberHit) return false;
			}
			return true;
		});
	});

	// Unified Unread section (pinned at top when filter === 'all' and no search query).
	const attentionItems = $derived.by((): RosterItem[] => {
		if (isFiltering) return [];
		const list: RosterItem[] = [];
		for (const grp of visibleGroups) {
			if (groupNeedsAttention(grp)) list.push({ kind: 'group', group: grp });
		}
		for (const bot of visibleAgents) {
			if (needsAttention(bot.name)) list.push({ kind: 'bot', bot });
		}
		return list.sort(compareRosterItems);
	});

	const attentionKeys = $derived.by(() => {
		const keys = new Set<string>();
		for (const item of attentionItems) {
			keys.add(
				item.kind === 'group' ? itemKey('group', item.group.id) : itemKey('agent', item.bot.name)
			);
		}
		return keys;
	});

	const unreadBadgeTotal = $derived(
		attentionItems.reduce((sum, item) => sum + itemApprovals(item) + itemUnreads(item), 0)
	);
	const unreadHasApprovals = $derived(attentionItems.some((item) => itemApprovals(item) > 0));

	// Partition remaining (non-unread) items into custom sections and built-in Groups / Agents.
	const sectionsView = $derived.by((): SidebarSectionView[] => {
		const groupById = new Map<string, GroupSummary>();
		for (const grp of visibleGroups) {
			if (!attentionKeys.has(itemKey('group', grp.id))) {
				groupById.set(grp.id, grp);
			}
		}
		const botByName = new Map<string, AgentInfo>();
		for (const bot of visibleAgents) {
			if (!attentionKeys.has(itemKey('agent', bot.name))) {
				botByName.set(bot.name, bot);
			}
		}

		const assignedGroups = new Set<string>();
		const assignedBots = new Set<string>();
		const customMap = new Map<string, SidebarSectionView>();

		for (const sec of botsSidebarStore.sections) {
			const items: RosterItem[] = [];
			for (const ref of sec.items) {
				if (ref.kind === 'group') {
					assignedGroups.add(ref.id);
					const grp = groupById.get(ref.id);
					if (grp) items.push({ kind: 'group', group: grp });
				} else {
					assignedBots.add(ref.id);
					const bot = botByName.get(ref.id);
					if (bot) items.push({ kind: 'bot', bot });
				}
			}
			items.sort(compareRosterItems);
			customMap.set(sec.id, {
				id: sec.id,
				kind: 'custom',
				label: sec.name,
				items
			});
		}

		const unassignedGroups: RosterItem[] = [];
		for (const grp of groupById.values()) {
			if (!assignedGroups.has(grp.id)) {
				unassignedGroups.push({ kind: 'group', group: grp });
			}
		}
		unassignedGroups.sort(compareRosterItems);

		const unassignedBots: RosterItem[] = [];
		for (const bot of botByName.values()) {
			if (!assignedBots.has(bot.name)) {
				unassignedBots.push({ kind: 'bot', bot });
			}
		}
		unassignedBots.sort(compareRosterItems);

		const ordered: SidebarSectionView[] = [];
		for (const id of botsSidebarStore.sectionOrder) {
			if (id === BOTS_SECTION_GROUPS) {
				ordered.push({
					id: BOTS_SECTION_GROUPS,
					kind: 'groups',
					label: t('myaaw.groups.section_groups'),
					items: unassignedGroups
				});
			} else if (id === BOTS_SECTION_AGENTS) {
				ordered.push({
					id: BOTS_SECTION_AGENTS,
					kind: 'agents',
					label: t('myaaw.bots.section_all'),
					items: unassignedBots
				});
			} else {
				const custom = customMap.get(id);
				if (custom) ordered.push(custom);
			}
		}
		return ordered;
	});

	function shouldShowSection(sec: SidebarSectionView): boolean {
		if (isFiltering) {
			return sec.items.length > 0;
		}
		if (sec.kind === 'custom') {
			return true;
		}
		if (sec.items.length > 0) {
			return true;
		}
		// Reveal empty built-in Groups/Agents as a drop target while dragging a compatible item.
		if (dragState?.mode === 'item') {
			return botsSidebarStore.canAcceptItem(sec.id, dragState.kind);
		}
		return false;
	}

	const visibleSections = $derived(sectionsView.filter(shouldShowSection));

	function isSectionCollapsed(sectionId: string): boolean {
		if (isFiltering) return false;
		return botsSidebarStore.isCollapsed(sectionId);
	}

	function sectionLabelById(id: string): string {
		if (id === BOTS_SECTION_GROUPS) return t('myaaw.groups.section_groups');
		if (id === BOTS_SECTION_AGENTS) return t('myaaw.bots.section_all');
		return botsSidebarStore.getSection(id)?.name ?? id;
	}

	function canMoveSection(id: string, delta: -1 | 1): boolean {
		const order = botsSidebarStore.sectionOrder;
		const idx = order.indexOf(id);
		if (idx < 0) return false;
		const target = idx + delta;
		return target >= 0 && target < order.length;
	}

	// Sections rendered in the icon-only compact rail.
	const compactBlocks = $derived.by(() => {
		const blocks: { id: string; items: RosterItem[] }[] = [];
		if (attentionItems.length > 0) {
			blocks.push({ id: BOTS_SECTION_UNREAD, items: attentionItems });
		}
		for (const sec of sectionsView) {
			if (sec.items.length > 0) {
				blocks.push({ id: sec.id, items: sec.items });
			}
		}
		return blocks;
	});

	// Canonical top-to-bottom visible sidebar order used by keyboard shortcuts.
	const orderedRoster = $derived.by((): RosterItem[] => {
		if (contentCompact) {
			return compactBlocks.flatMap((block) => block.items);
		}
		const out: RosterItem[] = [];
		if (attentionItems.length > 0 && !isSectionCollapsed(BOTS_SECTION_UNREAD)) {
			out.push(...attentionItems);
		}
		for (const sec of visibleSections) {
			if (!isSectionCollapsed(sec.id)) {
				out.push(...sec.items);
			}
		}
		return out;
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
		const formatted = formatChatTimestamp(raw, locale.value, t('myaaw.chat.yesterday'));
		const parts = formatted.split(' · ');
		return parts[0] ?? formatted;
	}

	function openCreateGroup() {
		groupsStore.openCreateDialog();
		onClose();
	}

	function openCreateSection() {
		editingSectionId = null;
		sectionDialogOpen = true;
	}

	function openEditSection(id: string) {
		openMenuSection = '';
		editingSectionId = id;
		sectionDialogOpen = true;
	}

	async function deleteCustomSection(sec: SidebarSectionView) {
		openMenuSection = '';
		const confirmed = await askConfirm({
			title: t('myaaw.bots.section_delete_title'),
			message: t('myaaw.bots.section_delete_confirm', { name: sec.label }),
			confirmText: t('myaaw.bots.section_delete'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		if (botsSidebarStore.deleteSection(sec.id)) {
			toastStore.success(t('myaaw.bots.section_deleted', { name: sec.label }));
		}
	}

	function handleMoveItem(ref: BotsSidebarItemRef, targetSectionId: string, showToast = false) {
		openMenuBot = '';
		openMenuGroup = '';
		if (!botsSidebarStore.moveItem(ref, targetSectionId)) return;
		botsSidebarStore.setCollapsed(targetSectionId, false);
		if (showToast) {
			toastStore.success(
				t('myaaw.bots.moved_to_section', { name: sectionLabelById(targetSectionId) })
			);
		}
	}

	// Drag-and-drop handlers
	function resetDrag() {
		dragState = null;
		dropItemTarget = null;
		dropSectionTarget = null;
	}

	function handleItemDragStart(
		event: DragEvent,
		kind: BotsSidebarItemKind,
		id: string,
		sourceSectionId: string
	) {
		if (isFiltering || openMenuBot || openMenuGroup || openMenuSection) {
			event.preventDefault();
			return;
		}
		dragState = { mode: 'item', kind, id, sourceSectionId };
		dropItemTarget = null;
		dropSectionTarget = null;
		if (event.dataTransfer) {
			event.dataTransfer.effectAllowed = 'move';
			event.dataTransfer.setData('application/x-myaaw-sidebar-item', JSON.stringify({ kind, id }));
		}
	}

	function handleSectionDragStart(event: DragEvent, sectionId: string) {
		event.stopPropagation();
		if (isFiltering || sectionId === BOTS_SECTION_UNREAD || openMenuSection) {
			event.preventDefault();
			return;
		}
		dragState = { mode: 'section', sectionId };
		dropItemTarget = null;
		dropSectionTarget = null;
		if (event.dataTransfer) {
			event.dataTransfer.effectAllowed = 'move';
			event.dataTransfer.setData('application/x-myaaw-sidebar-section', sectionId);
		}
	}

	function handleSectionDragOver(event: DragEvent, sectionId: string) {
		if (!dragState) return;
		if (dragState.mode === 'item') {
			if (botsSidebarStore.canAcceptItem(sectionId, dragState.kind)) {
				event.preventDefault();
				if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
				dropItemTarget = sectionId;
				dropSectionTarget = null;
			} else {
				if (dropItemTarget === sectionId) dropItemTarget = null;
			}
			return;
		}
		if (dragState.mode === 'section' && dragState.sectionId !== sectionId) {
			event.preventDefault();
			if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
			const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
			const position: 'before' | 'after' =
				event.clientY < rect.top + rect.height / 2 ? 'before' : 'after';
			dropSectionTarget = { sectionId, position };
			dropItemTarget = null;
		}
	}

	function handleSectionDragLeave(event: DragEvent, sectionId: string) {
		const current = event.currentTarget as HTMLElement;
		const related = event.relatedTarget as Node | null;
		if (related && current.contains(related)) return;
		if (dropItemTarget === sectionId) dropItemTarget = null;
		if (dropSectionTarget?.sectionId === sectionId) dropSectionTarget = null;
	}

	function handleSectionDrop(event: DragEvent, sectionId: string) {
		event.preventDefault();
		if (!dragState) {
			resetDrag();
			return;
		}
		if (dragState.mode === 'item') {
			const fromUnread = dragState.sourceSectionId === BOTS_SECTION_UNREAD;
			handleMoveItem({ kind: dragState.kind, id: dragState.id }, sectionId, fromUnread);
		} else if (dragState.mode === 'section' && dropSectionTarget) {
			botsSidebarStore.moveSectionRelative(
				dragState.sectionId,
				dropSectionTarget.sectionId,
				dropSectionTarget.position
			);
		}
		resetDrag();
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
		onClose();
		createAgentDialog.openCreate({
			onSuccess: () => {
				void goto('/app/chat');
			}
		});
	}

	function selectRosterItem(item: RosterItem) {
		if (item.kind === 'group') {
			selectGroup(item.group);
		} else {
			selectBot(item.bot);
		}
	}

	function activeRosterIndex(list: RosterItem[]): number {
		const groupIdx = list.findIndex(
			(item) => item.kind === 'group' && isGroupActive(item.group.id)
		);
		if (groupIdx >= 0) return groupIdx;
		if (!onChatRoute) return -1;
		return list.findIndex(
			(item) => item.kind === 'bot' && item.bot.name === agentSelection.selected
		);
	}

	// Quick keyboard switching between groups and agents (Alt+Shift+Up/Down and Alt+1..9).
	// Scoped strictly to Bots mode because this component only mounts there.
	$effect(() => {
		if (typeof window === 'undefined') return;
		const onKey = (event: KeyboardEvent) => {
			if (event.defaultPrevented || isOverlayOpen()) return;
			if (isTypingTarget(event.target)) return;
			const list = orderedRoster;
			if (list.length === 0) return;

			if (matchesShortcut(event, 'alt+shift+arrowup')) {
				event.preventDefault();
				const idx = activeRosterIndex(list);
				const prev = idx <= 0 ? list[list.length - 1] : list[idx - 1];
				if (prev) selectRosterItem(prev);
				return;
			}
			if (matchesShortcut(event, 'alt+shift+arrowdown')) {
				event.preventDefault();
				const idx = activeRosterIndex(list);
				const next = idx < 0 || idx >= list.length - 1 ? list[0] : list[idx + 1];
				if (next) selectRosterItem(next);
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
				selectRosterItem(target);
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	function previewLine(bot: AgentInfo): string {
		const run = activeStream(bot.name);
		if (run) {
			return streamingReplyFromItems(run.items);
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

{#snippet typingDots(label: string)}
	<span class="inline-flex items-center gap-0.5 align-middle" aria-label={label}>
		<span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:-0.2s]"></span>
		<span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:-0.1s]"></span>
		<span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary"></span>
	</span>
{/snippet}

{#snippet moveToSectionSubmenu(kind: BotsSidebarItemKind, itemId: string)}
	{@const currentHome = botsSidebarStore.homeSectionOf(kind, itemId)}
	{@const builtinTarget = kind === 'group' ? BOTS_SECTION_GROUPS : BOTS_SECTION_AGENTS}
	{@const builtinLabel =
		kind === 'group' ? t('myaaw.groups.section_groups') : t('myaaw.bots.section_all')}
	<DropdownMenu.Sub>
		<DropdownMenu.SubTrigger class="gap-2">
			<FolderInput class="h-3.5 w-3.5" />
			{t('myaaw.bots.move_to_section')}
		</DropdownMenu.SubTrigger>
		<DropdownMenu.SubContent class="w-44">
			<DropdownMenu.Item
				onclick={() => handleMoveItem({ kind, id: itemId }, builtinTarget, true)}
				class="justify-between gap-2"
			>
				<span class="truncate">{builtinLabel}</span>
				<Check
					class={currentHome === builtinTarget
						? 'h-3.5 w-3.5 shrink-0'
						: 'h-3.5 w-3.5 shrink-0 opacity-0'}
				/>
			</DropdownMenu.Item>
			{#if botsSidebarStore.sections.length > 0}
				<DropdownMenu.Separator />
				{#each botsSidebarStore.sections as sec (sec.id)}
					<DropdownMenu.Item
						onclick={() => handleMoveItem({ kind, id: itemId }, sec.id, true)}
						class="justify-between gap-2"
					>
						<span class="truncate">{sec.name}</span>
						<Check
							class={currentHome === sec.id
								? 'h-3.5 w-3.5 shrink-0'
								: 'h-3.5 w-3.5 shrink-0 opacity-0'}
						/>
					</DropdownMenu.Item>
				{/each}
			{/if}
			<DropdownMenu.Separator />
			<DropdownMenu.Item onclick={openCreateSection} class="gap-2">
				<FolderPlus class="h-3.5 w-3.5" />
				{t('myaaw.bots.new_section')}
			</DropdownMenu.Item>
		</DropdownMenu.SubContent>
	</DropdownMenu.Sub>
{/snippet}

{#snippet botRow(bot: AgentInfo, sectionId: string)}
	{@const active = onChatRoute && agentSelection.selected === bot.name}
	{@const approvals = approvalCount(bot.name)}
	{@const unreads = unreadCount(bot.name)}
	{@const streaming = isStreaming(bot.name)}
	{@const timeLabel = previewTime(bot)}
	{@const busy = Boolean(busyMap[bot.name])}
	{@const menuOpen = openMenuBot === bot.name}
	{@const isDragging =
		dragState?.mode === 'item' && dragState.kind === 'agent' && dragState.id === bot.name}
	<div
		role="listitem"
		draggable={!isFiltering}
		ondragstart={(event) => handleItemDragStart(event, 'agent', bot.name, sectionId)}
		ondragend={resetDrag}
		class={['group relative', isDragging ? 'opacity-40' : '']}
	>
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
				<RuntimeBadge runtimeType={bot.runtime_type} runtimePreset={bot.runtime_preset} />
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
							'flex min-h-4 min-w-0 items-center truncate text-xs',
							streaming ? 'font-medium text-primary' : 'text-muted-foreground'
						]}
					>
						{#if streaming && !previewLine(bot)}
							{@render typingDots(t('myaaw.bots.thinking'))}
						{:else}
							<span class="truncate">{previewLine(bot)}</span>
						{/if}
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
				<DropdownMenu.Content align="end" class="w-48">
					{@render moveToSectionSubmenu('agent', bot.name)}
					<DropdownMenu.Separator />
					<AgentMoreMenuItems
						agent={bot.name}
						running={bot.running}
						{busy}
						onBusyChange={(next) => {
							busyMap = { ...busyMap, [bot.name]: next };
						}}
						includeChatActions
						includeServerControls
						includeUtilityLinks
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

{#snippet groupRow(grp: GroupSummary, sectionId: string)}
	{@const active = isGroupActive(grp.id)}
	{@const timeLabel = groupPreviewTime(grp)}
	{@const menuOpen = openMenuGroup === grp.id}
	{@const approvals = groupApprovals(grp)}
	{@const unreads = groupUnreads(grp)}
	{@const isDragging =
		dragState?.mode === 'item' && dragState.kind === 'group' && dragState.id === grp.id}
	<div
		role="listitem"
		draggable={!isFiltering}
		ondragstart={(event) => handleItemDragStart(event, 'group', grp.id, sectionId)}
		ondragend={resetDrag}
		class={['group relative', isDragging ? 'opacity-40' : '']}
	>
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
					<span
						class={[
							'grid place-items-center',
							grp.members.length === 1 ? 'grid-cols-1' : 'grid-cols-2'
						]}
						aria-hidden="true"
					>
						{#each grp.members.slice(0, grp.members.length > 4 ? 3 : 4) as member (member.name)}
							<span class="grid size-4 place-items-center">
								<Bloub mood="attentif" size={16} tight color={colorOf(member.agent_color)} />
							</span>
						{/each}
						{#if grp.members.length > 4}
							<span
								class="grid size-4 place-items-center rounded-full bg-muted text-[8px] leading-none font-semibold text-foreground tabular-nums"
							>
								+{grp.members.length - 3}
							</span>
						{/if}
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
							'flex min-h-4 min-w-0 items-center gap-1.5 truncate text-xs',
							approvals > 0
								? 'font-medium text-amber-600 dark:text-amber-400'
								: grp.running
									? 'font-medium text-primary'
									: 'text-muted-foreground'
						]}
					>
						{#if approvals > 0}
							<span class="truncate">{t('myaaw.chat.need_approval')}</span>
						{:else if grp.running}
							{#if grp.current_agent}
								<span class="truncate">{grp.current_agent}</span>
							{/if}
							{@render typingDots(
								grp.current_agent
									? t('myaaw.groups.agent_thinking', { name: grp.current_agent })
									: t('myaaw.bots.thinking')
							)}
						{:else}
							<span class="truncate">{groupPreviewLine(grp)}</span>
						{/if}
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
				<DropdownMenu.Content align="end" class="w-48">
					{@render moveToSectionSubmenu('group', grp.id)}
					<DropdownMenu.Separator />
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

{#snippet rosterRow(item: RosterItem, sectionId: string)}
	{#if item.kind === 'group'}
		{@render groupRow(item.group, sectionId)}
	{:else}
		{@render botRow(item.bot, sectionId)}
	{/if}
{/snippet}

{#snippet collapsedAvatarStack(items: RosterItem[], sectionId: string)}
	{@const slice = inlineAvatarSlice(sectionId, items.length)}
	{@const shown = items.slice(0, slice.count)}
	{@const overflow = slice.overflow}
	<div
		use:trackSlotWidth={sectionId}
		class="-my-1 flex min-w-0 flex-1 items-center -space-x-1.5 overflow-x-clip overflow-y-visible py-1.5 pr-1 pl-1.5"
	>
		{#each shown as item (item.kind === 'group' ? `c-g:${item.group.id}` : `c-a:${item.bot.name}`)}
			{#if item.kind === 'group'}
				{@const grp = item.group}
				{@const active = isGroupActive(grp.id)}
				<button
					type="button"
					onclick={(event) => {
						event.stopPropagation();
						selectGroup(grp);
					}}
					title={grp.name}
					aria-label={grp.name}
					class={[
						'relative grid h-5 w-5 shrink-0 place-items-center rounded-full bg-muted ring-2 transition-transform hover:z-20 hover:scale-110',
						active ? 'ring-primary' : 'ring-background'
					]}
				>
					{#if grp.members.length > 0}
						<span class="grid grid-cols-2 gap-[1.5px] p-0.75">
							{#each grp.members.slice(0, 4) as member (member.name)}
								<span
									aria-hidden="true"
									class={['h-1.25 w-1.25 rounded-full', groupAccent(member.agent_color).dot]}
								></span>
							{/each}
						</span>
					{:else}
						<Users class="h-2.5 w-2.5 text-muted-foreground" />
					{/if}
				</button>
			{:else}
				{@const bot = item.bot}
				{@const active = onChatRoute && agentSelection.selected === bot.name}
				<button
					type="button"
					onclick={(event) => {
						event.stopPropagation();
						selectBot(bot);
					}}
					title={bot.name}
					aria-label={bot.name}
					class={[
						'relative grid h-5 w-5 shrink-0 place-items-center rounded-full bg-muted ring-2 transition-transform hover:z-20 hover:scale-110',
						active ? 'ring-primary' : 'ring-background'
					]}
				>
					<Bloub mood="attentif" size={15} tight color={colorOf(bot.agent_color)} />
				</button>
			{/if}
		{/each}
		{#if overflow > 0}
			<button
				type="button"
				onclick={(event) => {
					event.stopPropagation();
					botsSidebarStore.toggleCollapsed(sectionId);
				}}
				title={`+${overflow}`}
				aria-label={`+${overflow}`}
				class="relative z-0 grid h-5 min-w-5 shrink-0 place-items-center rounded-full bg-muted px-1 text-[9px] font-semibold text-muted-foreground ring-2 ring-background tabular-nums transition-colors hover:bg-accent hover:text-foreground"
			>
				+{overflow}
			</button>
		{/if}
	</div>
{/snippet}

{#snippet compactRosterButton(item: RosterItem)}
	{#if item.kind === 'group'}
		{@const grp = item.group}
		{@const active = isGroupActive(grp.id)}
		{@const compactGroupUnreads = groupUnreads(grp)}
		{@const compactGroupApprovals = groupApprovals(grp)}
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
			{#if compactGroupApprovals > 0}
				<span
					class="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-amber-500 px-1 text-[9px] leading-4 font-semibold text-white tabular-nums"
				>
					{compactGroupApprovals > 9 ? '9+' : compactGroupApprovals}
				</span>
			{:else if compactGroupUnreads > 0}
				<span
					class="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[9px] leading-4 font-semibold text-primary-foreground tabular-nums"
				>
					{compactGroupUnreads > 9 ? '9+' : compactGroupUnreads}
				</span>
			{:else if grp.running}
				<span
					aria-hidden="true"
					class="absolute right-1 bottom-1 h-2 w-2 animate-pulse rounded-full bg-primary ring-2 ring-card"
				></span>
			{/if}
		</button>
	{:else}
		{@const bot = item.bot}
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
			<span class="relative inline-flex items-center justify-center">
				<Bloub mood="attentif" size={28} tight color={colorOf(bot.agent_color)} />
				<RuntimeBadge
					runtimeType={bot.runtime_type}
					runtimePreset={bot.runtime_preset}
					class="-top-1.5 -left-1.5"
				/>
			</span>
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
	{/if}
{/snippet}

<!--
	Bots mode turns the left sidebar into a direct roster of Agents, Groups, and
	user-created mixed sections with pinned Unread, collapsible headers, and
	typed drag-and-drop synced via controlplane.db.
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
				<DropdownMenu.Item onclick={openCreateSection} class="gap-2">
					<FolderPlus class="h-3.5 w-3.5" />
					{t('myaaw.bots.new_section')}
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
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
		{#each compactBlocks as block, idx (block.id)}
			{#if idx > 0}
				<div class="mx-auto my-1.5 h-px w-6 bg-border"></div>
			{/if}
			{#each block.items as item (item.kind === 'group' ? `g:${item.group.id}` : `a:${item.bot.name}`)}
				{@render compactRosterButton(item)}
			{/each}
		{/each}
	</nav>
{:else}
	<div class="flex shrink-0 items-center gap-1.5 px-3 py-2">
		<div class="relative min-w-0 flex-1">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<input
				type="text"
				inputmode="search"
				role="searchbox"
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
						{groupsStore.groups.length + agentSelection.agents.length}
					</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'groups')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'groups' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_groups')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">
						{groupsStore.groups.length}
					</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'bots')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'bots' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_bots')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">
						{agentSelection.agents.length}
					</span>
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={() => (filter = 'running')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'running' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_running')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">{runningTotal}</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'stopped')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'stopped' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_stopped')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">{stoppedTotal}</span>
				</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => (filter = 'unread')} class="justify-between gap-2">
					<span class="flex items-center gap-2">
						<Check class={filter === 'unread' ? 'h-3.5 w-3.5' : 'h-3.5 w-3.5 opacity-0'} />
						{t('myaaw.bots.filter_unread')}
					</span>
					<span class="text-[11px] text-muted-foreground tabular-nums">{attentionFilterCount}</span>
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
				<DropdownMenu.Item onclick={openCreateSection} class="gap-2">
					<FolderPlus class="h-3.5 w-3.5" />
					{t('myaaw.bots.new_section')}
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
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
		class="min-h-0 flex-1 space-y-2 overflow-y-auto px-2 py-1"
		aria-label={t('myaaw.bots.list_label')}
	>
		{#if listError}
			<p
				role="alert"
				class="mx-2 my-2 rounded-xl border border-destructive/30 bg-destructive/10 p-2 text-xs wrap-break-word text-destructive"
			>
				{listError}
			</p>
		{/if}

		{#if !listError && listLoading && visibleAgents.length === 0 && visibleGroups.length === 0}
			<BotsSidebarSkeleton />
		{:else if !listError && visibleAgents.length === 0 && visibleGroups.length === 0 && visibleSections.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">
				{searchTerm
					? t('myaaw.bots.no_matches')
					: filter === 'groups'
						? t('myaaw.bots.empty_groups')
						: filter !== 'all'
							? t('myaaw.bots.empty_filter')
							: t('myaaw.agent.none')}
			</p>
		{:else}
			<!-- Pinned Unified Unread Section (Agents + Groups) -->
			{#if attentionItems.length > 0}
				{@const unreadCollapsed = isSectionCollapsed(BOTS_SECTION_UNREAD)}
				<section class="space-y-0.5" aria-label={t('myaaw.bots.section_unread')}>
					<div class="flex items-center justify-between gap-1 px-1.5 py-1">
						<button
							type="button"
							onclick={() => botsSidebarStore.toggleCollapsed(BOTS_SECTION_UNREAD)}
							aria-expanded={!unreadCollapsed}
							class={[
								'flex min-w-0 items-center gap-1 rounded-md px-0.5 py-0.5 text-left text-muted-foreground transition-colors hover:text-foreground',
								unreadCollapsed ? 'max-w-[46%] shrink-0' : 'flex-1'
							]}
						>
							<ChevronDown
								class={unreadCollapsed
									? 'h-3 w-3 shrink-0 -rotate-90 transition-transform'
									: 'h-3 w-3 shrink-0 transition-transform'}
							/>
							<span class="truncate text-[10px] font-semibold tracking-widest uppercase">
								{t('myaaw.bots.section_unread')}
							</span>
						</button>
						{#if unreadCollapsed}
							{@render collapsedAvatarStack(attentionItems, BOTS_SECTION_UNREAD)}
						{/if}
						{#if unreadCollapsed}
							<span
								class={[
									'grid h-4 min-w-4 shrink-0 place-items-center rounded-full px-1 text-[10px] leading-none font-semibold tabular-nums',
									unreadHasApprovals
										? 'bg-amber-500 text-white'
										: 'bg-primary text-primary-foreground'
								]}
							>
								{unreadBadgeTotal > 99 ? '99+' : unreadBadgeTotal}
							</span>
						{:else}
							<span class="px-1 text-[10px] font-medium text-muted-foreground tabular-nums">
								{unreadBadgeTotal}
							</span>
						{/if}
					</div>
					{#if !unreadCollapsed}
						<div role="list" class="space-y-0.5">
							{#each attentionItems as item (item.kind === 'group' ? `u-g:${item.group.id}` : `u-a:${item.bot.name}`)}
								{@render rosterRow(item, BOTS_SECTION_UNREAD)}
							{/each}
						</div>
					{/if}
				</section>
			{/if}

			<!-- Ordered Reorderable Sections (Custom + Built-in Groups / Agents) -->
			{#each visibleSections as sec (sec.id)}
				{@const secCollapsed = isSectionCollapsed(sec.id)}
				{@const menuOpen = openMenuSection === sec.id}
				{@const activeItemDrag = dragState?.mode === 'item' ? dragState : null}
				{@const itemAllowed =
					activeItemDrag !== null && botsSidebarStore.canAcceptItem(sec.id, activeItemDrag.kind)}
				{@const itemDisallowed = activeItemDrag !== null && !itemAllowed}
				{@const isItemDropActive = dropItemTarget === sec.id && itemAllowed}
				{@const isSectionDragging = dragState?.mode === 'section' && dragState.sectionId === sec.id}
				{@const showDropBefore =
					dropSectionTarget?.sectionId === sec.id && dropSectionTarget.position === 'before'}
				{@const showDropAfter =
					dropSectionTarget?.sectionId === sec.id && dropSectionTarget.position === 'after'}
				{@const hasRunningInside = sec.items.some((item) => itemRunning(item))}

				<section
					aria-label={sec.label}
					ondragover={(event) => handleSectionDragOver(event, sec.id)}
					ondragleave={(event) => handleSectionDragLeave(event, sec.id)}
					ondrop={(event) => handleSectionDrop(event, sec.id)}
					class={[
						'relative space-y-0.5 rounded-xl transition-all',
						isItemDropActive ? 'bg-primary/6 ring-1 ring-primary/50' : '',
						itemDisallowed ? 'opacity-55' : '',
						isSectionDragging ? 'opacity-40' : ''
					]}
				>
					{#if showDropBefore}
						<div
							aria-hidden="true"
							class="pointer-events-none absolute -top-1 right-1.5 left-1.5 z-10 h-0.5 rounded-full bg-primary"
						></div>
					{/if}

					<div
						role="group"
						draggable={!isFiltering}
						ondragstart={(event) => handleSectionDragStart(event, sec.id)}
						ondragend={resetDrag}
						class="group/sec flex items-center justify-between gap-1 px-1.5 py-1"
					>
						<button
							type="button"
							onclick={() => botsSidebarStore.toggleCollapsed(sec.id)}
							aria-expanded={!secCollapsed}
							class={[
								'flex min-w-0 items-center gap-1 rounded-md px-0.5 py-0.5 text-left text-muted-foreground transition-colors hover:text-foreground',
								secCollapsed && sec.items.length > 0 ? 'max-w-[46%] shrink-0' : 'flex-1'
							]}
						>
							<ChevronDown
								class={secCollapsed
									? 'h-3 w-3 shrink-0 -rotate-90 transition-transform'
									: 'h-3 w-3 shrink-0 transition-transform'}
							/>
							{#if !isFiltering}
								<GripVertical
									class="hidden h-3 w-3 shrink-0 cursor-grab text-muted-foreground/50 opacity-0 transition-opacity group-hover/sec:opacity-100 sm:block"
								/>
							{/if}
							<span class="truncate text-[10px] font-semibold tracking-widest uppercase">
								{sec.label}
							</span>
						</button>

						{#if secCollapsed && sec.items.length > 0}
							{@render collapsedAvatarStack(sec.items, sec.id)}
						{/if}

						<div class="flex shrink-0 items-center gap-1">
							{#if secCollapsed && hasRunningInside}
								<span aria-hidden="true" class="h-1.5 w-1.5 animate-pulse rounded-full bg-primary"
								></span>
							{/if}
							<span class="px-0.5 text-[10px] font-medium text-muted-foreground tabular-nums">
								{sec.items.length}
							</span>

							{#if !isFiltering}
								<DropdownMenu.Root
									open={menuOpen}
									onOpenChange={(open) => {
										openMenuSection = open
											? sec.id
											: openMenuSection === sec.id
												? ''
												: openMenuSection;
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
												aria-label={t('myaaw.bots.section_actions')}
												title={t('myaaw.bots.section_actions')}
												class={[
													'grid h-5 w-5 place-items-center rounded text-muted-foreground transition-opacity hover:bg-muted hover:text-foreground',
													menuOpen
														? 'opacity-100'
														: 'opacity-0 group-focus-within/sec:opacity-100 group-hover/sec:opacity-100 [@media(hover:none)]:opacity-100'
												]}
											>
												{#if sec.kind === 'agents' && fleetBulk.busy}
													<Loader2 class="h-3.5 w-3.5 animate-spin" />
												{:else}
													<MoreHorizontal class="h-3.5 w-3.5" />
												{/if}
											</button>
										{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class={sec.kind === 'agents' ? 'w-60' : 'w-44'}>
										{#if sec.kind === 'custom'}
											<DropdownMenu.Item onclick={() => openEditSection(sec.id)} class="gap-2">
												<Pencil class="h-3.5 w-3.5" />
												{t('myaaw.bots.section_edit')}
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
										{/if}
										{#if sec.kind === 'agents'}
											<DropdownMenu.Item
												disabled={fleetBulk.busy !== '' ||
													runningTotal === agentSelection.agents.length}
												onclick={() => {
													openMenuSection = '';
													void fleetBulk.run('start', t);
												}}
												class="gap-2"
											>
												{#if fleetBulk.busy === 'start'}
													<Loader2 class="h-3.5 w-3.5 animate-spin" />
												{:else}
													<Play class="h-3.5 w-3.5" />
												{/if}
												{t('myaaw.server.start_all')}
											</DropdownMenu.Item>
											<DropdownMenu.Item
												disabled={fleetBulk.busy !== '' || runningTotal === 0}
												onclick={() => {
													openMenuSection = '';
													void fleetBulk.run('restart', t);
												}}
												class="gap-2"
											>
												{#if fleetBulk.busy === 'restart'}
													<Loader2 class="h-3.5 w-3.5 animate-spin" />
												{:else}
													<RotateCw class="h-3.5 w-3.5" />
												{/if}
												{t('myaaw.server.restart_all')}
											</DropdownMenu.Item>
											<DropdownMenu.Item
												disabled={fleetBulk.busy !== '' || runningTotal === 0}
												onclick={() => {
													openMenuSection = '';
													void fleetBulk.run('stop', t);
												}}
												class="gap-2"
											>
												{#if fleetBulk.busy === 'stop'}
													<Loader2 class="h-3.5 w-3.5 animate-spin" />
												{:else}
													<Square class="h-3.5 w-3.5" />
												{/if}
												{t('myaaw.server.stop_all')}
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
										{/if}
										<DropdownMenu.Item
											disabled={!canMoveSection(sec.id, -1)}
											onclick={() => {
												openMenuSection = '';
												botsSidebarStore.moveSectionBy(sec.id, -1);
											}}
											class="gap-2"
										>
											<ArrowUp class="h-3.5 w-3.5" />
											{t('myaaw.bots.section_move_up')}
										</DropdownMenu.Item>
										<DropdownMenu.Item
											disabled={!canMoveSection(sec.id, 1)}
											onclick={() => {
												openMenuSection = '';
												botsSidebarStore.moveSectionBy(sec.id, 1);
											}}
											class="gap-2"
										>
											<ArrowDown class="h-3.5 w-3.5" />
											{t('myaaw.bots.section_move_down')}
										</DropdownMenu.Item>
										{#if sec.kind === 'custom'}
											<DropdownMenu.Separator />
											<DropdownMenu.Item
												onclick={() => void deleteCustomSection(sec)}
												class="gap-2 text-destructive focus:text-destructive"
											>
												<Trash2 class="h-3.5 w-3.5" />
												{t('myaaw.bots.section_delete')}
											</DropdownMenu.Item>
										{/if}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						</div>
					</div>

					{#if !secCollapsed}
						{#if sec.items.length > 0}
							<div role="list" class="space-y-0.5">
								{#each sec.items as item (item.kind === 'group' ? `${sec.id}-g:${item.group.id}` : `${sec.id}-a:${item.bot.name}`)}
									{@render rosterRow(item, sec.id)}
								{/each}
							</div>
						{:else if sec.kind === 'custom'}
							<div
								class={[
									'mx-1 flex items-center justify-between gap-2 rounded-lg border border-dashed px-2.5 py-2 text-[11px] text-muted-foreground transition-colors',
									isItemDropActive ? 'border-primary/60 text-foreground' : 'border-border/70'
								]}
							>
								<span class="truncate">{t('myaaw.bots.section_empty_drop')}</span>
								<button
									type="button"
									onclick={() => openEditSection(sec.id)}
									class="shrink-0 font-medium text-primary hover:underline"
								>
									{t('myaaw.bots.section_add_items')}
								</button>
							</div>
						{:else}
							<div
								class="mx-1 rounded-lg border border-dashed border-primary/50 px-2.5 py-2 text-center text-[11px] text-primary"
							>
								{sec.kind === 'groups'
									? t('myaaw.bots.drop_group_here')
									: t('myaaw.bots.drop_agent_here')}
							</div>
						{/if}
					{/if}

					{#if showDropAfter}
						<div
							aria-hidden="true"
							class="pointer-events-none absolute right-1.5 -bottom-1 left-1.5 z-10 h-0.5 rounded-full bg-primary"
						></div>
					{/if}
				</section>
			{/each}
		{/if}
	</nav>
{/if}

<BotsSectionDialog bind:open={sectionDialogOpen} sectionId={editingSectionId} />

<SidebarAccount {compact} />
