<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { mode } from 'mode-watcher';
	import {
		Brain,
		Check,
		ChevronDown,
		Clock,
		Code,
		FolderOpen,
		Pause,
		Play,
		RotateCcw,
		SquareTerminal
	} from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import ChatStep from '$lib/components/common/ChatStep.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card } from '$lib/components/ui/card';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { cn } from '$lib/utils';

	let { class: className = '' }: { class?: string } = $props();

	const { t } = useTranslation();
	const id = $props.id();
	const dark = $derived(mode.current === 'dark');

	// Display fixtures only: none of these commands or filesystem operations run.
	const tasksCsv =
		'id,task,status\n1,landing,done\n2,navigation,done\n3,chat,done\n4,tools,done\n5,docs,done\n6,tests,done\n7,mobile_qa,pending\n8,release_review,pending\n';
	const rows = tasksCsv.trim().split('\n').slice(1);
	const done = rows.filter((row) => row.split(',')[2] === 'done').length;
	const stats = {
		done,
		total: rows.length,
		pending: rows.length - done,
		percent: Math.round((done / rows.length) * 100)
	};
	const pythonCode = [
		'import csv, json, sys',
		'rows = list(csv.DictReader(sys.stdin))',
		'done = sum(row["status"] == "done" for row in rows)',
		'total = len(rows)',
		'print(json.dumps({"done": done, "total": total, "pending": total - done, "percent": round(done / total * 100)}, indent=2))'
	].join('\n');

	interface DemoTool {
		id: string;
		name: 'bash' | 'filesystem' | 'execute_python';
		args: {
			command?: string;
			code?: string;
			input?: string;
			async?: boolean;
			tool_name?: string;
			path?: string;
			content?: string;
		};
		result: string;
		outcome: string;
	}

	// Match the registered schemas and result shapes in internal/agent/tools.
	const tools: DemoTool[] = $derived([
		{
			id: 'inspect',
			name: 'bash',
			args: { command: 'find notes data reports -maxdepth 2 -type f | sort', async: false },
			result: 'data/tasks.csv\nnotes/week.md\n',
			outcome: t('landing.stream.inspect_summary')
		},
		{
			id: 'read',
			name: 'filesystem',
			args: {
				tool_name: 'read_multiple_files',
				path: JSON.stringify(['notes/week.md', 'data/tasks.csv'])
			},
			result: JSON.stringify([
				{ path: 'notes/week.md', content: t('landing.stream.week_notes') },
				{ path: 'data/tasks.csv', content: tasksCsv }
			]),
			outcome: t('landing.stream.read_summary')
		},
		{
			id: 'compute',
			name: 'execute_python',
			args: { code: pythonCode, input: tasksCsv, async: false },
			result: `${JSON.stringify(stats, null, 2)}\n`,
			outcome: t('landing.stream.compute_summary', stats)
		},
		{
			id: 'write',
			name: 'filesystem',
			args: {
				tool_name: 'write_file',
				path: 'reports/weekly.md',
				content: t('landing.stream.report_content', stats)
			},
			result: 'File reports/weekly.md written successfully.',
			outcome: t('landing.stream.write_summary')
		}
	]);

	// Thinking, then four call/result pairs: 14 seconds, with every stage >= 1 second.
	const delays = [1200, 1800, 1400, 1800, 1400, 1800, 1400, 1800, 1400];
	const finished = delays.length;
	const scriptedDurationMs = delays.reduce((total, delay) => total + delay, 0);
	let frame = $state(finished);
	let elapsedMs = $state(scriptedDurationMs);
	let mounted = $state(false);
	let preview = $state<HTMLDivElement | null>(null);
	let transcript = $state<HTMLDivElement | null>(null);
	let followTail = $state(true);
	let enteredView = $state(false);
	let systemReduced = $state(false);
	let paused = $state(false);
	let activityOpen = $state(true);
	let phaseOpen = $state(true);
	let expandedTool = $state<string | null>(null);

	const reduced = $derived(preferences.value.reduceMotion || systemReduced);
	const phase = $derived(reduced ? finished : frame);
	const busy = $derived(phase < finished);
	const completed = $derived(Math.min(tools.length, Math.floor(phase / 2)));
	const visibleTools = $derived(tools.slice(0, Math.ceil(phase / 2)));
	const duration = $derived(`${Math.floor(elapsedMs / 1000)}s`);
	const status = $derived(
		busy
			? paused
				? t('landing.stream.paused')
				: phase === 0
					? t('myaaw.chat.thinking')
					: t('myaaw.chat.tool_running')
			: t('myaaw.chat.completed')
	);

	// Same command/code/filesystem summary precedence as the real chat formatter.
	function invocation(tool: DemoTool) {
		return tool.args.command ?? tool.args.code ?? `${tool.args.tool_name} ${tool.args.path}`;
	}

	function toolTitle(tool: DemoTool) {
		if (tool.name === 'bash') return 'Bash';
		if (tool.name === 'execute_python') return 'Python';
		return t('myaaw.nav.files');
	}

	onMount(() => {
		const query = window.matchMedia('(prefers-reduced-motion: reduce)');
		systemReduced = query.matches;
		const reduceMotion = preferences.value.reduceMotion || query.matches;
		frame = reduceMotion ? finished : 0;
		elapsedMs = reduceMotion ? scriptedDurationMs : 0;
		mounted = true;
		const syncMotion = () => (systemReduced = query.matches);
		query.addEventListener('change', syncMotion);
		// On stacked phone layouts, let visitors see the tool sequence before it ends.
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					enteredView = true;
					observer.disconnect();
				}
			},
			{ threshold: 0.15 }
		);
		if (preview) observer.observe(preview);
		return () => {
			query.removeEventListener('change', syncMotion);
			observer.disconnect();
		};
	});

	$effect(() => {
		if (reduced) {
			frame = finished;
			elapsedMs = scriptedDurationMs;
			paused = false;
		}
	});

	// Cleanup cancels the only pending timer on pause, preference change or unmount.
	// Locale changes replace all prose/fixtures in place without restarting playback.
	$effect(() => {
		const current = frame;
		if (!mounted || !enteredView || reduced || paused || current >= finished) return;
		const timer = setTimeout(() => (frame = current + 1), delays[current]);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		if (!mounted || !enteredView || reduced || paused || !busy) return;
		const previous = untrack(() => elapsedMs);
		const started = performance.now();
		const update = () => {
			if (!reduced) elapsedMs = previous + performance.now() - started;
		};
		const timer = setInterval(update, 100);
		return () => {
			clearInterval(timer);
			update();
		};
	});

	$effect(() => {
		const current = phase;
		const viewport = transcript;
		if (!viewport || !enteredView || reduced || !followTail) return;
		let cancelled = false;
		void tick().then(() => {
			if (!cancelled && phase === current) viewport.scrollTop = viewport.scrollHeight;
		});
		return () => {
			cancelled = true;
		};
	});

	function trackScroll() {
		if (!transcript) return;
		followTail = transcript.scrollHeight - transcript.clientHeight - transcript.scrollTop < 32;
	}

	function toggleStep(step: string) {
		followTail = false;
		expandedTool = expandedTool === step ? null : step;
	}

	function replay() {
		if (reduced) return;
		followTail = true;
		if (transcript) transcript.scrollTop = 0;
		activityOpen = true;
		phaseOpen = true;
		expandedTool = null;
		paused = false;
		elapsedMs = 0;
		frame = 0;
	}
