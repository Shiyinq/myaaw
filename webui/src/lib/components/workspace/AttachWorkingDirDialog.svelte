<script lang="ts">
	import { untrack } from 'svelte';
	import {
		ArrowUp,
		Check,
		ChevronRight,
		Folder,
		FolderGit2,
		FolderOpen,
		FolderPlus,
		HardDrive,
		Loader2,
		Server,
		Unlink
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
		WorkspaceRootsResponse
	} from '$lib/types';

	interface Props {
		open?: boolean;
		agent: string;
		currentPath?: string;
		onSelect: (path: string) => void | Promise<void>;
		onDetach?: () => void | Promise<void>;
	}

	let { open = $bindable(false), agent, currentPath = '', onSelect, onDetach }: Props = $props();

	const { t } = useTranslation();

	let rootsInfo = $state<WorkspaceRootsResponse | null>(null);
	let browseData = $state<WorkspaceBrowseResponse | null>(null);
	let pathInput = $state('');
	let loadingRoots = $state(false);
	let loadingDir = $state(false);
	let error = $state('');
	let showNewFolder = $state(false);
	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let submitting = $state(false);

	const isRemoteApi = $derived(!isLoopbackApiBase(getApiBase()));

	$effect(() => {
		if (!open) return;
		untrack(() => {
			showNewFolder = false;
			newFolderName = '';
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

	async function handleDetach() {
		if (!onDetach || submitting) return;
		submitting = true;
		try {
			await onDetach();
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
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="flex max-h-[85vh] flex-col gap-3 sm:max-w-xl">
		<Dialog.Header>
			<Dialog.Title class="flex flex-wrap items-center gap-2">
				<FolderOpen class="h-4 w-4 text-primary" />
				<span>{t('myaaw.workspace.picker_title')}</span>
				{#if rootsInfo?.hostname}
					<Badge
						variant={isRemoteApi ? 'default' : 'secondary'}
						class="gap-1 font-mono text-[10px]"
					>
						{#if isRemoteApi}
							<Server class="h-3 w-3" />
						{:else}
							<HardDrive class="h-3 w-3" />
						{/if}
						{rootsInfo.hostname}
						{#if isRemoteApi}
							· {t('myaaw.workspace.remote_server')}
						{/if}
					</Badge>
				{/if}
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				{isRemoteApi
					? t('myaaw.workspace.picker_desc_remote')
					: t('myaaw.workspace.picker_desc_local')}
			</Dialog.Description>
		</Dialog.Header>

		{#if loadingRoots}
			<div class="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if rootsInfo && !rootsInfo.allow_host_browse}
			<div
				class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive"
			>
				{t('myaaw.workspace.host_browse_disabled')}
			</div>
		{:else}
			{#if rootsInfo?.roots?.length}
				<div class="flex flex-wrap items-center gap-1.5">
					{#each rootsInfo.roots as root (root.path)}
						<button
							type="button"
							class={[
								'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-medium transition-colors',
								browseData?.path === root.path
									? 'border-primary bg-primary/10 text-foreground'
									: 'bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground'
							]}
							onclick={() => void loadDir(root.path)}
							title={root.path}
						>
							<Folder class="h-3 w-3 shrink-0" />
							{root.label}
						</button>
					{/each}
				</div>
			{/if}

			<form class="flex items-center gap-1.5" onsubmit={handleJumpPath}>
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					disabled={!browseData?.parent || loadingDir}
					onclick={() => browseData?.parent && void loadDir(browseData.parent)}
					title={t('myaaw.workspace.parent_dir')}
					aria-label={t('myaaw.workspace.parent_dir')}
				>
					<ArrowUp class="h-3.5 w-3.5" />
				</Button>
				<Input
					bind:value={pathInput}
					placeholder="/path/to/project"
					class="h-8 flex-1 font-mono text-xs"
				/>
				<Button type="submit" variant="outline" size="sm" disabled={loadingDir}>
					{t('myaaw.workspace.go')}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					disabled={!browseData?.path || loadingDir}
					onclick={() => (showNewFolder = !showNewFolder)}
					title={t('myaaw.workspace.new_folder')}
					aria-label={t('myaaw.workspace.new_folder')}
				>
					<FolderPlus class="h-3.5 w-3.5" />
				</Button>
			</form>

			{#if showNewFolder}
				<form
					class="flex items-center gap-1.5 rounded-lg border bg-muted/30 p-2"
					onsubmit={handleCreateFolder}
				>
					<Input
						bind:value={newFolderName}
						placeholder={t('myaaw.workspace.new_folder_placeholder')}
						class="h-8 flex-1 text-xs"
						autofocus
					/>
					<Button type="submit" size="sm" disabled={creatingFolder || !newFolderName.trim()}>
						{#if creatingFolder}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{/if}
						{t('common.create')}
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						onclick={() => {
							showNewFolder = false;
							newFolderName = '';
						}}
					>
						{t('common.cancel')}
					</Button>
				</form>
			{/if}

			{#if error}
				<div
					class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
				>
					{error}
				</div>
			{/if}

			<div class="min-h-48 max-h-72 flex-1 overflow-y-auto rounded-lg border bg-card">
				{#if loadingDir}
					<div class="flex items-center justify-center gap-2 py-10 text-xs text-muted-foreground">
						<Loader2 class="h-4 w-4 animate-spin" />
						{t('common.loading')}
					</div>
				{:else if !browseData || browseData.entries.length === 0}
					<div
						class="flex flex-col items-center justify-center gap-1 py-10 text-center text-xs text-muted-foreground"
					>
						<FolderOpen class="h-6 w-6 opacity-50" />
						<p>{t('myaaw.workspace.dir_empty_hint')}</p>
					</div>
				{:else}
					<div class="divide-y">
						{#each browseData.entries as entry (entry.path)}
							{@const Icon = entryIcon(entry)}
							<div
								class="group flex items-center gap-2 px-2.5 py-2 text-xs transition-colors hover:bg-muted/60"
							>
								<button
									type="button"
									class="flex min-w-0 flex-1 items-center gap-2 text-left"
									onclick={() => void loadDir(entry.path)}
								>
									<Icon
										class={entry.is_git
											? 'h-4 w-4 shrink-0 text-primary'
											: 'h-4 w-4 shrink-0 text-muted-foreground'}
									/>
									<span class="truncate font-medium">{entry.name}</span>
									{#if entry.is_git}
										<Badge variant="secondary" class="shrink-0 px-1.5 py-0 text-[10px]">Git</Badge>
									{/if}
									{#if entry.is_empty}
										<Badge variant="outline" class="shrink-0 px-1.5 py-0 text-[10px]">
											{t('myaaw.workspace.empty_badge')}
										</Badge>
									{/if}
									{#each (entry.markers ?? [])
										.filter((m) => m !== 'git')
										.slice(0, 2) as marker (marker)}
										<Badge
											variant="outline"
											class="hidden shrink-0 px-1.5 py-0 text-[10px] sm:inline-flex"
										>
											{marker}
										</Badge>
									{/each}
								</button>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									class="h-6 shrink-0 px-2 text-[11px] opacity-80 group-hover:opacity-100"
									disabled={submitting}
									onclick={() => void attachPath(entry.path)}
								>
									{t('myaaw.workspace.select')}
								</Button>
								<button
									type="button"
									class="grid h-6 w-6 shrink-0 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
									onclick={() => void loadDir(entry.path)}
									aria-label={entry.name}
								>
									<ChevronRight class="h-3.5 w-3.5" />
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<Dialog.Footer class="flex flex-wrap items-center justify-between gap-2 sm:justify-between">
				<div class="flex items-center gap-2">
					{#if currentPath && onDetach}
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={submitting}
							onclick={() => void handleDetach()}
						>
							<Unlink class="h-3.5 w-3.5" />
							{t('myaaw.workspace.detach')}
						</Button>
					{/if}
				</div>
				<div class="flex items-center gap-2">
					<Button type="button" variant="ghost" size="sm" onclick={() => (open = false)}>
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
