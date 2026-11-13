<script lang="ts">
	import { onMount } from 'svelte';
	import { AlertTriangle, ExternalLink, FolderOpen, Loader2, RefreshCw, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { projects as projectsApi } from '$lib/apis/projects';
	import { getApiBase } from '$lib/constants';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ProjectStatus } from '$lib/types';

	interface Props {
		agent: string;
		path: string;
		name: string;
		projectType?: string;
		onClose: () => void;
	}

	let { agent, path, name, projectType = '', onClose }: Props = $props();

	const { t } = useTranslation();

	let status = $state<ProjectStatus | null>(null);
	let error = $state('');
	let loading = $state(true);
	let timer: ReturnType<typeof setTimeout> | undefined;
	let disposed = false;

	const type = $derived(status?.type || projectType);

	function buildable(value: string): boolean {
		return ['react', 'svelte', 'next', 'vite'].includes(value);
	}

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

	function absoluteUrl(url: string): string {
		if (/^https?:\/\//.test(url)) return url;
		return `${getApiBase()}${url}`;
	}

	async function refresh(autoBuild: boolean) {
		clearTimeout(timer);
		if (!status) loading = true;
		error = '';
		try {
			let next = await projectsApi.status(agent, path);
			// A built project has no preview until it is built once, so opening the
			// drawer starts the bounded build instead of showing an empty frame.
			if (autoBuild && next.state === 'idle' && buildable(next.type)) {
				next = await projectsApi.build(agent, path);
			}
			if (disposed) return;
			status = next;
			if (next.state === 'building') {
				timer = setTimeout(() => void refresh(false), 1500);
			}
		} catch (err) {
			if (disposed) return;
			error = getErrorMessage(err);
		} finally {
			if (!disposed) loading = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || event.defaultPrevented) return;
		event.preventDefault();
		onClose();
	}

	onMount(() => {
		const previousOverflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		void refresh(true);
		return () => {
			disposed = true;
			clearTimeout(timer);
			document.body.style.overflow = previousOverflow;
		};
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<div
	class="fixed inset-0 z-[70] flex flex-col bg-background"
	role="dialog"
	aria-modal="true"
	aria-label={name}
>
	<header class="flex shrink-0 items-center gap-2.5 border-b px-3 py-2">
		<span class="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground">
			<FolderOpen class="h-4 w-4" />
		</span>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium">{name}</p>
			<p class="flex items-center gap-1.5 truncate text-[11px] text-muted-foreground">
				{#if type}
					<span
						class="shrink-0 rounded-full border border-primary/30 bg-primary/10 px-1.5 py-px font-medium text-primary"
					>
						{typeLabel(type)}
					</span>
				{/if}
				{#if status?.state === 'building'}
					<span class="flex items-center gap-1">
						<Loader2 class="h-3 w-3 animate-spin" />
						{t('myaaw.chat.project_building')}
					</span>
				{:else if status?.state === 'failed'}
					<span class="text-destructive">{t('myaaw.chat.project_failed')}</span>
				{:else if status?.file_count}
					<span>{status.file_count} files</span>
				{/if}
				<span class="truncate">·</span>
				<span class="truncate">{path}</span>
			</p>
		</div>
		{#if status?.state === 'ready' && status.url}
			<a
				href={absoluteUrl(status.url)}
				target="_blank"
				rel="noopener noreferrer"
				class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors hover:bg-muted"
			>
				<ExternalLink class="h-3.5 w-3.5" />
				<span class="hidden sm:inline">{t('myaaw.chat.project_open_browser')}</span>
			</a>
		{/if}
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={onClose}
			aria-label={t('myaaw.chat.project_close')}
			title={t('myaaw.chat.project_close')}
		>
			<X class="h-4 w-4" />
		</Button>
	</header>

	<div class="min-h-0 flex-1 overflow-auto bg-muted/20">
		{#if loading}
			<div
				class="flex h-full min-h-48 flex-col items-center justify-center gap-2 text-muted-foreground"
			>
				<Loader2 class="h-5 w-5 animate-spin" />
				<p class="text-xs">{t('myaaw.chat.project_loading')}</p>
			</div>
		{:else if error}
			<div class="flex h-full min-h-48 flex-col items-center justify-center gap-3 p-6 text-center">
				<AlertTriangle class="h-6 w-6 text-destructive" />
				<p class="max-w-md text-sm text-destructive">{error}</p>
				<Button variant="outline" size="sm" onclick={() => void refresh(true)}>
					<RefreshCw class="mr-1.5 h-3.5 w-3.5" />
					{t('myaaw.chat.project_retry')}
				</Button>
			</div>
		{:else if status?.state === 'ready' && status.url}
			<iframe
				src={absoluteUrl(status.url)}
				title={name}
				sandbox="allow-scripts allow-forms allow-modals allow-popups"
				class="h-full min-h-96 w-full border-0 bg-white"
			></iframe>
		{:else if status?.state === 'building'}
			<div
				class="flex h-full min-h-48 flex-col items-center justify-center gap-2 text-muted-foreground"
			>
				<Loader2 class="h-5 w-5 animate-spin" />
				<p class="text-xs">{t('myaaw.chat.project_building')}</p>
				{#if status.logs}
					<pre
						class="mt-2 max-h-56 w-full max-w-2xl overflow-auto rounded-lg border bg-background p-3 text-left text-[11px] leading-relaxed whitespace-pre-wrap">{status.logs}</pre>
				{/if}
			</div>
		{:else}
			<div class="flex h-full min-h-48 flex-col items-center justify-center gap-3 p-6 text-center">
				<AlertTriangle class="h-6 w-6 text-muted-foreground" />
				<p class="max-w-md text-sm font-medium">
					{status?.error || t('myaaw.chat.project_download_only')}
				</p>
				{#if status?.logs}
					<div class="w-full max-w-2xl text-left">
						<p class="mb-1 text-[11px] font-medium text-muted-foreground">
							{t('myaaw.chat.project_logs')}
						</p>
						<pre
							class="max-h-64 overflow-auto rounded-lg border bg-background p-3 text-[11px] leading-relaxed whitespace-pre-wrap">{status.logs}</pre>
					</div>
				{/if}
				<Button variant="outline" size="sm" onclick={() => void refresh(true)}>
					<RefreshCw class="mr-1.5 h-3.5 w-3.5" />
					{t('myaaw.chat.project_retry')}
				</Button>
			</div>
		{/if}
	</div>
</div>
