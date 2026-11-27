<script lang="ts">
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import '@xterm/xterm/css/xterm.css';
	import { terminalStore } from '$lib/stores/terminal.svelte';
	import { terminalMux } from '$lib/stores/terminalMux.svelte';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { RefreshCw, X } from 'lucide-svelte';
	import type { Terminal as XTerm } from '@xterm/xterm';
	import type { FitAddon as XFitAddon } from '@xterm/addon-fit';
	import type { SerializeAddon as XSerializeAddon } from '@xterm/addon-serialize';

	type Phase = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error';

	interface Props {
		agent: string;
		/** Optional working directory on the API host to start the shell in. */
		cwd?: string;
		/** Optional conversation id used to scope session persistence and server cwd lookup. */
		conversationId?: string;
		/** Optional slot id so multiple terminal tabs in the same project stay independent. */
		slot?: string;
		/** Compact chrome for embedded drawers (such as the chat project terminal). */
		compact?: boolean;
		/** Hide the inner status header bar when the parent renders its own terminal tab bar. */
		hideHeader?: boolean;
		onClose?: () => void;
		onStateChange?: (state: { phase: Phase; enabled: boolean | null }) => void;
	}

	let {
		agent,
		cwd = '',
		conversationId = '',
		slot = '',
		compact = false,
		hideHeader = false,
		onClose,
		onStateChange
	}: Props = $props();

	const { t } = useTranslation();

	let container = $state<HTMLDivElement | null>(null);
	let phase = $state<Phase>('idle');
	let message = $state('');
	let enabled = $state<boolean | null>(null);
	let sessionCwd = $state('');
	let sessionShell = $state('');
	let hasTerminal = $state(false);

	let term: XTerm | null = null;
	let fitAddon: XFitAddon | null = null;
	let serializeAddon: XSerializeAddon | null = null;
	let sessionId = '';
	let sessionKey = '';
	let controller: AbortController | null = null;
	let unsubMux: (() => void) | null = null;
	let observer: ResizeObserver | null = null;
	let inputQueue: string[] = [];
	let inputTimer: ReturnType<typeof setTimeout> | null = null;
	let resizeTimer: ReturnType<typeof setTimeout> | null = null;
	let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let connectedFor = '';
	let lastCols = 0;
	let lastRows = 0;

	const phaseLabel = $derived(
		phase === 'connected'
			? t('myaaw.terminal.connected')
			: phase === 'connecting'
				? t('myaaw.terminal.connecting')
				: phase === 'disconnected'
					? t('myaaw.terminal.disconnected')
					: phase === 'error'
						? t('myaaw.terminal.failed')
						: t('myaaw.terminal.idle')
	);

	function storageKeyFor(
		targetAgent: string,
		targetCwd: string,
		targetConvId: string,
		targetSlot: string
	): string {
		// Scope project terminals by (agent, cwd, slot) so starting a new chat or
		// switching chats within the same project keeps running PTYs untouched.
		if (targetCwd) {
			return `${targetAgent}:cwd:${targetCwd}:${targetSlot}`;
		}
		if (targetConvId || targetSlot) {
			return `${targetAgent}:${targetConvId}::${targetSlot}`;
		}
		return targetAgent;
	}

	$effect(() => {
		const currentPhase = phase;
		const currentEnabled = enabled;
		untrack(() => {
			onStateChange?.({ phase: currentPhase, enabled: currentEnabled });
		});
	});

	$effect(() => {
		const target = storageKeyFor(agent, cwd, conversationId, slot);
		if (!agent || target === connectedFor) return;
		connectedFor = target;
		untrack(() => {
			void connect();
		});
	});

	onDestroy(() => {
		teardown();
	});

	onMount(() => {
		const onHide = () => saveSnapshot();
		window.addEventListener('pagehide', onHide);
		return () => window.removeEventListener('pagehide', onHide);
	});

	const STORAGE_PREFIX = 'myaaw.terminal.';
	const MAX_SNAPSHOT_LINES = 500;

	interface StoredSession {
		id: string;
		buffer: string;
	}

	function readStored(key: string): StoredSession | null {
		if (!browser) return null;
		try {
			const raw = sessionStorage.getItem(STORAGE_PREFIX + key);
			if (!raw) return null;
			const parsed = JSON.parse(raw) as StoredSession;
			return parsed && typeof parsed.id === 'string' ? parsed : null;
		} catch {
			return null;
		}
	}

	function writeStored(key: string, id: string, buffer: string) {
		if (!browser) return;
		try {
			sessionStorage.setItem(STORAGE_PREFIX + key, JSON.stringify({ id, buffer }));
		} catch {
			// A full quota only costs the scrollback, never the shell.
		}
	}

	function clearStored(key: string) {
		if (!browser) return;
		try {
			sessionStorage.removeItem(STORAGE_PREFIX + key);
		} catch {
			// Nothing to do.
		}
	}

	function saveSnapshot() {
		if (!term || !serializeAddon || !sessionId || !sessionKey) return;
		try {
			const buffer = serializeAddon.serialize({ scrollback: MAX_SNAPSHOT_LINES });
			writeStored(sessionKey, sessionId, buffer);
		} catch {
			// Ignore snapshot errors during teardown.
		}
	}

	async function ensureGlyphFont() {
		if (!document.fonts) return;
		try {
			await Promise.race([
				document.fonts.load('13px "Myaaw Mono"', 'M\ue0b0\uf121'),
				new Promise((resolve) => setTimeout(resolve, 250))
			]);
		} catch {
			// A missing font only costs the look; the shell keeps working.
		}
	}

	export async function reconnect() {
		await connect();
	}

	export function focus() {
		term?.focus();
	}

	/** Explicitly closes the remote PTY session and clears its stored snapshot. */
	export async function terminate() {
		const currentAgent = agent;
		const id = sessionId;
		const key = sessionKey;
		sessionId = '';
		sessionKey = '';
		if (key) clearStored(key);
		teardown();
		if (currentAgent && id) {
			try {
				await terminalStore.close(currentAgent, id);
			} catch {
				// Ignore close errors for already-exited sessions.
			}
		}
	}

	async function connect() {
		const current = agent;
		const targetCwd = cwd;
		const targetConvId = conversationId;
		const targetSlot = slot;
		if (!current) return;

		teardown();
		const targetKey = storageKeyFor(current, targetCwd, targetConvId, targetSlot);
		connectedFor = targetKey;
		phase = 'connecting';
		message = '';

		try {
			const status = await terminalStore.status(current);
			enabled = status.enabled;
			if (!status.enabled) {
				phase = 'idle';
				return;
			}

			const [{ Terminal }, { FitAddon }, { SerializeAddon }] = await Promise.all([
				import('@xterm/xterm'),
				import('@xterm/addon-fit'),
				import('@xterm/addon-serialize')
			]);
			await tick();
			if (!container) return;
			await ensureGlyphFont();

			term = new Terminal({
				cursorBlink: true,
				fontSize: compact ? 12 : 13,
				fontFamily: '"Myaaw Mono", ui-monospace, SFMono-Regular, Menlo, monospace',
				scrollback: 5000,
				theme: readTheme()
			});
			fitAddon = new FitAddon();
			serializeAddon = new SerializeAddon();
			term.loadAddon(fitAddon);
			term.loadAddon(serializeAddon);
			term.open(container);
			fitAddon.fit();
			hasTerminal = true;

			const { info, buffer } = await attachSession(
				current,
				targetKey,
				term.cols || 80,
				term.rows || 24,
				targetCwd,
				targetConvId
			);
			sessionKey = targetKey;
			sessionId = info.id;
			sessionCwd = info.cwd;
			sessionShell = info.shell;
			lastCols = term.cols || 80;
			lastRows = term.rows || 24;
			if (buffer) term.write(buffer);

			controller = new AbortController();
			const activeController = controller;
			term.onData(queueInput);
			phase = 'connected';
			startHeartbeat(current, targetKey, info.id);
			observer = new ResizeObserver(scheduleResize);
			observer.observe(container);
			unsubMux = terminalMux.subscribe(current, info.id, {
				onData: (bytes) => term?.write(bytes),
				onClosed: () => {
					if (phase === 'connected') phase = 'disconnected';
				},
				onFallback: () => {
					if (!activeController.signal.aborted) {
						void pumpStream(current, info.id, activeController.signal);
					}
				}
			});
		} catch (err) {
			phase = 'error';
			message = getErrorMessage(err);
			term?.dispose();
			term = null;
			fitAddon = null;
			hasTerminal = false;
		}
	}

	async function pumpStream(targetAgent: string, id: string, signal: AbortSignal) {
		let response: Response;
		try {
			response = await terminalStore.stream(targetAgent, id, signal);
		} catch {
			return;
		}
		if (!response.ok || !response.body) {
			if (phase === 'connected') {
				phase = 'error';
				message = `stream failed (${response.status})`;
			}
			return;
		}
		await pump(response.body.getReader());
	}

	async function openWithRetry(
		targetAgent: string,
		cols: number,
		rows: number,
		targetCwd: string,
		targetConvId: string
	) {
		const attempts = 5;
		for (let attempt = 1; ; attempt += 1) {
			try {
				return await terminalStore.open(targetAgent, cols, rows, {
					cwd: targetCwd || undefined,
					conversationId: targetConvId || undefined
				});
			} catch (err) {
				const status = (err as { status?: number }).status;
				if (status !== 429 || attempt >= attempts) throw err;
				await new Promise((resolve) => setTimeout(resolve, 500 * attempt));
			}
		}
	}

	async function attachSession(
		targetAgent: string,
		key: string,
		cols: number,
		rows: number,
		targetCwd: string,
		targetConvId: string
	) {
		const stored = readStored(key);
		if (stored) {
			try {
				const info = await terminalStore.get(targetAgent, stored.id);
				await terminalStore.resize(targetAgent, info.id, cols, rows).catch(() => {});
				return { info, buffer: stored.buffer };
			} catch {
				clearStored(key);
			}
		}

		const info = await openWithRetry(targetAgent, cols, rows, targetCwd, targetConvId);
		writeStored(key, info.id, '');
		return { info, buffer: '' };
	}

	function startHeartbeat(targetAgent: string, key: string, id: string) {
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = setInterval(() => {
			void terminalStore.heartbeat(targetAgent, id).catch((err) => {
				if ((err as { status?: number }).status !== 404) return;
				clearStored(key);
				if (phase === 'connected') phase = 'disconnected';
			});
		}, 30_000);
	}

	async function pump(reader: ReadableStreamDefaultReader<Uint8Array>) {
		try {
			while (true) {
				const { value, done } = await reader.read();
				if (done) break;
				if (value) term?.write(value);
			}
		} catch {
			// Aborted by teardown.
		}
		if (phase === 'connected') phase = 'disconnected';
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
		const current = agent;
		if (!current || !sessionId || inputQueue.length === 0) return;
		const payload = new TextEncoder().encode(inputQueue.join(''));
		inputQueue = [];
		try {
			await terminalStore.input(current, sessionId, payload);
		} catch {
			// Dead session is reported by the stream.
		}
	}

	function countPromptWraps(instance: XTerm): number {
		const buf = instance.buffer.active;
		if (buf.type !== 'normal') return 0;
		const absY = buf.baseY + buf.cursorY;
		let wraps = 0;
		// Count wrapped continuations on the cursor line (1-line prompts).
		let y = absY;
		while (y > 0 && buf.getLine(y)?.isWrapped) {
			wraps += 1;
			y -= 1;
		}
		// Count wrapped continuations on the line directly above (2-line prompts like Powerlevel10k).
		if (y > 0) {
			let above = y - 1;
			while (above > 0 && buf.getLine(above)?.isWrapped) {
				wraps += 1;
				above -= 1;
			}
		}
		return wraps;
	}

	function scheduleResize() {
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = setTimeout(() => {
			resizeTimer = null;
			const current = agent;
			const el = container;
			if (!term || !fitAddon || !el) return;
			// Never fit or send SIGWINCH while the pane is hidden (display: none -> 0x0).
			if (el.clientWidth < 40 || el.clientHeight < 20) return;

			const prevCols = term.cols;
			const prevRows = term.rows;
			const bufBefore = term.buffer.active;
			const isNormal = bufBefore.type === 'normal';
			const curLineBefore = isNormal
				? (bufBefore.getLine(bufBefore.baseY + bufBefore.cursorY)?.translateToString(true) ?? '')
				: '';
			const wrapsBefore = isNormal ? countPromptWraps(term) : 0;

			fitAddon.fit();
			const nextCols = term.cols;
			const nextRows = term.rows;
			if (nextCols < 10 || nextRows < 2) return;
			// Only notify the remote PTY when cols or rows genuinely changed, preventing
			// duplicate prompt redraws (e.g. zsh / Powerlevel10k) on spurious SIGWINCH.
			if (nextCols === lastCols && nextRows === lastRows) return;
			if (
				nextCols === prevCols &&
				nextRows === prevRows &&
				lastCols === nextCols &&
				lastRows === nextRows
			) {
				return;
			}
			lastCols = nextCols;
			lastRows = nextRows;
			if (!current || !sessionId) return;

			const sendResize = () => {
				if (!current || !sessionId) return;
				void terminalStore.resize(current, sessionId, nextCols, nextRows).catch(() => {});
			};

			// When shrinking horizontally while sitting at an idle prompt (e.g. switching from
			// Single to Split/Auto), xterm.js reflows wide prompts (like Powerlevel10k) into
			// extra wrapped rows that the shell doesn't know to erase on SIGWINCH. Roll back
			// only those newly-wrapped prompt rows right before sending SIGWINCH so earlier
			// command output stays intact and the shell redraws one crisp prompt.
			if (isNormal && nextCols < prevCols && curLineBefore.length <= 20) {
				const extraWraps = countPromptWraps(term) - wrapsBefore;
				if (extraWraps > 0) {
					term.write(`\r\x1b[${extraWraps}A\x1b[0J`, sendResize);
					return;
				}
			}

			sendResize();
		}, 140);
	}

	function teardown() {
		saveSnapshot();

		observer?.disconnect();
		observer = null;
		unsubMux?.();
		unsubMux = null;
		controller?.abort();
		controller = null;
		if (inputTimer) clearTimeout(inputTimer);
		inputTimer = null;
		inputQueue = [];
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = null;
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = null;

		sessionId = '';
		sessionKey = '';
		term?.dispose();
		term = null;
		fitAddon = null;
		serializeAddon = null;
		hasTerminal = false;
	}

	function readTheme() {
		const style = getComputedStyle(document.documentElement);
		const value = (name: string, fallback: string) =>
			style.getPropertyValue(name).trim() || fallback;
		const dark = document.documentElement.classList.contains('dark');
		const background = value('--background', dark ? '#0a0a0a' : '#ffffff');
		const foreground = value('--foreground', dark ? '#f5f5f5' : '#1a1a1a');
		const palette = dark
			? {
					black: '#1f1f1f',
					red: '#ff7b72',
					green: '#7ee787',
					yellow: '#e3b341',
					blue: '#79c0ff',
					magenta: '#d2a8ff',
					cyan: '#56d4dd',
					white: '#c9d1d9',
					brightBlack: '#6e7681',
					brightRed: '#ffa198',
					brightGreen: '#a5e3a5',
					brightYellow: '#f2cc60',
					brightBlue: '#a5d6ff',
					brightMagenta: '#e2c5ff',
					brightCyan: '#9be9f7',
					brightWhite: '#ffffff'
				}
			: {
					black: '#1f1f1f',
					red: '#c0392b',
					green: '#2e7d32',
					yellow: '#8a6d00',
					blue: '#1565c0',
					magenta: '#8e24aa',
					cyan: '#00838f',
					white: '#5f6368',
					brightBlack: '#6b6b6b',
					brightRed: '#d93025',
					brightGreen: '#1e8e3e',
					brightYellow: '#a67c00',
					brightBlue: '#1a73e8',
					brightMagenta: '#a142f4',
					brightCyan: '#0097a7',
					brightWhite: '#202124'
				};

		return {
			background,
			foreground,
			cursor: foreground,
			cursorAccent: background,
			selectionBackground: value('--accent', dark ? '#2a2a2a' : '#e8e8e8'),
			...palette
		};
	}
