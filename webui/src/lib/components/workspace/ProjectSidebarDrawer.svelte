<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ProjectSidebarTab } from '$lib/stores/projectWorkspace.svelte';
	import ProjectSidebarPanel from './ProjectSidebarPanel.svelte';
	import type { ProjectDiffTarget, ProjectMultiDiffTarget } from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		activeConversationId?: string;
		workingDir: string;
		workingDirExists?: boolean;
		hasGit?: boolean;
		refreshKey?: number;
		open: boolean;
		focusMode?: boolean;
		terminalOpen?: boolean;
		activeFilePath?: string;
		subTab?: ProjectSidebarTab;
		onSubTabChange?: (tab: ProjectSidebarTab) => void;
		onOpenFile: (virtualPath: string) => void;
		onOpenDiff: (req: ProjectDiffTarget) => void;
		onOpenMultiDiff: (req: ProjectMultiDiffTarget) => void;
		onToggleTerminal: () => void;
		onChangeWorkingDir?: () => void;
		onSelectConversation?: (id: string) => void;
		onNewChatInProject?: () => void;
		onFileRenamed?: (fromPath: string, toPath: string) => void;
		onFileDeleted?: (deletedPath: string) => void;
		onClose: () => void;
	}

	let {
		agent,
		conversationId,
		activeConversationId = '',
		workingDir,
		workingDirExists = true,
		hasGit = false,
		refreshKey = 0,
		open,
		focusMode = false,
		terminalOpen = false,
		activeFilePath = '',
		subTab,
		onSubTabChange,
		onOpenFile,
		onOpenDiff,
		onOpenMultiDiff,
		onToggleTerminal,
		onChangeWorkingDir,
		onSelectConversation,
		onNewChatInProject,
		onFileRenamed,
		onFileDeleted,
		onClose
	}: Props = $props();

	const { t } = useTranslation();
</script>

{#if agent && workingDir}
	<!-- Dedicated Project sidebar: completely independent from the main agent /
	     history / attachments sidebar on the right. -->
	{#if open}
		<button
			type="button"
			class="fixed inset-0 z-40 bg-black/40 lg:hidden"
			aria-label={t('common.close')}
			onclick={onClose}
		></button>
	{/if}
	<aside
		inert={!open}
		class={[
			'fixed inset-y-0 right-0 z-50 w-full flex-col overflow-hidden border-l bg-card lg:static lg:z-auto lg:h-full lg:max-w-none frost-drawer lg:transition-[width] lg:duration-200',
			open ? 'flex lg:w-80' : 'hidden lg:flex lg:w-0 lg:border-l-0'
		]}
	>
		<div
			class={[
				'flex h-full w-full flex-col lg:w-80 lg:min-w-80',
				focusMode ? 'lg:pt-16' : 'lg:pt-14'
			]}
		>
			<ProjectSidebarPanel
				{agent}
				{conversationId}
				{activeConversationId}
				{workingDir}
				{workingDirExists}
				{hasGit}
				{refreshKey}
				{terminalOpen}
				{activeFilePath}
				{subTab}
				{onSubTabChange}
				{onOpenFile}
				{onOpenDiff}
				{onOpenMultiDiff}
				{onToggleTerminal}
				{onChangeWorkingDir}
				{onSelectConversation}
				{onNewChatInProject}
				{onFileRenamed}
				{onFileDeleted}
				{onClose}
			/>
		</div>
	</aside>
{/if}
