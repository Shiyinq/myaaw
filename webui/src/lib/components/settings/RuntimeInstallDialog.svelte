<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { Check, Copy, Download, ExternalLink, Loader2, RefreshCw } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { runtimesManageStore } from '$lib/stores/runtimes.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ManagedRuntimeView, RuntimeInstallMethod } from '$lib/types';

	interface Props {
		open?: boolean;
		runtime: ManagedRuntimeView | null;
		mode?: 'install' | 'update';
		onCompleted?: (updated: ManagedRuntimeView) => void;
	}

	let { open = $bindable(false), runtime, mode = 'install', onCompleted }: Props = $props();

	const { t } = useTranslation();

	let selectedMethodId = $state('');
	let running = $state(false);
	let rechecking = $state(false);
	let finished = $state(false);
	let errorMessage = $state('');
	let logs = $state<string[]>([]);
	let copied = $state(false);
	let logContainer = $state<HTMLPreElement | null>(null);
	let abortController: AbortController | null = null;
	let wasOpen = false;

	const methods = $derived<RuntimeInstallMethod[]>(
		mode === 'update'
			? runtime?.update_method
				? [runtime.update_method]
				: []
			: (runtime?.install_methods ?? [])
	);

	const activeMethod = $derived<RuntimeInstallMethod | null>(
		methods.find((m) => m.id === selectedMethodId) ?? methods[0] ?? null
	);

	$effect(() => {
		if (open && !wasOpen) {
			wasOpen = true;
			running = false;
			finished = false;
			errorMessage = '';
			logs = [];
			copied = false;
			const preferred =
				methods.find((m) => m.available && m.recommended) ??
				methods.find((m) => m.available) ??
				methods[0];
			selectedMethodId = preferred?.id ?? '';
		} else if (!open && wasOpen) {
			wasOpen = false;
			abortController?.abort();
			abortController = null;
			running = false;
		}
	});

	onDestroy(() => {
		abortController?.abort();
	});

	async function appendLog(line: string) {
		logs = [...logs, line];
		await tick();
		if (logContainer) {
			logContainer.scrollTop = logContainer.scrollHeight;
		}
	}

	async function copyCommand() {
		if (!activeMethod?.command_preview || typeof navigator === 'undefined') return;
		try {
			await navigator.clipboard.writeText(activeMethod.command_preview);
			copied = true;
			setTimeout(() => {
				copied = false;
			}, 1500);
		} catch {
			// Ignore clipboard failures in restricted contexts.
		}
	}

	async function recheckInstalled() {
		if (!runtime || rechecking) return;
		rechecking = true;
		errorMessage = '';
		try {
			const updated = await runtimesManageStore.getOne(runtime.id);
			onCompleted?.(updated);
			if (updated.installed) {
				finished = true;
				toastStore.success(
					t('myaaw.settings.runtimes_install_success', {
						name: runtime.label,
						version: updated.version || t('myaaw.settings.runtimes_installed')
					})
				);
			}
		} catch (err) {
			errorMessage = getErrorMessage(err);
		} finally {
			rechecking = false;
		}
	}

	async function runJob() {
		if (!runtime || !activeMethod || running) return;
		running = true;
		finished = false;
		errorMessage = '';
		logs = [];

		const ctrl = new AbortController();
		abortController = ctrl;
		const targetId = runtime.id;
		const targetLabel = runtime.label;

		try {
			const handleEvent = (ev: { event: string; data: unknown }) => {
				if (ev.event === 'line') {
					const payload = ev.data as { line?: string } | null;
					if (payload?.line) void appendLog(payload.line);
				} else if (ev.event === 'error') {
					const payload = ev.data as { error?: string } | null;
					errorMessage = payload?.error || t('common.error');
				} else if (ev.event === 'done') {
					const updated = ev.data as ManagedRuntimeView;
					finished = true;
					onCompleted?.(updated);
					toastStore.success(
						t(
							mode === 'update'
								? 'myaaw.settings.runtimes_update_success'
								: 'myaaw.settings.runtimes_install_success',
							{
								name: targetLabel,
								version: updated.version || t('myaaw.settings.runtimes_installed')
							}
						)
					);
				}
			};

			if (mode === 'update') {
				await runtimesManageStore.streamUpdate(targetId, handleEvent, ctrl.signal);
			} else {
				await runtimesManageStore.streamInstall(
					targetId,
					activeMethod.id,
					handleEvent,
					ctrl.signal
				);
			}
		} catch (err) {
			if (!ctrl.signal.aborted) {
				errorMessage = getErrorMessage(err);
			}
		} finally {
			if (abortController === ctrl) {
				abortController = null;
			}
			running = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-[calc(100vw-2rem)] sm:max-w-2xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				{#if mode === 'update'}
					<RefreshCw class="h-4 w-4 shrink-0 text-primary" />
					{t('myaaw.settings.runtimes_update_title', { name: runtime?.label ?? '' })}
				{:else}
					<Download class="h-4 w-4 shrink-0 text-primary" />
					{t('myaaw.settings.runtimes_install_title', { name: runtime?.label ?? '' })}
				{/if}
			</Dialog.Title>
			<Dialog.Description>
				{t('myaaw.settings.runtimes_install_desc')}
			</Dialog.Description>
		</Dialog.Header>

		<div
			class="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-3"
		>
			{#if mode === 'install' && methods.length > 1}
				<div class="grid gap-1.5">
					<Label>{t('myaaw.settings.runtimes_install_method')}</Label>
					<div class="grid gap-2">
						{#each methods as method (method.id)}
							{@const selected = activeMethod?.id === method.id}
							<button
								type="button"
								disabled={!method.available || running}
								onclick={() => (selectedMethodId = method.id)}
								class={[
									'flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-left text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 disabled:cursor-not-allowed disabled:opacity-55',
									selected ? 'border-primary bg-accent/60' : 'border-border hover:bg-muted/50'
								]}
							>
								<span class="font-medium text-foreground">{method.label}</span>
								<span class="flex shrink-0 items-center gap-1.5">
									{#if !method.available && method.missing_tool}
										<Badge variant="outline" class="text-[10px]">
											{t('myaaw.settings.runtimes_missing_tool', { tool: method.missing_tool })}
										</Badge>
									{:else if method.recommended}
										<Badge variant="secondary" class="text-[10px]">
											{t('myaaw.settings.runtimes_recommended')}
										</Badge>
									{/if}
								</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}

			{#if activeMethod?.manual_download}
				<div class="space-y-3 rounded-xl border bg-muted/30 p-4">
					<div class="space-y-1">
						<p class="text-xs font-medium text-foreground">
							{t('myaaw.settings.runtimes_download_title')}
						</p>
						<p class="text-xs leading-relaxed text-muted-foreground">
							{t('myaaw.settings.runtimes_download_hint', { name: runtime?.label ?? '' })}
						</p>
					</div>
					{#if activeMethod.source_url}
						<a
							href={activeMethod.source_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 font-mono text-xs text-primary hover:underline"
						>
							{activeMethod.source_url}
							<ExternalLink class="h-3.5 w-3.5" />
						</a>
					{/if}
				</div>
			{:else}
				{#if activeMethod}
					<div class="space-y-1.5">
						<div class="flex items-center justify-between gap-2">
							<Label class="text-xs">{t('myaaw.settings.runtimes_command_preview')}</Label>
							{#if activeMethod.source_url}
								<a
									href={activeMethod.source_url}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-1 text-[11px] text-primary hover:underline"
								>
									{t('myaaw.settings.runtimes_source_link')}
									<ExternalLink class="h-3 w-3" />
								</a>
							{/if}
						</div>
						<div
							class="flex min-w-0 items-center justify-between gap-2 rounded-lg border bg-muted/40 px-3 py-2"
						>
							<code class="min-w-0 flex-1 break-all font-mono text-xs text-foreground">
								{activeMethod.command_preview}
							</code>
							<button
								type="button"
								onclick={() => void copyCommand()}
								class="grid h-7 w-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
								title={t('common.markdown.copy')}
							>
								{#if copied}
									<Check class="h-3.5 w-3.5 text-emerald-500" />
								{:else}
									<Copy class="h-3.5 w-3.5" />
								{/if}
							</button>
						</div>
					</div>
				{/if}

				<div
					class="flex min-h-[220px] min-w-0 flex-1 overflow-hidden rounded-xl border bg-zinc-950 text-zinc-100 sm:h-64 sm:flex-none"
				>
					<pre
						bind:this={logContainer}
						class="h-full w-full overflow-y-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words">{#if logs.length === 0}<span
								class="text-zinc-400">{t('myaaw.settings.runtimes_log_waiting')}</span
							>{:else}{logs.join('\n')}{/if}</pre>
				</div>
			{/if}

			{#if errorMessage}
				<div
					class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
				>
					{errorMessage}
				</div>
			{/if}
		</div>

		<Dialog.Footer
			class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
		>
			<Button variant="outline" size="sm" onclick={() => (open = false)}>
				{finished ? t('common.markdown.close') : t('common.cancel')}
			</Button>
			{#if !finished}
				{#if activeMethod?.manual_download}
					<Button
						variant="outline"
						size="sm"
						disabled={rechecking}
						onclick={() => void recheckInstalled()}
						class="gap-1.5"
					>
						<RefreshCw class={`h-3.5 w-3.5 ${rechecking ? 'animate-spin' : ''}`} />
						{t('myaaw.settings.runtimes_download_recheck')}
					</Button>
					{#if activeMethod.source_url}
						<Button
							size="sm"
							href={activeMethod.source_url}
							target="_blank"
							rel="noopener noreferrer"
							class="gap-1.5"
						>
							<Download class="h-3.5 w-3.5" />
							{t('myaaw.settings.runtimes_download_open')}
							<ExternalLink class="h-3 w-3" />
						</Button>
					{/if}
				{:else}
					<Button
						size="sm"
						disabled={!activeMethod?.available || running}
						onclick={() => void runJob()}
						class="gap-1.5"
					>
						{#if running}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
							{t('myaaw.settings.runtimes_installing')}
						{:else if mode === 'update'}
							<RefreshCw class="h-3.5 w-3.5" />
							{t('myaaw.settings.runtimes_update_start')}
						{:else}
							<Download class="h-3.5 w-3.5" />
							{t('myaaw.settings.runtimes_install_start')}
						{/if}
					</Button>
				{/if}
			{/if}
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
