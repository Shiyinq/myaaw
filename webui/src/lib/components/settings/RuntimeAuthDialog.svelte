<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { browser } from '$app/environment';
	import '@xterm/xterm/css/xterm.css';
	import { Copy, ExternalLink, KeyRound, Loader2, LogOut, RotateCcw } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { runtimesManageStore } from '$lib/stores/runtimes.svelte';
	import { apiBase, toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { Terminal as XTerm } from '@xterm/xterm';
	import type { FitAddon as XFitAddon } from '@xterm/addon-fit';
	import type { ManagedRuntimeView } from '$lib/types';

	interface Props {
		open?: boolean;
		runtime: ManagedRuntimeView | null;
		mode?: 'login' | 'logout';
		onFinished?: (updated: ManagedRuntimeView) => void;
	}

	let { open = $bindable(false), runtime, mode = 'login', onFinished }: Props = $props();

	const { t } = useTranslation();

	/**
	 * Outer layout-owned wrapper observed by ResizeObserver. Because `container`
	 * is `position: absolute` inside `viewport`, xterm mutating its own canvas
	 * dimensions can never shrink or expand `viewport`, preventing both horizontal
	 * overflow and the FitAddon shrinking feedback loop.
	 */
	let viewport = $state<HTMLDivElement | null>(null);
	let container = $state<HTMLDivElement | null>(null);
	let headless = $state(false);
	let connecting = $state(false);
	let sessionExited = $state(false);
	let errorMessage = $state('');
	let commandPreview = $state('');
	let detectedUrl = $state('');
	let detectedCode = $state('');

	let term: XTerm | null = null;
	let fitAddon: XFitAddon | null = null;
	let sessionId = '';
	let sessionPreset = '';
	let controller: AbortController | null = null;
	let observer: ResizeObserver | null = null;
	let inputQueue: string[] = [];
	let inputTimer: ReturnType<typeof setTimeout> | null = null;
	let resizeTimer: ReturnType<typeof setTimeout> | null = null;
	let settleTimer: ReturnType<typeof setTimeout> | null = null;
	let lastSentCols = 0;
	let lastSentRows = 0;
	let lastObservedW = 0;
	let lastObservedH = 0;
	let plainBuffer = '';
	let wasOpen = false;

	function defaultHeadlessForHost(): boolean {
		if (!browser) return false;
		if (apiBase.isCustom) return true;
		const h = window.location.hostname;
		return h !== 'localhost' && h !== '127.0.0.1' && h !== '::1';
	}

	$effect(() => {
		if (open && !wasOpen) {
			wasOpen = true;
			headless = runtime?.auth.supports_headless ? defaultHeadlessForHost() : false;
			void startSession();
		} else if (!open && wasOpen) {
			wasOpen = false;
			const targetId = sessionPreset || runtime?.id || '';
			teardown(true);
			if (targetId) {
				void refreshRuntimeStatus(targetId);
			}
		}
	});

	onDestroy(() => {
		teardown(true);
	});

	function stripAnsiText(raw: string): string {
		// eslint-disable-next-line no-control-regex
		return raw.replace(/\x1B\[[0-?]*[ -/]*[@-~]/g, '').replace(/\x1B\][^\x07]*\x07/g, '');
	}

	function inspectOutputForAuthHints(chunkText: string) {
		plainBuffer = (plainBuffer + stripAnsiText(chunkText)).slice(-8192);

		const urlMatches = plainBuffer.match(/https:\/\/[^\s"'<>)\]]+/g) ?? [];
		if (urlMatches.length > 0) {
			const preferred =
				urlMatches.find((u) =>
					/oauth|authorize|login|device|auth|accounts\.google|claude\.ai|openai\.com/i.test(u)
				) ?? urlMatches[urlMatches.length - 1];
			if (preferred) {
				detectedUrl = preferred.replace(/[.,;]+$/, '');
			}
		}

		const codeMatch = plainBuffer.match(/\b([A-Z0-9]{4}-[A-Z0-9]{4,5})\b/);
		if (codeMatch?.[1]) {
			detectedCode = codeMatch[1];
		}
	}

	function readTerminalTheme() {
		return {
			background: '#09090b',
			foreground: '#f4f4f5',
			cursor: '#f4f4f5',
			cursorAccent: '#09090b',
			selectionBackground: '#3f3f46'
		};
	}

	function applyFitAndSync() {
		if (!term || !fitAddon || !container) return;
		if (container.clientWidth < 40 || container.clientHeight < 40) return;
		fitAddon.fit();
		if (!sessionPreset || !sessionId) return;
		if (term.cols === lastSentCols && term.rows === lastSentRows) return;
		lastSentCols = term.cols;
		lastSentRows = term.rows;
		void runtimesManageStore
			.resizeAuthSession(sessionPreset, sessionId, term.cols, term.rows)
			.catch(() => {});
	}

	async function startSession() {
		if (!browser || !runtime) return;
		const presetId = runtime.id;
		teardown(true);

		connecting = true;
		sessionExited = false;
		errorMessage = '';
		detectedUrl = '';
		detectedCode = '';
		plainBuffer = '';
		commandPreview =
			mode === 'logout' ? (runtime.auth.logout_command ?? '') : (runtime.auth.login_command ?? '');

		try {
			const [{ Terminal }, { FitAddon }] = await Promise.all([
				import('@xterm/xterm'),
				import('@xterm/addon-fit')
			]);
			await tick();
			if (!container || !viewport || !open) return;

			if (document.fonts) {
				try {
					await document.fonts.ready;
				} catch {
					// Proceed with fallback monospace metrics if font loading fails.
				}
			}

			term = new Terminal({
				cursorBlink: true,
				fontSize: 13,
				fontFamily: '"Myaaw Mono", ui-monospace, SFMono-Regular, Menlo, monospace',
				scrollback: 2000,
				theme: readTerminalTheme()
			});
			fitAddon = new FitAddon();
			term.loadAddon(fitAddon);
			term.open(container);
			fitAddon.fit();
			term.focus();

			const initialCols = Math.max(40, term.cols || 80);
			const initialRows = Math.max(12, term.rows || 24);
			lastSentCols = initialCols;
			lastSentRows = initialRows;

			const info = await runtimesManageStore.openAuthSession(presetId, {
				mode,
				headless: mode === 'login' && Boolean(runtime.auth.supports_headless) && headless,
				cols: initialCols,
				rows: initialRows
			});
			sessionPreset = presetId;
			sessionId = info.id;
			commandPreview = info.command_preview;

			const ctrl = new AbortController();
			controller = ctrl;
			term.onData(queueInput);

			observer = new ResizeObserver((entries) => {
				const entry = entries[0];
				if (!entry) return;
				const w = Math.round(entry.contentRect.width);
				const h = Math.round(entry.contentRect.height);
				if (w === lastObservedW && h === lastObservedH) return;
				lastObservedW = w;
				lastObservedH = h;
				scheduleResize();
			});
			observer.observe(viewport);

			// Re-fit once after the 100ms dialog zoom-in entrance animation settles.
			settleTimer = setTimeout(() => {
				settleTimer = null;
				applyFitAndSync();
			}, 160);

			connecting = false;
			void pumpAuthStream(presetId, info.id, ctrl.signal);
		} catch (err) {
			connecting = false;
			errorMessage = getErrorMessage(err);
		}
	}

	async function pumpAuthStream(presetId: string, sid: string, signal: AbortSignal) {
		const decoder = new TextDecoder();
		try {
			const res = await runtimesManageStore.streamAuthSession(presetId, sid, signal);
			if (!res.ok || !res.body) {
				if (!signal.aborted) {
					errorMessage = `Stream failed (${res.status})`;
				}
				return;
			}
			const reader = res.body.getReader();
			while (true) {
				const { value, done } = await reader.read();
				if (done) break;
				if (value) {
					term?.write(value);
					inspectOutputForAuthHints(decoder.decode(value, { stream: true }));
				}
			}
			if (!signal.aborted) {
				sessionExited = true;
				await refreshRuntimeStatus(presetId);
			}
		} catch {
			// Aborted on modal close or session restart.
		}
	}

	async function refreshRuntimeStatus(presetId: string) {
		try {
			const updated = await runtimesManageStore.getOne(presetId);
			onFinished?.(updated);
		} catch {
			// Ignore transient refresh errors.
		}
	}

	function queueInput(data: string) {
		inputQueue.push(data);
		if (inputTimer) return;
		inputTimer = setTimeout(() => {
			inputTimer = null;
			void flushInput();
		}, 16);
	}

	async function flushInput() {
		if (!sessionPreset || !sessionId || inputQueue.length === 0) return;
		const bytes = new TextEncoder().encode(inputQueue.join(''));
		inputQueue = [];
		try {
			await runtimesManageStore.inputAuthSession(sessionPreset, sessionId, bytes);
		} catch {
			// Session may have already exited cleanly.
		}
	}

	function scheduleResize() {
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = setTimeout(() => {
			resizeTimer = null;
			applyFitAndSync();
		}, 120);
	}

	function teardown(closeServerSession: boolean) {
		const oldPreset = sessionPreset;
		const oldSid = sessionId;
		sessionPreset = '';
		sessionId = '';
		lastSentCols = 0;
		lastSentRows = 0;
		lastObservedW = 0;
		lastObservedH = 0;

		observer?.disconnect();
		observer = null;
		controller?.abort();
		controller = null;
		if (inputTimer) clearTimeout(inputTimer);
		inputTimer = null;
		inputQueue = [];
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = null;
		if (settleTimer) clearTimeout(settleTimer);
		settleTimer = null;
		term?.dispose();
		term = null;
		fitAddon = null;

		if (closeServerSession && oldPreset && oldSid) {
			void runtimesManageStore.closeAuthSession(oldPreset, oldSid).catch(() => {});
		}
	}

	async function copyDeviceCode() {
		if (!detectedCode || !browser) return;
		try {
			await navigator.clipboard.writeText(detectedCode);
			toastStore.success(t('myaaw.settings.runtimes_auth_code_copied'));
		} catch {
			// Ignore clipboard failure.
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-[calc(100vw-2rem)] sm:max-w-3xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				{#if mode === 'logout'}
					<LogOut class="h-4 w-4 shrink-0 text-primary" />
					{t('myaaw.settings.runtimes_auth_logout_title', { name: runtime?.label ?? '' })}
				{:else}
					<KeyRound class="h-4 w-4 shrink-0 text-primary" />
					{t('myaaw.settings.runtimes_auth_login_title', { name: runtime?.label ?? '' })}
				{/if}
			</Dialog.Title>
			<Dialog.Description>
				{t('myaaw.settings.runtimes_auth_desc')}
			</Dialog.Description>
		</Dialog.Header>

		<div class="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-hidden px-4 py-3 sm:px-5">
			<div class="flex shrink-0 flex-wrap items-center justify-between gap-2">
				{#if commandPreview}
					<code
						class="rounded-md border bg-muted/40 px-2.5 py-1 font-mono text-xs text-muted-foreground"
					>
						$ {commandPreview}
					</code>
				{/if}
				{#if mode === 'login' && runtime?.auth.supports_headless}
					<label class="flex cursor-pointer items-center gap-2 text-xs text-foreground">
						<Checkbox
							checked={headless}
							onCheckedChange={(v) => {
								headless = Boolean(v);
								void startSession();
							}}
						/>
						<span>{t('myaaw.settings.runtimes_auth_headless')}</span>
					</label>
				{/if}
			</div>

			{#if detectedUrl || detectedCode}
				<div
					class="flex shrink-0 flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2"
				>
					{#if detectedUrl}
						<Button
							size="sm"
							variant="default"
							href={detectedUrl}
							target="_blank"
							rel="noopener noreferrer"
							class="gap-1.5 text-xs"
						>
							{t('myaaw.settings.runtimes_auth_open_url')}
							<ExternalLink class="h-3.5 w-3.5" />
						</Button>
					{/if}
					{#if detectedCode}
						<Button
							size="sm"
							variant="outline"
							onclick={() => void copyDeviceCode()}
							class="gap-1.5 font-mono text-xs"
						>
							<Copy class="h-3.5 w-3.5" />
							{t('myaaw.settings.runtimes_auth_copy_code', { code: detectedCode })}
						</Button>
					{/if}
				</div>
			{/if}

			{#if errorMessage}
				<div
					class="shrink-0 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
				>
					{errorMessage}
				</div>
			{/if}

			<div
				bind:this={viewport}
				class="relative min-h-[280px] w-full min-w-0 flex-1 overflow-hidden rounded-xl border bg-zinc-950 sm:h-[420px] sm:flex-none"
			>
				{#if connecting}
					<div
						class="absolute inset-0 z-10 flex items-center justify-center gap-2 bg-zinc-950/85 text-xs text-zinc-300"
					>
						<Loader2 class="h-4 w-4 animate-spin" />
						{t('common.loading')}
					</div>
				{/if}
				<div bind:this={container} class="absolute inset-2.5 overflow-hidden"></div>
			</div>

			{#if sessionExited}
				<p class="shrink-0 text-xs text-emerald-600 dark:text-emerald-400">
					{t('myaaw.settings.runtimes_auth_finished')}
				</p>
			{/if}
		</div>

		<Dialog.Footer
			class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
		>
			<Button
				variant="outline"
				size="sm"
				onclick={() => void startSession()}
				disabled={connecting}
				class="gap-1.5"
			>
				<RotateCcw class="h-3.5 w-3.5" />
				{t('myaaw.settings.runtimes_auth_restart')}
			</Button>
			<Button size="sm" onclick={() => (open = false)}>
				{t('common.markdown.close')}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