</script>

{#if enabled === false}
	{#if compact}
		<div
			class="flex h-full flex-col justify-between gap-2 bg-card p-3 text-xs text-muted-foreground"
		>
			<div class="flex items-center justify-between gap-2">
				<span class="font-medium text-foreground">{t('myaaw.terminal.disabled_title')}</span>
				{#if onClose}
					<Button variant="ghost" size="icon-sm" onclick={onClose} aria-label={t('common.close')}>
						<X class="h-3.5 w-3.5" />
					</Button>
				{/if}
			</div>
			<p>{t('myaaw.terminal.disabled_desc')}</p>
			<code class="block rounded-md border bg-muted/40 px-2.5 py-1.5 font-mono text-[11px]">
				myaaw api restart --terminal
			</code>
		</div>
	{:else}
		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.terminal.disabled_title')}</CardTitle>
			</CardHeader>
			<CardContent class="space-y-3 text-sm text-muted-foreground">
				<p>{t('myaaw.terminal.disabled_desc')}</p>
				<code class="block rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs">
					myaaw api restart --terminal
				</code>
				<p class="text-[11px]">{t('myaaw.terminal.host_warning')}</p>
			</CardContent>
		</Card>
	{/if}
{:else if phase === 'error' && !hasTerminal}
	<div class="flex h-full flex-col gap-2 p-3">
		<div class="flex items-center justify-between gap-2">
			<div
				role="alert"
				class="flex-1 rounded-xl border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive"
			>
				{message || t('myaaw.terminal.failed_title')}
			</div>
			{#if onClose}
				<Button variant="ghost" size="icon-sm" onclick={onClose} aria-label={t('common.close')}>
					<X class="h-3.5 w-3.5" />
				</Button>
			{/if}
		</div>
		<p class="text-xs text-muted-foreground">{t('myaaw.terminal.failed_hint')}</p>
	</div>
{:else}
	<div
		class={[
			'flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-background',
			compact ? '' : 'rounded-xl border'
		]}
	>
		{#if !hideHeader}
			<div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-1.5">
				<div class="flex min-w-0 items-center gap-2">
					<Badge variant={phase === 'connected' ? 'secondary' : 'outline'}>{phaseLabel}</Badge>
					{#if sessionShell && !compact}
						<span class="truncate font-mono text-[11px] text-muted-foreground">
							{sessionShell}
						</span>
					{/if}
					{#if sessionCwd || cwd}
						<span
							class="truncate font-mono text-[11px] text-muted-foreground"
							title={sessionCwd || cwd}
						>
							{sessionCwd || cwd}
						</span>
					{/if}
				</div>
				<div class="flex shrink-0 items-center gap-1">
					{#if !compact}
						<span class="text-[11px] text-muted-foreground">
							{message || t('myaaw.terminal.host_warning')}
						</span>
					{:else}
						<Button
							variant="ghost"
							size="icon-sm"
							onclick={() => void connect()}
							disabled={phase === 'connecting'}
							title={t('myaaw.terminal.reconnect')}
							aria-label={t('myaaw.terminal.reconnect')}
						>
							<RefreshCw
								class={phase === 'connecting' ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'}
							/>
						</Button>
						{#if onClose}
							<Button
								variant="ghost"
								size="icon-sm"
								onclick={onClose}
								title={t('common.close')}
								aria-label={t('common.close')}
							>
								<X class="h-3.5 w-3.5" />
							</Button>
						{/if}
					{/if}
				</div>
			</div>
		{/if}
		<div bind:this={container} class="min-h-0 flex-1 p-2"></div>
		{#if phase === 'disconnected' || phase === 'error'}
			<div class="shrink-0 border-t px-3 py-1.5 text-[11px] text-muted-foreground">
				{t('myaaw.terminal.session_note')}
			</div>
		{/if}
	</div>
{/if}
