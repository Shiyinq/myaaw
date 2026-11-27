<script lang="ts">
	import { FolderGit2, FolderOpen } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		workingDir: string;
		workingDirName: string;
		hasGit?: boolean;
		onOpenPicker: () => void;
		onOpenProject?: () => void;
	}

	let { workingDir, workingDirName, hasGit = false, onOpenPicker, onOpenProject }: Props = $props();

	const { t } = useTranslation();
</script>

<div class="mt-4 flex max-w-sm flex-col items-center gap-1.5">
	{#if workingDir}
		<button
			type="button"
			class="flex max-w-full items-center gap-2 rounded-full border border-primary/25 bg-primary/5 px-3 py-1 text-xs transition-colors hover:bg-primary/10"
			onclick={() => onOpenProject?.()}
			title={workingDir}
		>
			{#if hasGit}
				<FolderGit2 class="h-3.5 w-3.5 shrink-0 text-primary" />
			{:else}
				<FolderOpen class="h-3.5 w-3.5 shrink-0 text-primary" />
			{/if}
			<span class="max-w-52 truncate font-medium text-foreground">
				{workingDirName}
			</span>
		</button>
		<p
			class="max-w-full truncate font-mono text-[11px] text-muted-foreground/80"
			title={workingDir}
		>
			{workingDir}
		</p>
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
	{/if}
</div>
