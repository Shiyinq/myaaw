<script lang="ts">
	import {
		Check,
		ChevronDown,
		ChevronRight,
		ChevronsDownUp,
		Copy,
		File as FileIcon,
		FileCode2,
		FilePlus2,
		Folder,
		FolderOpen,
		FolderPlus,
		Loader2,
		Pencil,
		Search,
		Trash2,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { filesStore } from '$lib/stores/files.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { buildGitStatusMaps, gitStatusLetter, gitStatusTextClass } from './gitUi';
	import type { FileEntry, GitStatusResponse } from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		childrenMap: Record<string, FileEntry[]>;
		expanded: Record<string, boolean>;
		loadingDir: Record<string, boolean>;
		filesError?: string;
		gitStatus: GitStatusResponse | null;
		activeFilePath?: string;
		onToggleDir: (entry: FileEntry) => void;
		onCollapseAll: () => void;
		onOpenFile: (virtualPath: string) => void;
		onRefresh: () => Promise<void> | void;
		onFileRenamed?: (fromPath: string, toPath: string) => void;
		onFileDeleted?: (deletedPath: string) => void;
	}

	let {
		agent,
		conversationId,
		childrenMap,
		expanded,
		loadingDir,
		filesError = '',
		gitStatus,
		activeFilePath = '',
		onToggleDir,
		onCollapseAll,
		onOpenFile,
		onRefresh,
		onFileRenamed,
		onFileDeleted
	}: Props = $props();

	const { t } = useTranslation();

	let filter = $state('');
	let creatingKind = $state<'file' | 'folder' | null>(null);
	let createName = $state('');
	let creating = $state(false);

	let renamingPath = $state('');
	let renameDraft = $state('');
	let renaming = $state(false);

	const gitMaps = $derived(buildGitStatusMaps(gitStatus));
	const hasExpandedSubdirs = $derived(
		Object.entries(expanded).some(([dir, open]) => dir !== '/project' && open)
	);

	interface FlatRow {
		entry: FileEntry;
		depth: number;
	}

	const flatRows = $derived.by(() => {
		const needle = filter.trim().toLowerCase();
		const rows: FlatRow[] = [];
		const walk = (dirPath: string, depth: number) => {
			const entries = childrenMap[dirPath] ?? [];
			for (const entry of entries) {
				if (entry.name === '.git') continue;
				const matches = !needle || entry.name.toLowerCase().includes(needle);
				if (entry.is_dir) {
					const isOpen = Boolean(expanded[entry.path]);
					const childStart = rows.length;
					if (matches) {
						rows.push({ entry, depth });
					}
					if (isOpen) {
						walk(entry.path, depth + 1);
					}
					if (!matches && rows.length > childStart) {
						rows.splice(childStart, 0, { entry, depth });
					}
				} else if (matches) {
					rows.push({ entry, depth });
				}
			}
		};
		walk('/project', 0);
		return rows;
	});

	function isCodeFile(name: string): boolean {
		return /\.(ts|tsx|js|jsx|svelte|go|py|rs|java|kt|c|cpp|h|css|scss|html|json|yaml|yml|toml|sql|sh)$/i.test(
			name
		);
	}

	function parentDirOf(virtualPath: string): string {
		const idx = virtualPath.lastIndexOf('/');
		return idx <= 0 ? '/project' : virtualPath.slice(0, idx);
	}

	async function handleCreate(event: SubmitEvent) {
		event.preventDefault();
		const rel = createName.trim().replace(/^\/+/, '');
		if (!rel || !creatingKind || !agent || !conversationId) return;
		creating = true;
		try {
			const virtualPath = `/project/${rel}`;
			if (creatingKind === 'folder') {
				await filesStore.mkdir(agent, virtualPath, '/project', conversationId);
			} else {
				await filesStore.write(agent, virtualPath, '', '/project', conversationId);
				onOpenFile(virtualPath);
			}
			creatingKind = null;
			createName = '';
			await onRefresh();
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			creating = false;
		}
	}

	function startRename(entry: FileEntry) {
		renamingPath = entry.path;
		renameDraft = entry.name;
	}

	function cancelRename() {
		renamingPath = '';
		renameDraft = '';
	}

	async function commitRename(entry: FileEntry) {
		const nextName = renameDraft.trim().replace(/^\/+|\/+$/g, '');
		if (!nextName || nextName === entry.name || nextName.includes('/')) {
			cancelRename();
			return;
		}
		const targetPath = `${parentDirOf(entry.path)}/${nextName}`;
		renaming = true;
		try {
			await filesStore.rename(
				agent,
				entry.path,
				targetPath,
				parentDirOf(entry.path),
				conversationId
			);
			toastStore.success(t('myaaw.workspace.renamed'));
			onFileRenamed?.(entry.path, targetPath);
			cancelRename();
			await onRefresh();
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			renaming = false;
		}
	}

	async function handleDelete(entry: FileEntry) {
		const rel = entry.path.replace(/^\/project\/?/, '') || entry.name;
		const confirmed = await askConfirm({
			title: t('myaaw.workspace.delete_confirm_title', { name: entry.name }),
			message: entry.is_dir
				? t('myaaw.workspace.delete_confirm_folder', { path: rel })
				: t('myaaw.workspace.delete_confirm_file', { path: rel }),
			confirmText: t('myaaw.workspace.delete'),
			variant: 'destructive'
		});
		if (!confirmed) return;

		try {
			await filesStore.remove(
				agent,
				entry.path,
				parentDirOf(entry.path),
				entry.is_dir,
				conversationId
			);
			toastStore.success(t('myaaw.workspace.deleted'));
			onFileDeleted?.(entry.path);
			await onRefresh();
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function copyRelativePath(entry: FileEntry) {
		const rel = entry.path.replace(/^\/project\/?/, '') || entry.name;
		try {
			await navigator.clipboard.writeText(rel);
			toastStore.success(t('myaaw.workspace.path_copied'));
		} catch {
			// Ignore clipboard failures in restricted contexts.
		}
	}
</script>

<div class="flex shrink-0 items-center gap-1 border-b px-2.5 py-1.5">
	<div class="relative flex-1">
		<Search
			class="pointer-events-none absolute top-1/2 left-2 h-3 w-3 -translate-y-1/2 text-muted-foreground"
		/>
		<Input
			bind:value={filter}
			placeholder={t('myaaw.workspace.filter_files')}
			class="h-7 pl-6.5 text-xs"
		/>
	</div>
	<Button
		variant="ghost"
		size="icon-sm"
		onclick={() => {
			creatingKind = creatingKind === 'file' ? null : 'file';
			createName = '';
		}}
		title={t('myaaw.workspace.new_file')}
		aria-label={t('myaaw.workspace.new_file')}
	>
		<FilePlus2 class="h-3.5 w-3.5" />
	</Button>
	<Button
		variant="ghost"
		size="icon-sm"
		onclick={() => {
			creatingKind = creatingKind === 'folder' ? null : 'folder';
			createName = '';
		}}
		title={t('myaaw.workspace.new_folder')}
		aria-label={t('myaaw.workspace.new_folder')}
	>
		<FolderPlus class="h-3.5 w-3.5" />
	</Button>
	<Button
		variant="ghost"
		size="icon-sm"
		disabled={!hasExpandedSubdirs}
		onclick={onCollapseAll}
		title={t('myaaw.workspace.git_collapse_all')}
		aria-label={t('myaaw.workspace.git_collapse_all')}
	>
		<ChevronsDownUp class="h-3.5 w-3.5" />
	</Button>
</div>

{#if creatingKind}
	<form class="flex items-center gap-1 border-b bg-muted/30 px-2.5 py-1.5" onsubmit={handleCreate}>
		<Input
			bind:value={createName}
			placeholder={creatingKind === 'file'
				? t('myaaw.workspace.new_file_placeholder')
				: t('myaaw.workspace.new_folder_placeholder')}
			class="h-7 flex-1 text-xs"
			autofocus
		/>
		<Button
			type="submit"
			size="sm"
			class="h-7 px-2 text-xs"
			disabled={creating || !createName.trim()}
		>
			{t('common.create')}
		</Button>
	</form>
{/if}

<div class="min-h-0 flex-1 overflow-y-auto px-1.5 py-1.5">
	{#if filesError}
		<p class="p-2 text-xs text-destructive">{filesError}</p>
	{:else if loadingDir['/project'] && flatRows.length === 0}
		<div class="flex items-center gap-2 p-3 text-xs text-muted-foreground">
			<Loader2 class="h-3.5 w-3.5 animate-spin" />
			{t('common.loading')}
		</div>
	{:else if flatRows.length === 0}
		<p class="p-3 text-center text-xs text-muted-foreground">
			{filter.trim() ? t('myaaw.workspace.no_matches') : t('myaaw.workspace.dir_empty_hint')}
		</p>
	{:else}
		<div class="space-y-0.5">
			{#each flatRows as row (row.entry.path)}
				{@const fileBadge = row.entry.is_dir ? undefined : gitMaps.files[row.entry.path]}
				{@const dirCode = row.entry.is_dir ? gitMaps.dirs[row.entry.path] : undefined}
				{@const isActive = !row.entry.is_dir && activeFilePath === row.entry.path}
				{@const isRenaming = renamingPath === row.entry.path}

				{#if isRenaming}
					<form
						class="flex items-center gap-1 rounded-md bg-muted/60 py-0.5 pr-1"
						style:padding-left="{8 + row.depth * 14}px"
						onsubmit={(e) => {
							e.preventDefault();
							void commitRename(row.entry);
						}}
					>
						<Input
							bind:value={renameDraft}
							placeholder={t('myaaw.workspace.rename_placeholder')}
							class="h-6 flex-1 px-1.5 text-xs"
							autofocus
							onkeydown={(e) => {
								if (e.key === 'Escape') {
									e.preventDefault();
									cancelRename();
								}
							}}
						/>
						<button
							type="submit"
							disabled={renaming}
							class="grid h-6 w-6 shrink-0 place-items-center rounded text-emerald-600 hover:bg-muted dark:text-emerald-400"
							aria-label={t('common.save')}
						>
							<Check class="h-3 w-3" />
						</button>
						<button
							type="button"
							onclick={cancelRename}
							class="grid h-6 w-6 shrink-0 place-items-center rounded text-muted-foreground hover:bg-muted"
							aria-label={t('common.cancel')}
						>
							<X class="h-3 w-3" />
						</button>
					</form>
				{:else}
					<div
						class={[
							'group/row flex items-center rounded-md pr-1 transition-colors hover:bg-muted/70',
							isActive && 'bg-primary/10 text-foreground'
						]}
					>
						<button
							type="button"
							class="flex min-w-0 flex-1 items-center gap-1.5 py-1 text-left text-xs"
							style:padding-left="{8 + row.depth * 14}px"
							onclick={() =>
								row.entry.is_dir ? onToggleDir(row.entry) : onOpenFile(row.entry.path)}
							title={row.entry.path.replace(/^\/project\/?/, '') || row.entry.name}
						>
							{#if row.entry.is_dir}
								{#if loadingDir[row.entry.path]}
									<Loader2 class="h-3 w-3 shrink-0 animate-spin text-muted-foreground" />
								{:else if expanded[row.entry.path]}
									<ChevronDown class="h-3 w-3 shrink-0 text-muted-foreground" />
								{:else}
									<ChevronRight class="h-3 w-3 shrink-0 text-muted-foreground" />
								{/if}
								{#if expanded[row.entry.path]}
									<FolderOpen class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{:else}
									<Folder class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{/if}
							{:else}
								<span class="w-3 shrink-0"></span>
								{#if isCodeFile(row.entry.name)}
									<FileCode2 class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{:else}
									<FileIcon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{/if}
							{/if}
							<span
								class={[
									'min-w-0 flex-1 truncate',
									fileBadge
										? gitStatusTextClass(fileBadge.code)
										: dirCode
											? gitStatusTextClass(dirCode)
											: ''
								]}
							>
								{row.entry.name}
							</span>
						</button>

						<div
							class="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100 [@media(hover:none)]:opacity-100"
						>
							<button
								type="button"
								class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground"
								onclick={() => void copyRelativePath(row.entry)}
								title={t('myaaw.workspace.copy_path')}
								aria-label={t('myaaw.workspace.copy_path')}
							>
								<Copy class="h-3 w-3" />
							</button>
							<button
								type="button"
								class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground"
								onclick={() => startRename(row.entry)}
								title={t('myaaw.workspace.rename')}
								aria-label={t('myaaw.workspace.rename')}
							>
								<Pencil class="h-3 w-3" />
							</button>
							<button
								type="button"
								class="grid h-5 w-5 place-items-center rounded text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
								onclick={() => void handleDelete(row.entry)}
								title={t('myaaw.workspace.delete')}
								aria-label={t('myaaw.workspace.delete')}
							>
								<Trash2 class="h-3 w-3" />
							</button>
						</div>

						{#if fileBadge}
							<span
								class={[
									'ml-1 w-4 shrink-0 text-center font-mono text-[10px] font-bold',
									gitStatusTextClass(fileBadge.code)
								]}
							>
								{gitStatusLetter(fileBadge.code)}
							</span>
						{:else if dirCode}
							<span
								class={[
									'ml-1 mr-1 h-1.5 w-1.5 shrink-0 rounded-full bg-current opacity-75',
									gitStatusTextClass(dirCode)
								]}
							></span>
						{/if}
					</div>
				{/if}
			{/each}
		</div>
	{/if}
</div>
