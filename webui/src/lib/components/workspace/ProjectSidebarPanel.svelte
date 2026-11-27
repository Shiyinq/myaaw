<script lang="ts">
	import { untrack } from 'svelte';
	import {
		FolderGit2,
		FolderOpen,
		GitBranch,
		RefreshCw,
		SquareTerminal,
		Unlink,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { filesStore } from '$lib/stores/files.svelte';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import ProjectFileTree from './ProjectFileTree.svelte';
	import ProjectSourceControl from './ProjectSourceControl.svelte';
	import type {
		FileEntry,
		GitStatusResponse,
		ProjectDiffTarget,
		ProjectMultiDiffTarget
	} from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		workingDir: string;
		workingDirExists?: boolean;
		hasGit?: boolean;
		/** Incremented after an agent turn or file save so files & git status refresh. */
		refreshKey?: number;
		terminalOpen?: boolean;
		activeFilePath?: string;
		onOpenFile: (virtualPath: string) => void;
		onOpenDiff: (req: ProjectDiffTarget) => void;
		onOpenMultiDiff: (req: ProjectMultiDiffTarget) => void;
		onToggleTerminal: () => void;
		onChangeWorkingDir: () => void;
		onDetachWorkingDir?: () => void;
		onFileRenamed?: (fromPath: string, toPath: string) => void;
		onFileDeleted?: (deletedPath: string) => void;
		onClose?: () => void;
		embedded?: boolean;
	}

	let {
		agent,
		conversationId,
		workingDir,
		workingDirExists = true,
		hasGit = false,
		refreshKey = 0,
		terminalOpen = false,
		activeFilePath = '',
		onOpenFile,
		onOpenDiff,
		onOpenMultiDiff,
		onToggleTerminal,
		onChangeWorkingDir,
		onDetachWorkingDir,
		onFileRenamed,
		onFileDeleted,
		onClose,
		embedded = false
	}: Props = $props();

	const { t } = useTranslation();

	let subTab = $state<'files' | 'git'>('files');
	let expanded = $state<Record<string, boolean>>({ '/project': true });
	let childrenMap = $state<Record<string, FileEntry[]>>({});
	let loadingDir = $state<Record<string, boolean>>({});
	let filesError = $state('');

	let gitStatus = $state<GitStatusResponse | null>(null);
	let loadingGit = $state(false);
	let gitError = $state('');
	let localRefreshSeq = $state(0);

	let loadedKey = '';

	const folderName = $derived(
		workingDir ? workingDir.replace(/\/+$/, '').split('/').pop() || workingDir : ''
	);
	const effectiveHasGit = $derived(hasGit || Boolean(gitStatus?.has_git));
	const gitChangeCount = $derived(
		(gitStatus?.staged?.length ?? 0) +
			(gitStatus?.unstaged?.length ?? 0) +
			(gitStatus?.untracked?.length ?? 0)
	);

	$effect(() => {
		const key = `${agent}:${conversationId}:${workingDir}:${refreshKey}`;
		if (!agent || !conversationId || !workingDir) {
			loadedKey = '';
			childrenMap = {};
			gitStatus = null;
			return;
		}
		if (key === loadedKey) return;
		loadedKey = key;
		untrack(() => {
			void refreshAll();
		});
	});

	async function refreshAll() {
		if (!agent || !conversationId || !workingDir) return;
		const dirsToReload = Object.entries(expanded)
			.filter(([, open]) => open)
			.map(([dir]) => dir);
		if (!dirsToReload.includes('/project')) dirsToReload.unshift('/project');
		await Promise.all([...dirsToReload.map((dir) => loadDirectory(dir)), loadGit()]);
		localRefreshSeq += 1;
	}

	async function loadDirectory(dirPath: string) {
		if (!agent || !conversationId) return;
		loadingDir = { ...loadingDir, [dirPath]: true };
		filesError = '';
		try {
			const res = await filesStore.listDir(agent, dirPath, conversationId);
			childrenMap = { ...childrenMap, [dirPath]: res.entries ?? [] };
		} catch (err) {
			if (dirPath === '/project') {
				filesError = getErrorMessage(err);
			}
		} finally {
			loadingDir = { ...loadingDir, [dirPath]: false };
		}
	}

	async function toggleDir(entry: FileEntry) {
		const next = !expanded[entry.path];
		expanded = { ...expanded, [entry.path]: next };
		if (next && !childrenMap[entry.path]) {
			await loadDirectory(entry.path);
		}
	}

	async function loadGit() {
		if (!agent || !conversationId || !workingDir) return;
		loadingGit = true;
		gitError = '';
		try {
			gitStatus = await workspaceStore.gitStatus(agent, conversationId);
		} catch (err) {
			gitError = getErrorMessage(err);
		} finally {
			loadingGit = false;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	{#if !embedded}
		<div class="flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2.5 lg:hidden">
			<div class="flex min-w-0 items-center gap-1.5">
				<FolderGit2 class="h-4 w-4 shrink-0 text-primary" />
				<span class="truncate text-sm font-semibold">{t('myaaw.workspace.tab_project')}</span>
			</div>
			{#if onClose}
				<Button variant="ghost" size="icon-sm" onclick={onClose} aria-label={t('common.close')}>
					<X class="h-4 w-4" />
				</Button>
			{/if}
		</div>
	{/if}

	{#if !workingDir}
		<div class="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
			<span class="grid h-11 w-11 place-items-center rounded-xl bg-muted text-muted-foreground">
				<FolderOpen class="h-5 w-5" />
			</span>
			<div class="space-y-1">
				<p class="text-sm font-medium">{t('myaaw.workspace.no_project_title')}</p>
				<p class="text-xs text-muted-foreground">{t('myaaw.workspace.no_project_desc')}</p>
			</div>
			<Button size="sm" variant="outline" onclick={onChangeWorkingDir}>
				<FolderOpen class="h-3.5 w-3.5" />
				{t('myaaw.workspace.attach_button')}
			</Button>
		</div>
	{:else}
		<div class="shrink-0 space-y-2 border-b px-3 py-2.5">
			<div class="flex items-start justify-between gap-2">
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-1.5">
						<p class="truncate text-xs font-semibold" title={workingDir}>{folderName}</p>
						{#if gitStatus?.has_git && gitStatus.branch}
							<Badge variant="secondary" class="h-4 shrink-0 gap-1 px-1.5 font-mono text-[10px]">
								<GitBranch class="h-2.5 w-2.5" />
								{gitStatus.branch}
							</Badge>
						{/if}
					</div>
					<p class="truncate font-mono text-[10px] text-muted-foreground" title={workingDir}>
						{workingDir}
					</p>
				</div>
				<div class="flex shrink-0 items-center gap-0.5">
					<Button
						variant={terminalOpen ? 'secondary' : 'ghost'}
						size="icon-sm"
						onclick={onToggleTerminal}
						title={t('myaaw.workspace.open_terminal')}
						aria-label={t('myaaw.workspace.open_terminal')}
					>
						<SquareTerminal class="h-3.5 w-3.5" />
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						onclick={() => void refreshAll()}
						title={t('common.refresh')}
						aria-label={t('common.refresh')}
					>
						<RefreshCw
							class={loadingDir['/project'] || loadingGit
								? 'h-3.5 w-3.5 animate-spin'
								: 'h-3.5 w-3.5'}
						/>
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						onclick={onChangeWorkingDir}
						title={t('myaaw.workspace.change_dir')}
						aria-label={t('myaaw.workspace.change_dir')}
					>
						<FolderOpen class="h-3.5 w-3.5" />
					</Button>
					{#if onDetachWorkingDir}
						<Button
							variant="ghost"
							size="icon-sm"
							onclick={onDetachWorkingDir}
							title={t('myaaw.workspace.detach')}
							aria-label={t('myaaw.workspace.detach')}
						>
							<Unlink class="h-3.5 w-3.5" />
						</Button>
					{/if}
				</div>
			</div>

			{#if !workingDirExists}
				<div
					class="rounded-md border border-destructive/30 bg-destructive/10 px-2 py-1 text-[11px] text-destructive"
				>
					{t('myaaw.workspace.dir_missing')}
				</div>
			{/if}

			{#if !conversationId}
				<p class="text-[11px] text-muted-foreground">{t('myaaw.workspace.pending_attach_hint')}</p>
			{:else}
				<div class="grid grid-cols-2 gap-1 rounded-md border bg-muted/30 p-0.5">
					<button
						type="button"
						class={[
							'rounded px-2 py-1 text-[11px] font-medium transition-colors',
							subTab === 'files'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						]}
						onclick={() => (subTab = 'files')}
					>
						{t('myaaw.workspace.subtab_files')}
					</button>
					<button
						type="button"
						class={[
							'flex items-center justify-center gap-1 rounded px-2 py-1 text-[11px] font-medium transition-colors',
							subTab === 'git'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						]}
						onclick={() => {
							subTab = 'git';
							void loadGit();
						}}
					>
						<span>{t('myaaw.workspace.subtab_git')}</span>
						{#if effectiveHasGit && gitChangeCount > 0}
							<span
								class="rounded-full bg-primary/15 px-1.5 text-[10px] font-semibold text-primary tabular-nums"
							>
								{gitChangeCount}
							</span>
						{/if}
					</button>
				</div>
			{/if}
		</div>

		{#if conversationId}
			{#if subTab === 'files'}
				<ProjectFileTree
					{agent}
					{conversationId}
					{childrenMap}
					{expanded}
					{loadingDir}
					{filesError}
					{gitStatus}
					{activeFilePath}
					onToggleDir={(entry) => void toggleDir(entry)}
					onCollapseAll={() => (expanded = { '/project': true })}
					{onOpenFile}
					onRefresh={refreshAll}
					{onFileRenamed}
					{onFileDeleted}
				/>
			{:else}
				<ProjectSourceControl
					{agent}
					{conversationId}
					{gitStatus}
					{loadingGit}
					{gitError}
					refreshKey={refreshKey + localRefreshSeq}
					{onOpenFile}
					{onOpenDiff}
					{onOpenMultiDiff}
					onMutated={refreshAll}
				/>
			{/if}
		{/if}
	{/if}
</div>
