<script lang="ts">
	import { browser } from '$app/environment';
	import { untrack } from 'svelte';
	import {
		Archive,
		ArrowDown,
		ArrowUp,
		Check,
		ChevronDown,
		ChevronRight,
		ChevronsDownUp,
		ChevronsUpDown,
		CloudDownload,
		File as FileIcon,
		FileCode2,
		FileDiff,
		Folder,
		FolderOpen,
		GitBranch,
		GitBranchPlus,
		GitCommitHorizontal,
		List,
		ListTree,
		Loader2,
		Minus,
		Plus,
		RotateCcw,
		Undo2
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Input } from '$lib/components/ui/input';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import {
		projectScopeKey,
		readGitUiState,
		writeGitUiState
	} from '$lib/stores/projectWorkspace.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import ProjectSidebarSkeleton from './ProjectSidebarSkeleton.svelte';
	import {
		buildChangeTreeRows,
		collectChangeTreeDirs,
		gitStatusLetter,
		gitStatusTextClass,
		splitFilePath,
		type GitChangeItem
	} from './gitUi';
	import type {
		GitBranchesResponse,
		GitCommitDetailResponse,
		GitCommitEntry,
		GitStashEntry,
		GitStatusResponse,
		ProjectDiffTarget,
		ProjectMultiDiffTarget
	} from '$lib/types';

	const VIEW_MODE_KEY = 'myaaw_git_changes_view_mode';
	const COMMIT_PAGE_SIZE = 25;

	interface Props {
		agent: string;
		conversationId: string;
		workingDir?: string;
		gitStatus: GitStatusResponse | null;

		gitError?: string;
		refreshKey?: number;
		onOpenFile: (virtualPath: string) => void;
		onOpenDiff: (req: ProjectDiffTarget) => void;
		onOpenMultiDiff: (req: ProjectMultiDiffTarget) => void;
		onMutated: () => Promise<void> | void;
	}

	let {
		agent,
		conversationId,
		workingDir = '',
		gitStatus,

		gitError = '',
		refreshKey = 0,
		onOpenFile,
		onOpenDiff,
		onOpenMultiDiff,
		onMutated
	}: Props = $props();

	const { t } = useTranslation();

	const gitScopeKey = $derived(
		workingDir
			? projectScopeKey(agent, workingDir)
			: agent && conversationId
				? `${agent}:${conversationId}`
				: ''
	);
	const initialGitUi = untrack(() => readGitUiState(gitScopeKey));

	// VS Code-style accordion layout: Changes open by default, Commits/Graph closed by default.
	let changesSectionOpen = $state(initialGitUi?.changesSectionOpen ?? true);
	let stagedSubOpen = $state(initialGitUi?.stagedSubOpen ?? true);
	let workingSubOpen = $state(initialGitUi?.workingSubOpen ?? true);
	let stashesSectionOpen = $state(initialGitUi?.stashesSectionOpen ?? false);
	let commitsSectionOpen = $state(initialGitUi?.commitsSectionOpen ?? false);

	// Tree vs List view mode for changes (persisted in localStorage).
	let viewMode = $state<'tree' | 'list'>(
		browser && localStorage.getItem(VIEW_MODE_KEY) === 'list' ? 'list' : 'tree'
	);
	let collapsedStagedDirs = $state<Record<string, boolean>>(
		initialGitUi ? { ...initialGitUi.collapsedStagedDirs } : {}
	);
	let collapsedWorkingDirs = $state<Record<string, boolean>>(
		initialGitUi ? { ...initialGitUi.collapsedWorkingDirs } : {}
	);
	let restoredGitScope = untrack(() => gitScopeKey);

	// Restore per-conversation expand/collapse state when switching agents or conversations.
	$effect(() => {
		const scope = gitScopeKey;
		if (!scope || scope === restoredGitScope) return;
		restoredGitScope = scope;
		untrack(() => {
			const saved = readGitUiState(scope);
			changesSectionOpen = saved?.changesSectionOpen ?? true;
			stagedSubOpen = saved?.stagedSubOpen ?? true;
			workingSubOpen = saved?.workingSubOpen ?? true;
			stashesSectionOpen = saved?.stashesSectionOpen ?? false;
			commitsSectionOpen = saved?.commitsSectionOpen ?? false;
			collapsedStagedDirs = saved ? { ...saved.collapsedStagedDirs } : {};
			collapsedWorkingDirs = saved ? { ...saved.collapsedWorkingDirs } : {};
			if (stashesSectionOpen) void loadStashes();
		});
	});

	// Persist expand/collapse state across tab switches and page refreshes.
	$effect(() => {
		const scope = gitScopeKey;
		if (!scope || scope !== restoredGitScope) return;
		const snapshot = {
			changesSectionOpen,
			stagedSubOpen,
			workingSubOpen,
			stashesSectionOpen,
			commitsSectionOpen,
			collapsedStagedDirs: { ...collapsedStagedDirs },
			collapsedWorkingDirs: { ...collapsedWorkingDirs }
		};
		untrack(() => {
			writeGitUiState(scope, snapshot);
		});
	});

	// If Stashes accordion was restored open on mount, fetch stashes right away.
	let loadedInitialStashes = false;
	$effect(() => {
		if (
			loadedInitialStashes ||
			!stashesSectionOpen ||
			!gitStatus?.has_git ||
			!agent ||
			!conversationId
		) {
			return;
		}
		loadedInitialStashes = true;
		untrack(() => {
			void loadStashes();
		});
	});

	let busyAction = $state('');
	let commitMessage = $state('');

	let branchesOpen = $state(false);
	let branchesData = $state<GitBranchesResponse | null>(null);
	let loadingBranches = $state(false);
	let newBranchName = $state('');
	let newBranchFrom = $state('');

	let stashes = $state<GitStashEntry[]>([]);
	let loadingStashes = $state(false);
	let stashMessage = $state('');
	let stashIncludeUntracked = $state(true);

	// Paginated commit history (infinite scroll).
	let commits = $state<GitCommitEntry[]>([]);
	let commitsHasMore = $state(false);
	let loadingCommits = $state(false);
	let loadingMoreCommits = $state(false);
	let commitsLoadedKey = '';
	let commitsScrollRoot = $state<HTMLElement | null>(null);
	let commitsSentinel = $state<HTMLElement | null>(null);

	$effect(() => {
		const sentinel = commitsSentinel;
		const root = commitsScrollRoot;
		if (!sentinel || !commitsSectionOpen || !commitsHasMore) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					void loadCommitsPage(false);
				}
			},
			{ root, rootMargin: '120px' }
		);
		observer.observe(sentinel);
		return () => observer.disconnect();
	});

	let expandedCommits = $state<Record<string, boolean>>({});
	let commitDetails = $state<Record<string, GitCommitDetailResponse>>({});
	let loadingCommit = $state<Record<string, boolean>>({});

	const stagedItems = $derived<GitChangeItem[]>(
		(gitStatus?.staged ?? []).map((entry) => ({
			...entry,
			staged: true,
			untracked: false
		}))
	);

	// Like VS Code, "Changes" combines unstaged modifications and untracked files.
	const workingItems = $derived<GitChangeItem[]>([
		...(gitStatus?.unstaged ?? []).map((entry) => ({
			...entry,
			staged: false,
			untracked: false
		})),
		...(gitStatus?.untracked ?? []).map((entry) => ({
			...entry,
			status: '?',
			staged: false,
			untracked: true
		}))
	]);

	const stagedTreeRows = $derived(
		viewMode === 'tree' ? buildChangeTreeRows(stagedItems, collapsedStagedDirs) : []
	);
	const workingTreeRows = $derived(
		viewMode === 'tree' ? buildChangeTreeRows(workingItems, collapsedWorkingDirs) : []
	);
	const stagedDirPaths = $derived(viewMode === 'tree' ? collectChangeTreeDirs(stagedItems) : []);
	const workingDirPaths = $derived(viewMode === 'tree' ? collectChangeTreeDirs(workingItems) : []);
	const hasTreeDirs = $derived(stagedDirPaths.length > 0 || workingDirPaths.length > 0);
	const allTreeDirsCollapsed = $derived(
		hasTreeDirs &&
			stagedDirPaths.every((dir) => Boolean(collapsedStagedDirs[dir])) &&
			workingDirPaths.every((dir) => Boolean(collapsedWorkingDirs[dir]))
	);

	function toggleCollapseAllTreeDirs() {
		if (!hasTreeDirs) return;
		if (allTreeDirsCollapsed) {
			collapsedStagedDirs = {};
			collapsedWorkingDirs = {};
		} else {
			const nextStaged: Record<string, boolean> = {};
			for (const dir of stagedDirPaths) nextStaged[dir] = true;
			const nextWorking: Record<string, boolean> = {};
			for (const dir of workingDirPaths) nextWorking[dir] = true;
			collapsedStagedDirs = nextStaged;
			collapsedWorkingDirs = nextWorking;
		}
	}

	type CommitMode = 'commit' | 'amend' | 'commit-push' | 'commit-sync';

	const hasAnyChanges = $derived(stagedItems.length > 0 || workingItems.length > 0);
	const canCommit = $derived(
		Boolean(commitMessage.trim()) && stagedItems.length > 0 && busyAction === ''
	);
	const canAmend = $derived(Boolean(commitMessage.trim()) && busyAction === '');
	const isCommitBusy = $derived(
		busyAction === 'commit' ||
			busyAction === 'amend' ||
			busyAction === 'commit-push' ||
			busyAction === 'commit-sync'
	);

	function toggleViewMode() {
		viewMode = viewMode === 'tree' ? 'list' : 'tree';
		if (browser) localStorage.setItem(VIEW_MODE_KEY, viewMode);
	}

	function isCodeFile(name: string): boolean {
		return /\.(ts|tsx|js|jsx|svelte|go|py|rs|java|kt|c|cpp|h|css|scss|html|json|yaml|yml|toml|sql|sh)$/i.test(
			name
		);
	}

	// Load / reset first commit page whenever commits accordion opens or repo changes.
	$effect(() => {
		const open = commitsSectionOpen;
		const hasGit = Boolean(gitStatus?.has_git);
		const key = `${agent}:${conversationId}:${gitStatus?.head_oid ?? ''}:${refreshKey}`;
		if (!hasGit || !agent || !conversationId) {
			commits = [];
			commitsHasMore = false;
			commitsLoadedKey = '';
			return;
		}
		if (!open || key === commitsLoadedKey) return;
		commitsLoadedKey = key;
		untrack(() => {
			void loadCommitsPage(true);
		});
	});

	async function loadCommitsPage(reset: boolean) {
		if (!agent || !conversationId) return;
		if (reset) {
			if (loadingCommits) return;
			loadingCommits = true;
		} else {
			if (loadingCommits || loadingMoreCommits || !commitsHasMore) return;
			loadingMoreCommits = true;
		}
		try {
			const offset = reset ? 0 : commits.length;
			const page = await workspaceStore.gitLog(agent, conversationId, COMMIT_PAGE_SIZE, offset);
			const incoming = page.commits ?? [];
			if (reset) {
				commits = incoming;
				commitsHasMore =
					typeof page.has_more === 'boolean' ? page.has_more : incoming.length >= COMMIT_PAGE_SIZE;
			} else {
				const seen = new Set(commits.map((c) => c.hash));
				const appended = incoming.filter((c) => !seen.has(c.hash));
				commits = [...commits, ...appended];
				// Stop paging if the server reports no more OR returned no new unseen commits.
				commitsHasMore =
					appended.length > 0 &&
					(typeof page.has_more === 'boolean'
						? page.has_more
						: incoming.length >= COMMIT_PAGE_SIZE);
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			loadingCommits = false;
			loadingMoreCommits = false;
		}
	}

	function onCommitsScroll() {
		const el = commitsScrollRoot;
		if (!el || !commitsHasMore || loadingCommits || loadingMoreCommits) return;
		if (el.scrollHeight - el.scrollTop - el.clientHeight < 140) {
			void loadCommitsPage(false);
		}
	}

	async function runAction(key: string, fn: () => Promise<void>) {
		if (busyAction) return;
		busyAction = key;
		try {
			await fn();
			await onMutated();
			if (stashesSectionOpen) void loadStashes();
			if (branchesOpen) void loadBranches();
			if (commitsSectionOpen) {
				commitsLoadedKey = '';
				void loadCommitsPage(true);
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			busyAction = '';
		}
	}

	async function loadBranches() {
		loadingBranches = true;
		try {
			branchesData = await workspaceStore.gitBranches(agent, conversationId);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			loadingBranches = false;
		}
	}

	async function loadStashes() {
		loadingStashes = true;
		try {
			const res = await workspaceStore.gitStashes(agent, conversationId);
			stashes = res.stashes ?? [];
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			loadingStashes = false;
		}
	}

	function toggleBranches() {
		branchesOpen = !branchesOpen;
		if (branchesOpen) void loadBranches();
	}

	function toggleStashes() {
		stashesSectionOpen = !stashesSectionOpen;
		if (stashesSectionOpen) void loadStashes();
	}

	async function handleCommit(mode: CommitMode = 'commit') {
		const useAmend = mode === 'amend';
		if (useAmend ? !canAmend : !canCommit) return;
		await runAction(mode, async () => {
			await workspaceStore.gitCommit(agent, conversationId, {
				message: commitMessage.trim(),
				amend: useAmend
			});
			commitMessage = '';
			if (mode === 'commit-push') {
				await workspaceStore.gitPush(agent, conversationId, !gitStatus?.upstream);
				toastStore.success(t('myaaw.workspace.git_committed_pushed'));
			} else if (mode === 'commit-sync') {
				if (gitStatus?.upstream) {
					await workspaceStore.gitPull(agent, conversationId);
				}
				await workspaceStore.gitPush(agent, conversationId, !gitStatus?.upstream);
				toastStore.success(t('myaaw.workspace.git_committed_synced'));
			} else {
				toastStore.success(t('myaaw.workspace.git_committed'));
			}
		});
	}

	async function handleDiscardPath(path: string) {
		const confirmed = await askConfirm({
			title: t('myaaw.workspace.git_discard_confirm_title'),
			message: t('myaaw.workspace.git_discard_confirm_file', { path }),
			confirmText: t('myaaw.workspace.git_discard'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		await runAction(`discard:${path}`, async () => {
			await workspaceStore.gitDiscard(agent, conversationId, { paths: [path] });
			toastStore.success(t('myaaw.workspace.git_discarded'));
		});
	}

	async function handleDiscardAll() {
		const confirmed = await askConfirm({
			title: t('myaaw.workspace.git_discard_confirm_title'),
			message: t('myaaw.workspace.git_discard_confirm_all'),
			confirmText: t('myaaw.workspace.git_discard_all'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		await runAction('discard-all', async () => {
			await workspaceStore.gitDiscard(agent, conversationId, {
				all: true,
				include_untracked: true
			});
			toastStore.success(t('myaaw.workspace.git_discarded'));
		});
	}

	async function handleCheckout(branch: string, create = false, from = '') {
		const target = branch.trim();
		if (!target) return;
		const baseRef = create ? from.trim() : '';
		await runAction(`checkout:${target}`, async () => {
			await workspaceStore.gitCheckout(agent, conversationId, {
				branch: target,
				create,
				from: baseRef || undefined
			});
			newBranchName = '';
			newBranchFrom = '';
			branchesOpen = false;
			toastStore.success(t('myaaw.workspace.git_branch_switched', { branch: target }));
		});
	}

	async function handleDropStash(ref: string) {
		const confirmed = await askConfirm({
			title: t('myaaw.workspace.git_stash_drop'),
			message: t('myaaw.workspace.git_stash_drop_confirm', { ref }),
			confirmText: t('myaaw.workspace.git_stash_drop'),
			variant: 'destructive'
		});
		if (!confirmed) return;
		await runAction(`stash-drop:${ref}`, async () => {
			await workspaceStore.gitStashDrop(agent, conversationId, ref);
			toastStore.success(t('myaaw.workspace.git_stash_dropped'));
		});
	}

	async function toggleCommitDetail(hash: string) {
		const next = !expandedCommits[hash];
		expandedCommits = { ...expandedCommits, [hash]: next };
		if (next && !commitDetails[hash] && !loadingCommit[hash]) {
			loadingCommit = { ...loadingCommit, [hash]: true };
			try {
				const detail = await workspaceStore.gitCommitDetail(agent, conversationId, hash);
				commitDetails = { ...commitDetails, [hash]: detail };
			} catch (err) {
				toastStore.error(getErrorMessage(err));
			} finally {
				loadingCommit = { ...loadingCommit, [hash]: false };
			}
		}
	}

	function shortDate(iso: string): string {
		if (!iso) return '';
		try {
			const d = new Date(iso);
			return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
		} catch {
			return '';
		}
	}
</script>

{#snippet changeRow(item: GitChangeItem, depth: number, showDirSuffix: boolean)}
	{@const parts = splitFilePath(item.path)}
	<div
		class="group/git relative flex items-center rounded-md pr-1 transition-colors hover:bg-muted/70"
	>
		<button
			type="button"
			class="flex min-w-0 flex-1 items-center gap-2 py-1.5 text-left text-[13px] leading-tight"
			style:padding-left="{6 + depth * 12}px"
			onclick={() =>
				onOpenDiff({
					path: item.path,
					staged: item.staged,
					untracked: item.untracked
				})}
			title={item.path}
		>
			{#if isCodeFile(parts.name)}
				<FileCode2 class="h-4 w-4 shrink-0 text-muted-foreground" />
			{:else}
				<FileIcon class="h-4 w-4 shrink-0 text-muted-foreground" />
			{/if}
			{#if showDirSuffix && parts.dir}
				<span class="max-w-[70%] shrink-0 truncate text-[13px]">{parts.name}</span>
				<span class="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground/75">
					{parts.dir}
				</span>
			{:else}
				<span class="min-w-0 flex-1 truncate text-[13px]">{parts.name}</span>
			{/if}
		</button>

		<div
			class="pointer-events-none absolute top-1/2 right-6 flex -translate-y-1/2 items-center gap-0.5 rounded-md bg-muted/95 px-0.5 opacity-0 shadow-2xs transition-opacity group-focus-within/git:pointer-events-auto group-focus-within/git:opacity-100 group-hover/git:pointer-events-auto group-hover/git:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:static [@media(hover:none)]:translate-y-0 [@media(hover:none)]:bg-transparent [@media(hover:none)]:px-0 [@media(hover:none)]:opacity-100 [@media(hover:none)]:shadow-none"
		>
			{#if item.status !== 'D'}
				<button
					type="button"
					class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground"
					onclick={() => onOpenFile(`/project/${item.path.replace(/^\/+/, '')}`)}
					title={t('myaaw.workspace.open_file')}
					aria-label={t('myaaw.workspace.open_file')}
				>
					<FileCode2 class="h-3 w-3" />
				</button>
			{/if}
			{#if item.staged}
				<button
					type="button"
					disabled={busyAction !== ''}
					class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground disabled:opacity-50"
					onclick={() =>
						void runAction(`unstage:${item.path}`, async () => {
							await workspaceStore.gitUnstage(agent, conversationId, { paths: [item.path] });
						})}
					title={t('myaaw.workspace.git_unstage')}
					aria-label={t('myaaw.workspace.git_unstage')}
				>
					<Minus class="h-3 w-3" />
				</button>
			{:else}
				<button
					type="button"
					disabled={busyAction !== ''}
					class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
					onclick={() => void handleDiscardPath(item.path)}
					title={t('myaaw.workspace.git_discard')}
					aria-label={t('myaaw.workspace.git_discard')}
				>
					<Undo2 class="h-3 w-3" />
				</button>
				<button
					type="button"
					disabled={busyAction !== ''}
					class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground disabled:opacity-50"
					onclick={() =>
						void runAction(`stage:${item.path}`, async () => {
							await workspaceStore.gitStage(agent, conversationId, { paths: [item.path] });
						})}
					title={t('myaaw.workspace.git_stage')}
					aria-label={t('myaaw.workspace.git_stage')}
				>
					<Plus class="h-3 w-3" />
				</button>
			{/if}
		</div>

		<span
			class={[
				'ml-1 w-4 shrink-0 text-center font-mono text-[10px] font-bold',
				gitStatusTextClass(item.status)
			]}
		>
			{gitStatusLetter(item.status)}
		</span>
	</div>
{/snippet}

{#if gitError}
	<div class="p-3 text-xs text-destructive">{gitError}</div>
{/if}
{#if !gitStatus}
	{#if !gitError}
		<ProjectSidebarSkeleton variant="git" />
	{/if}
{:else if !gitStatus.has_git}
	<div class="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
		<GitBranch class="h-6 w-6 text-muted-foreground" />
		<p class="text-xs font-medium">{t('myaaw.workspace.not_git_title')}</p>
		<p class="text-[11px] text-muted-foreground">{t('myaaw.workspace.not_git_desc')}</p>
	</div>
{:else}
	<div class="flex min-h-0 flex-1 flex-col">
		<!-- TOP PANE: Changes (open by default, VS Code Source Control layout) -->
		<section class={['flex min-h-0 flex-col', changesSectionOpen ? 'flex-1' : 'shrink-0']}>
			<div class="flex shrink-0 items-center justify-between gap-1 border-b bg-muted/25 px-2 py-1">
				<button
					type="button"
					class="flex min-w-0 items-center gap-1 rounded px-1 py-0.5 text-[11px] font-semibold tracking-wide text-foreground uppercase hover:bg-muted/70"
					onclick={() => (changesSectionOpen = !changesSectionOpen)}
					aria-expanded={changesSectionOpen}
				>
					{#if changesSectionOpen}
						<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else}
						<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{/if}
					<span>{t('myaaw.workspace.git_changes')}</span>
				</button>

				<div class="flex shrink-0 items-center gap-0.5">
					{#if viewMode === 'tree' && hasTreeDirs}
						<button
							type="button"
							class="grid h-6 w-6 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
							onclick={toggleCollapseAllTreeDirs}
							title={allTreeDirsCollapsed
								? t('myaaw.workspace.git_expand_all')
								: t('myaaw.workspace.git_collapse_all')}
							aria-label={allTreeDirsCollapsed
								? t('myaaw.workspace.git_expand_all')
								: t('myaaw.workspace.git_collapse_all')}
						>
							{#if allTreeDirsCollapsed}
								<ChevronsUpDown class="h-3.5 w-3.5" />
							{:else}
								<ChevronsDownUp class="h-3.5 w-3.5" />
							{/if}
						</button>
					{/if}
					<button
						type="button"
						class="grid h-6 w-6 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
						onclick={toggleViewMode}
						title={viewMode === 'tree'
							? t('myaaw.workspace.git_view_list')
							: t('myaaw.workspace.git_view_tree')}
						aria-label={viewMode === 'tree'
							? t('myaaw.workspace.git_view_list')
							: t('myaaw.workspace.git_view_tree')}
					>
						{#if viewMode === 'tree'}
							<ListTree class="h-3.5 w-3.5" />
						{:else}
							<List class="h-3.5 w-3.5" />
						{/if}
					</button>
					<button
						type="button"
						disabled={busyAction !== ''}
						class="grid h-6 w-6 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
						onclick={() =>
							void runAction('fetch', async () => {
								await workspaceStore.gitFetch(agent, conversationId);
								toastStore.success(t('myaaw.workspace.git_fetched'));
							})}
						title={t('myaaw.workspace.git_fetch')}
						aria-label={t('myaaw.workspace.git_fetch')}
					>
						{#if busyAction === 'fetch'}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else}
							<CloudDownload class="h-3.5 w-3.5" />
						{/if}
					</button>
					<button
						type="button"
						disabled={busyAction !== ''}
						class="grid h-6 w-6 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
						onclick={() =>
							void runAction('pull', async () => {
								await workspaceStore.gitPull(agent, conversationId);
								toastStore.success(t('myaaw.workspace.git_pulled'));
							})}
						title={t('myaaw.workspace.git_pull')}
						aria-label={t('myaaw.workspace.git_pull')}
					>
						{#if busyAction === 'pull'}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else}
							<ArrowDown class="h-3.5 w-3.5" />
						{/if}
					</button>
					<button
						type="button"
						disabled={busyAction !== ''}
						class="grid h-6 w-6 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
						onclick={() =>
							void runAction('push', async () => {
								await workspaceStore.gitPush(agent, conversationId, !gitStatus?.upstream);
								toastStore.success(t('myaaw.workspace.git_pushed'));
							})}
						title={t('myaaw.workspace.git_push')}
						aria-label={t('myaaw.workspace.git_push')}
					>
						{#if busyAction === 'push'}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else}
							<ArrowUp class="h-3.5 w-3.5" />
						{/if}
					</button>
				</div>
			</div>

			{#if changesSectionOpen}
				<div class="min-h-0 flex-1 overflow-y-auto px-2.5 py-2">
					<div class="space-y-2.5">
						<!-- Branch Switcher Row -->
						<div class="space-y-1.5">
							<button
								type="button"
								class="flex w-full items-center justify-between gap-1.5 rounded-md border bg-muted/20 px-2 py-1 text-left transition-colors hover:bg-muted/50"
								onclick={toggleBranches}
								title={t('myaaw.workspace.git_branches')}
							>
								<span class="flex min-w-0 items-center gap-1.5">
									<GitBranch class="h-3.5 w-3.5 shrink-0 text-primary" />
									<span class="truncate font-mono text-xs font-medium">
										{gitStatus.branch || 'HEAD'}
									</span>
								</span>
								<span class="flex shrink-0 items-center gap-1">
									{#if gitStatus.ahead > 0}
										<span class="font-mono text-[10px] text-emerald-600 dark:text-emerald-400">
											↑{gitStatus.ahead}
										</span>
									{/if}
									{#if gitStatus.behind > 0}
										<span class="font-mono text-[10px] text-amber-600 dark:text-amber-400">
											↓{gitStatus.behind}
										</span>
									{/if}
									<ChevronDown
										class={branchesOpen
											? 'h-3 w-3 rotate-180 text-muted-foreground transition-transform'
											: 'h-3 w-3 text-muted-foreground transition-transform'}
									/>
								</span>
							</button>

							{#if branchesOpen}
								<div class="space-y-1.5 rounded-md border bg-card p-2">
									<form
										class="space-y-1.5"
										onsubmit={(e) => {
											e.preventDefault();
											void handleCheckout(newBranchName, true, newBranchFrom);
										}}
									>
										<div class="flex items-center gap-1">
											<Input
												bind:value={newBranchName}
												placeholder={t('myaaw.workspace.git_branch_new_placeholder')}
												class="h-7 flex-1 font-mono text-[11px]"
											/>
											<Button
												type="submit"
												size="sm"
												variant="outline"
												class="h-7 px-2 text-[11px]"
												disabled={busyAction !== '' || !newBranchName.trim()}
											>
												<Plus class="h-3 w-3" />
												{t('myaaw.workspace.git_branch_create')}
											</Button>
										</div>
										<div class="flex items-center gap-1.5">
											<span class="shrink-0 text-[10px] text-muted-foreground">
												{t('myaaw.workspace.git_branch_from')}:
											</span>
											<select
												bind:value={newBranchFrom}
												aria-label={t('myaaw.workspace.git_branch_from')}
												class="h-6 min-w-0 flex-1 rounded border bg-background px-1.5 font-mono text-[10px] text-foreground outline-none focus-visible:border-ring"
											>
												<option value="">
													{t('myaaw.workspace.git_branch_from_current', {
														branch: gitStatus.branch || 'HEAD'
													})}
												</option>
												{#each branchesData?.branches ?? [] as b (b.name)}
													{#if b.name !== gitStatus.branch}
														<option value={b.name}>{b.name}</option>
													{/if}
												{/each}
												{#each branchesData?.remotes ?? [] as r (r)}
													<option value={r}>{r}</option>
												{/each}
											</select>
										</div>
									</form>

									{#if loadingBranches}
										<div class="flex items-center gap-1.5 py-1 text-[11px] text-muted-foreground">
											<Loader2 class="h-3 w-3 animate-spin" />
											{t('common.loading')}
										</div>
									{:else if branchesData?.branches.length}
										<div class="max-h-36 space-y-0.5 overflow-y-auto">
											{#each branchesData.branches as b (b.name)}
												<div
													class={[
														'group/branch flex items-center gap-0.5 rounded pr-0.5 transition-colors',
														b.current ? 'bg-primary/10' : 'hover:bg-muted'
													]}
												>
													<button
														type="button"
														disabled={b.current || busyAction !== ''}
														class={[
															'flex min-w-0 flex-1 items-center gap-1.5 px-1.5 py-1 text-left font-mono text-[11px]',
															b.current ? 'font-semibold text-primary' : 'disabled:opacity-60'
														]}
														onclick={() => void handleCheckout(b.name, false)}
													>
														<span class="w-3 shrink-0">
															{#if b.current}
																<Check class="h-3 w-3" />
															{/if}
														</span>
														<span class="min-w-0 flex-1 truncate">{b.name}</span>
														{#if b.upstream}
															<span class="truncate text-[9px] text-muted-foreground">
																{b.upstream}
															</span>
														{/if}
													</button>
													<button
														type="button"
														class="grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-background hover:text-foreground group-hover/branch:opacity-100 [@media(hover:none)]:opacity-100"
														onclick={() => {
															newBranchFrom = b.name === gitStatus?.branch ? '' : b.name;
														}}
														title={t('myaaw.workspace.git_branch_create_from', { branch: b.name })}
														aria-label={t('myaaw.workspace.git_branch_create_from', {
															branch: b.name
														})}
													>
														<GitBranchPlus class="h-3 w-3" />
													</button>
												</div>
											{/each}
										</div>
									{/if}
								</div>
							{/if}
						</div>

						<!-- Commit Box (VS Code style) -->
						<div class="space-y-1.5">
							<textarea
								bind:value={commitMessage}
								rows="2"
								placeholder={t('myaaw.workspace.git_commit_placeholder')}
								class="w-full resize-none rounded-md border bg-background px-2.5 py-1.5 text-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring"
								onkeydown={(e) => {
									if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && canCommit) {
										e.preventDefault();
										void handleCommit();
									}
								}}
							></textarea>
							<div class="flex h-7.5 w-full overflow-hidden rounded-md">
								<Button
									size="sm"
									class="h-7.5 flex-1 rounded-r-none text-xs"
									disabled={!canCommit}
									onclick={() => void handleCommit('commit')}
								>
									{#if isCommitBusy}
										<Loader2 class="h-3.5 w-3.5 animate-spin" />
									{:else}
										<Check class="h-3.5 w-3.5" />
									{/if}
									{t('myaaw.workspace.git_commit_btn')}
								</Button>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}
											<Button
												{...props}
												size="sm"
												class="h-7.5 w-7 shrink-0 rounded-l-none border-l border-primary-foreground/20 px-0"
												disabled={!canAmend}
												aria-label={t('myaaw.workspace.git_commit_more')}
												title={t('myaaw.workspace.git_commit_more')}
											>
												<ChevronDown class="h-3.5 w-3.5" />
											</Button>
										{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										<DropdownMenu.Item
											disabled={!canCommit}
											onclick={() => void handleCommit('commit')}
										>
											{t('myaaw.workspace.git_commit_btn')}
										</DropdownMenu.Item>
										<DropdownMenu.Item
											disabled={!canAmend}
											onclick={() => void handleCommit('amend')}
										>
											{t('myaaw.workspace.git_commit_amend')}
										</DropdownMenu.Item>
										<DropdownMenu.Separator />
										<DropdownMenu.Item
											disabled={!canCommit}
											onclick={() => void handleCommit('commit-push')}
										>
											{t('myaaw.workspace.git_commit_push')}
										</DropdownMenu.Item>
										<DropdownMenu.Item
											disabled={!canCommit}
											onclick={() => void handleCommit('commit-sync')}
										>
											{t('myaaw.workspace.git_commit_sync')}
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							</div>
						</div>

						{#if gitStatus.clean}
							<div class="flex items-center justify-center rounded-md border border-dashed py-3">
								<Badge variant="outline" class="text-[11px]">
									{t('myaaw.workspace.git_clean')}
								</Badge>
							</div>
						{/if}

						<!-- Staged Changes -->
						{#if stagedItems.length > 0}
							<div class="space-y-0.5">
								<div class="flex items-center justify-between gap-1 rounded px-0.5 py-0.5">
									<button
										type="button"
										class="flex min-w-0 items-center gap-1 text-left text-[11px] font-medium text-foreground"
										onclick={() => (stagedSubOpen = !stagedSubOpen)}
									>
										{#if stagedSubOpen}
											<ChevronDown class="h-3 w-3 shrink-0 text-muted-foreground" />
										{:else}
											<ChevronRight class="h-3 w-3 shrink-0 text-muted-foreground" />
										{/if}
										<span class="truncate">{t('myaaw.workspace.git_staged')}</span>
									</button>
									<div class="flex shrink-0 items-center gap-0.5">
										<button
											type="button"
											class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
											onclick={() =>
												onOpenMultiDiff({
													scope: 'staged',
													label: `${t('myaaw.workspace.git_staged')} (${stagedItems.length})`
												})}
											title={t('myaaw.workspace.git_open_changes')}
											aria-label={t('myaaw.workspace.git_open_changes')}
										>
											<FileDiff class="h-3.5 w-3.5" />
										</button>
										<button
											type="button"
											disabled={busyAction !== ''}
											class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
											onclick={() =>
												void runAction('unstage-all', async () => {
													await workspaceStore.gitUnstage(agent, conversationId, { all: true });
												})}
											title={t('myaaw.workspace.git_unstage_all')}
											aria-label={t('myaaw.workspace.git_unstage_all')}
										>
											<Minus class="h-3.5 w-3.5" />
										</button>
										<span
											class="ml-0.5 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground tabular-nums"
										>
											{stagedItems.length}
										</span>
									</div>
								</div>

								{#if stagedSubOpen}
									{#if viewMode === 'tree'}
										{#each stagedTreeRows as row (row.id)}
											{#if row.kind === 'dir'}
												{@const dirCollapsed = Boolean(collapsedStagedDirs[row.path])}
												<button
													type="button"
													class="flex w-full items-center gap-2 rounded-md py-1.5 pr-2 text-left text-[13px] leading-tight transition-colors hover:bg-muted/70"
													style:padding-left="{6 + row.depth * 12}px"
													onclick={() =>
														(collapsedStagedDirs = {
															...collapsedStagedDirs,
															[row.path]: !dirCollapsed
														})}
												>
													{#if dirCollapsed}
														<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
														<Folder class="h-4 w-4 shrink-0 text-muted-foreground" />
													{:else}
														<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
														<FolderOpen class="h-4 w-4 shrink-0 text-muted-foreground" />
													{/if}
													<span class="min-w-0 flex-1 truncate">{row.name}</span>
													<span
														class={[
															'h-1.5 w-1.5 shrink-0 rounded-full bg-current opacity-80',
															gitStatusTextClass(row.statusCode)
														]}
													></span>
												</button>
											{:else if row.item}
												{@render changeRow(row.item, row.depth, false)}
											{/if}
										{/each}
									{:else}
										{#each stagedItems as item (`s:${item.path}`)}
											{@render changeRow(item, 0, true)}
										{/each}
									{/if}
								{/if}
							</div>
						{/if}

						<!-- Working Changes (Unstaged + Untracked combined, matching VS Code) -->
						{#if workingItems.length > 0}
							<div class="space-y-0.5">
								<div class="flex items-center justify-between gap-1 rounded px-0.5 py-0.5">
									<button
										type="button"
										class="flex min-w-0 items-center gap-1 text-left text-[11px] font-medium text-foreground"
										onclick={() => (workingSubOpen = !workingSubOpen)}
									>
										{#if workingSubOpen}
											<ChevronDown class="h-3 w-3 shrink-0 text-muted-foreground" />
										{:else}
											<ChevronRight class="h-3 w-3 shrink-0 text-muted-foreground" />
										{/if}
										<span class="truncate">{t('myaaw.workspace.git_changes')}</span>
									</button>
									<!-- Exact VS Code order: Open Changes | Discard All | Stage All | Count -->
									<div class="flex shrink-0 items-center gap-0.5">
										<button
											type="button"
											class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
											onclick={() =>
												onOpenMultiDiff({
													scope: 'changes',
													label: `${t('myaaw.workspace.git_changes')} (${workingItems.length})`
												})}
											title={t('myaaw.workspace.git_open_changes')}
											aria-label={t('myaaw.workspace.git_open_changes')}
										>
											<FileDiff class="h-3.5 w-3.5" />
										</button>
										<button
											type="button"
											disabled={busyAction !== ''}
											class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
											onclick={() => void handleDiscardAll()}
											title={t('myaaw.workspace.git_discard_all')}
											aria-label={t('myaaw.workspace.git_discard_all')}
										>
											<Undo2 class="h-3.5 w-3.5" />
										</button>
										<button
											type="button"
											disabled={busyAction !== ''}
											class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
											onclick={() =>
												void runAction('stage-all', async () => {
													await workspaceStore.gitStage(agent, conversationId, { all: true });
												})}
											title={t('myaaw.workspace.git_stage_all')}
											aria-label={t('myaaw.workspace.git_stage_all')}
										>
											<Plus class="h-3.5 w-3.5" />
										</button>
										<span
											class="ml-0.5 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground tabular-nums"
										>
											{workingItems.length}
										</span>
									</div>
								</div>

								{#if workingSubOpen}
									{#if viewMode === 'tree'}
										{#each workingTreeRows as row (row.id)}
											{#if row.kind === 'dir'}
												{@const dirCollapsed = Boolean(collapsedWorkingDirs[row.path])}
												<button
													type="button"
													class="flex w-full items-center gap-2 rounded-md py-1.5 pr-2 text-left text-[13px] leading-tight transition-colors hover:bg-muted/70"
													style:padding-left="{6 + row.depth * 12}px"
													onclick={() =>
														(collapsedWorkingDirs = {
															...collapsedWorkingDirs,
															[row.path]: !dirCollapsed
														})}
												>
													{#if dirCollapsed}
														<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
														<Folder class="h-4 w-4 shrink-0 text-muted-foreground" />
													{:else}
														<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
														<FolderOpen class="h-4 w-4 shrink-0 text-muted-foreground" />
													{/if}
													<span class="min-w-0 flex-1 truncate">{row.name}</span>
													<span
														class={[
															'h-1.5 w-1.5 shrink-0 rounded-full bg-current opacity-80',
															gitStatusTextClass(row.statusCode)
														]}
													></span>
												</button>
											{:else if row.item}
												{@render changeRow(row.item, row.depth, false)}
											{/if}
										{/each}
									{:else}
										{#each workingItems as item (`w:${item.untracked ? 'q' : 'u'}:${item.path}`)}
											{@render changeRow(item, 0, true)}
										{/each}
									{/if}
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</section>

		<!-- MIDDLE COLLAPSIBLE BAR: Stashes (collapsed by default) -->
		<section class="shrink-0 border-t">
			<button
				type="button"
				class="flex w-full items-center justify-between bg-muted/25 px-2 py-1.5 text-[11px] font-semibold tracking-wide text-foreground uppercase hover:bg-muted/50"
				onclick={toggleStashes}
				aria-expanded={stashesSectionOpen}
			>
				<span class="flex items-center gap-1">
					{#if stashesSectionOpen}
						<ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
					{:else}
						<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
					{/if}
					<Archive class="h-3 w-3 text-muted-foreground" />
					<span>{t('myaaw.workspace.git_stashes')}</span>
				</span>
				{#if stashes.length > 0}
					<span
						class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground tabular-nums"
					>
						{stashes.length}
					</span>
				{/if}
			</button>

			{#if stashesSectionOpen}
				<div class="max-h-52 space-y-2 overflow-y-auto border-t px-2.5 py-2">
					{#if hasAnyChanges}
						<form
							class="space-y-1.5 rounded-md border bg-muted/20 p-2"
							onsubmit={(e) => {
								e.preventDefault();
								void runAction('stash-save', async () => {
									await workspaceStore.gitStashSave(agent, conversationId, {
										message: stashMessage.trim() || undefined,
										include_untracked: stashIncludeUntracked
									});
									stashMessage = '';
									toastStore.success(t('myaaw.workspace.git_stash_saved'));
								});
							}}
						>
							<Input
								bind:value={stashMessage}
								placeholder={t('myaaw.workspace.git_stash_placeholder')}
								class="h-7 text-[11px]"
							/>
							<div class="flex items-center justify-between gap-2">
								<label
									class="flex cursor-pointer items-center gap-1.5 text-[10px] text-muted-foreground select-none"
								>
									<input
										type="checkbox"
										bind:checked={stashIncludeUntracked}
										class="h-3 w-3 rounded border-border accent-primary"
									/>
									<span>{t('myaaw.workspace.git_stash_include_untracked')}</span>
								</label>
								<Button
									type="submit"
									size="sm"
									variant="outline"
									class="h-6 px-2 text-[11px]"
									disabled={busyAction !== ''}
								>
									{t('myaaw.workspace.git_stash_save')}
								</Button>
							</div>
						</form>
					{/if}

					{#if loadingStashes}
						<div class="flex items-center gap-1.5 text-[11px] text-muted-foreground">
							<Loader2 class="h-3 w-3 animate-spin" />
							{t('common.loading')}
						</div>
					{:else if stashes.length === 0}
						<p class="text-[11px] text-muted-foreground">{t('myaaw.workspace.git_no_stashes')}</p>
					{:else}
						<div class="space-y-1">
							{#each stashes as stash (stash.ref)}
								<div class="rounded-md border bg-card/50 px-2 py-1.5">
									<p class="truncate text-[11px] font-medium" title={stash.message}>
										{stash.message}
									</p>
									<p class="font-mono text-[10px] text-muted-foreground">
										{stash.ref}{#if stash.date}
											· {shortDate(stash.date)}{/if}
									</p>
									<div class="mt-1 flex items-center gap-1">
										<Button
											size="sm"
											variant="ghost"
											class="h-6 px-1.5 text-[10px]"
											disabled={busyAction !== ''}
											onclick={() =>
												void runAction(`stash-apply:${stash.ref}`, async () => {
													await workspaceStore.gitStashApply(agent, conversationId, stash.ref);
													toastStore.success(t('myaaw.workspace.git_stash_applied'));
												})}
										>
											{t('myaaw.workspace.git_stash_apply')}
										</Button>
										<Button
											size="sm"
											variant="ghost"
											class="h-6 px-1.5 text-[10px]"
											disabled={busyAction !== ''}
											onclick={() =>
												void runAction(`stash-pop:${stash.ref}`, async () => {
													await workspaceStore.gitStashPop(agent, conversationId, stash.ref);
													toastStore.success(t('myaaw.workspace.git_stash_popped'));
												})}
										>
											<RotateCcw class="h-2.5 w-2.5" />
											{t('myaaw.workspace.git_stash_pop')}
										</Button>
										<Button
											size="sm"
											variant="ghost"
											class="h-6 px-1.5 text-[10px] text-destructive hover:text-destructive"
											disabled={busyAction !== ''}
											onclick={() => void handleDropStash(stash.ref)}
										>
											{t('myaaw.workspace.git_stash_drop')}
										</Button>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</section>

		<!-- BOTTOM PANE: Commits / Graph (CLOSED by default, paged + infinite scroll) -->
		<section
			class={[
				'flex min-h-0 flex-col border-t',
				commitsSectionOpen ? (changesSectionOpen ? 'h-[42%] shrink-0' : 'flex-1') : 'shrink-0'
			]}
		>
			<div class="flex shrink-0 items-center justify-between gap-1 bg-muted/25 px-2 py-1.5">
				<button
					type="button"
					class="flex min-w-0 flex-1 items-center gap-1 text-left text-[11px] font-semibold tracking-wide text-foreground uppercase hover:text-primary"
					onclick={() => (commitsSectionOpen = !commitsSectionOpen)}
					aria-expanded={commitsSectionOpen}
				>
					{#if commitsSectionOpen}
						<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else}
						<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{/if}
					<span>{t('myaaw.workspace.git_graph')}</span>
				</button>
				{#if gitStatus.branch}
					<span
						class="max-w-28 truncate rounded-full bg-primary/15 px-2 py-0.5 font-mono text-[10px] font-medium text-primary"
						title={gitStatus.branch}
					>
						{gitStatus.branch}
					</span>
				{/if}
			</div>

			{#if commitsSectionOpen}
				<div
					bind:this={commitsScrollRoot}
					onscroll={onCommitsScroll}
					class="min-h-0 flex-1 overflow-y-auto border-t px-2 py-1.5"
				>
					{#if loadingCommits && commits.length === 0}
						<div class="flex items-center gap-2 p-2 text-xs text-muted-foreground">
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
							{t('common.loading')}
						</div>
					{:else if commits.length === 0}
						<p class="p-2 text-center text-xs text-muted-foreground">
							{t('myaaw.workspace.git_no_commits')}
						</p>
					{:else}
						<div class="space-y-0.5">
							{#each commits as commit (commit.hash)}
								{@const isOpen = Boolean(expandedCommits[commit.hash])}
								{@const detail = commitDetails[commit.hash]}
								<div class="rounded-md">
									<div
										class="group/commit flex items-center gap-1 rounded-md pr-1 transition-colors hover:bg-muted/70"
									>
										<button
											type="button"
											class="flex min-w-0 flex-1 items-start gap-1.5 px-1.5 py-1 text-left"
											onclick={() => void toggleCommitDetail(commit.hash)}
											title={`${commit.short_hash} · ${commit.author} · ${shortDate(commit.date)}\n${commit.subject}`}
										>
											{#if isOpen}
												<ChevronDown class="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
											{:else}
												<ChevronRight class="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
											{/if}
											<GitCommitHorizontal
												class="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground"
											/>
											<div class="min-w-0 flex-1">
												<p class="truncate text-xs">{commit.subject}</p>
												<p class="truncate text-[10px] text-muted-foreground">
													<span class="font-mono">{commit.short_hash}</span> · {commit.author} · {shortDate(
														commit.date
													)}
												</p>
											</div>
										</button>

										<button
											type="button"
											class="grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-background hover:text-foreground group-hover/commit:opacity-100 [@media(hover:none)]:opacity-100"
											onclick={() =>
												onOpenMultiDiff({
													scope: 'commit',
													commit: commit.hash,
													label: `${commit.short_hash} · ${commit.subject.slice(0, 24)}`
												})}
											title={t('myaaw.workspace.git_open_changes')}
											aria-label={t('myaaw.workspace.git_open_changes')}
										>
											<FileDiff class="h-3 w-3" />
										</button>
									</div>

									{#if isOpen}
										<div class="mb-1 ml-4 space-y-0.5 border-l pl-2">
											{#if loadingCommit[commit.hash]}
												<div
													class="flex items-center gap-1.5 py-1 text-[11px] text-muted-foreground"
												>
													<Loader2 class="h-3 w-3 animate-spin" />
													{t('common.loading')}
												</div>
											{:else if detail?.files.length}
												{#each detail.files as file (file.path)}
													<button
														type="button"
														class="flex w-full items-center gap-1.5 rounded px-1.5 py-0.5 text-left transition-colors hover:bg-muted/70"
														onclick={() =>
															onOpenDiff({
																path: file.path,
																commit: commit.hash
															})}
														title={file.path}
													>
														<span
															class={[
																'w-3 shrink-0 font-mono text-[10px] font-bold',
																gitStatusTextClass(file.status)
															]}
														>
															{gitStatusLetter(file.status)}
														</span>
														<span class="min-w-0 flex-1 truncate font-mono text-[10px]">
															{file.path}
														</span>
														{#if !file.binary}
															<span
																class="shrink-0 font-mono text-[9px] text-muted-foreground tabular-nums"
															>
																{#if file.additions > 0}
																	<span class="text-emerald-600 dark:text-emerald-400">
																		+{file.additions}
																	</span>
																{/if}
																{#if file.deletions > 0}
																	<span class="ml-0.5 text-rose-600 dark:text-rose-400">
																		-{file.deletions}
																	</span>
																{/if}
															</span>
														{/if}
													</button>
												{/each}
											{/if}
										</div>
									{/if}
								</div>
							{/each}

							{#if commitsHasMore}
								<div bind:this={commitsSentinel} class="flex justify-center py-2">
									{#if loadingMoreCommits}
										<span class="flex items-center gap-1.5 text-[11px] text-muted-foreground">
											<Loader2 class="h-3 w-3 animate-spin" />
											{t('myaaw.workspace.git_loading_more')}
										</span>
									{:else}
										<Button
											variant="ghost"
											size="sm"
											class="h-6 text-[11px] text-muted-foreground"
											onclick={() => void loadCommitsPage(false)}
										>
											{t('myaaw.workspace.git_load_more')}
										</Button>
									{/if}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/if}
		</section>
	</div>
{/if}
