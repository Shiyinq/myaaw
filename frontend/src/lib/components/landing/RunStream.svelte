<script lang="ts">
	import { onDestroy } from 'svelte';
	import { Check, FileText, ShieldCheck, Terminal } from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { Card } from '$lib/components/ui/card';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { cn } from '$lib/utils';

	/**
	 * A small, self-playing run: a message arrives through a channel, context is
	 * assembled, two tools run, an approval stops the run, and the answer streams
	 * back. It is the landing page's "what does this thing actually do" panel.
	 *
	 * The script restarts on a loop, restarts when the language changes, and
	 * collapses to its finished state when motion is reduced.
	 */
	let { class: className = '' }: { class?: string } = $props();

	const { t } = useTranslation();

	let step = $state(0);
	let typed = $state(0);
	let runId = 0;
	let alive = true;

	const reduced = $derived(preferences.value.reduceMotion);
	const dark = $derived(mode.current === 'dark');
	const answer = $derived(t('landing.stream.answer'));
	const visibleAnswer = $derived(reduced ? answer : answer.slice(0, typed));
	const status = $derived(
		step >= 7
			? t('landing.stream.done')
			: step === 5
				? t('landing.stream.waiting')
				: t('landing.stream.running')
	);
	const busy = $derived(step > 0 && step < 7);

	function wait(ms: number) {
		return new Promise<void>((resolve) => setTimeout(resolve, ms));
	}

	async function typeOut(id: number, text: string) {
		for (let index = 0; index <= text.length; index += 2) {
			if (!alive || id !== runId) return;
			typed = index;
			await wait(16);
		}
		if (alive && id === runId) typed = text.length;
	}

	async function play(id: number) {
		while (alive && id === runId) {
			step = 0;
			typed = 0;
			await wait(380);
			if (!alive || id !== runId) return;
			step = 1;
			await wait(720);
			if (!alive || id !== runId) return;
			step = 2;
			await wait(780);
			if (!alive || id !== runId) return;
			step = 3;
			await wait(720);
			if (!alive || id !== runId) return;
			step = 4;
			await wait(820);
			if (!alive || id !== runId) return;
			step = 5;
			await wait(1200);
			if (!alive || id !== runId) return;
			step = 6;
			await typeOut(id, t('landing.stream.answer'));
			if (!alive || id !== runId) return;
			step = 7;
			await wait(4500);
		}
	}

	$effect(() => {
		// Subscribe to both the motion preference and the language: either one
		// changes what the demo should show, so the run starts over.
		const reduce = preferences.value.reduceMotion;
		const body = t('landing.stream.answer');
		runId += 1;
		const id = runId;
		if (reduce) {
			step = 7;
			typed = body.length;
			return;
		}
		step = 0;
		typed = 0;
		void play(id);
	});

	onDestroy(() => {
		alive = false;
	});
</script>

