<script lang="ts">
	import { untrack } from 'svelte';
	import {
		Bot,
		Brain,
		CalendarClock,
		ChevronDown,
		ChevronRight,
		Clock,
		FileText,
		Fingerprint,
		FolderGit2,
		HeartPulse,
		History,
		Layers,
		Play,
		Puzzle,
		RotateCw,
		ScrollText,
		Server,
		ShieldCheck,
		Sparkles,
		Square,
		UserRound,
		Wrench,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import ConversationList from '$lib/components/app/ConversationList.svelte';
	import { ProjectHistoryList } from '$lib/components/workspace';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import { contextStore } from '$lib/stores/context.svelte';
	import { cronStore } from '$lib/stores/cron.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ContextFile, ConversationProject, CronJob } from '$lib/types';

	type Tab = 'history' | 'projects' | 'context' | 'control';
	type Status = 'connected' | 'stopped' | 'unknown';

	interface Props {
		/** The selected bot this panel inspects. */
		agent: string;
		/** The open conversation, highlighted in the history list. */
		activeId?: string;
		/** Currently attached working directory, highlighted in the Projects tab. */
		activeWorkingDir?: string;
		/** Incremented when conversations/projects mutate so the Projects tab stays fresh. */
		projectsRefreshKey?: number;
		/** Tab the panel opens on. */
		tab?: Tab;
		onOpen: (id: string) => void;
		onNew?: () => void;
		onNewInFolder?: (folderId: string) => void;
		onNewInProject?: (project: ConversationProject) => void;
		onAttachProject?: () => void;
		onDeleted?: (id: string) => void;
		onRenamed?: (id: string, title: string) => void;
		onClose: () => void;
		/** Opens the inline editor for one context file. */
		onOpenContext: (path: string) => void;
	}

	let {
		agent,
		activeId = '',
		activeWorkingDir = '',
		projectsRefreshKey = 0,
		tab = $bindable<Tab>('history'),
		onOpen,
		onNew,
		onNewInFolder,
		onNewInProject,
		onAttachProject,
		onDeleted,
		onRenamed,
		onClose,
		onOpenContext
	}: Props = $props();

	const { t } = useTranslation();

	const CONTEXT_SOUL = 'SOUL.md';
	const CONTEXT_MEMORY = 'MEMORY.md';
	const DATED_MEMORY = /^\d{4}-\d{2}-\d{2}\.md$/;

	let status = $state<Status>('unknown');
	let files = $state<Record<string, ContextFile>>({});
	let schedules = $state<CronJob[]>([]);
	let memoryOpen = $state(false);
	let controlling = $state(false);

	const soul = $derived(files[CONTEXT_SOUL]);
	const memory = $derived(files[CONTEXT_MEMORY]);
	const contextFiles = $derived(
		Object.values(files)
			.filter(
				(file) =>
					file.path !== CONTEXT_SOUL &&
					file.path !== CONTEXT_MEMORY &&
					!(file.folder === 'memory' && DATED_MEMORY.test(file.name))
			)
			.sort((left, right) => {
				const depth = Number(left.folder !== 'root') - Number(right.folder !== 'root');
				return depth !== 0 ? depth : left.path.localeCompare(right.path);
			})
	);
	const datedMemory = $derived(
		Object.values(files)
			.filter((file) => file.folder === 'memory' && DATED_MEMORY.test(file.name))
			.sort((left, right) => right.name.localeCompare(left.name))
	);
	const initial = $derived((agent || '?').slice(0, 1).toUpperCase());
	const agentColor = $derived(
		colorOf(agentSelection.agents.find((entry) => entry.name === agent)?.agent_color)
	);

	// Reload status, context, and schedules whenever the selected bot changes.
	let loadedAgent = '';
	$effect(() => {
		const target = agent;
		if (!target || target === loadedAgent) return;
		loadedAgent = target;
		untrack(() => {
			void loadStatus(target);
			void loadContext(target);
			void loadSchedules(target);
		});
	});

	// Keep the right panel status in step whenever the bot is started or stopped
	// from the left sidebar or command palette.
	$effect(() => {
		const known = agentSelection.agents.find((entry) => entry.name === agent);
		if (!known) return;
		status = known.running ? 'connected' : 'stopped';
	});

	async function loadStatus(target: string) {
		const known = agentSelection.agents.find((entry) => entry.name === target);
		if (known) status = known.running ? 'connected' : 'stopped';
		try {
			const res = await serverStore.fetchStatus(target);
			if (agent !== target) return;
			status = res.running ? 'connected' : 'stopped';
		} catch {
			if (agent === target && !known) status = 'unknown';
		}
	}

	async function loadContext(target: string) {
		try {
			const res = await contextStore.fetchList(target);
			if (agent !== target) return;
			const next: Record<string, ContextFile> = {};
			for (const file of res.files ?? []) next[file.path] = file;
			files = next;
		} catch {
			if (agent === target) files = {};
		}
	}

	async function loadSchedules(target: string) {
		try {
			await cronStore.ensure(target);
			if (agent !== target) return;
			schedules = cronStore.list.peek({ agent: target })?.jobs ?? [];
		} catch {
			if (agent === target) schedules = [];
		}
	}

	async function control(kind: 'start' | 'restart' | 'stop') {
		if (!agent || controlling) return;
		const target = agent;
		controlling = true;
		const running = kind !== 'stop';
		agentSelection.setRunning(target, running);
		status = running ? 'connected' : 'stopped';
		try {
			if (kind === 'start') await serverStore.start(target);
			else if (kind === 'restart') await serverStore.restart(target);
			else await serverStore.stop(target);
			agentSelection.setRunning(target, running);
			status = running ? 'connected' : 'stopped';
			toastStore.success(
				t(
					kind === 'start'
						? 'myaaw.server.start_done'
						: kind === 'restart'
							? 'myaaw.server.restart_done'
							: 'myaaw.server.stop_done'
				)
			);
			setTimeout(() => {
				if (agent === target) {
					void loadStatus(target);
					if (running) {
						conversationHistory.refresh();
						void loadContext(target);
						void loadSchedules(target);
					}
				}
			}, 1600);
		} catch (err) {
			agentSelection.clearOptimisticRunning(target);
			await agentSelection.load(true);
			await loadStatus(target);
			toastStore.error(getErrorMessage(err));
		} finally {
			controlling = false;
		}
	}

	const tabIndex = $derived(
		tab === 'history' ? 0 : tab === 'projects' ? 1 : tab === 'context' ? 2 : 3
	);

	const statusLabel = $derived(
		status === 'connected'
			? t('myaaw.focus.status_connected')
			: status === 'stopped'
				? t('myaaw.focus.status_stopped')
				: t('myaaw.focus.status_unknown')
	);

	function formatSize(bytes: number) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function fileMeta(file: ContextFile | undefined) {
		if (!file) return t('myaaw.focus.context_empty');
		return formatSize(file.size);
	}

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

<!--
	Dedicated right panel for Bots mode on the chat page: mirrors Focus mode's
	clean hero + three-tab structure (History, Context, Bot controls & routines)
	so clicking any bot in the left roster opens its full workspace sidebar.
-->
<div class="flex h-full min-h-0 flex-col">
	<div class="relative flex shrink-0 flex-col items-center gap-1 px-4 pt-4 pb-2 lg:pt-5">
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

	<div role="tablist" class="relative mx-3 mb-2 grid shrink-0 grid-cols-4 rounded-lg border p-0.5">
		<span
			aria-hidden="true"
			class="pointer-events-none absolute inset-y-0.5 left-0.5 w-[calc((100%-0.25rem)/4)] rounded-md bg-primary transition-transform duration-200 ease-out will-change-transform"
			style:transform={`translateX(${tabIndex * 100}%)`}
		></span>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'history'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-[11px] font-medium select-none transition-colors duration-150',
				tab === 'history'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'history')}
		>
			<History class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_history')}</span>
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'projects'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-[11px] font-medium select-none transition-colors duration-150',
				tab === 'projects'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'projects')}
		>
			<FolderGit2 class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_projects')}</span>
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'context'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-[11px] font-medium select-none transition-colors duration-150',
				tab === 'context'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'context')}
		>
			<Brain class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_context')}</span>
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'control'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-[11px] font-medium select-none transition-colors duration-150',
				tab === 'control'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (tab = 'control')}
		>
			<Server class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_control')}</span>
		</button>
	</div>

	{#if tab === 'history'}
		<div class="min-h-0 flex-1 overflow-hidden">
			<ConversationList
				{activeId}
				{onOpen}
				{onNew}
				{onNewInFolder}
				{onDeleted}
				{onRenamed}
				groupRecentsByDate
			/>
		</div>
	{:else if tab === 'projects'}
		<div class="min-h-0 flex-1 overflow-hidden">
			<ProjectHistoryList
				{agent}
				{activeId}
				{activeWorkingDir}
				refreshKey={projectsRefreshKey}
				{onOpen}
				{onNewInProject}
				{onAttachProject}
				{onDeleted}
				{onRenamed}
			/>
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
			{#if tab === 'context'}
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
				<div class="space-y-3 pt-1">
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

					<!-- Routines / Schedules overview (inspired by Grok Bots right panel). -->
					<div class="space-y-1.5">
						<div class="flex items-center justify-between px-1">
							<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.bots.routines')}
							</p>
							<a
								href={`/app/schedule?agent=${encodeURIComponent(agent)}`}
								class="text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
							>
								{t('myaaw.bots.manage')}
							</a>
						</div>
						{#if schedules.length === 0}
							<a
								href={`/app/schedule?agent=${encodeURIComponent(agent)}`}
								class="flex items-center gap-2.5 rounded-xl border border-dashed p-3 text-xs text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
							>
								<Clock class="h-4 w-4 shrink-0" />
								<span>{t('myaaw.bots.routines_empty')}</span>
							</a>
						{:else}
							<div class="space-y-1">
								{#each schedules.slice(0, 5) as job (job.id)}
									<a
										href={`/app/schedule?agent=${encodeURIComponent(agent)}`}
										class="flex items-start gap-2.5 rounded-xl border p-2.5 transition-colors hover:bg-muted/60"
									>
										<span
											class={[
												'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full',
												job.enabled
													? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400'
													: 'bg-muted text-muted-foreground'
											]}
										>
											<Clock class="h-3.5 w-3.5" />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-xs font-medium">{job.name}</span>
											<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
												{job.enabled ? job.schedule.expr : t('myaaw.bots.routine_paused')}
											</span>
										</span>
									</a>
								{/each}
							</div>
						{/if}
					</div>

					<!-- Quick shortcuts for this bot's artifacts and plugins. -->
					<div class="grid grid-cols-2 gap-2">
						<a
							href={`/app/artifacts?agent=${encodeURIComponent(agent)}`}
							class="flex items-center gap-2 rounded-xl border p-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
						>
							<Layers class="h-4 w-4 shrink-0" />
							<span class="truncate">{t('myaaw.nav.artifacts')}</span>
						</a>
						<a
							href={`/app/plugins?agent=${encodeURIComponent(agent)}`}
							class="flex items-center gap-2 rounded-xl border p-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
						>
							<Puzzle class="h-4 w-4 shrink-0" />
							<span class="truncate">{t('myaaw.plugins.title')}</span>
						</a>
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>
