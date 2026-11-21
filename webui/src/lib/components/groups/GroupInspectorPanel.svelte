<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		Eraser,
		Loader2,
		Play,
		Plus,
		Save,
		Settings2,
		Trash2,
		UserMinus,
		Users,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, serverStore, toastStore } from '$lib/stores';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatShortcut } from '$lib/utils/keys';
	import type { DiscussionMode, Group, GroupMember } from '$lib/types';

	type Tab = 'members' | 'settings';

	interface Props {
		group: Group;
		tab?: Tab;
		onClose: () => void;
		onUpdated: (group: Group) => void;
		onCleared: () => void;
	}

	let { group, tab = $bindable<Tab>('members'), onClose, onUpdated, onCleared }: Props = $props();

	const { t } = useTranslation();

	let editName = $state('');
	let editDescription = $state('');
	let editMode = $state<DiscussionMode>('autonomous');
	let editMaxTurns = $state(8);
	let saving = $state(false);
	let startingMap = $state<Record<string, boolean>>({});
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

	const tabIndex = $derived(tab === 'members' ? 0 : 1);

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

	async function startMember(agentName: string) {
		if (startingMap[agentName]) return;
		startingMap = { ...startingMap, [agentName]: true };
		agentSelection.setRunning(agentName, true);
		try {
			await serverStore.start(agentName);
			toastStore.success(t('myaaw.server.start_done'));
			setTimeout(() => {
				void agentSelection.load(true);
				void groupsStore
					.get(group.id)
					.then((detail) => onUpdated(detail.group))
					.catch(() => {});
			}, 1400);
		} catch (err) {
			agentSelection.clearOptimisticRunning(agentName);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			startingMap = { ...startingMap, [agentName]: false };
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

	<div role="tablist" class="relative mx-3 mb-2 grid shrink-0 grid-cols-2 rounded-lg border p-0.5">
		<span
			aria-hidden="true"
			class="pointer-events-none absolute inset-y-0.5 left-0.5 w-[calc((100%-0.25rem)/2)] rounded-md bg-primary transition-transform duration-200 ease-out will-change-transform"
			style:transform={`translateX(${tabIndex * 100}%)`}
		></span>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'members'}
			class={[
				'relative z-10 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium select-none transition-colors duration-150',
				tab === 'members'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'members')}
		>
			<Users class="h-3.5 w-3.5" />
			{t('myaaw.groups.tab_members')}
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'settings'}
			class={[
				'relative z-10 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium select-none transition-colors duration-150',
				tab === 'settings'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'settings')}
		>
			<Settings2 class="h-3.5 w-3.5" />
			{t('myaaw.groups.tab_settings')}
		</button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
		{#if tab === 'members'}
			<div class="space-y-3 pt-1">
				<div class="space-y-1.5">
					<p class="px-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
						{t('myaaw.groups.current_members')} ({group.members.length})
					</p>
					{#each group.members as member (member.name)}
						{@const running = isMemberRunning(member)}
						{@const starting = Boolean(startingMap[member.name])}
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
										disabled={starting}
										onclick={() => void startMember(member.name)}
									>
										{#if starting}
											<Loader2 class="h-3 w-3 animate-spin" />
										{:else}
											<Play class="h-3 w-3" />
										{/if}
										{t('myaaw.server.start')}
									</Button>
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
		{:else}
			<form onsubmit={saveSettings} class="space-y-3.5 pt-1">
				<div class="space-y-1.5">
					<Label for="inspector-group-name" class="text-xs">
						{t('myaaw.groups.name_label')}
					</Label>
					<Input
						id="inspector-group-name"
						bind:value={editName}
						maxlength={100}
						required
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
						class="w-full resize-none rounded-md border border-input bg-transparent px-2.5 py-2 text-xs shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
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
