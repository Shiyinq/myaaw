<script lang="ts">
	import { Bot, FolderGit2, FolderOpen, MessageSquarePlus, Paperclip } from 'lucide-svelte';
	import type { ChatRightPanelKind } from '$lib/stores/projectWorkspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { formatShortcut } from '$lib/utils/keys';
	import { cn } from '$lib/utils';

	interface Props {
		/** Active panel currently open in the unified right sidebar, or null when hidden. */
		activePanel: ChatRightPanelKind | null;
		/** Show Bot Info & New Chat actions (false in Classic mode where the left sidebar owns them). */
		showInfoAndNewChat?: boolean;
		/** Show Project toggle (hidden on empty state when no project is attached yet). */
		showProject?: boolean;
		hasProject?: boolean;
		hasGit?: boolean;
		workingDir?: string;
		attachmentCount?: number;
		onToggleInfo: () => void;
		onToggleProject: () => void;
		onToggleAttachments: () => void;
		onNewChat: () => void;
	}

	let {
		activePanel,
		showInfoAndNewChat = true,
		showProject = false,
		hasProject = false,
		hasGit = false,
		workingDir = '',
		attachmentCount = 0,
		onToggleInfo,
		onToggleProject,
		onToggleAttachments,
		onNewChat
	}: Props = $props();

	const { t } = useTranslation();
	const attachmentsActive = $derived(activePanel === 'attachments');
</script>

<nav
	aria-label={t('myaaw.chat.bot_info')}
	class="relative z-40 hidden h-full w-12 shrink-0 flex-col items-center gap-1 border-l bg-card/70 py-2.5 lg:flex"
>
	{#if showInfoAndNewChat}
		{@const active = activePanel === 'info'}
		<button
			type="button"
			class={cn(
				'relative grid h-9 w-9 place-items-center rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none',
				active
					? 'bg-muted text-foreground ring-1 ring-border/70'
					: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
			)}
			onclick={onToggleInfo}
			title={`${t('myaaw.chat.bot_info')} (${formatShortcut('mod+shift+h')})`}
			aria-label={t('myaaw.chat.bot_info')}
			aria-pressed={active}
			aria-keyshortcuts="Control+Shift+H Meta+Shift+H"
		>
			{#if active}
				<span
					aria-hidden="true"
					class="pointer-events-none absolute top-1.5 -right-1.5 bottom-1.5 w-0.5 rounded-full bg-primary"
				></span>
			{/if}
			<Bot class="h-[18px] w-[18px]" />
		</button>
	{/if}

	{#if showProject}
		{@const active = activePanel === 'project' && hasProject}
		<button
			type="button"
			class={cn(
				'relative grid h-9 w-9 place-items-center rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none',
				active
					? 'bg-muted text-foreground ring-1 ring-border/70'
					: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
			)}
			onclick={onToggleProject}
			title={workingDir
				? `${t('myaaw.workspace.tab_project')}: ${workingDir}`
				: t('myaaw.workspace.attach_tooltip')}
			aria-label={t('myaaw.workspace.tab_project')}
			aria-pressed={active}
		>
			{#if active}
				<span
					aria-hidden="true"
					class="pointer-events-none absolute top-1.5 -right-1.5 bottom-1.5 w-0.5 rounded-full bg-primary"
				></span>
			{/if}
			{#if hasGit}
				<FolderGit2 class={cn('h-[18px] w-[18px]', hasProject && !active && 'text-primary')} />
			{:else}
				<FolderOpen class={cn('h-[18px] w-[18px]', hasProject && !active && 'text-primary')} />
			{/if}
		</button>
	{/if}

	<button
		type="button"
		class={cn(
			'relative grid h-9 w-9 place-items-center rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none',
			attachmentsActive
				? 'bg-muted text-foreground ring-1 ring-border/70'
				: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
		)}
		onclick={onToggleAttachments}
		title={`${t('myaaw.chat.attachments_open')} (${formatShortcut('mod+shift+a')})`}
		aria-label={t('myaaw.chat.attachments_open')}
		aria-pressed={attachmentsActive}
		aria-keyshortcuts="Control+Shift+A Meta+Shift+A"
	>
		{#if attachmentsActive}
			<span
				aria-hidden="true"
				class="pointer-events-none absolute top-1.5 -right-1.5 bottom-1.5 w-0.5 rounded-full bg-primary"
			></span>
		{/if}
		<Paperclip class="h-[18px] w-[18px]" />
		{#if attachmentCount > 0}
			<span
				class="absolute top-0.5 right-0.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[9px] leading-4 font-semibold text-primary-foreground tabular-nums"
			>
				{attachmentCount > 99 ? '99+' : attachmentCount}
			</span>
		{/if}
	</button>

	{#if showInfoAndNewChat}
		<div class="my-0.5 h-px w-5 bg-border/60" role="separator"></div>
		<button
			type="button"
			class="grid h-9 w-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
			onclick={onNewChat}
			title={t('myaaw.chat.new')}
			aria-label={t('myaaw.chat.new')}
		>
			<MessageSquarePlus class="h-[18px] w-[18px]" />
		</button>
	{/if}
</nav>
