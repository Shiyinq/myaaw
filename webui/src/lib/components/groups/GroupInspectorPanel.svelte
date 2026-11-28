<script lang="ts">
	import AttachmentPanelSkeleton from '$lib/components/app/AttachmentPanelSkeleton.svelte';
	import { goto } from '$app/navigation';
	import { untrack } from 'svelte';
	import {
		Eraser,
		File as FileIcon,
		FileText,
		Image as ImageIcon,
		Loader2,
		MessageSquare,
		Music,
		Paperclip,
		Play,
		Plus,
		Save,
		Settings2,
		ShieldAlert,
		Square,
		Trash2,
		UserMinus,
		Users,
		Video,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import ChatTime from '$lib/components/common/ChatTime.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, serverStore, toastStore } from '$lib/stores';
	import type { PendingApproval } from '$lib/stores/approvalQueue.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { groupAttachmentUrl } from '$lib/utils/attachments';
	import { formatShortcut } from '$lib/utils/keys';
	import type {
		ApprovalDecision,
		DiscussionMode,
		Group,
		GroupAttachment,
		GroupMember
	} from '$lib/types';

	type Tab = 'members' | 'attachments' | 'settings';
	type AttachmentFilter = 'all' | 'media' | 'documents';

	interface Props {
		group: Group;
		tab?: Tab;
		pendingApprovals?: PendingApproval[];
		attachmentsRefreshKey?: number;
		onDecideApproval?: (entry: PendingApproval, decision: ApprovalDecision) => void;
		onOpenAttachment?: (item: GroupAttachment) => void;
		onShowAttachmentInChat?: (item: GroupAttachment) => void;
		onClose: () => void;
		onUpdated: (group: Group) => void;
		onCleared: () => void;
	}

	let {
		group,
		tab = $bindable<Tab>('members'),
		pendingApprovals = [],
		attachmentsRefreshKey = 0,
		onDecideApproval,
		onOpenAttachment,
		onShowAttachmentInChat,
		onClose,
		onUpdated,
		onCleared
	}: Props = $props();

	const { t } = useTranslation();

	let editName = $state('');
	let editDescription = $state('');
	let editMode = $state<DiscussionMode>('autonomous');
	let editMaxTurns = $state(8);
	let saving = $state(false);
	let savingDisplay = $state(false);
	let busyMap = $state<Record<string, boolean>>({});
	let mutatingMembers = $state(false);
	let clearing = $state(false);
	let deleting = $state(false);

	// Re-sync draft fields whenever the viewed group changes or is saved externally.
	$effect(() => {
		editName = group.name;
		editDescription = group.description ?? '';
		editMode = group.discussion_mode;
		editMaxTurns = group.max_turns;
	});

	const tabIndex = $derived(tab === 'members' ? 0 : tab === 'attachments' ? 1 : 2);

	const ATTACHMENT_PAGE = 50;
	let attachmentFilter = $state<AttachmentFilter>('all');
	let attachmentItems = $state<GroupAttachment[]>([]);
	let attachmentTotal = $state(0);
	let attachmentHasMore = $state(false);
	let attachmentLoading = $state(false);
	let attachmentLoadingMore = $state(false);
	let attachmentError = $state('');
	let attachmentGen = 0;
	let attachmentLoadKey = '';
	let attachmentSettledScope = $state('');
	const attachmentScope = $derived(JSON.stringify([group.id, attachmentFilter]));
	const attachmentPending = $derived(attachmentSettledScope !== attachmentScope);

	const attachmentKindParam = $derived(
		attachmentFilter === 'media'
			? 'image,video,audio'
			: attachmentFilter === 'documents'
				? 'pdf,file,project'
				: ''
	);

	$effect(() => {
		if (tab !== 'attachments') return;
		const key = `${group.id}:${attachmentFilter}:${attachmentsRefreshKey}`;
		if (key === attachmentLoadKey) return;
		attachmentLoadKey = key;
		untrack(() => void loadAttachments(true));
	});

	async function loadAttachments(reset: boolean) {
		const scope = attachmentScope;
		if (reset) {
			if (attachmentPending) {
				attachmentItems = [];
				attachmentHasMore = false;
				attachmentTotal = 0;
			}
			attachmentGen += 1;
			attachmentLoading = true;
			attachmentError = '';
		} else {
			if (attachmentLoading || attachmentLoadingMore || !attachmentHasMore) return;
			attachmentLoadingMore = true;
		}
		const current = attachmentGen;
		const offset = reset ? 0 : attachmentItems.length;
		try {
			const page = await groupsStore.fetchAttachments(group.id, {
				limit: ATTACHMENT_PAGE,
				offset,
				kind: attachmentKindParam || undefined
			});
			if (current !== attachmentGen) return;
			const fresh = page.items ?? [];
			attachmentItems = reset
				? fresh
				: [
						...attachmentItems,
						...fresh.filter((item) => !attachmentItems.some((known) => known.id === item.id))
					];
			attachmentTotal = page.total ?? attachmentItems.length;
			attachmentHasMore = Boolean(page.has_more) && fresh.length > 0;
			attachmentError = '';
			const byAgent = new Map<string, string[]>();
			for (const entry of fresh) {
				if (entry.agent && entry.path && entry.kind === 'image') {
					const list = byAgent.get(entry.agent) ?? [];
					list.push(entry.path);
					byAgent.set(entry.agent, list);
				}
			}
			for (const [agentName, paths] of byAgent) {
				void ensureSigned(agentName, paths);
			}
		} catch (err) {
			if (current !== attachmentGen) return;
			attachmentError = getErrorMessage(err);
		} finally {
			if (current === attachmentGen) {
				if (reset) attachmentSettledScope = scope;
				attachmentLoading = false;
				attachmentLoadingMore = false;
			}
		}
	}

	function attachmentIcon(kind: string) {
		switch (kind) {
			case 'image':
				return ImageIcon;
			case 'video':
				return Video;
			case 'audio':
				return Music;
			case 'pdf':
				return FileText;
			default:
				return FileIcon;
		}
	}

	function formatSize(bytes?: number): string {
		if (!bytes || bytes <= 0) return '';
		const units = ['B', 'KB', 'MB', 'GB'];
		let value = bytes;
		let unit = 0;
		while (value >= 1024 && unit < units.length - 1) {
			value /= 1024;
			unit += 1;
		}
		return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
	}

	const memberSet = $derived(new Set(group.members.map((m) => m.name)));
	const availableToAdd = $derived(
		agentSelection.agents.filter((agent) => !memberSet.has(agent.name))
	);

	const settingsDirty = $derived(
		editName.trim() !== group.name ||
			editDescription.trim() !== (group.description ?? '') ||
			editMode !== group.discussion_mode ||
			Math.round(editMaxTurns) !== group.max_turns
	);

	const canSaveSettings = $derived(
		settingsDirty &&
			editName.trim().length > 0 &&
			editMaxTurns >= 1 &&
			editMaxTurns <= 50 &&
			!saving
	);

	const modeOptions: { id: DiscussionMode; labelKey: string; descKey: string }[] = [
		{
			id: 'autonomous',
			labelKey: 'myaaw.groups.mode_autonomous',
			descKey: 'myaaw.groups.mode_autonomous_desc'
		},
		{
			id: 'all',
			labelKey: 'myaaw.groups.mode_all',
			descKey: 'myaaw.groups.mode_all_desc'
		},
		{
			id: 'mention',
			labelKey: 'myaaw.groups.mode_mention',
			descKey: 'myaaw.groups.mode_mention_desc'
		}
	];

	function isMemberRunning(member: GroupMember): boolean {
		const live = agentSelection.agents.find((a) => a.name === member.name);
		return live ? live.running : member.running;
	}

	function colorForMember(member: GroupMember): string {
		const live = agentSelection.agents.find((a) => a.name === member.name);
		return live?.agent_color || member.agent_color;
	}

	const onlineMembersCount = $derived(
		group.members.filter((member) => isMemberRunning(member)).length
	);

	async function controlMember(agentName: string, action: 'start' | 'stop') {
		if (busyMap[agentName]) return;
		busyMap = { ...busyMap, [agentName]: true };
		agentSelection.setRunning(agentName, action === 'start');
		try {
			if (action === 'start') {
				await serverStore.start(agentName);
				toastStore.success(t('myaaw.server.start_done'));
			} else {
				await serverStore.stop(agentName);
				toastStore.success(t('myaaw.server.stop_done'));
			}
			setTimeout(() => {
				void agentSelection.load(true);
				void groupsStore
					.get(group.id, { limit: 1, offset: -1 })
					.then((detail) => onUpdated(detail.group))
					.catch(() => {});
			}, 1400);
		} catch (err) {
			agentSelection.clearOptimisticRunning(agentName);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			busyMap = { ...busyMap, [agentName]: false };
		}
	}

	async function addMember(agentName: string) {
		if (mutatingMembers) return;
		mutatingMembers = true;
		try {
			const next = [...group.members.map((m) => m.name), agentName];
			const updated = await groupsStore.update(group.id, { members: next });
			onUpdated(updated);
			toastStore.success(t('myaaw.groups.member_added', { name: agentName }));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			mutatingMembers = false;
		}
	}

	async function removeMember(agentName: string) {
		if (mutatingMembers || group.members.length <= 1) return;
		mutatingMembers = true;
		try {
			const next = group.members.map((m) => m.name).filter((name) => name !== agentName);
			const updated = await groupsStore.update(group.id, { members: next });
			onUpdated(updated);
			toastStore.success(t('myaaw.groups.member_removed', { name: agentName }));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			mutatingMembers = false;
		}
	}

	async function toggleDisplayFlag(field: 'hide_thinking' | 'hide_tools', next: boolean) {
		if (savingDisplay) return;
		const previous = group;
		// Apply immediately to local state so the transcript hides/reveals steps
		// instantaneously without re-fetching messages, while persisting to SQLite.
		onUpdated({ ...group, [field]: next });
		savingDisplay = true;
		try {
			const updated = await groupsStore.update(group.id, { [field]: next });
			onUpdated(updated);
		} catch (err) {
			onUpdated(previous);
			toastStore.error(getErrorMessage(err));
		} finally {
			savingDisplay = false;
		}
	}

	async function saveSettings(event: SubmitEvent) {
		event.preventDefault();
		if (!canSaveSettings) return;
		saving = true;
		try {
			const updated = await groupsStore.update(group.id, {
				name: editName.trim(),
				description: editDescription.trim(),
				discussion_mode: editMode,
				max_turns: Math.min(50, Math.max(1, Math.round(editMaxTurns)))
			});
			onUpdated(updated);
			toastStore.success(t('myaaw.groups.settings_saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function handleClearChat() {
		const confirmed = await askConfirm({
			title: t('myaaw.groups.clear_title'),
			message: t('myaaw.groups.clear_confirm'),
			confirmText: t('myaaw.groups.clear_action'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		clearing = true;
		try {
			await groupsStore.clearMessages(group.id);
			onCleared();
			toastStore.success(t('myaaw.groups.cleared'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			clearing = false;
		}
	}

	async function handleDeleteGroup() {
		const confirmed = await askConfirm({
			title: t('myaaw.groups.delete_title'),
			message: t('myaaw.groups.delete_confirm', { name: group.name }),
			confirmText: t('myaaw.groups.delete_action'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		deleting = true;
		try {
			await groupsStore.remove(group.id);
			toastStore.success(t('myaaw.groups.deleted', { name: group.name }));
			await goto('/app/chat');
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			deleting = false;
		}
	}

	const onlineCount = $derived(group.members.filter((m) => isMemberRunning(m)).length);
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="relative flex shrink-0 flex-col items-center gap-1.5 px-4 pt-4 pb-3 lg:pt-16">
		<Button
			variant="ghost"
			size="icon-sm"
			class="absolute top-2 right-2 lg:hidden"
			onclick={onClose}
			aria-label={t('myaaw.focus.panel_close')}
			aria-keyshortcuts="Control+Shift+H Meta+Shift+H"
			title={`${t('myaaw.focus.panel_close')} (${formatShortcut('mod+shift+h')})`}
		>
			<X class="h-4 w-4" />
		</Button>

		<div class="flex items-center -space-x-2">
			{#each group.members.slice(0, 4) as member (member.name)}
				<span
					class="grid h-10 w-10 place-items-center rounded-full bg-muted/80 ring-2 ring-card"
					title={member.name}
				>
					<Bloub mood="attentif" size={30} tight color={colorOf(colorForMember(member))} />
				</span>
			{/each}
			{#if group.members.length > 4}
				<span
					class="grid h-10 w-10 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground ring-2 ring-card"
				>
					+{group.members.length - 4}
				</span>
			{/if}
		</div>

		<p class="mt-1 max-w-full truncate text-sm font-semibold">{group.name}</p>
		<p class="flex items-center gap-1.5 text-xs text-muted-foreground">
			<span
				>{t('myaaw.groups.members_online', {
					online: onlineCount,
					total: group.members.length
				})}</span
			>
			<span>·</span>
			<span class="capitalize">{group.discussion_mode}</span>
		</p>
	</div>

	<div role="tablist" class="relative mx-3 mb-2 grid shrink-0 grid-cols-3 rounded-lg border p-0.5">
		<span
			aria-hidden="true"
			class="pointer-events-none absolute inset-y-0.5 left-0.5 w-[calc((100%-0.25rem)/3)] rounded-md bg-primary transition-transform duration-200 ease-out will-change-transform"
			style:transform={`translateX(${tabIndex * 100}%)`}
		></span>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'members'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-xs font-medium select-none transition-colors duration-150',
				tab === 'members'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'members')}
		>
			<Users class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.groups.tab_members')}</span>
			{#if pendingApprovals.length > 0}
				<span
					class="inline-flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full bg-amber-500 px-1 text-[10px] leading-none font-semibold text-white tabular-nums"
				>
					{pendingApprovals.length > 9 ? '9+' : pendingApprovals.length}
				</span>
			{/if}
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'attachments'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-xs font-medium select-none transition-colors duration-150',
				tab === 'attachments'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'attachments')}
		>
			<Paperclip class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.groups.tab_attachments')}</span>
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'settings'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-xs font-medium select-none transition-colors duration-150',
				tab === 'settings'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'settings')}
		>
			<Settings2 class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.groups.tab_settings')}</span>
		</button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
		{#if tab === 'members'}
			<div class="space-y-3 pt-1">
				{#if pendingApprovals.length > 0}
					<div class="space-y-1.5">
						<p
							class="flex items-center gap-1 px-1 text-[10px] font-semibold tracking-widest text-amber-600 uppercase dark:text-amber-400"
						>
							<ShieldAlert class="h-3 w-3 shrink-0" />
							{t('myaaw.groups.pending_approvals')} ({pendingApprovals.length})
						</p>
						{#each pendingApprovals as entry (entry.approvalId)}
							{@const accent = groupAccent(
								agentSelection.agents.find((a) => a.name === entry.agent)?.agent_color ||
									entry.agentColor
							)}
							<div class="rounded-xl border border-amber-500/35 bg-amber-500/10 p-2.5 text-xs">
								<div class="flex flex-wrap items-center gap-1.5">
									<span
										class={[
											'inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold ring-1',
											accent.softBg,
											accent.text,
											accent.ring
										]}
									>
										<span class={['h-1.5 w-1.5 rounded-full', accent.dot]}></span>
										@{entry.agent}
									</span>
									{#if entry.subAgent}
										<span
											class="rounded-full bg-amber-500/25 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-amber-800 dark:text-amber-100"
										>
											{entry.subAgent}
										</span>
									{/if}
									<span class="truncate font-mono text-[11px] font-medium text-foreground">
										{entry.toolName}
									</span>
								</div>
								{#if entry.argsSummary}
									<pre
										class="mt-1 max-h-20 overflow-auto whitespace-pre-wrap break-all font-mono text-[10px] text-muted-foreground">{entry.argsSummary}</pre>
								{/if}
								{#if onDecideApproval}
									<div class="mt-2 flex flex-wrap gap-1.5">
										<Button size="xs" onclick={() => onDecideApproval(entry, 'allow_once')}>
											{t('myaaw.chat.allow_once')}
										</Button>
										<Button
											size="xs"
											variant="secondary"
											onclick={() => onDecideApproval(entry, 'allow_session')}
										>
											{t('myaaw.chat.allow_session')}
										</Button>
										<Button
											size="xs"
											variant="secondary"
											onclick={() => onDecideApproval(entry, 'allow_always')}
										>
											{t('myaaw.chat.allow_always')}
										</Button>
										<Button
											size="xs"
											variant="destructive"
											onclick={() => onDecideApproval(entry, 'deny')}
										>
											{t('myaaw.chat.deny')}
										</Button>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

				<div class="space-y-1.5">
					<div class="flex items-center justify-between gap-2 px-1">
						<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
							{t('myaaw.groups.current_members')} ({group.members.length})
						</p>
						<span class="flex items-center gap-1 text-[10px] text-muted-foreground tabular-nums">
							<span
								aria-hidden="true"
								class={[
									'h-1.5 w-1.5 rounded-full',
									onlineMembersCount > 0 ? 'bg-emerald-500' : 'bg-muted-foreground/40'
								]}
							></span>
							{onlineMembersCount}/{group.members.length}
							{t('myaaw.groups.online')}
						</span>
					</div>
					{#each group.members as member (member.name)}
						{@const running = isMemberRunning(member)}
						{@const busy = Boolean(busyMap[member.name])}
						<div class="flex items-center gap-2.5 rounded-xl border bg-background/60 px-2.5 py-2">
							<span class="relative shrink-0">
								<Bloub mood="attentif" size={28} tight color={colorOf(colorForMember(member))} />
								<span
									aria-hidden="true"
									class={[
										'absolute -right-0.5 -bottom-0.5 h-2.5 w-2.5 rounded-full ring-2 ring-card',
										running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
									]}
								></span>
							</span>

							<div class="min-w-0 flex-1">
								<p class="truncate text-xs font-semibold">{member.name}</p>
								<p class="text-[11px] text-muted-foreground">
									{running ? t('myaaw.groups.online') : t('myaaw.bots.stopped')}
								</p>
							</div>

							<div class="flex shrink-0 items-center gap-1">
								{#if !running}
									<Button
										size="sm"
										variant="outline"
										class="h-7 gap-1 px-2 text-[11px]"
										disabled={busy}
										onclick={() => void controlMember(member.name, 'start')}
									>
										{#if busy}
											<Loader2 class="h-3 w-3 animate-spin" />
										{:else}
											<Play class="h-3 w-3" />
										{/if}
										{t('myaaw.server.start')}
									</Button>
								{:else}
									<button
										type="button"
										disabled={busy}
										onclick={() => void controlMember(member.name, 'stop')}
										title={t('myaaw.server.stop')}
										aria-label={t('myaaw.server.stop')}
										class="grid h-7 w-7 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
									>
										{#if busy}
											<Loader2 class="h-3.5 w-3.5 animate-spin" />
										{:else}
											<Square class="h-3 w-3" />
										{/if}
									</button>
								{/if}

								{#if group.members.length > 1}
									<button
										type="button"
										disabled={mutatingMembers}
										onclick={() => void removeMember(member.name)}
										title={t('myaaw.groups.remove_member')}
										aria-label={t('myaaw.groups.remove_member')}
										class="grid h-7 w-7 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-40"
									>
										<UserMinus class="h-3.5 w-3.5" />
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>

				{#if availableToAdd.length > 0}
					<div class="space-y-1.5 pt-2">
						<p
							class="px-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
						>
							{t('myaaw.groups.add_member')}
						</p>
						{#each availableToAdd as agent (agent.name)}
							<div class="flex items-center gap-2.5 rounded-xl border border-dashed px-2.5 py-2">
								<span class="relative shrink-0">
									<Bloub mood="attentif" size={24} tight color={colorOf(agent.agent_color)} />
									<span
										aria-hidden="true"
										class={[
											'absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 ring-card',
											agent.running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
										]}
									></span>
								</span>
								<div class="min-w-0 flex-1">
									<p class="truncate text-xs font-medium">{agent.name}</p>
									<p class="text-[10px] text-muted-foreground">
										{agent.running ? t('myaaw.groups.online') : t('myaaw.bots.stopped')}
									</p>
								</div>
								<Button
									size="sm"
									variant="ghost"
									class="h-7 gap-1 px-2 text-xs"
									disabled={mutatingMembers}
									onclick={() => void addMember(agent.name)}
								>
									<Plus class="h-3.5 w-3.5" />
									{t('myaaw.groups.add_action')}
								</Button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{:else if tab === 'attachments'}
			<div class="space-y-3 pt-1">
				<div
					role="group"
					aria-label={t('myaaw.chat.attachments_filters')}
					class="relative grid grid-cols-3 rounded-lg border p-0.5"
				>
					<span
						aria-hidden="true"
						class="pointer-events-none absolute inset-y-0.5 left-0.5 w-[calc((100%-0.25rem)/3)] rounded-md bg-primary transition-transform duration-200 ease-out"
						style:transform={`translateX(${(attachmentFilter === 'all' ? 0 : attachmentFilter === 'media' ? 1 : 2) * 100}%)`}
					></span>
					{#each [{ id: 'all', label: t('myaaw.chat.attachments_filter_all') }, { id: 'media', label: t('myaaw.chat.attachments_filter_media') }, { id: 'documents', label: t('myaaw.chat.attachments_filter_documents') }] as chip (chip.id)}
						<button
							type="button"
							onclick={() => (attachmentFilter = chip.id as AttachmentFilter)}
							aria-pressed={attachmentFilter === chip.id}
							class={[
								'relative truncate rounded-md px-2 py-1.5 text-xs font-medium transition-colors duration-150',
								attachmentFilter === chip.id
									? 'text-primary-foreground'
									: 'text-muted-foreground hover:text-foreground'
							]}
						>
							{chip.label}
						</button>
					{/each}
				</div>

				{#if attachmentPending}
					<AttachmentPanelSkeleton />
				{:else if attachmentError}
					<p class="px-1 py-4 text-center text-xs text-destructive">{attachmentError}</p>
				{:else if attachmentItems.length === 0}
					<div class="flex flex-col items-center gap-2 px-2 py-8 text-center">
						<Paperclip class="h-6 w-6 text-muted-foreground/60" />
						<p class="text-xs text-muted-foreground">{t('myaaw.groups.attachments_empty')}</p>
					</div>
				{:else}
					<div class="space-y-1.5">
						<p
							class="px-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
						>
							{t('myaaw.groups.tab_attachments')} ({attachmentTotal})
						</p>
						{#each attachmentItems as item (item.id)}
							{@const KindIcon = attachmentIcon(item.kind)}
							<div
								class="group/att flex items-center gap-2 rounded-xl border bg-background/60 p-2 transition-colors hover:border-foreground/25 hover:bg-muted/50"
							>
								<button
									type="button"
									class="flex min-w-0 flex-1 items-center gap-2.5 text-left"
									onclick={() => onOpenAttachment?.(item)}
								>
									{#if item.kind === 'image'}
										<img
											src={groupAttachmentUrl(item)}
											alt={item.file_name}
											loading="lazy"
											class="h-10 w-10 shrink-0 rounded-lg border object-cover"
										/>
									{:else}
										<span
											class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border bg-muted/60 text-muted-foreground"
										>
											<KindIcon class="h-4 w-4" />
										</span>
									{/if}
									<div class="min-w-0 flex-1">
										<p class="truncate text-xs font-medium text-foreground">{item.file_name}</p>
										<p class="flex items-center gap-1.5 text-[10px] text-muted-foreground">
											{#if item.size}
												<span>{formatSize(item.size)}</span>
												<span>·</span>
											{/if}
											{#if item.created_at}
												<ChatTime at={item.created_at} class="text-[10px]" />
											{/if}
										</p>
									</div>
								</button>
								{#if onShowAttachmentInChat}
									<button
										type="button"
										onclick={() => onShowAttachmentInChat(item)}
										title={t('myaaw.chat.attachments_show_in_chat')}
										aria-label={t('myaaw.chat.attachments_show_in_chat')}
										class="grid h-7 w-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
									>
										<MessageSquare class="h-3.5 w-3.5" />
									</button>
								{/if}
							</div>
						{/each}
					</div>
					{#if attachmentHasMore}
						<div class="pt-1 text-center">
							<Button
								size="sm"
								variant="ghost"
								class="h-7 text-xs"
								disabled={attachmentLoadingMore}
								onclick={() => void loadAttachments(false)}
							>
								{#if attachmentLoadingMore}
									<Loader2 class="mr-1.5 h-3 w-3 animate-spin" />
								{/if}
								{t('common.load_more')}
							</Button>
						</div>
					{/if}
				{/if}
			</div>
		{:else}
			<form onsubmit={saveSettings} autocomplete="off" class="space-y-3.5 pt-1">
				<div class="space-y-1.5">
					<Label for="inspector-group-name" class="text-xs">
						{t('myaaw.groups.name_label')}
					</Label>
					<Input
						id="inspector-group-name"
						bind:value={editName}
						maxlength={100}
						required
						autocomplete="off"
						class="h-8 text-xs"
					/>
				</div>

				<div class="space-y-1.5">
					<Label for="inspector-group-topic" class="text-xs">
						{t('myaaw.groups.topic_label')}
					</Label>
					<textarea
						id="inspector-group-topic"
						bind:value={editDescription}
						maxlength={2000}
						rows={3}
						placeholder={t('myaaw.groups.topic_placeholder')}
						class="max-h-64 min-h-20 w-full resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-xs shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
					></textarea>
				</div>

				<div class="space-y-1.5">
					<Label class="text-xs">{t('myaaw.groups.mode_label')}</Label>
					<div class="space-y-1.5">
						{#each modeOptions as opt (opt.id)}
							{@const active = editMode === opt.id}
							<button
								type="button"
								onclick={() => (editMode = opt.id)}
								aria-pressed={active}
								class={[
									'flex w-full flex-col rounded-xl border px-2.5 py-2 text-left transition-colors',
									active
										? 'border-primary bg-primary/8 text-foreground'
										: 'border-border hover:bg-muted/60'
								]}
							>
								<span class="text-xs font-semibold">{t(opt.labelKey)}</span>
								<span class="mt-0.5 text-[11px] leading-snug text-muted-foreground">
									{t(opt.descKey)}
								</span>
							</button>
						{/each}
					</div>
				</div>

				<div class="flex items-center justify-between gap-2 rounded-xl border px-2.5 py-2">
					<div class="min-w-0">
						<Label for="inspector-max-turns" class="text-xs">
							{t('myaaw.groups.max_turns_label')}
						</Label>
						<p class="text-[10px] text-muted-foreground">1–50</p>
					</div>
					<Input
						id="inspector-max-turns"
						type="number"
						min={1}
						max={50}
						bind:value={editMaxTurns}
						class="h-8 w-16 text-center text-xs tabular-nums"
					/>
				</div>

				<div class="space-y-2.5 rounded-xl border px-2.5 py-2.5">
					<div class="flex items-center justify-between gap-2.5">
						<div class="min-w-0">
							<p class="text-xs font-medium">{t('myaaw.groups.hide_thinking_label')}</p>
							<p class="text-[10px] leading-snug text-muted-foreground">
								{t('myaaw.groups.hide_thinking_hint')}
							</p>
						</div>
						<Switch
							checked={Boolean(group.hide_thinking)}
							label={t('myaaw.groups.hide_thinking_label')}
							disabled={savingDisplay}
							onchange={(next) => void toggleDisplayFlag('hide_thinking', next)}
						/>
					</div>
					<div class="h-px bg-border/60"></div>
					<div class="flex items-center justify-between gap-2.5">
						<div class="min-w-0">
							<p class="text-xs font-medium">{t('myaaw.groups.hide_tools_label')}</p>
							<p class="text-[10px] leading-snug text-muted-foreground">
								{t('myaaw.groups.hide_tools_hint')}
							</p>
						</div>
						<Switch
							checked={Boolean(group.hide_tools)}
							label={t('myaaw.groups.hide_tools_label')}
							disabled={savingDisplay}
							onchange={(next) => void toggleDisplayFlag('hide_tools', next)}
						/>
					</div>
				</div>

				<Button type="submit" size="sm" class="w-full" disabled={!canSaveSettings}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else}
						<Save class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{t('common.save')}
				</Button>
			</form>

			<div class="mt-5 space-y-2 border-t pt-4">
				<p class="px-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
					{t('myaaw.groups.danger_zone')}
				</p>
				<Button
					type="button"
					size="sm"
					variant="outline"
					class="w-full justify-start gap-2 text-xs"
					disabled={clearing}
					onclick={() => void handleClearChat()}
				>
					{#if clearing}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<Eraser class="h-3.5 w-3.5" />
					{/if}
					{t('myaaw.groups.clear_action')}
				</Button>
				<Button
					type="button"
					size="sm"
					variant="ghost"
					class="w-full justify-start gap-2 text-xs text-destructive hover:bg-destructive/10 hover:text-destructive"
					disabled={deleting}
					onclick={() => void handleDeleteGroup()}
				>
					{#if deleting}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<Trash2 class="h-3.5 w-3.5" />
					{/if}
					{t('myaaw.groups.delete_action')}
				</Button>
			</div>
		{/if}
	</div>
</div>
