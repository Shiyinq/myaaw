<script lang="ts">
	import { Columns2, FileCode2, GitCompare, Loader2, Rows2, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { buildSplitDiffRows, parseUnifiedDiff } from './gitUi';
	import type { GitDiffResponse } from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		path: string;
		staged?: boolean;
		untracked?: boolean;
		commit?: string;
		/** When hosted inside the multi-tab ProjectWorkspacePane, the tab bar handles top-bar spacing. */
		embedded?: boolean;
		onClose: () => void;
		onOpenFile?: (virtualPath: string) => void;
	}

	let {
		agent,
		conversationId,
		path,
		staged = false,
		untracked = false,
		commit = '',
		embedded = false,
		onClose,
		onOpenFile
	}: Props = $props();

	const { t } = useTranslation();

	let diffData = $state<GitDiffResponse | null>(null);
	let loading = $state(false);
	let error = $state('');
	let viewMode = $state<'inline' | 'split'>('inline');
	let loadKey = '';
	let gutter = $state(12);
	let root = $state<HTMLElement | null>(null);

	function measureGutter() {
		if (embedded || !root) {
			gutter = 12;
			return;
		}
		const bar = document.querySelector('header');
		const rootRect = root.getBoundingClientRect();
		if (!bar) {
			gutter = 12;
			return;
		}
		const barRect = bar.getBoundingClientRect();
		if (barRect.bottom <= rootRect.top + 1) {
			gutter = 12;
			return;
		}
		const edges = [...bar.querySelectorAll('button, a')]
			.map((el) => el.getBoundingClientRect())
			.filter((rect) => rect.width > 0 && rect.left > window.innerWidth / 2)
			.map((rect) => rect.left);
		const actionLeft = edges.length > 0 ? Math.min(...edges) : window.innerWidth;
		gutter = Math.max(12, Math.round(rootRect.right - actionLeft + 8));
	}

	$effect(() => {
		const el = root;
		if (!el || embedded) {
			gutter = 12;
			return;
		}
		measureGutter();
		const observer = new ResizeObserver(() => measureGutter());
		observer.observe(el);
		const bar = document.querySelector('header');
		if (bar) observer.observe(bar);
		window.addEventListener('resize', measureGutter);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measureGutter);
		};
	});

	$effect(() => {
		const key = `${agent}:${conversationId}:${path}:${staged}:${untracked}:${commit}`;
		if (!agent || !conversationId || !path || key === loadKey) return;
		loadKey = key;
		void load();
	});

	async function load() {
		loading = true;
		error = '';
		diffData = null;
		try {
			diffData = await workspaceStore.gitDiff(agent, conversationId, path, {
				staged,
				untracked,
				commit: commit || undefined
			});
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	const unifiedRows = $derived(parseUnifiedDiff(diffData?.diff ?? ''));
	const splitRows = $derived(viewMode === 'split' ? buildSplitDiffRows(unifiedRows) : []);

	function handleKeydown(event: KeyboardEvent) {
		if (!embedded && event.key === 'Escape' && !event.defaultPrevented) {
			event.preventDefault();
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div bind:this={root} class="flex h-full min-h-0 flex-col bg-background">
	<div
		class="flex min-h-12 shrink-0 items-center gap-2 border-b py-1.5 pl-3"
		style:padding-right="{gutter}px"
	>
		<span
			class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground"
		>
			<GitCompare class="h-3.5 w-3.5" />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<p class="truncate font-mono text-xs font-medium" title={path}>{path}</p>
				{#if commit}
					<Badge variant="secondary" class="font-mono text-[10px]">{commit.slice(0, 7)}</Badge>
				{:else if staged}
					<Badge variant="secondary" class="text-[10px]">{t('myaaw.workspace.git_staged')}</Badge>
				{:else if untracked}
					<Badge variant="outline" class="text-[10px]">{t('myaaw.workspace.git_untracked')}</Badge>
				{/if}
			</div>
			{#if diffData && !diffData.binary}
				<p class="flex items-center gap-2 font-mono text-[11px]">
					<span class="text-emerald-600 dark:text-emerald-400">+{diffData.additions}</span>
					<span class="text-rose-600 dark:text-rose-400">-{diffData.deletions}</span>
					{#if diffData.truncated}
						<span class="text-muted-foreground">({t('myaaw.workspace.diff_truncated')})</span>
					{/if}
				</p>
			{/if}
		</div>

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

		{#if onOpenFile}
			<Button
				variant="outline"
				size="sm"
				class="h-7 px-2 text-xs"
				onclick={() => onOpenFile(`/project/${path.replace(/^\/+/, '')}`)}
			>
				<FileCode2 class="h-3.5 w-3.5" />
				<span class="hidden sm:inline">{t('myaaw.workspace.open_file')}</span>
			</Button>
		{/if}
		{#if !embedded}
			<Button
				variant="ghost"
				size="icon-sm"
				onclick={onClose}
				title={t('common.close')}
				aria-label={t('common.close')}
			>
				<X class="h-4 w-4" />
			</Button>
		{/if}
	</div>

	<div class="min-h-0 flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center gap-2 p-6 text-sm text-muted-foreground">
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if error}
			<div class="p-6 text-sm text-destructive">{error}</div>
		{:else if diffData?.binary}
			<div class="p-6 text-sm text-muted-foreground">{t('myaaw.workspace.binary_diff')}</div>
		{:else if !diffData?.diff?.trim()}
			<div class="p-6 text-sm text-muted-foreground">{t('myaaw.workspace.diff_empty')}</div>
		{:else if viewMode === 'inline'}
			<div class="min-w-full py-1 font-mono text-xs leading-5">
				{#each unifiedRows as row, idx (idx)}
					{#if row.kind === 'meta'}
						<div class="bg-muted/40 px-3 py-0.5 font-semibold text-muted-foreground">
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
			<div class="min-w-[640px] py-1 font-mono text-xs leading-5">
				{#each splitRows as row, idx (idx)}
					{#if row.kind === 'meta'}
						<div class="bg-muted/40 px-3 py-0.5 font-semibold text-muted-foreground">
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
								<span class="px-2 whitespace-pre-wrap break-all">{row.leftText || ' '}</span>
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
								<span class="px-2 whitespace-pre-wrap break-all">{row.rightText || ' '}</span>
							</div>
						</div>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
</div>