<div class={cn('relative', className)}>
	<p class="sr-only">{t('landing.stream.description')}</p>

	<Card class="gap-0 py-0">
		<div class="flex items-center gap-3 border-b border-border/70 px-4 py-3" aria-hidden="true">
			<div
				class="grid size-10 shrink-0 place-items-center overflow-hidden rounded-lg border border-border/70 bg-background/40"
			>
				<Bloub cycle="idle2" size={36} eager color={dark ? 'orange' : 'encre'} />
			</div>
			<div class="min-w-0 flex-1">
				<div class="flex items-center gap-2">
					<span class="truncate text-sm font-semibold">default</span>
					<span
						class="mono rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground"
						>:8080</span
					>
				</div>
				<p class="mono truncate text-[10px] tracking-[0.14em] text-muted-foreground uppercase">
					agent · sqlite · workspace
				</p>
			</div>
			<span class="flex items-center gap-1.5 rounded-full border border-border/70 px-2 py-1">
				<span class="status-dot" class:pulsing={busy}></span>
				<span class="mono text-[10px] tracking-[0.12em] uppercase">{status}</span>
			</span>
		</div>

		<div class="flex min-h-[21rem] flex-col gap-2.5 px-4 py-4" aria-hidden="true">
			<div class="entry" class:on={step >= 1}>
				<p class="mono mb-1 text-[10px] tracking-[0.14em] text-muted-foreground uppercase">
					{t('landing.stream.you')}
				</p>
				<p
					class="rounded-lg border border-border/70 bg-muted/40 px-3 py-2 text-[13px] leading-snug"
				>
					{t('landing.stream.prompt')}
				</p>
			</div>

			<div class="entry flex items-center gap-2" class:on={step >= 2}>
				<span class="mono text-[10px] tracking-[0.14em] text-muted-foreground uppercase">
					{t('landing.stream.agent')}
				</span>
				{#if step === 2}
					<span class="flex items-center gap-2 text-muted-foreground">
						<span class="typing"><i></i><i></i><i></i></span>
						<span class="text-[11px]">{t('landing.stream.thinking')}</span>
					</span>
				{:else}
					<span class="flex items-center gap-1 text-[11px] text-muted-foreground">
						<Check class="size-3" />
						{t('landing.stream.thought')}
					</span>
				{/if}
			</div>

			<div class="entry tool-row" class:on={step >= 3}>
				<Terminal class="size-3.5 shrink-0 text-muted-foreground" />
				<span class="mono text-[10px] tracking-[0.12em] uppercase">bash</span>
				<code class="mono truncate text-[11px] text-muted-foreground">ls -la ~/workspace</code>
				<span class="mono ml-auto shrink-0 text-[10px] text-muted-foreground">0.3s</span>
			</div>

			<div class="entry tool-row" class:on={step >= 4}>
				<FileText class="size-3.5 shrink-0 text-muted-foreground" />
				<span class="mono text-[10px] tracking-[0.12em] uppercase">files</span>
				<code class="mono truncate text-[11px] text-muted-foreground">read notes/today.md</code>
				<span class="mono ml-auto shrink-0 text-[10px] text-muted-foreground">94ms</span>
			</div>

			<div class="entry approval-row" class:on={step >= 5}>
				<ShieldCheck class="size-3.5 shrink-0 text-muted-foreground" />
				<span class="min-w-0 flex-1 text-[11px] leading-snug">
					{t('landing.stream.approval')}
				</span>
				{#if step >= 6}
					<span
						class="inline-flex items-center gap-1 rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground"
					>
						<Check class="size-3" />
						{t('landing.stream.approved')}
					</span>
				{:else}
					<span
						class="rounded bg-primary px-1.5 py-0.5 text-[10px] font-medium text-primary-foreground"
						>{t('landing.stream.allow')}</span
					>
					<span
						class="rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground"
						>{t('landing.stream.deny')}</span
					>
				{/if}
			</div>

			<div class="entry" class:on={step >= 6}>
				<p class="mono mb-1 text-[10px] tracking-[0.14em] text-muted-foreground uppercase">
					{t('landing.stream.agent')}
				</p>
				<p class="text-[13px] leading-relaxed">
					{visibleAnswer}<span class="caret" class:blink={step >= 7}></span>
				</p>
			</div>
		</div>

		<div
			class="flex items-center justify-between border-t border-border/70 px-4 py-2"
			aria-hidden="true"
		>
			<span class="mono text-[10px] text-muted-foreground">run_events</span>
			<span class="mono text-[10px] text-muted-foreground">--follow</span>
			<span class="mono text-[10px] text-muted-foreground">{t('landing.stream.events')}</span>
		</div>
	</Card>
</div>

<style>
	.mono {
		font-family: var(--font-ui-mono);
	}

	.entry {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.45s ease,
			transform 0.45s cubic-bezier(0.22, 1, 0.36, 1);
	}

	.entry.on {
		opacity: 1;
		transform: none;
	}

	.tool-row,
	.approval-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		border-radius: calc(var(--radius) - 2px);
		padding: 0.375rem 0.5rem;
	}

	.tool-row {
		border: 1px solid color-mix(in oklab, var(--border) 70%, transparent);
	}

	.approval-row {
		border: 1px dashed color-mix(in oklab, var(--border) 85%, transparent);
		padding: 0.5rem;
	}

	.status-dot {
		width: 0.4rem;
		height: 0.4rem;
		border-radius: 9999px;
		background: var(--muted-foreground);
	}

	.status-dot.pulsing {
		background: var(--primary);
		animation: status 1.8s ease-out infinite;
	}

	@keyframes status {
		0%,
		100% {
			opacity: 1;
			transform: scale(1);
		}
		50% {
			opacity: 0.45;
			transform: scale(0.78);
		}
	}

	.typing {
		display: inline-flex;
		gap: 2px;
	}

	.typing i {
		width: 3px;
		height: 3px;
		border-radius: 9999px;
		background: currentColor;
		animation: typing 1s ease-in-out infinite;
	}

	.typing i:nth-child(2) {
		animation-delay: 0.15s;
	}

	.typing i:nth-child(3) {
		animation-delay: 0.3s;
	}

	@keyframes typing {
		0%,
		60%,
		100% {
			opacity: 0.25;
			transform: translateY(0);
		}
		30% {
			opacity: 1;
			transform: translateY(-2px);
		}
	}

	.caret {
		display: inline-block;
		width: 2px;
		height: 1em;
		margin-left: 2px;
		vertical-align: -0.15em;
		background: currentColor;
		opacity: 0;
	}

	.caret.blink {
		animation: caret 1.1s steps(1) infinite;
	}

	@keyframes caret {
		0%,
		49% {
			opacity: 1;
		}
		50%,
		100% {
			opacity: 0;
		}
	}
</style>
