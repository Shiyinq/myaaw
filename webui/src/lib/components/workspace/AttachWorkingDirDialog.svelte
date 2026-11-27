<script lang="ts">
	import { untrack } from 'svelte';
	import {
		AlertCircle,
		ArrowUp,
		Bot,
		Check,
		ChevronRight,
		Code2,
		Folder,
		FolderGit2,
		FolderOpen,
		FolderPlus,
		HardDrive,
		Home,
		Loader2,
		Pencil,
		Search,
		Server,
		X
	} from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import { getApiBase, isLoopbackApiBase } from '$lib/constants/api';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type {
		WorkspaceBrowseEntry,
		WorkspaceBrowseResponse,
		WorkspaceRootItem,
		WorkspaceRootsResponse
	} from '$lib/types';

	interface Props {
		open?: boolean;
		agent: string;
		currentPath?: string;
		onSelect: (path: string) => void | Promise<void>;
	}

	let { open = $bindable(false), agent, currentPath = '', onSelect }: Props = $props();

	const { t } = useTranslation();

	let rootsInfo = $state<WorkspaceRootsResponse | null>(null);
	let browseData = $state<WorkspaceBrowseResponse | null>(null);
	let pathInput = $state('');
	let editingPath = $state(false);
	let folderQuery = $state('');
	let loadingRoots = $state(false);
	let loadingDir = $state(false);
	let error = $state('');
	let showNewFolder = $state(false);
	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let submitting = $state(false);

	const isRemoteApi = $derived(!isLoopbackApiBase(getApiBase()));

	const filteredEntries = $derived.by(() => {
		const list = browseData?.entries ?? [];
		const needle = folderQuery.trim().toLowerCase();
		if (!needle) return list;
		return list.filter(
			(entry) =>
				entry.name.toLowerCase().includes(needle) ||
				(entry.markers ?? []).some((m) => m.toLowerCase().includes(needle))
		);
	});

	const showFilter = $derived((browseData?.entries?.length ?? 0) > 6 || folderQuery.length > 0);

	/** Longest matching root prefix for the current path, used for clean breadcrumbs. */
	const matchedRoot = $derived.by(() => {
		const cur = browseData?.path ?? '';
		const roots = rootsInfo?.roots ?? [];
		if (!cur || roots.length === 0) return null;
		let best: WorkspaceRootItem | null = null;
		for (const root of roots) {
			const cleanRoot = root.path.replace(/[\\/]+$/, '');
			if (
				cur === cleanRoot ||
				cur.startsWith(cleanRoot + '/') ||
				cur.startsWith(cleanRoot + '\\')
			) {
				if (!best || cleanRoot.length > best.path.length) {
					best = root;
				}
			}
		}
		return best;
	});

	interface Crumb {
		label: string;
		path: string;
	}

	const breadcrumbs = $derived.by((): Crumb[] => {
		const cur = browseData?.path ?? '';
		if (!cur) return [];
		const sep = cur.includes('\\') ? '\\' : '/';
		if (matchedRoot) {
			const base = matchedRoot.path.replace(/[\\/]+$/, '');
			const crumbs: Crumb[] = [{ label: matchedRoot.label, path: base }];
			const rel = cur.slice(base.length).replace(/^[\\/]+/, '');
			if (rel) {
				let acc = base;
				for (const part of rel.split(/[\\/]+/).filter(Boolean)) {
					acc = `${acc}${sep}${part}`;
					crumbs.push({ label: part, path: acc });
				}
			}
			return crumbs;
		}
		const parts = cur.split(/[\\/]+/).filter(Boolean);
		const crumbs: Crumb[] = [];
		let acc = cur.startsWith('/') ? '' : '';
		for (const part of parts) {
			acc = acc ? `${acc}${sep}${part}` : cur.startsWith('/') ? `/${part}` : part;
			crumbs.push({ label: part, path: acc });
		}
		return crumbs;
	});

	const currentFolderName = $derived.by(() => {
		const cur = browseData?.path ?? '';
		if (!cur) return '';
		const clean = cur.replace(/[\\/]+$/, '');
		const parts = clean.split(/[\\/]/).filter(Boolean);
		return parts[parts.length - 1] ?? cur;
	});

	const homeRootPath = $derived(
		rootsInfo?.roots?.find((r) => r.label === 'Home')?.path.replace(/[\\/]+$/, '') ?? ''
	);

	const compactPath = $derived.by(() => {
		const cur = browseData?.path ?? '';
		if (!cur) return '';
		if (homeRootPath && (cur === homeRootPath || cur.startsWith(homeRootPath + '/'))) {
			return '~' + cur.slice(homeRootPath.length);
		}
		return cur;
	});

	$effect(() => {
		if (!open) return;
		untrack(() => {
			showNewFolder = false;
			newFolderName = '';
			editingPath = false;
			folderQuery = '';
			error = '';
			void initialize();
		});
	});

	async function initialize() {
		loadingRoots = true;
		error = '';
		try {
			const roots = await workspaceStore.roots(agent);
			rootsInfo = roots;
			const startPath = currentPath || roots.roots[0]?.path || '';
			if (roots.allow_host_browse) {
				await loadDir(startPath);
			}
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loadingRoots = false;
		}
	}

	async function loadDir(targetPath: string) {
		loadingDir = true;
		error = '';
		folderQuery = '';
		editingPath = false;
		try {
			const res = await workspaceStore.browse(agent, targetPath);
			browseData = res;
			pathInput = res.path;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loadingDir = false;
		}
	}

	async function handleJumpPath(event: SubmitEvent) {
		event.preventDefault();
		const target = pathInput.trim();
		if (!target) return;
		await loadDir(target);
	}

	async function handleCreateFolder(event: SubmitEvent) {
		event.preventDefault();
		const name = newFolderName.trim();
		const parent = browseData?.path || '';
		if (!name || !parent || !agent) return;
		creatingFolder = true;
		error = '';
		try {
			const res = await workspaceStore.mkdir(agent, parent, name);
			showNewFolder = false;
			newFolderName = '';
			toastStore.success(t('myaaw.workspace.folder_created'));
			await loadDir(res.path);
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			creatingFolder = false;
		}
	}

	async function attachPath(targetPath: string) {
		if (!targetPath || submitting) return;
		submitting = true;
		try {
			await onSelect(targetPath);
			open = false;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			submitting = false;
		}
	}

	function entryIcon(entry: WorkspaceBrowseEntry) {
		return entry.is_git ? FolderGit2 : Folder;
	}

	function rootIcon(root: WorkspaceRootItem) {
		const lower = root.label.toLowerCase();
		if (lower === 'home') return Home;
		if (root.kind === 'agent_workspace') {
			return lower.includes('project') ? FolderGit2 : Bot;
		}
		if (lower === 'development' || lower === 'projects' || lower === 'code') {
			return Code2;
		}
		return Folder;
	}

	function isRootActive(root: WorkspaceRootItem): boolean {
		return browseData?.path === root.path;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-[35rem] sm:max-h-[86vh] sm:w-full sm:max-w-2xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<!-- Header -->
		<Dialog.Header class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:px-5 sm:py-4 sm:pr-12">
			<div class="flex items-start gap-3">
				<span
					class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"
				>
					<FolderOpen class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1 space-y-0.5">
					<Dialog.Title class="flex flex-wrap items-center gap-2 text-sm sm:text-base">
						<span>{t('myaaw.workspace.picker_title')}</span>
						{#if rootsInfo?.hostname}
							<Badge
								variant={isRemoteApi ? 'default' : 'secondary'}
								class="h-5 gap-1 px-1.5 font-mono text-[10px] font-normal"
							>
								{#if isRemoteApi}
									<Server class="h-2.5 w-2.5" />
								{:else}
									<HardDrive class="h-2.5 w-2.5" />
								{/if}
								<span class="max-w-40 truncate">{rootsInfo.hostname}</span>
								{#if isRemoteApi}
									· {t('myaaw.workspace.remote_server')}
								{/if}
							</Badge>
						{/if}
					</Dialog.Title>
					<Dialog.Description class="text-xs leading-relaxed">
						{isRemoteApi
							? t('myaaw.workspace.picker_desc_remote')
							: t('myaaw.workspace.picker_desc_local')}
					</Dialog.Description>
				</div>
			</div>
		</Dialog.Header>

		{#if loadingRoots}
			<div
				class="flex min-h-0 flex-1 items-center justify-center gap-2 px-4 py-12 text-sm text-muted-foreground"
			>
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if rootsInfo && !rootsInfo.allow_host_browse}
			<div class="flex min-h-0 flex-1 items-center justify-center p-6">
				<div
					class="flex max-w-md items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-xs text-destructive"
				>
					<AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
					<p>{t('myaaw.workspace.host_browse_disabled')}</p>
				</div>
			</div>
		{:else}
			<!-- Mobile horizontal Places strip -->
			{#if rootsInfo?.roots?.length}
				<div
					class="flex shrink-0 flex-nowrap items-center gap-1.5 overflow-x-auto border-b bg-muted/20 px-3 py-2 sm:hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
				>
					{#each rootsInfo.roots as root (root.path)}
						{@const Icon = rootIcon(root)}
						{@const active = isRootActive(root)}
						<button
							type="button"
							aria-pressed={active}
							class={[
								'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium whitespace-nowrap transition-colors',
								active
									? 'border-primary/60 bg-primary/10 text-foreground'
									: 'border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground'
							]}
							onclick={() => void loadDir(root.path)}
							title={root.path}
						>
							<Icon class="h-3.5 w-3.5 shrink-0" />
							{root.label}
						</button>
					{/each}
				</div>
			{/if}

			<!-- Body: two-pane on desktop, single full-height column on mobile -->
			<div class="flex min-h-0 flex-1 flex-col overflow-hidden sm:flex-row">
				<!-- Desktop Places sidebar -->
				{#if rootsInfo?.roots?.length}
					<aside
						class="hidden shrink-0 flex-col gap-1 overflow-y-auto border-r bg-muted/20 p-2.5 sm:flex sm:w-44"
					>
						<p
							class="px-2 pt-0.5 pb-1 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase"
						>
							{t('myaaw.workspace.picker_places')}
						</p>
						{#each rootsInfo.roots as root (root.path)}
							{@const Icon = rootIcon(root)}
							{@const active = isRootActive(root)}
							<button
								type="button"
								aria-pressed={active}
								class={[
									'flex items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors',
									active
										? 'bg-primary/10 font-medium text-foreground'
										: 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
								]}
								onclick={() => void loadDir(root.path)}
								title={root.path}
							>
								<Icon
									class={active
										? 'h-3.5 w-3.5 shrink-0 text-primary'
										: 'h-3.5 w-3.5 shrink-0 text-muted-foreground'}
								/>
								<span class="truncate">{root.label}</span>
							</button>
						{/each}
					</aside>
				{/if}

				<!-- Main browser column -->
				<div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
					<!-- Navigation / Breadcrumb toolbar -->
					<div class="flex shrink-0 items-center gap-1.5 border-b bg-muted/10 px-3 py-2">
						<Button
							type="button"
							variant="ghost"
							size="icon-sm"
							class="h-7 w-7 shrink-0"
							disabled={!browseData?.parent || loadingDir}
							onclick={() => browseData?.parent && void loadDir(browseData.parent)}
							title={t('myaaw.workspace.parent_dir')}
							aria-label={t('myaaw.workspace.parent_dir')}
						>
							<ArrowUp class="h-3.5 w-3.5" />
						</Button>

						{#if editingPath}
							<form class="flex min-w-0 flex-1 items-center gap-1" onsubmit={handleJumpPath}>
								<Input
									bind:value={pathInput}
									placeholder="/path/to/project"
									class="h-7 min-w-0 flex-1 font-mono text-xs"
									autofocus
									onkeydown={(e) => {
										if (e.key === 'Escape') {
											e.stopPropagation();
											editingPath = false;
											pathInput = browseData?.path ?? '';
										}
									}}
								/>
								<Button
									type="submit"
									variant="secondary"
									size="sm"
									class="h-7 px-2.5 text-xs"
									disabled={loadingDir}
								>
									{t('myaaw.workspace.go')}
								</Button>
								<Button
									type="button"
									variant="ghost"
									size="icon-sm"
									class="h-7 w-7 shrink-0"
									onclick={() => {
										editingPath = false;
										pathInput = browseData?.path ?? '';
									}}
									aria-label={t('common.cancel')}
								>
									<X class="h-3.5 w-3.5" />
								</Button>
							</form>
						{:else}
							<div
								class="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto rounded-md border bg-background px-2 py-1 text-xs [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
								title={browseData?.path ?? ''}
							>
								{#if breadcrumbs.length === 0}
									<span class="truncate font-mono text-muted-foreground">
										{browseData?.path || '/'}
									</span>
								{:else}
									{#each breadcrumbs as crumb, idx (crumb.path)}
										{@const isLast = idx === breadcrumbs.length - 1}
										{#if idx > 0}
											<ChevronRight class="h-3 w-3 shrink-0 text-muted-foreground/60" />
										{/if}
										<button
											type="button"
											disabled={isLast || loadingDir}
											onclick={() => void loadDir(crumb.path)}
											class={[
												'max-w-36 shrink-0 truncate rounded px-1 py-0.5 transition-colors',
												isLast
													? 'font-medium text-foreground'
													: 'text-muted-foreground hover:bg-muted hover:text-foreground'
											]}
										>
											{crumb.label}
										</button>
									{/each}
								{/if}
							</div>

							<Button
								type="button"
								variant="ghost"
								size="icon-sm"
								class="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
								disabled={loadingDir}
								onclick={() => {
									pathInput = browseData?.path ?? '';
									editingPath = true;
								}}
								title={t('myaaw.workspace.picker_edit_path')}
								aria-label={t('myaaw.workspace.picker_edit_path')}
							>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
						{/if}

						<Button
							type="button"
							variant={showNewFolder ? 'secondary' : 'ghost'}
							size="icon-sm"
							class="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
							disabled={!browseData?.path || loadingDir}
							onclick={() => (showNewFolder = !showNewFolder)}
							title={t('myaaw.workspace.new_folder')}
							aria-label={t('myaaw.workspace.new_folder')}
						>
							<FolderPlus class="h-3.5 w-3.5" />
						</Button>
					</div>

					<!-- Inline New Folder bar -->
					{#if showNewFolder}
						<form
							class="flex shrink-0 items-center gap-1.5 border-b bg-muted/25 px-3 py-2"
							onsubmit={handleCreateFolder}
						>
							<FolderPlus class="h-3.5 w-3.5 shrink-0 text-primary" />
							<Input
								bind:value={newFolderName}
								placeholder={t('myaaw.workspace.new_folder_placeholder')}
								class="h-7 min-w-0 flex-1 bg-background text-xs"
								autofocus
								onkeydown={(e) => {
									if (e.key === 'Escape') {
										e.stopPropagation();
										showNewFolder = false;
										newFolderName = '';
									}
								}}
							/>
							<Button
								type="submit"
								size="sm"
								class="h-7 px-2.5 text-xs"
								disabled={creatingFolder || !newFolderName.trim()}
							>
								{#if creatingFolder}
									<Loader2 class="h-3 w-3 animate-spin" />
								{/if}
								{t('common.create')}
							</Button>
							<Button
								type="button"
								variant="ghost"
								size="icon-sm"
								class="h-7 w-7 shrink-0"
								onclick={() => {
									showNewFolder = false;
									newFolderName = '';
								}}
								aria-label={t('common.cancel')}
							>
								<X class="h-3.5 w-3.5" />
							</Button>
						</form>
					{/if}

					<!-- Inline Filter bar when directory has many subfolders -->
					{#if showFilter && !loadingDir}
						<div class="shrink-0 border-b px-3 py-1.5">
							<div class="relative">
								<Search
									class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
								/>
								<Input
									bind:value={folderQuery}
									placeholder={t('myaaw.workspace.picker_filter_folders')}
									autocomplete="off"
									class="h-7 pl-8 pr-7 text-xs"
								/>
								{#if folderQuery}
									<button
										type="button"
										class="absolute top-1/2 right-1.5 grid h-5 w-5 -translate-y-1/2 place-items-center rounded text-muted-foreground hover:text-foreground"
										onclick={() => (folderQuery = '')}
										aria-label={t('common.cancel')}
									>
										<X class="h-3 w-3" />
									</button>
								{/if}
							</div>
						</div>
					{/if}

					<!-- Error banner -->
					{#if error}
						<div
							class="mx-3 mt-2.5 flex shrink-0 items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
						>
							<AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span class="min-w-0 flex-1 break-words">{error}</span>
							<button
								type="button"
								class="shrink-0 rounded p-0.5 opacity-70 hover:opacity-100"
								onclick={() => (error = '')}
								aria-label={t('common.close')}
							>
								<X class="h-3 w-3" />
							</button>
						</div>
					{/if}

					<!-- Folder list -->
					<div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
						{#if loadingDir}
							<div
								class="flex h-full min-h-40 items-center justify-center gap-2 text-xs text-muted-foreground"
							>
								<Loader2 class="h-4 w-4 animate-spin" />
								{t('common.loading')}
							</div>
						{:else if !browseData || browseData.entries.length === 0}
							<div
								class="flex h-full min-h-44 flex-col items-center justify-center gap-2.5 px-6 py-10 text-center"
							>
								<span
									class="grid h-11 w-11 place-items-center rounded-full bg-muted/70 text-muted-foreground"
								>
									<FolderOpen class="h-5 w-5" />
								</span>
								<div class="space-y-0.5">
									<p class="text-xs font-medium text-foreground">
										{t('myaaw.workspace.dir_empty_hint')}
									</p>
									{#if compactPath}
										<p class="font-mono text-[11px] text-muted-foreground">{compactPath}</p>
									{/if}
								</div>
								{#if browseData?.path && !showNewFolder}
									<Button
										type="button"
										variant="outline"
										size="sm"
										class="mt-1 h-7 gap-1.5 text-xs"
										onclick={() => (showNewFolder = true)}
									>
										<FolderPlus class="h-3.5 w-3.5" />
										{t('myaaw.workspace.new_folder')}
									</Button>
								{/if}
							</div>
						{:else if filteredEntries.length === 0}
							<div
								class="flex h-full min-h-36 flex-col items-center justify-center gap-1.5 px-6 py-8 text-center text-xs text-muted-foreground"
							>
								<Search class="h-4 w-4 opacity-50" />
								<p>{t('myaaw.workspace.picker_no_matching_folders')}</p>
							</div>
						{:else}
							<div class="divide-y divide-border/50 p-1.5">
								{#each filteredEntries as entry (entry.path)}
									{@const Icon = entryIcon(entry)}
									<div
										class="group flex items-center gap-2 rounded-lg px-2 py-2 text-[13px] transition-colors hover:bg-muted/60"
									>
										<button
											type="button"
											class="flex min-w-0 flex-1 items-center gap-2.5 text-left"
											onclick={() => void loadDir(entry.path)}
										>
											<span
												class={[
													'grid h-7 w-7 shrink-0 place-items-center rounded-md',
													entry.is_git
														? 'bg-primary/10 text-primary'
														: 'bg-muted/70 text-muted-foreground'
												]}
											>
												<Icon class="h-4 w-4" />
											</span>
											<span class="min-w-0 flex-1 truncate font-medium text-foreground">
												{entry.name}
											</span>
											<span class="flex shrink-0 items-center gap-1">
												{#if entry.is_git}
													<Badge variant="secondary" class="h-4.5 px-1.5 text-[10px] font-medium">
														Git
													</Badge>
												{/if}
												{#if entry.is_empty}
													<Badge
														variant="outline"
														class="h-4.5 px-1.5 text-[10px] text-muted-foreground"
													>
														{t('myaaw.workspace.empty_badge')}
													</Badge>
												{/if}
												{#each (entry.markers ?? [])
													.filter((m) => m !== 'git')
													.slice(0, 2) as marker (marker)}
													<Badge
														variant="outline"
														class="hidden h-4.5 px-1.5 text-[10px] text-muted-foreground sm:inline-flex"
													>
														{marker}
													</Badge>
												{/each}
											</span>
										</button>

										<Button
											type="button"
											variant="outline"
											size="sm"
											class="h-7 shrink-0 rounded-md px-2.5 text-xs transition-opacity sm:opacity-0 sm:group-focus-within:opacity-100 sm:group-hover:opacity-100 [@media(hover:none)]:opacity-100"
											disabled={submitting}
											onclick={() => void attachPath(entry.path)}
										>
											{t('myaaw.workspace.select')}
										</Button>

										<button
											type="button"
											class="grid h-7 w-7 shrink-0 place-items-center rounded-md text-muted-foreground/70 transition-colors hover:bg-muted hover:text-foreground"
											onclick={() => void loadDir(entry.path)}
											aria-label={entry.name}
										>
											<ChevronRight class="h-4 w-4" />
										</button>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			</div>

			<!-- Sticky Footer -->
			<Dialog.Footer
				class="mx-0 mb-0 flex shrink-0 flex-row items-center justify-between gap-3 rounded-none border-t bg-muted/40 px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<div class="min-w-0 flex-1">
					{#if browseData?.can_attach && currentFolderName}
						<div class="flex items-center gap-1.5 text-xs font-medium text-foreground">
							<Folder class="h-3.5 w-3.5 shrink-0 text-primary" />
							<span class="truncate">{currentFolderName}</span>
						</div>
						<p class="truncate font-mono text-[10px] text-muted-foreground" title={browseData.path}>
							{compactPath}
						</p>
					{:else if browseData?.path}
						<p class="truncate text-xs text-muted-foreground">
							{t('myaaw.workspace.picker_pick_subfolder')}
						</p>
					{/if}
				</div>

				<div class="flex shrink-0 items-center gap-2">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onclick={() => (open = false)}
						disabled={submitting}
					>
						{t('common.cancel')}
					</Button>
					<Button
						type="button"
						size="sm"
						disabled={!browseData?.can_attach || !browseData?.path || submitting}
						onclick={() => browseData?.path && void attachPath(browseData.path)}
					>
						{#if submitting}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Check class="h-3.5 w-3.5" />
						{/if}
						{t('myaaw.workspace.attach_current')}
					</Button>
				</div>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
