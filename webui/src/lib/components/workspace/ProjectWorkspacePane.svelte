<script lang="ts">
	import FileViewerPane from '$lib/components/files/FileViewerPane.svelte';
	import GitDiffViewer from './GitDiffViewer.svelte';
	import GitMultiDiffViewer from './GitMultiDiffViewer.svelte';
	import type { ProjectWorkspaceTab } from '$lib/types';

	interface Props {
		agent: string;
		conversationId: string;
		activeTab: ProjectWorkspaceTab | null;
		refreshKey?: number;
		onCloseTab: (id: string) => void;
		onOpenFile: (virtualPath: string) => void;
		onSaved?: () => void;
	}

	let {
		agent,
		conversationId,
		activeTab,
		refreshKey = 0,
		onCloseTab,
		onOpenFile,
		onSaved
	}: Props = $props();
</script>

{#if activeTab}
	<div class="min-h-0 flex-1 bg-background">
		{#if activeTab.kind === 'file'}
			{#key activeTab.id}
				<FileViewerPane
					{agent}
					path={activeTab.path}
					{conversationId}
					embedded
					onClose={() => onCloseTab(activeTab.id)}
					{onSaved}
				/>
			{/key}
		{:else if activeTab.kind === 'multi-diff' && conversationId}
			{#key activeTab.id}
				<GitMultiDiffViewer
					{agent}
					{conversationId}
					scope={activeTab.scope}
					commit={activeTab.commit}
					{refreshKey}
					embedded
					onClose={() => onCloseTab(activeTab.id)}
					{onOpenFile}
				/>
			{/key}
		{:else if activeTab.kind === 'diff' && conversationId}
			{#key activeTab.id}
				<GitDiffViewer
					{agent}
					{conversationId}
					path={activeTab.path}
					staged={activeTab.staged}
					untracked={activeTab.untracked}
					commit={activeTab.commit}
					embedded
					onClose={() => onCloseTab(activeTab.id)}
					{onOpenFile}
				/>
			{/key}
		{/if}
	</div>
{/if}
