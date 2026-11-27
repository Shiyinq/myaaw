<script lang="ts">
	import { untrack } from 'svelte';
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { FolderGit2, History, MessageSquarePlus } from 'lucide-svelte';
	import ConversationList from '$lib/components/app/ConversationList.svelte';
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarAgent from '$lib/components/app/SidebarAgent.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import ClassicAgentMenu from '$lib/components/classic/ClassicAgentMenu.svelte';
	import { ProjectHistoryList } from '$lib/components/workspace';
	import { Button } from '$lib/components/ui/button';
	import { agentSelection } from '$lib/stores';
	import { activeProjectScope } from '$lib/stores/projectWorkspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ConversationProject } from '$lib/types';

	const { t } = useTranslation();
	const CLASSIC_SIDEBAR_TAB_KEY = 'myaaw_classic_sidebar_tab';

	interface Props {
		/** Icon-only rail: a new-chat button, since the list needs its labels. */
		compact?: boolean;
		/** The conversation the page is showing, highlighted in the list. */
		activeId?: string;
		/** Lets the shell focus the search box from a keyboard shortcut. */
		searchInput?: HTMLInputElement | null;
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, activeId = '', searchInput = $bindable(null), onClose }: Props = $props();

	let sidebarTab = $state<'history' | 'projects'>(
		browser && localStorage.getItem(CLASSIC_SIDEBAR_TAB_KEY) === 'projects' ? 'projects' : 'history'
	);
	const tabIndex = $derived(sidebarTab === 'projects' ? 1 : 0);
	const activeWorkingDir = $derived(
		activeProjectScope.agent === agentSelection.selected ? activeProjectScope.workingDir : ''
	);

	$effect(() => {
		if (browser) localStorage.setItem(CLASSIC_SIDEBAR_TAB_KEY, sidebarTab);
	});

	// Swap the list for the compact new-chat button only after the sidebar width
	// animation finishes. Mounting the list while the rail is still narrow shows
	// clipped titles and looks like a glitch during expansion.
	let contentCompact = $state(untrack(() => compact));
	$effect(() => {
		const target = compact;
		if (target === contentCompact) return;
		const timer = setTimeout(() => (contentCompact = target), 200);
		return () => clearTimeout(timer);
	});

	function newChat() {
		void goto('/app/chat?new=1');
	}

	function openConversation(id: string) {
		void goto(`/app/chat/${id}`);
	}

	function newChatInFolder(folderId: string) {
		void goto(`/app/chat?new=1&folder=${encodeURIComponent(folderId)}`);
	}

	function newChatInProject(proj: ConversationProject) {
		const params = new URLSearchParams({
			new: '1',
			working_dir: proj.working_dir,
			has_git: proj.has_git ? '1' : '0'
		});
		if (proj.latest_conversation_id) {
			params.set('anchor', proj.latest_conversation_id);
		}
		void goto(`/app/chat?${params.toString()}`);
	}

	function attachProject() {
		void goto('/app/chat?new=1&attach=1');
	}
</script>

<!--
	Classic mode is the chat-first shell: the sidebar keeps the brand and the
	agent picker, then becomes the conversation list, so there is no second list
	to find and no navigation to learn.
-->
<SidebarBrand {compact} {onClose} />
<SidebarAgent {compact}>
	{#snippet action()}
		<ClassicAgentMenu onNavigate={onClose} side="bottom" align="end" />
	{/snippet}
</SidebarAgent>

{#if contentCompact}
	<div class="flex shrink-0 flex-col items-center gap-1 px-2 py-1">
		<Button
			variant="ghost"
			size="icon-sm"
			class="flex"
			onclick={newChat}
			title={t('myaaw.chat.new')}
			aria-label={t('myaaw.chat.new')}
		>
			<MessageSquarePlus class="h-4 w-4" />
		</Button>
		<ClassicAgentMenu onNavigate={onClose} side="right" align="start" />
	</div>
	<div class="flex-1"></div>
{:else}
	<div
		role="tablist"
		class="relative mx-3 mt-1 mb-1.5 grid shrink-0 grid-cols-2 rounded-lg border p-0.5"
	>
		<span
			aria-hidden="true"
			class="pointer-events-none absolute inset-y-0.5 left-0.5 w-[calc((100%-0.25rem)/2)] rounded-md bg-primary transition-transform duration-200 ease-out will-change-transform"
			style:transform={`translateX(${tabIndex * 100}%)`}
		></span>
		<button
			type="button"
			role="tab"
			aria-selected={sidebarTab === 'history'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1 text-[11px] font-medium select-none transition-colors duration-150',
				sidebarTab === 'history'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (sidebarTab = 'history')}
		>
			<History class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_history')}</span>
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={sidebarTab === 'projects'}
			class={[
				'relative z-10 flex items-center justify-center gap-1 rounded-md px-1.5 py-1 text-[11px] font-medium select-none transition-colors duration-150',
				sidebarTab === 'projects'
					? 'text-primary-foreground'
					: 'text-muted-foreground hover:text-foreground'
			]}
			onclick={() => (sidebarTab = 'projects')}
		>
			<FolderGit2 class="h-3.5 w-3.5 shrink-0" />
			<span class="truncate">{t('myaaw.focus.tab_projects')}</span>
		</button>
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
		{#if sidebarTab === 'history'}
			<ConversationList
				{activeId}
				onOpen={openConversation}
				onNew={newChat}
				onNewInFolder={newChatInFolder}
				onDeleted={(id) => {
					if (id === activeId) newChat();
				}}
				bind:searchInput
			/>
		{:else}
			<div class="flex min-h-0 flex-1 flex-col pt-1">
				<ProjectHistoryList
					agent={agentSelection.selected}
					{activeId}
					{activeWorkingDir}
					refreshKey={activeProjectScope.refreshSeq}
					onOpen={(id) => {
						openConversation(id);
						onClose();
					}}
					onNewInProject={(proj) => {
						newChatInProject(proj);
						onClose();
					}}
					onAttachProject={() => {
						attachProject();
						onClose();
					}}
				/>
			</div>
		{/if}
	</div>
{/if}

<SidebarAccount {compact} />
