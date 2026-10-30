<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { agentSelection } from '$lib/stores';
	import { terminal as terminalApi } from '$lib/apis/terminal';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { RefreshCw } from 'lucide-svelte';
	import type { Terminal as XTerm } from '@xterm/xterm';
	import type { FitAddon as XFitAddon } from '@xterm/addon-fit';

	const { t } = useTranslation();

	type Phase = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error';

	let container = $state<HTMLDivElement | null>(null);
	let phase = $state<Phase>('idle');
	let message = $state('');
	let enabled = $state<boolean | null>(null);
	let sessionCwd = $state('');
	let sessionShell = $state('');
	let hasTerminal = $state(false);

	let term: XTerm | null = null;
	let fitAddon: XFitAddon | null = null;
	let sessionId = '';
	let controller: AbortController | null = null;
	let observer: ResizeObserver | null = null;
	let inputQueue: string[] = [];
	let inputTimer: ReturnType<typeof setTimeout> | null = null;
	let resizeTimer: ReturnType<typeof setTimeout> | null = null;
	let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let connectedFor = '';

	let phaseLabel = $derived(
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

	$effect(() => {
		const current = agentSelection.selected;
		if (!current || current === connectedFor) return;
		connectedFor = current;
		void connect();
	});

	onDestroy(() => {
		void teardown();
	});

	onMount(() => {
		// A refresh or a closed tab has to free the server-side shell immediately.
		// The stream usually reports the drop too, but only once the shell writes
		// something, so the unload request is what makes a reload feel instant.
		const onHide = () => {
			void closeForUnload();
		};
		window.addEventListener('pagehide', onHide);
		return () => window.removeEventListener('pagehide', onHide);
	});

	// Prompt themes (powerlevel10k, starship, oh-my-posh) paint with Nerd Fonts
	// Private Use Area codepoints. The bundled symbol font has to be decoded
	// before xterm creates its renderer, otherwise the first prompt is drawn as
	// boxes and the cell grid is measured against the fallback font.
	async function ensureGlyphFont() {
		if (!document.fonts) return;
		try {
			await Promise.all([
				document.fonts.load('13px "Myaaw Nerd Symbols"', '\ue0b0\uf121'),
				document.fonts.ready
			]);
		} catch {
			// Missing icons are cosmetic; the shell keeps working without them.
		}
	}

	async function connect() {
		const current = agentSelection.selected;
		if (!current) return;

		await teardown();
		connectedFor = current;
		phase = 'connecting';
		message = '';

		try {
			const status = await terminalApi.status(current);
			enabled = status.enabled;
			if (!status.enabled) {
				phase = 'idle';
				return;
			}

			const [{ Terminal }, { FitAddon }] = await Promise.all([
				import('@xterm/xterm'),
				import('@xterm/addon-fit')
			]);
			await tick();
			if (!container) return;
			await ensureGlyphFont();

			term = new Terminal({
				cursorBlink: true,
				fontSize: 13,
				fontFamily:
					'"JetBrains Mono", "Myaaw Nerd Symbols", ui-monospace, SFMono-Regular, Menlo, monospace',
				scrollback: 5000,
				theme: readTheme()
			});
			fitAddon = new FitAddon();
			term.loadAddon(fitAddon);
			term.open(container);
			fitAddon.fit();
			hasTerminal = true;

			const info = await openWithRetry(current, term.cols || 80, term.rows || 24);
			sessionId = info.id;
			sessionCwd = info.cwd;
			sessionShell = info.shell;

			controller = new AbortController();
			const response = await terminalApi.stream(current, info.id, controller.signal);
			if (!response.ok || !response.body) {
				throw new Error(`stream failed (${response.status})`);
			}

			term.onData(queueInput);
			phase = 'connected';
			startHeartbeat(current, info.id);
			observer = new ResizeObserver(scheduleResize);
			observer.observe(container);
			void pump(response.body.getReader());
		} catch (err) {
			phase = 'error';
			message = getErrorMessage(err);
			term?.dispose();
			term = null;
			fitAddon = null;
			hasTerminal = false;
		}
	}

	// A reload can race the previous tab's unload request, so a fresh page may see
	// the per-agent session limit for a moment. Wait it out instead of failing.
	async function openWithRetry(agent: string, cols: number, rows: number) {
		const attempts = 4;
		for (let attempt = 1; ; attempt += 1) {
			try {
				return await terminalApi.open(agent, cols, rows);
			} catch (err) {
				const status = (err as { status?: number }).status;
				if (status !== 429 || attempt >= attempts) throw err;
				await new Promise((resolve) => setTimeout(resolve, 400 * attempt));
			}
		}
	}

	/** Keeps a legitimately idle tab alive so its shell is not reaped. */
	function startHeartbeat(agent: string, id: string) {
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = setInterval(() => {
			void terminalApi.heartbeat(agent, id).catch(() => {});
		}, 30_000);
	}

	/** Frees the shell while the page is being torn down by the browser. */
	function closeForUnload() {
		const agent = connectedFor;
		const id = sessionId;
		if (!agent || !id) return;
		sessionId = '';
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = null;
		return terminalApi.close(agent, id, true).catch(() => {});
	}

	async function pump(reader: ReadableStreamDefaultReader<Uint8Array>) {
		try {
			while (true) {
				const { value, done } = await reader.read();
				if (done) break;
				if (value) term?.write(value);
			}
		} catch {
			// Aborting the stream on teardown lands here; the phase is set below.
		}
		if (phase === 'connected') phase = 'disconnected';
	}

	/** Batches keystrokes into one request per frame. */
	function queueInput(data: string) {
		inputQueue.push(data);
		if (inputTimer) return;
		inputTimer = setTimeout(() => {
			inputTimer = null;
			void flushInput();
		}, 16);
	}

	async function flushInput() {
		const current = agentSelection.selected;
		if (!current || !sessionId || inputQueue.length === 0) return;
		const payload = new TextEncoder().encode(inputQueue.join(''));
		inputQueue = [];
		try {
			await terminalApi.input(current, sessionId, payload);
		} catch {
			// A dead session is reported by the stream; do not spam toasts per key.
		}
	}

	function scheduleResize() {
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = setTimeout(() => {
			resizeTimer = null;
			const current = agentSelection.selected;
			if (!term || !fitAddon) return;
			fitAddon.fit();
			if (!current || !sessionId) return;
			void terminalApi.resize(current, sessionId, term.cols, term.rows).catch(() => {});
		}, 150);
	}

	async function teardown() {
		observer?.disconnect();
		observer = null;
		controller?.abort();
		controller = null;
		if (inputTimer) clearTimeout(inputTimer);
		inputTimer = null;
		inputQueue = [];
		if (resizeTimer) clearTimeout(resizeTimer);
		resizeTimer = null;
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = null;

		const stopFor = connectedFor;
		const stopId = sessionId;
		sessionId = '';
		if (stopFor && stopId) {
			try {
				await terminalApi.close(stopFor, stopId);
			} catch {
				// The shell may already be gone.
			}
		}

		term?.dispose();
		term = null;
		fitAddon = null;
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

<TopBarActions id="terminal-actions">
	{#snippet children()}
		<TopBarButton
			label={t('myaaw.terminal.reconnect')}
			icon={RefreshCw}
			onclick={() => void connect()}
			loading={phase === 'connecting'}
			disabled={enabled === false}
		/>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col p-4 lg:p-6">
	{#if enabled === false}
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
	{:else if phase === 'error' && !hasTerminal}
		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.terminal.failed_title')}</CardTitle>
			</CardHeader>
			<CardContent class="space-y-3 text-sm text-muted-foreground">
				<p class="font-mono text-xs">{message}</p>
				<p>{t('myaaw.terminal.failed_hint')}</p>
			</CardContent>
		</Card>
	{:else}
		<div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-background">
			<div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-1.5">
				<div class="flex min-w-0 items-center gap-2">
					<Badge variant={phase === 'connected' ? 'secondary' : 'outline'}>{phaseLabel}</Badge>
					{#if sessionShell}
						<span class="truncate font-mono text-[11px] text-muted-foreground">
							{sessionShell}
						</span>
					{/if}
					{#if sessionCwd}
						<span class="truncate font-mono text-[11px] text-muted-foreground">
							{sessionCwd}
						</span>
					{/if}
				</div>
				<span class="text-[11px] text-muted-foreground">
					{message || t('myaaw.terminal.host_warning')}
				</span>
			</div>
			<div bind:this={container} class="min-h-0 flex-1 p-2"></div>
			{#if phase === 'disconnected' || phase === 'error'}
				<div class="shrink-0 border-t px-3 py-1.5 text-[11px] text-muted-foreground">
					{t('myaaw.terminal.session_note')}
				</div>
			{/if}
		</div>
	{/if}
</div>
