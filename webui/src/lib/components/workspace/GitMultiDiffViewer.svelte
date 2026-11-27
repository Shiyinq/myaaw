<script lang="ts">
	import {
		ChevronDown,
		ChevronRight,
		Columns2,
		FileCode2,
		FileDiff,
		Loader2,
		Rows2,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { buildSplitDiffRows, parseUnifiedDiff } from './gitUi';
	import type { GitMultiDiffResponse } from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		scope: 'changes' | 'staged' | 'all' | 'commit';
		commit?: string;
		refreshKey?: number;
		embedded?: boolean;
		onClose: () => void;
		onOpenFile?: (virtualPath: string) => void;
	}

	let {
		agent,
		conversationId,
		scope,
		commit = '',
		refreshKey = 0,
		embedded = false,
		onClose,
		onOpenFile
	}: Props = $props();

	const { t } = useTranslation();

	let data = $state<GitMultiDiffResponse | null>(null);
	let loading = $state(false);
	let error = $state('');
	let viewMode = $state<'inline' | 'split'>('inline');
	let collapsed = $state<Record<string, boolean>>({});
	let loadKey = '';

	const totalAdditions = $derived(
		(data?.files ?? []).reduce((sum, f) => sum + (f.additions ?? 0), 0)
	);
	const totalDeletions = $derived(
		(data?.files ?? []).reduce((sum, f) => sum + (f.deletions ?? 0), 0)
	);
	const allCollapsed = $derived(
		(data?.files.length ?? 0) > 0 &&
			(data?.files ?? []).every((f) => Boolean(collapsed[fileKey(f.path, f.staged)]))
	);

	function fileKey(path: string, staged: boolean): string {
		return `${staged ? 's' : 'w'}:${path}`;
	}

	$effect(() => {
		const key = `${agent}:${conversationId}:${scope}:${commit}:${refreshKey}`;
		if (!agent || !conversationId || key === loadKey) return;
		loadKey = key;
		void load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			data = await workspaceStore.gitDiffs(agent, conversationId, {
				scope,
				commit: commit || undefined
			});
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	function toggleAll() {
		if (!data) return;
		const nextState = !allCollapsed;
		const map: Record<string, boolean> = {};
		for (const f of data.files) {
			map[fileKey(f.path, f.staged)] = nextState;
		}
		collapsed = map;
	}

	function toggleOne(key: string) {
		collapsed = { ...collapsed, [key]: !collapsed[key] };
	}

	const headerTitle = $derived(
		commit
			? `Commit ${commit.slice(0, 7)}`
			: scope === 'staged'
				? t('myaaw.workspace.git_staged')
				: t('myaaw.workspace.git_changes')
	);
</script>

<div class="flex h-full min-h-0 flex-col bg-background">
	<div class="flex min-h-12 shrink-0 items-center gap-2 border-b px-3 py-1.5">
		<span
			class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground"
		>
			<FileDiff class="h-3.5 w-3.5" />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<p class="truncate text-xs font-semibold">{headerTitle}</p>
				{#if data}
					<Badge variant="secondary" class="text-[10px] tabular-nums">
						{t('myaaw.workspace.git_files_count', { count: data.total })}
					</Badge>
				{/if}
			</div>
			{#if data && data.files.length > 0}
				<p class="flex items-center gap-2 font-mono text-[11px]">
					<span class="text-emerald-600 dark:text-emerald-400">+{totalAdditions}</span>
					<span class="text-rose-600 dark:text-rose-400">-{totalDeletions}</span>
					{#if data.truncated}
						<span class="text-muted-foreground">({t('myaaw.workspace.diff_truncated')})</span>
					{/if}
				</p>
			{/if}
		</div>

		{#if data && data.files.length > 1}
			<Button variant="ghost" size="sm" class="h-7 px-2 text-xs" onclick={toggleAll}>
				{allCollapsed ? t('myaaw.workspace.git_expand_all') : t('myaaw.workspace.git_collapse_all')}
			</Button>
		{/if}

		<div class="flex shrink-0 items-center rounded-md border bg-muted/40 p-0.5">
			<button
				type="button"
				class={[
					'inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium transition-colors',
					viewMode === 'inline'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				]}
				onclick={() => (viewMode = 'inline')}
				title={t('myaaw.workspace.diff_inline')}
			>
				<Rows2 class="h-3 w-3" />
				<span class="hidden sm:inline">{t('myaaw.workspace.diff_inline')}</span>
			</button>
			<button
				type="button"
				class={[
					'inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium transition-colors',
					viewMode === 'split'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				]}
				onclick={() => (viewMode = 'split')}
				title={t('myaaw.workspace.diff_split')}
			>
				<Columns2 class="h-3 w-3" />
				<span class="hidden sm:inline">{t('myaaw.workspace.diff_split')}</span>
			</button>
		</div>

		{#if !embedded}
			<Button variant="ghost" size="icon-sm" onclick={onClose} aria-label={t('common.close')}>
				<X class="h-4 w-4" />
			</Button>
		{/if}
	</div>

	<div class="min-h-0 flex-1 overflow-auto">
		{#if loading && !data}
			<div class="flex items-center gap-2 p-6 text-sm text-muted-foreground">
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if error}
			<div class="p-6 text-sm text-destructive">{error}</div>
		{:else if !data || data.files.length === 0}
			<div class="p-6 text-sm text-muted-foreground">{t('myaaw.workspace.diff_empty')}</div>
		{:else}
			<div class="divide-y">
				{#each data.files as file (fileKey(file.path, file.staged))}
					{@const key = fileKey(file.path, file.staged)}
					{@const isCollapsed = Boolean(collapsed[key])}
					{@const unifiedRows = isCollapsed ? [] : parseUnifiedDiff(file.diff ?? '')}
					{@const splitRows =
						!isCollapsed && viewMode === 'split' ? buildSplitDiffRows(unifiedRows) : []}
					<section class="bg-background">
						<div
							class="sticky top-0 z-10 flex items-center gap-2 border-b bg-muted/40 px-3 py-1.5 backdrop-blur"
						>
							<button
								type="button"
								class="flex min-w-0 flex-1 items-center gap-1.5 text-left"
								onclick={() => toggleOne(key)}
							>
								{#if isCollapsed}
									<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{:else}
									<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								{/if}
								<span
									class="min-w-0 flex-1 truncate font-mono text-xs font-medium"
									title={file.path}
								>
									{file.path}
								</span>
								{#if file.staged}
									<Badge variant="secondary" class="text-[10px]">
										{t('myaaw.workspace.git_staged')}
									</Badge>
								{:else if file.untracked}
									<Badge variant="outline" class="text-[10px]">
										{t('myaaw.workspace.git_untracked')}
									</Badge>
								{/if}
							</button>

							{#if !file.binary}
								<span class="shrink-0 font-mono text-[11px] tabular-nums">
									<span class="text-emerald-600 dark:text-emerald-400">+{file.additions}</span>
									<span class="ml-1 text-rose-600 dark:text-rose-400">-{file.deletions}</span>
								</span>
							{/if}

							{#if onOpenFile}
								<Button
									variant="ghost"
									size="icon-sm"
									class="h-6 w-6 shrink-0"
									onclick={() => onOpenFile(`/project/${file.path.replace(/^\/+/, '')}`)}
									title={t('myaaw.workspace.open_file')}
									aria-label={t('myaaw.workspace.open_file')}
								>
									<FileCode2 class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>

						{#if !isCollapsed}
							{#if file.binary}
								<p class="px-4 py-3 text-xs text-muted-foreground">
									{t('myaaw.workspace.binary_diff')}
								</p>
							{:else if !file.diff?.trim()}
								<p class="px-4 py-3 text-xs text-muted-foreground">
									{t('myaaw.workspace.diff_empty')}
								</p>
							{:else if viewMode === 'inline'}
								<div class="min-w-full py-1 font-mono text-xs leading-5">
									{#each unifiedRows as row, idx (idx)}
										{#if row.kind === 'meta'}
											<div class="bg-muted/30 px-3 py-0.5 font-semibold text-muted-foreground">
												{row.text}
											</div>
										{:else if row.kind === 'hunk'}
											<div
												class="border-y border-blue-500/15 bg-blue-500/10 px-3 py-0.5 text-blue-600 dark:text-blue-400"
											>
												{row.text}
											</div>
										{:else}
											<div
												class={[
													'grid grid-cols-[3rem_3rem_1.25rem_1fr] items-stretch',
													row.kind === 'add'
														? 'bg-emerald-500/12 text-emerald-800 dark:text-emerald-200'
														: row.kind === 'del'
															? 'bg-rose-500/12 text-rose-800 dark:text-rose-200'
															: 'text-foreground/85'
												]}
											>
												<span
													class="select-none border-r border-border/40 px-2 text-right text-[10px] text-muted-foreground/70 tabular-nums"
												>
													{row.oldLine ?? ''}
												</span>
												<span
													class="select-none border-r border-border/40 px-2 text-right text-[10px] text-muted-foreground/70 tabular-nums"
												>
													{row.newLine ?? ''}
												</span>
												<span class="select-none text-center font-semibold opacity-70">
													{row.kind === 'add' ? '+' : row.kind === 'del' ? '-' : ' '}
												</span>
												<span class="pr-3 whitespace-pre-wrap break-all">{row.text || ' '}</span>
											</div>
										{/if}
									{/each}
								</div>
							{:else}
								<div class="min-w-[640px] overflow-x-auto py-1 font-mono text-xs leading-5">
									{#each splitRows as row, idx (idx)}
										{#if row.kind === 'meta'}
											<div class="bg-muted/30 px-3 py-0.5 font-semibold text-muted-foreground">
												{row.metaText}
											</div>
										{:else if row.kind === 'hunk'}
											<div
												class="border-y border-blue-500/15 bg-blue-500/10 px-3 py-0.5 text-blue-600 dark:text-blue-400"
											>
												{row.metaText}
											</div>
										{:else}
											<div class="grid grid-cols-2 divide-x divide-border/50">
												<div
													class={[
														'grid grid-cols-[2.75rem_1fr]',
														row.leftKind === 'del'
															? 'bg-rose-500/12 text-rose-800 dark:text-rose-200'
															: row.leftKind === 'empty'
																? 'bg-muted/20'
																: 'text-foreground/85'
													]}
												>
													<span
														class="select-none border-r border-border/40 px-1.5 text-right text-[10px] text-muted-foreground/70 tabular-nums"
													>
														{row.leftLine ?? ''}
													</span>
													<span class="px-2 whitespace-pre-wrap break-all">
														{row.leftText || ' '}
													</span>
												</div>
												<div
													class={[
														'grid grid-cols-[2.75rem_1fr]',
														row.rightKind === 'add'
															? 'bg-emerald-500/12 text-emerald-800 dark:text-emerald-200'
															: row.rightKind === 'empty'
																? 'bg-muted/20'
																: 'text-foreground/85'
													]}
												>
													<span
														class="select-none border-r border-border/40 px-1.5 text-right text-[10px] text-muted-foreground/70 tabular-nums"
													>
														{row.rightLine ?? ''}
													</span>
													<span class="px-2 whitespace-pre-wrap break-all">
														{row.rightText || ' '}
													</span>
												</div>
											</div>
										{/if}
									{/each}
								</div>
							{/if}
						{/if}
					</section>
				{/each}
			</div>
		{/if}
	</div>
</div>
