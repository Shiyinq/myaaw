<script lang="ts">
	import { FolderGit2, FolderOpen, GitBranch, SquareTerminal } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { cn } from '$lib/utils';

	interface Props {
		workingDir: string;
		workingDirName: string;
		hasGit?: boolean;
		gitBranch?: string;
		terminalOpen?: boolean;
		onFolderClick: () => void;
		onToggleTerminal: () => void;
	}

	let {
		workingDir,
		workingDirName,
		hasGit = false,
		gitBranch = '',
		terminalOpen = false,
		onFolderClick,
		onToggleTerminal
	}: Props = $props();

	const { t } = useTranslation();
</script>

<button
	type="button"
	class={cn(
		'flex h-8 w-8 shrink-0 items-center justify-center gap-1.5 rounded-full text-[11px] font-medium transition-colors focus-visible:outline-none @[33rem]:w-auto @[33rem]:px-2',
		workingDir
			? 'bg-primary/10 text-primary hover:bg-primary/15'
			: 'text-muted-foreground hover:bg-muted hover:text-foreground'
	)}
	onclick={onFolderClick}
	title={workingDir
		? gitBranch
			? `${workingDir} (${gitBranch})`
			: workingDir
		: t('myaaw.workspace.attach_tooltip')}
	aria-label={workingDir
		? `${t('myaaw.workspace.tab_project')}: ${workingDirName}${gitBranch ? ` (${gitBranch})` : ''}`
		: t('myaaw.workspace.attach_button')}
>
	{#if hasGit}
		<FolderGit2 class="h-3.5 w-3.5 shrink-0" />
	{:else}
		<FolderOpen class="h-3.5 w-3.5 shrink-0" />
	{/if}
	<span class="hidden truncate @[33rem]:inline @[33rem]:max-w-20 @[46rem]:max-w-28">
		{workingDir ? workingDirName : t('myaaw.workspace.attach_short')}
	</span>
	{#if workingDir && gitBranch}
		<span
			class="hidden min-w-0 items-center gap-0.5 rounded-full bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] leading-none font-normal text-primary/90 @[38rem]:inline-flex @[38rem]:max-w-20 @[46rem]:max-w-28"
		>
			<GitBranch class="h-2.5 w-2.5 shrink-0" />
			<span class="min-w-0 truncate">{gitBranch}</span>
		</span>
	{/if}
</button>
{#if workingDir}
	<Button
		type="button"
		size="icon-sm"
		variant={terminalOpen ? 'secondary' : 'ghost'}
		class="shrink-0 rounded-full"
		onclick={onToggleTerminal}
		title={t('myaaw.workspace.open_terminal')}
		aria-label={t('myaaw.workspace.open_terminal')}
		aria-pressed={terminalOpen}
	>
		<SquareTerminal class="h-3.5 w-3.5" />
	</Button>
{/if}
