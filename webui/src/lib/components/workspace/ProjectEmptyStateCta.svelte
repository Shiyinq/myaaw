<script lang="ts">
	import { FolderGit2, FolderOpen, GitBranch, History, MessageSquare, Unlink } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ConversationProject } from '$lib/types';

	interface Props {
		workingDir: string;
		workingDirName: string;
		hasGit?: boolean;
		recentProjects?: ConversationProject[];
		onOpenPicker: () => void;
		onSelectRecent?: (project: ConversationProject) => void;
		onOpenProject?: () => void;
		onStartWithoutProject?: () => void;
	}

	let {
		workingDir,
		workingDirName,
		hasGit = false,
		recentProjects = [],
		onOpenPicker,
		onSelectRecent,
		onOpenProject,
		onStartWithoutProject
	}: Props = $props();

	const { t } = useTranslation();

	const visibleRecents = $derived(recentProjects.filter((proj) => proj.exists).slice(0, 6));

	function shortenPath(fullPath: string): string {
		const parts = fullPath.replace(/\/+$/, '').split('/').filter(Boolean);
		if (parts.length <= 3) return fullPath;
		return `…/${parts.slice(-3).join('/')}`;
	}
</script>

<div class="mt-4 flex w-full max-w-md flex-col items-center gap-2.5 px-2">
	{#if workingDir}
		<div
			class="flex w-full max-w-sm flex-col items-center gap-2 rounded-2xl border border-primary/20 bg-primary/5 px-4 py-3 text-center"
		>
			<div class="flex items-center gap-1.5 text-[11px] font-medium text-primary">
				{#if hasGit}
					<FolderGit2 class="h-3.5 w-3.5 shrink-0" />
				{:else}
					<FolderOpen class="h-3.5 w-3.5 shrink-0" />
				{/if}
				<span>{t('myaaw.workspace.new_chat_bound_hint')}</span>
			</div>

			<button
				type="button"
				class="flex max-w-full flex-col items-center gap-0.5 rounded-xl px-2.5 py-1 transition-colors hover:bg-primary/10"
				onclick={() => onOpenProject?.()}
				title={workingDir}
			>
				<span class="max-w-full truncate text-sm font-semibold text-foreground">
					{workingDirName}
				</span>
				<span class="max-w-full truncate font-mono text-[11px] text-muted-foreground">
					{workingDir}
				</span>
			</button>

			{#if onStartWithoutProject}
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-full border border-border/70 bg-background/80 px-2.5 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={onStartWithoutProject}
				>
					<Unlink class="h-3 w-3" />
					<span>{t('myaaw.workspace.start_without_project')}</span>
				</button>
			{/if}
		</div>
	{:else}
		<button
			type="button"
			class="inline-flex items-center gap-2 rounded-full border bg-card/80 px-3.5 py-1.5 text-xs font-medium text-foreground shadow-2xs transition-colors hover:bg-muted"
			onclick={onOpenPicker}
		>
			<FolderOpen class="h-3.5 w-3.5 text-muted-foreground" />
			<span>{t('myaaw.workspace.empty_attach_cta')}</span>
			<span
				class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-normal text-muted-foreground"
			>
				{t('myaaw.workspace.optional_badge')}
			</span>
		</button>
		<p class="text-center text-[11px] text-muted-foreground/75">
			{t('myaaw.workspace.empty_attach_hint')}
		</p>

		{#if visibleRecents.length > 0 && onSelectRecent}
			<div class="mt-2 w-full space-y-1.5">
				<div
					class="flex items-center justify-center gap-1.5 text-[10px] font-medium tracking-wide text-muted-foreground/80 uppercase"
				>
					<History class="h-3 w-3" />
					<span>{t('myaaw.workspace.recent_projects')}</span>
				</div>
				<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
					{#each visibleRecents as proj (proj.working_dir)}
						<button
							type="button"
							class="group flex min-w-0 items-center gap-2 rounded-xl border bg-card/70 px-2.5 py-2 text-left transition-colors hover:border-primary/40 hover:bg-muted/60"
							onclick={() => onSelectRecent(proj)}
							title={proj.working_dir}
						>
							{#if proj.has_git}
								<FolderGit2 class="h-4 w-4 shrink-0 text-primary" />
							{:else}
								<FolderOpen class="h-4 w-4 shrink-0 text-muted-foreground" />
							{/if}
							<span class="min-w-0 flex-1">
								<span class="flex min-w-0 items-center gap-1">
									<span class="truncate text-xs font-medium text-foreground">
										{proj.name}
									</span>
									{#if proj.git_branch}
										<span
											class="inline-flex max-w-[50%] min-w-0 shrink items-center gap-0.5 rounded-full bg-muted/80 px-1.5 py-0 font-mono text-[9.5px] text-muted-foreground"
											title={proj.git_branch}
										>
											<GitBranch class="h-2.5 w-2.5 shrink-0" />
											<span class="min-w-0 truncate">{proj.git_branch}</span>
										</span>
									{/if}
								</span>
								<span class="block truncate font-mono text-[10px] text-muted-foreground">
									{shortenPath(proj.working_dir)}
								</span>
							</span>
							<span
								class="inline-flex shrink-0 items-center gap-0.5 rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground tabular-nums"
							>
								<MessageSquare class="h-2.5 w-2.5" />
								{proj.conversation_count}
							</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</div>
