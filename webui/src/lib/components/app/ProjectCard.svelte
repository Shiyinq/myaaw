<script lang="ts">
	import { FolderOpen } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import ProjectDrawer from '$lib/components/app/ProjectDrawer.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		agent: string;
		path: string;
		name: string;
		projectType?: string;
		size?: number;
		/** `card` is the chat bubble; `row` is a compact attachment-list entry. */
		variant?: 'card' | 'row';
	}

	let { agent, path, name, projectType = '', size, variant = 'card' }: Props = $props();

	const { t } = useTranslation();

	let open = $state(false);

	function typeLabel(value: string): string {
		switch (value) {
			case 'static':
				return t('myaaw.chat.project_type_static');
			case 'react':
				return t('myaaw.chat.project_type_react');
			case 'svelte':
				return t('myaaw.chat.project_type_svelte');
			case 'next':
				return t('myaaw.chat.project_type_next');
			case 'vite':
				return t('myaaw.chat.project_type_vite');
			default:
				return t('myaaw.chat.project_type_source');
		}
	}

	function sizeLabel(bytes?: number): string {
		if (!bytes || bytes <= 0) return '';
		const units = ['B', 'KB', 'MB', 'GB'];
		let value = bytes;
		let unit = 0;
		while (value >= 1024 && unit < units.length - 1) {
			value /= 1024;
			unit += 1;
		}
		return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
	}
</script>

{#if variant === 'row'}
	<button
		type="button"
		class="flex min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2 py-1.5 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
		title={path}
		onclick={() => (open = true)}
	>
		<span
			class="grid h-10 w-10 shrink-0 place-items-center rounded-md border bg-primary/10 text-primary"
		>
			<FolderOpen class="h-4 w-4" />
		</span>
		<span class="min-w-0 flex-1">
			<span class="block truncate text-xs font-medium">{name}</span>
			<span class="mt-0.5 flex items-center gap-1.5 text-[10px] text-muted-foreground">
				<span
					class="shrink-0 rounded-full border border-primary/30 bg-primary/10 px-1.5 py-px font-medium text-primary"
				>
					{t('myaaw.chat.project_open')}
				</span>
				{#if projectType}
					<span class="shrink-0">{typeLabel(projectType)}</span>
				{/if}
				{#if sizeLabel(size)}
					<span class="shrink-0">{sizeLabel(size)}</span>
				{/if}
			</span>
		</span>
	</button>
{:else}
	<div class="flex max-w-md items-center gap-3 rounded-xl border bg-card px-3 py-2.5">
		<span class="grid size-10 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
			<FolderOpen class="h-5 w-5" />
		</span>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium">{name}</p>
			<p class="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
				{#if projectType}
					<span
						class="rounded-full border border-primary/30 bg-primary/10 px-1.5 py-px font-medium text-primary"
					>
						{typeLabel(projectType)}
					</span>
				{/if}
				{#if sizeLabel(size)}
					<span>{sizeLabel(size)}</span>
				{/if}
				<span class="truncate">{path}</span>
			</p>
		</div>
		<Button variant="outline" size="sm" onclick={() => (open = true)}>
			<FolderOpen class="mr-1.5 h-3.5 w-3.5" />
			{t('myaaw.chat.project_open')}
		</Button>
	</div>
{/if}

{#if open}
	<ProjectDrawer {agent} {path} {name} {projectType} onClose={() => (open = false)} />
{/if}