</script>

<div bind:this={preview} class={cn('run-stream min-w-0', className)} class:reduced>
	<Card class="min-w-0 gap-0 overflow-hidden py-0">
		<div class="flex items-start justify-between gap-3 border-b px-4 py-3">
			<div class="min-w-0 space-y-1.5">
				<div class="flex flex-wrap items-center gap-2">
					<span class="text-sm font-semibold">{t('myaaw.chat.title')}</span>
					<span class="rounded-full border bg-muted/40 px-2 py-0.5 text-[10px] font-medium">
						{t('landing.stream.illustration')}
					</span>
				</div>
			</div>
			{#if busy}
				<Button
					type="button"
					size="icon-sm"
					variant="ghost"
					class="shrink-0 rounded-full"
					onclick={() => (paused = !paused)}
					aria-label={paused ? t('landing.stream.resume') : t('landing.stream.pause')}
					title={paused ? t('landing.stream.resume') : t('landing.stream.pause')}
				>
					{#if paused}
						<Play class="h-4 w-4" />
					{:else}
						<Pause class="h-4 w-4" />
					{/if}
				</Button>
			{:else}
				<Button
					type="button"
					size="icon-sm"
					variant="ghost"
					class="shrink-0 rounded-full"
					onclick={replay}
					disabled={reduced}
					aria-label={t('landing.stream.replay')}
					title={t('landing.stream.replay')}
				>
					<RotateCcw class="h-4 w-4" />
				</Button>
			{/if}
		</div>

		<!-- Reserve the transcript height so playback never moves the rest of the page. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div
			bind:this={transcript}
			class="h-136 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain"
			role="region"
			aria-label={t('landing.stream.illustration')}
			tabindex="0"
			onscroll={trackScroll}
		>
			<div class="mx-auto w-full max-w-4xl space-y-4 px-4 py-5">
				<div class="flex justify-end">
					<div class="flex min-w-0 max-w-[80%] flex-col items-end gap-2">
						<div
							class="max-w-full rounded-(--chat-bubble-radius) bg-secondary px-4 py-2.5 text-sm whitespace-pre-wrap text-secondary-foreground ring-1 ring-(--chat-bubble-ring) wrap-anywhere"
						>
							{t('landing.stream.report_prompt')}
						</div>
					</div>
				</div>

				<div class="min-w-0 space-y-3">
					<div class="flex items-center gap-2">
						<Bloub mood="neutre" color={dark ? 'orange' : 'encre'} size={28} eager />
						<span class="text-xs font-medium">Myaaw</span>
					</div>

					<div class="min-w-0 space-y-0.5">
						<button
							type="button"
							class="flex max-w-full flex-wrap items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
							onclick={() => {
								followTail = false;
								activityOpen = !activityOpen;
							}}
							aria-expanded={activityOpen}
							aria-controls={`${id}-activity`}
						>
							<Clock class="h-3.5 w-3.5 shrink-0" />
							<span class="font-medium">{status}</span>
							<span class="tabular-nums" role="timer" aria-live="off">· {duration}</span>
							<ChevronDown
								class={cn(
									'h-3.5 w-3.5 shrink-0 transition-transform',
									activityOpen && 'rotate-180'
								)}
							/>
						</button>
						{#if activityOpen}
							<div id={`${id}-activity`} class="min-w-0 space-y-1 pl-2">
								<button
									type="button"
									class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
									onclick={() => {
										followTail = false;
										phaseOpen = !phaseOpen;
									}}
									aria-expanded={phaseOpen}
									aria-controls={`${id}-steps`}
								>
									<span class="font-medium">{t('myaaw.chat.tools_and_thinking')}</span>
									<ChevronDown
										class={cn(
											'h-3.5 w-3.5 shrink-0 transition-transform',
											phaseOpen && 'rotate-180'
										)}
									/>
								</button>
								{#if phaseOpen}
									<div id={`${id}-steps`} class="min-w-0 space-y-0.5 pl-2">
										<ChatStep
											title={t('myaaw.chat.think')}
											summary={t('landing.stream.thinking_summary')}
											icon={Brain}
											expanded={expandedTool === 'thought'}
											ontoggle={() => toggleStep('thought')}
											running={phase === 0 && !paused && !reduced}
										>
											<p class="whitespace-pre-wrap text-muted-foreground">
												{t('landing.stream.thinking_detail')}
											</p>
										</ChatStep>
										{#each visibleTools as tool, index (tool.id)}
											{@const complete = index < completed}
											<div class="min-w-0 space-y-0.5" data-demo-tool={tool.id}>
												<ChatStep
													title={toolTitle(tool)}
													summary={invocation(tool).replace(/\s+/g, ' ').trim()}
													icon={tool.name === 'bash'
														? SquareTerminal
														: tool.name === 'execute_python'
															? Code
															: FolderOpen}
													expanded={expandedTool === tool.id}
													ontoggle={() => toggleStep(tool.id)}
													running={!complete && !paused && !reduced}
												>
													<!-- Keep bounded tool output keyboard-scrollable. -->
													<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
													<div
														class="max-h-72 overflow-auto overscroll-contain rounded-lg bg-zinc-900 text-zinc-100 ring-1 ring-white/10 ring-inset"
														tabindex="0"
														role="region"
														aria-label={`${tool.name}: ${t('landing.stream.tool_input')}, ${t('landing.stream.tool_output')}`}
													>
														<pre
															class="p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap wrap-anywhere"><span
																class="text-zinc-500 select-none"
																>$ </span>{invocation(tool)}</pre>
														<div class="border-t border-white/10 p-3">
															<p class="mb-1 text-[10px] font-medium text-zinc-400">
																{t('landing.stream.tool_input')} · {tool.name}
															</p>
															<pre
																class="font-mono text-[11px] leading-relaxed whitespace-pre-wrap wrap-anywhere">{JSON.stringify(
																	tool.args,
																	null,
																	2
																)}</pre>
														</div>
														<div class="border-t border-white/10 p-3">
															<p class="mb-1 text-[10px] font-medium text-zinc-400">
																{t('landing.stream.tool_output')}
															</p>
															<pre
																class="font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-zinc-300 wrap-anywhere">{complete
																	? tool.result
																	: status}</pre>
														</div>
													</div>
												</ChatStep>
												{#if complete}
													<p
														class="flex items-start gap-1.5 pl-6 text-[11px] leading-relaxed text-muted-foreground"
													>
														<Check class="mt-0.5 h-3 w-3 shrink-0" />
														<span class="min-w-0 wrap-anywhere">{tool.outcome}</span>
													</p>
												{/if}
											</div>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</div>

					{#if !busy}
						<div class="min-w-0 text-sm wrap-anywhere">
							<Markdown content={t('landing.stream.report_answer', stats)} />
						</div>
					{/if}
				</div>
			</div>
		</div>
	</Card>
</div>

<style>
	.run-stream.reduced :global(*) {
		animation: none !important;
		transition: none !important;
	}

	@media (prefers-reduced-motion: reduce) {
		.run-stream :global(*) {
			animation: none !important;
			transition: none !important;
		}
	}
</style>
