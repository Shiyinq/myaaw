<script lang="ts">
	import { FolderGit2, FolderOpen, SquareTerminal } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { cn } from '$lib/utils';

	interface Props {
		workingDir: string;
		workingDirName: string;
		hasGit?: boolean;
		terminalOpen?: boolean;
		onFolderClick: () => void;
		onToggleTerminal: () => void;
	}

	let {
		workingDir,
		workingDirName,
		hasGit = false,
		terminalOpen = false,
		onFolderClick,
		onToggleTerminal
	}: Props = $props();

	const { t } = useTranslation();
</script>

<button
	type="button"
	class={cn(
		'flex h-8 min-w-0 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium transition-colors focus-visible:outline-none',
		workingDir
			? 'bg-primary/10 text-primary hover:bg-primary/15'
			: 'text-muted-foreground hover:bg-muted hover:text-foreground'
	)}
	onclick={onFolderClick}
	title={workingDir || t('myaaw.workspace.attach_tooltip')}
	aria-label={workingDir
		? `${t('myaaw.workspace.tab_project')}: ${workingDirName}`
		: t('myaaw.workspace.attach_button')}
>
	{#if hasGit}
		<FolderGit2 class="h-3.5 w-3.5 shrink-0" />
	{:else}
		<FolderOpen class="h-3.5 w-3.5 shrink-0" />
	{/if}
	<span class="hidden max-w-28 truncate sm:inline">
		{workingDir ? workingDirName : t('myaaw.workspace.attach_short')}
	</span>
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
