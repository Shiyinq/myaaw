<script lang="ts">
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Separator } from '$lib/components/ui/separator';
	import { Activity, ChevronLeft, ChevronRight, RefreshCw, Trash2 } from 'lucide-svelte';
	import type {
		ObservabilitySummary,
		RunDetail,
		RunSummary,
		SessionDetail,
		SessionSummary,
		Span,
		StepStat
	} from '$lib/types';

	const { t } = useTranslation();
	const limit = 25;

	let summary = $state<ObservabilitySummary | null>(null);
	let runs = $state<RunSummary[]>([]);
	let detail = $state<RunDetail | null>(null);
	let loading = $state(false);
	let detailLoading = $state(false);
	let error = $state('');
	let loadKey = '';
	let offset = $state(0);
	let status = $state('');
	let trigger = $state('');
	let mode = $state<'runs' | 'sessions' | 'summary'>('summary');
	let summaryView = $state<'chart' | 'table'>('chart');
	let sessions = $state<SessionSummary[]>([]);
	let session = $state<SessionDetail | null>(null);
	let sessionLoading = $state(false);
	let highlightRuns = $state<Record<string, RunDetail | null>>({});
	let highlightKey = '';

	let agent = $derived(agentSelection.selected);

	interface SpanRow {
		span: Span;
		depth: number;
	}

	/** Flattens spans into indented rows, oldest first, without following cycles. */
	function spanRows(spans: Span[] | null | undefined): SpanRow[] {
		const list = spans ?? [];
		const byId = new Map(list.map((span) => [span.id, span]));
		return [...list]
			.sort((left, right) => (left.started_at ?? 0) - (right.started_at ?? 0))
			.map((span) => {
				let depth = 0;
				let parent = span.parent_id ?? null;
				const seen = new Set<string>();
				while (parent && byId.has(parent) && !seen.has(parent) && depth < 8) {
					seen.add(parent);
					depth += 1;
					parent = byId.get(parent)?.parent_id ?? null;
				}
				return { span, depth };
			});
	}

	let rows = $derived(spanRows(detail?.spans));

	interface HighlightCard {
		label: string;
		runId: string;
		value: string;
		caption: string;
		/** Bar width in percent, relative to the metric it is compared with. */
		share: number;
	}

	/** The summary only names the two runs, so the cards load their numbers. */
	let highlights: HighlightCard[] = $derived.by(() => {
		const cards: HighlightCard[] = [];
		const totalCost = summary?.cost_usd ?? 0;
		const expensiveId = summary?.most_expensive_run_id;
		if (expensiveId) {
			const run = highlightRuns[expensiveId]?.run;
			const cost = run?.model_cost_usd ?? 0;
			const share = totalCost > 0 ? (cost / totalCost) * 100 : 0;
			cards.push({
				label: t('myaaw.observability.most_expensive'),
				runId: expensiveId,
				value: run ? formatCost(cost) : '…',
				caption:
					run && totalCost > 0
						? t('myaaw.observability.share_of_cost', { percent: share.toFixed(1) })
						: '',
				share: Math.min(100, Math.max(1, share))
			});
		}
		const slowestId = summary?.slowest_run_id;
		if (slowestId) {
			const run = highlightRuns[slowestId]?.run;
			const totalMs = run?.total_ms ?? 0;
			const p95 = summary?.duration_ms_p95 ?? 0;
			const ratio = p95 > 0 ? totalMs / p95 : 0;
			cards.push({
				label: t('myaaw.observability.slowest'),
				runId: slowestId,
				value: run ? formatMs(totalMs) : '…',
				caption: run && p95 > 0 ? t('myaaw.observability.vs_p95', { ratio: ratio.toFixed(1) }) : '',
				share: Math.min(100, Math.max(1, ratio * 100))
			});
		}
		return cards;
	});

	interface ChartRow {
		label: string;
		value: number;
		/** Portion of `value` that failed, drawn in the error color. */
		error?: number;
		detail: string;
		/** Tooltip lines with the exact numbers behind the bar. */
		lines: string[];
	}

	/** Cost is the point of the model table, but unpriced models have none to plot. */
	let modelMetric: 'cost' | 'tokens' = $derived(
		(summary?.by_model ?? []).some((item) => item.cost_usd > 0) ? 'cost' : 'tokens'
	);

	let modelChart: ChartRow[] = $derived(
		(summary?.by_model ?? []).map((item) => ({
			label: `${item.model}${item.unpriced_calls > 0 ? ' *' : ''}`,
			value: modelMetric === 'cost' ? item.cost_usd : item.input_tokens + item.output_tokens,
			detail:
				modelMetric === 'cost'
					? `${formatTokens(item.input_tokens + item.output_tokens)} tok`
					: formatCost(item.cost_usd),
			lines: [
				`${t('myaaw.observability.calls')}: ${item.calls}`,
				`${t('myaaw.observability.input')}: ${formatTokens(item.input_tokens)}`,
				`${t('myaaw.observability.output')}: ${formatTokens(item.output_tokens)}`,
				`${t('myaaw.observability.cached')}: ${formatTokens(item.cache_hit_tokens)}`,
				`${t('myaaw.observability.cost')}: ${formatCost(item.cost_usd)}`
			]
		}))
	);

	let triggerChart: ChartRow[] = $derived(
		(summary?.by_trigger ?? []).map((item) => ({
			label: item.trigger,
			value: item.runs,
			error: item.errors,
			detail: item.errors > 0 ? `${item.runs} · ${item.errors} err` : String(item.runs),
			lines: [
				`${t('myaaw.observability.runs')}: ${item.runs}`,
				`${t('myaaw.observability.errors')}: ${item.errors}`,
				`${t('myaaw.observability.input')}: ${formatTokens(item.input_tokens)}`,
				`${t('myaaw.observability.output')}: ${formatTokens(item.output_tokens)}`,
				`${t('myaaw.observability.cost')}: ${formatCost(item.cost_usd)}`,
				`${t('myaaw.observability.tool_cost')}: ${formatCost(item.tool_cost_usd)}`
			]
		}))
	);

	let toolChart: ChartRow[] = $derived(
		(summary?.per_tool ?? []).map((item) => ({
			label: item.name,
			value: item.calls,
			error: item.errors,
			detail: formatMs(item.avg_ms),
			lines: [
				`${t('myaaw.observability.calls')}: ${item.calls}`,
				`${t('myaaw.observability.errors')}: ${item.errors}`,
				`${t('myaaw.observability.avg')}: ${formatMs(item.avg_ms)}`,
				`${t('myaaw.observability.cost')}: ${formatCost(item.cost_usd)}`
			]
		}))
	);

	interface StepPoint extends StepStat {
		x: number;
		avgY: number;
		p95Y: number;
		/** Hover zone boundaries in percent of the plot width. */
		zoneLeft: number;
		zoneWidth: number;
		crosshair: number;
		lines: string[];
	}

	interface StepChart {
		rows: StepPoint[];
		count: number;
		max: number;
		avg: string;
		p95: string;
		area: string;
		single: { avg: number; p95: number } | null;
	}

	/** Latency grows with the step index, so steps read best as a line chart. */
	let stepChart: StepChart = $derived.by(() => {
		const steps = summary?.per_step ?? [];
		const count = steps.length;
		const max = Math.max(1, ...steps.map((row) => Math.max(row.avg_ms, row.p95_ms)));
		const x = (index: number) => (count <= 1 ? 50 : (index / (count - 1)) * 100);
		const y = (value: number) => 100 - (value / max) * 100;
		const line = (pick: (row: StepStat) => number) =>
			steps.map((row, index) => `${x(index).toFixed(2)},${y(pick(row)).toFixed(2)}`).join(' ');
		const p95 = line((row) => row.p95_ms);
		const rows: StepPoint[] = steps.map((row, index) => {
			const zoneLeft = index === 0 ? 0 : (x(index - 1) + x(index)) / 2;
			const zoneRight = index === count - 1 ? 100 : (x(index) + x(index + 1)) / 2;
			const zoneWidth = Math.max(0.01, zoneRight - zoneLeft);
			return {
				...row,
				x: x(index),
				avgY: y(row.avg_ms),
				p95Y: y(row.p95_ms),
				zoneLeft,
				zoneWidth,
				crosshair: ((x(index) - zoneLeft) / zoneWidth) * 100,
				lines: [
					`${t('myaaw.observability.count')}: ${row.count}`,
					`${t('myaaw.observability.avg')}: ${formatMs(row.avg_ms)}`,
					`p95: ${formatMs(row.p95_ms)}`,
					`${t('myaaw.observability.cost')}: ${formatCost(row.cost_usd)}`
				]
			};
		});
		return {
			rows,
			count,
			max,
			avg: line((row) => row.avg_ms),
			p95,
			area: count > 1 ? `M0,100 L${p95.split(' ').join(' L')} L100,100 Z` : '',
			single: count === 1 ? { avg: y(steps[0].avg_ms), p95: y(steps[0].p95_ms) } : null
		};
	});

	function chartMax(rows: ChartRow[]) {
		return Math.max(1, ...rows.map((row) => row.value));
	}

	function barWidth(value: number, max: number) {
		if (value <= 0) return 0;
		return Math.max(1, Math.min(100, (value / max) * 100));
	}

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const summaryResult = await observabilityApi.summary(currentAgent, 7);
			summary = summaryResult;
			if (mode === 'sessions') {
				const sessionsResult = await observabilityApi.sessions(currentAgent);
				sessions = sessionsResult.sessions ?? [];
			} else if (mode === 'runs') {
				const runsResult = await observabilityApi.runs(currentAgent, {
					limit,
					offset,
					status: status || undefined,
					trigger: trigger || undefined
				});
				runs = runsResult.runs ?? [];
			}
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	function switchMode(next: 'runs' | 'sessions' | 'summary') {
		if (mode === next) return;
		mode = next;
		offset = 0;
		session = null;
	}

	async function openSession(item: SessionSummary) {
		if (!agent) return;
		sessionLoading = true;
		try {
			// Spans make the session answer render like a run trajectory instead of
			// a bare list of run ids.
			session = await observabilityApi.session(agent, item.conversation_id, true);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			sessionLoading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}:${mode}:${offset}:${status}:${trigger}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	$effect(() => {
		const currentAgent = agent;
		const ids = [summary?.most_expensive_run_id, summary?.slowest_run_id].filter(
			(id): id is string => Boolean(id)
		);
		if (!currentAgent || ids.length === 0) return;
		void loadHighlights(currentAgent, ids);
	});

	async function loadHighlights(currentAgent: string, ids: string[]) {
		const key = `${currentAgent}:${ids.join(',')}`;
		if (key === highlightKey) return;
		highlightKey = key;
		highlightRuns = {};
		const results = await Promise.all(
			ids.map((id) => observabilityApi.run(currentAgent, id).catch(() => null))
		);
		const next: Record<string, RunDetail | null> = {};
		ids.forEach((id, index) => (next[id] = results[index]));
		highlightRuns = next;
	}

	async function openRun(run: RunSummary) {
		if (!agent) return;
		detailLoading = true;
		try {
			detail = await observabilityApi.run(agent, run.id);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			detailLoading = false;
		}
	}

	async function purge() {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.observability.purge_confirm'),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await observabilityApi.purge(agent);
			detail = null;
			await load(agent);
			toastStore.success(t('myaaw.observability.purged'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function formatCost(value?: number) {
		return `$${(value ?? 0).toFixed(4)}`;
	}

	function formatMs(value?: number | null) {
		if (!value) return '—';
		return value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${value}ms`;
	}

	function formatTokens(value?: number) {
		return (value ?? 0).toLocaleString();
	}

	function formatBytes(value?: number) {
		if (!value) return '—';
		const units = ['B', 'KB', 'MB', 'GB'];
		let size = value;
		let unit = 0;
		while (size >= 1024 && unit < units.length - 1) {
			size /= 1024;
			unit += 1;
		}
		return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
	}

	function formatWhen(value?: number) {
		if (!value) return '—';
		// Observability timestamps are unix seconds; accept milliseconds too.
		const millis = value > 1e12 ? value : value * 1000;
		return new Date(millis).toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	interface SpanPart {
		label: string;
		text: string;
	}

	/** Splits an LLM payload into its thinking and answer, instead of one blob. */
	function spanParts(span: Span): SpanPart[] {
		if (span.kind === 'llm') {
			if (!span.output_json) return [];
			try {
				const parsed = JSON.parse(span.output_json) as {
					content?: unknown;
					thought?: unknown;
					tool_calls?: { function?: { name?: string } }[];
				};
				const parts: SpanPart[] = [];
				const thought = typeof parsed.thought === 'string' ? parsed.thought.trim() : '';
				const content = textFromContent(parsed.content).trim();
				if (thought) parts.push({ label: 'thinking', text: thought });
				if (content) parts.push({ label: 'text', text: content });
				if (parts.length > 0) return parts;
				if (parsed.tool_calls?.length) {
					const names = parsed.tool_calls
						.map((call) => call.function?.name)
						.filter(Boolean)
						.join(', ');
					return names ? [{ label: 'tool call', text: names }] : [];
				}
				return [];
			} catch {
				return [];
			}
		}
		const raw = truncate(span.input_json ?? span.output_json ?? '');
		return raw ? [{ label: '', text: raw }] : [];
	}

	/** Model content arrives as a string or as typed content parts. */
	function textFromContent(value: unknown): string {
		if (typeof value === 'string') return value;
		if (Array.isArray(value)) {
			return value
				.map((part) =>
					part && typeof part === 'object' && 'text' in part
						? String((part as { text?: unknown }).text ?? '')
						: ''
				)
				.filter(Boolean)
				.join('\n');
		}
		return '';
	}

	/** The last non-empty assistant answer in a run or a session turn. */
	function answerOf(spans: Span[] | null | undefined): string {
		const ordered = [...(spans ?? [])].sort(
			(left, right) => (right.started_at ?? 0) - (left.started_at ?? 0)
		);
		for (const span of ordered) {
			if (span.kind !== 'llm' || !span.output_json) continue;
			try {
				const parsed = JSON.parse(span.output_json) as { content?: unknown };
				const text = textFromContent(parsed.content).trim();
				if (text) return text;
			} catch {
				// Not JSON; skip this span.
			}
		}
		return '';
	}

	function truncate(value: string) {
		const oneLine = value.replace(/\s+/g, ' ').trim();
		return oneLine.length > 220 ? `${oneLine.slice(0, 220)}…` : oneLine;
	}
</script>

{#snippet answerBlock(text: string)}
	{#if text}
		<div class="rounded-md border bg-muted/40 p-2 text-[11px]">
			<div class="mb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
				{t('myaaw.observability.answer')}
			</div>
			<p class="whitespace-pre-wrap wrap-break-word">{text}</p>
		</div>
	{/if}
{/snippet}

{#snippet spanRow(row: SpanRow)}
	<div
		class="flex items-start gap-2 rounded-md border p-2 text-[11px]"
		style={`margin-left: ${row.depth * 16}px`}
	>
		<Badge variant="outline" class="shrink-0 font-mono text-[9px]">
			{row.span.kind}
		</Badge>
		<div class="min-w-0 flex-1">
			<div class="flex items-center justify-between gap-2">
				<span class="truncate font-medium">{row.span.name ?? row.span.kind}</span>
				<span class="shrink-0 text-muted-foreground">
					{formatMs(row.span.duration_ms)}
					{#if row.span.cost_usd}
						· {formatCost(row.span.cost_usd)}{/if}
				</span>
			</div>
			{#if row.span.input_tokens || row.span.output_tokens}
				<div class="text-[10px] text-muted-foreground">
					in {formatTokens(row.span.input_tokens)} · out {formatTokens(row.span.output_tokens)}
				</div>
			{/if}
			{#each spanParts(row.span) as part, index (index)}
				<p class="mt-1 wrap-break-word text-muted-foreground">
					{#if part.label}
						<span class="font-medium text-foreground/80">{part.label}:</span>
					{/if}
					{truncate(part.text)}
				</p>
			{/each}
		</div>
	</div>
{/snippet}

{#snippet summaryTable(title: string, headers: string[], rows: string[][])}
	<div>
		<div class="mb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
			{title}
		</div>
		<div class="overflow-x-auto rounded-md border">
			<table class="w-full border-collapse text-[11px]">
				<thead>
					<tr class="bg-muted/40 text-[10px] text-muted-foreground">
						{#each headers as header, index (index)}
							<th
								class={index === 0
									? 'px-2 py-1 text-left font-medium'
									: 'px-2 py-1 text-right font-medium'}
							>
								{header}
							</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each rows as row, rowIndex (rowIndex)}
						<tr class="border-t">
							{#each row as cell, cellIndex (cellIndex)}
								<td class={cellIndex === 0 ? 'px-2 py-1' : 'px-2 py-1 text-right tabular-nums'}>
									{cell}
								</td>
							{/each}
						</tr>
					{:else}
						<tr>
							<td class="px-2 py-1 text-muted-foreground" colspan={headers.length}>—</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/snippet}

{#snippet barChart(title: string, metric: string, rows: ChartRow[])}
	<div>
		<div class="mb-1 flex flex-wrap items-center justify-between gap-2">
			<div class="text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
				{title}
			</div>
			<div class="flex items-center gap-3 text-[10px] text-muted-foreground">
				{#if rows.some((row) => (row.error ?? 0) > 0)}
					<span class="flex items-center gap-1">
						<span class="h-2 w-2 rounded-sm bg-destructive/70"></span>
						{t('myaaw.observability.errors')}
					</span>
				{/if}
				<span>{metric}</span>
			</div>
		</div>
		{#if rows.length === 0}
			<div class="rounded-md border px-2 py-1 text-[11px] text-muted-foreground">—</div>
		{:else}
			<div class="space-y-1.5 rounded-md border p-2">
				{#each rows as row, index (index)}
					<div
						class="group relative flex flex-wrap items-center gap-x-2 gap-y-1 rounded-sm px-1 py-0.5 text-[11px] hover:bg-muted/40 sm:grid sm:grid-cols-[12rem_minmax(0,1fr)_8rem] sm:gap-x-3"
					>
						<span class="min-w-0 flex-1 truncate sm:order-1" title={row.label}>{row.label}</span>
						<span class="order-2 shrink-0 text-right text-muted-foreground tabular-nums sm:order-3">
							{row.detail}
						</span>
						<div
							class="order-last flex h-3 w-full overflow-hidden rounded-sm bg-muted/50 sm:order-2 sm:w-auto sm:min-w-0"
						>
							{#if row.value - (row.error ?? 0) > 0}
								<div
									class="h-full bg-primary/60"
									style={`width: ${barWidth(row.value - (row.error ?? 0), chartMax(rows))}%`}
								></div>
							{/if}
							{#if row.error}
								<div
									class="h-full bg-destructive/70"
									style={`width: ${barWidth(row.error, chartMax(rows))}%`}
								></div>
							{/if}
						</div>
						<div
							class="pointer-events-none absolute bottom-full left-0 z-20 mb-1 hidden w-max max-w-[min(16rem,80vw)] rounded-md border bg-background px-2 py-1 text-[10px] shadow-sm group-hover:block"
						>
							<div class="font-medium">{row.label}</div>
							{#each row.lines as line, lineIndex (lineIndex)}
								<div class="text-muted-foreground">{line}</div>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet lineChart(title: string, chart: StepChart)}
	<div>
		<div class="mb-1 flex flex-wrap items-center justify-between gap-2">
			<div class="text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
				{title}
			</div>
			<div class="flex items-center gap-3 text-[10px] text-muted-foreground">
				<span class="flex items-center gap-1">
					<span class="h-0.5 w-3 bg-primary"></span>
					{t('myaaw.observability.avg')}
				</span>
				<span class="flex items-center gap-1">
					<span class="h-0.5 w-3 bg-destructive/70"></span>
					p95
				</span>
				<span>{formatMs(chart.max)}</span>
			</div>
		</div>
		{#if chart.count === 0}
			<div class="rounded-md border px-2 py-1 text-[11px] text-muted-foreground">—</div>
		{:else}
			<div class="rounded-md border p-2" role="img" aria-label={title}>
				<div class="flex gap-2">
					<div
						class="flex flex-col justify-between text-right text-[9px] text-muted-foreground tabular-nums"
					>
						<span>{formatMs(chart.max)}</span>
						<span>0</span>
					</div>
					<div class="relative min-w-0 flex-1">
						<svg
							viewBox="0 0 100 100"
							preserveAspectRatio="none"
							class="h-24 w-full"
							aria-hidden="true"
						>
							{#each [0, 50, 100] as gridline (gridline)}
								<line
									x1="0"
									x2="100"
									y1={gridline}
									y2={gridline}
									class="stroke-border"
									vector-effect="non-scaling-stroke"
								/>
							{/each}
							{#if chart.area}
								<path d={chart.area} class="fill-primary/10" />
							{/if}
							{#if chart.single}
								<line
									x1="46"
									x2="54"
									y1={chart.single.avg}
									y2={chart.single.avg}
									class="stroke-primary"
									stroke-width="1.5"
									vector-effect="non-scaling-stroke"
								/>
								<line
									x1="46"
									x2="54"
									y1={chart.single.p95}
									y2={chart.single.p95}
									class="stroke-destructive/70"
									stroke-width="1.5"
									vector-effect="non-scaling-stroke"
								/>
							{:else}
								<polyline
									points={chart.avg}
									fill="none"
									class="stroke-primary"
									stroke-width="1.5"
									stroke-linejoin="round"
									vector-effect="non-scaling-stroke"
								/>
								<polyline
									points={chart.p95}
									fill="none"
									class="stroke-destructive/70"
									stroke-width="1.5"
									stroke-dasharray="3 2"
									stroke-linejoin="round"
									vector-effect="non-scaling-stroke"
								/>
							{/if}
						</svg>

						{#each chart.rows as point, index (index)}
							<span
								class="pointer-events-none absolute h-1 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary"
								style={`left: ${point.x}%; top: ${point.avgY}%`}
							></span>
						{/each}

						<div class="absolute inset-0 flex">
							{#each chart.rows as point, index (index)}
								<div
									class="group absolute inset-y-0"
									style={`left: ${point.zoneLeft}%; width: ${point.zoneWidth}%`}
								>
									<div
										class="absolute inset-y-0 hidden w-px bg-foreground/25 group-hover:block"
										style={`left: ${point.crosshair}%`}
									></div>
									<span
										class="pointer-events-none absolute hidden h-1.5 w-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary ring-2 ring-background group-hover:block"
										style={`left: ${point.crosshair}%; top: ${point.avgY}%`}
									></span>
									<div
										class={[
											'pointer-events-none absolute top-0 z-20 hidden w-max max-w-[min(16rem,80vw)] rounded-md border bg-background px-2 py-1 text-[10px] shadow-sm group-hover:block',
											index === 0
												? 'left-0'
												: index === chart.count - 1
													? 'right-0'
													: 'left-1/2 -translate-x-1/2'
										]}
									>
										<div class="font-medium">
											{t('myaaw.observability.step')}
											{point.step_index}
										</div>
										{#each point.lines as line, lineIndex (lineIndex)}
											<div class="text-muted-foreground">{line}</div>
										{/each}
									</div>
								</div>
							{/each}
						</div>
					</div>
				</div>
				<div class="mt-1 flex justify-between text-[9px] text-muted-foreground">
					<span>{t('myaaw.observability.step')} {chart.rows[0].step_index}</span>
					{#if chart.count > 2}
						<span>
							{t('myaaw.observability.step')}
							{chart.rows[Math.floor(chart.count / 2)].step_index}
						</span>
					{/if}
					<span>{t('myaaw.observability.step')} {chart.rows[chart.count - 1].step_index}</span>
				</div>
			</div>
		{/if}
	</div>
{/snippet}

<TopBarActions id="observability-actions">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
		<TopBarButton
			label={t('myaaw.observability.purge')}
			icon={Trash2}
			onclick={purge}
			variant="destructive"
		/>
	{/snippet}
</TopBarActions>

<div
	class={[
		'flex h-full min-h-0 flex-col gap-3 overflow-y-auto p-4 lg:gap-4 lg:p-6',
		mode === 'summary' ? 'lg:overflow-y-auto' : 'lg:overflow-hidden'
	]}
>
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="grid shrink-0 gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.observability.runs')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{summary?.runs ?? 0}</div>
					<p class="text-[10px] text-muted-foreground">
						{t('myaaw.observability.errors')}: {summary?.errors ?? 0}
						({((summary?.error_rate ?? 0) * 100).toFixed(1)}%)
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.observability.latency')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{formatMs(summary?.duration_ms_p50)}</div>
					<p class="text-[10px] text-muted-foreground">
						p95 {formatMs(summary?.duration_ms_p95)}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.observability.tokens')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{formatTokens(summary?.input_tokens)}</div>
					<p class="text-[10px] text-muted-foreground">
						out {formatTokens(summary?.output_tokens)}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.observability.cost')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{formatCost(summary?.cost_usd)}</div>
					<p class="text-[10px] text-muted-foreground">
						tools: {formatCost(summary?.tool_cost_usd)}
						{#if summary && !summary.cost_complete}
							· {t('myaaw.observability.incomplete')}{/if}
					</p>
				</CardContent>
			</Card>
		</div>

		<div
			class={mode === 'summary'
				? 'grid gap-3 lg:grid-cols-5 lg:gap-4'
				: 'grid gap-3 lg:min-h-0 lg:flex-1 lg:grid-cols-5 lg:gap-4 lg:overflow-visible'}
		>
			<Card
				class={mode === 'summary'
					? 'flex flex-col lg:col-span-5'
					: 'flex min-h-0 flex-col lg:col-span-2'}
			>
				<CardHeader class="shrink-0 space-y-2">
					<div class="flex items-center justify-between gap-2">
						<CardTitle class="flex items-center gap-2 text-sm">
							<Activity class="h-4 w-4 shrink-0" />
							{mode === 'summary'
								? t('myaaw.observability.summary')
								: t('myaaw.observability.recent')}
						</CardTitle>
						<div class="flex shrink-0 gap-1">
							<Button
								size="xs"
								variant={mode === 'summary' ? 'secondary' : 'ghost'}
								onclick={() => switchMode('summary')}
							>
								{t('myaaw.observability.summary')}
							</Button>
							<Button
								size="xs"
								variant={mode === 'runs' ? 'secondary' : 'ghost'}
								onclick={() => switchMode('runs')}
							>
								{t('myaaw.observability.runs')}
							</Button>
							<Button
								size="xs"
								variant={mode === 'sessions' ? 'secondary' : 'ghost'}
								onclick={() => switchMode('sessions')}
							>
								{t('myaaw.observability.sessions')}
							</Button>
						</div>
					</div>
					{#if mode === 'runs'}
						<div class="flex gap-2">
							<select
								bind:value={status}
								onchange={() => (offset = 0)}
								class="h-8 w-full rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="">{t('myaaw.observability.all_status')}</option>
								<option value="completed">completed</option>
								<option value="failed">failed</option>
								<option value="running">running</option>
							</select>
							<select
								bind:value={trigger}
								onchange={() => (offset = 0)}
								class="h-8 w-full rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="">{t('myaaw.observability.all_triggers')}</option>
								<option value="user">user</option>
								<option value="cron">cron</option>
								<option value="subagent">subagent</option>
								<option value="a2a">a2a</option>
							</select>
						</div>
					{/if}
				</CardHeader>
				<CardContent
					class={mode === 'summary' ? 'space-y-1' : 'min-h-0 flex-1 space-y-1 overflow-y-auto'}
				>
					{#if mode === 'summary'}
						<div class="space-y-4 p-1 text-[11px]">
							<div
								class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[10px] text-muted-foreground"
							>
								<span>
									{t('myaaw.observability.cached')}: {formatTokens(summary?.cache_hit_tokens)}
								</span>
								<span>p95: {formatMs(summary?.duration_ms_p95)}</span>
								<span>TTFT p50: {formatMs(summary?.first_token_ms_p50)}</span>
								<span>
									{t('myaaw.observability.database')}: {formatBytes(summary?.database_bytes)}
								</span>
								<div class="ml-auto flex gap-1">
									<Button
										size="xs"
										variant={summaryView === 'chart' ? 'secondary' : 'ghost'}
										onclick={() => (summaryView = 'chart')}
									>
										{t('myaaw.observability.charts')}
									</Button>
									<Button
										size="xs"
										variant={summaryView === 'table' ? 'secondary' : 'ghost'}
										onclick={() => (summaryView = 'table')}
									>
										{t('myaaw.observability.tables')}
									</Button>
								</div>
							</div>

							{#if summary && !summary.cost_complete}
								<p class="rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-[10px]">
									{t('myaaw.observability.unpriced_note', {
										runs: summary.unpriced_runs,
										input: formatTokens(summary.unpriced_input_tokens),
										output: formatTokens(summary.unpriced_output_tokens)
									})}
								</p>
							{/if}

							{#if summaryView === 'chart'}
								{@render barChart(
									t('myaaw.observability.per_model'),
									modelMetric === 'cost'
										? t('myaaw.observability.cost')
										: t('myaaw.observability.tokens'),
									modelChart
								)}
								{@render barChart(
									t('myaaw.observability.per_trigger'),
									t('myaaw.observability.runs'),
									triggerChart
								)}
								{@render barChart(
									t('myaaw.observability.per_tool'),
									t('myaaw.observability.calls'),
									toolChart
								)}
								{@render lineChart(t('myaaw.observability.per_step'), stepChart)}
							{:else}
								{@render summaryTable(
									t('myaaw.observability.per_model'),
									[
										t('myaaw.observability.model'),
										t('myaaw.observability.calls'),
										t('myaaw.observability.input'),
										t('myaaw.observability.output'),
										t('myaaw.observability.cost')
									],
									(summary?.by_model ?? []).map((item) => [
										`${item.model}${item.unpriced_calls > 0 ? ' *' : ''}`,
										String(item.calls),
										formatTokens(item.input_tokens),
										formatTokens(item.output_tokens),
										formatCost(item.cost_usd)
									])
								)}

								{@render summaryTable(
									t('myaaw.observability.per_trigger'),
									[
										t('myaaw.observability.trigger'),
										t('myaaw.observability.runs'),
										t('myaaw.observability.errors'),
										t('myaaw.observability.input'),
										t('myaaw.observability.output'),
										t('myaaw.observability.cost'),
										t('myaaw.observability.tool_cost')
									],
									(summary?.by_trigger ?? []).map((item) => [
										item.trigger,
										String(item.runs),
										String(item.errors),
										formatTokens(item.input_tokens),
										formatTokens(item.output_tokens),
										formatCost(item.cost_usd),
										formatCost(item.tool_cost_usd)
									])
								)}

								{@render summaryTable(
									t('myaaw.observability.per_tool'),
									[
										t('myaaw.observability.tool'),
										t('myaaw.observability.calls'),
										t('myaaw.observability.errors'),
										t('myaaw.observability.avg'),
										t('myaaw.observability.cost')
									],
									(summary?.per_tool ?? []).map((item) => [
										item.name,
										String(item.calls),
										String(item.errors),
										formatMs(item.avg_ms),
										formatCost(item.cost_usd)
									])
								)}

								{@render summaryTable(
									t('myaaw.observability.per_step'),
									[
										t('myaaw.observability.step'),
										t('myaaw.observability.count'),
										t('myaaw.observability.avg'),
										'p95',
										t('myaaw.observability.cost')
									],
									(summary?.per_step ?? []).map((item) => [
										String(item.step_index),
										String(item.count),
										formatMs(item.avg_ms),
										formatMs(item.p95_ms),
										formatCost(item.cost_usd)
									])
								)}
							{/if}

							{#if highlights.length > 0}
								{#if summaryView === 'chart'}
									<div>
										<div
											class="mb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase"
										>
											{t('myaaw.observability.highlights')}
										</div>
										<div class="grid gap-2 sm:grid-cols-2">
											{#each highlights as item (item.label)}
												<div class="rounded-md border p-2">
													<div class="flex items-baseline justify-between gap-2">
														<span class="text-[10px] text-muted-foreground">{item.label}</span>
														<span class="text-sm font-semibold tabular-nums">{item.value}</span>
													</div>
													<div class="mt-1 h-1.5 overflow-hidden rounded-sm bg-muted/50">
														<div class="h-full bg-primary/60" style={`width: ${item.share}%`}></div>
													</div>
													<div
														class="mt-1 flex items-center justify-between gap-2 text-[9px] text-muted-foreground"
													>
														<span class="truncate font-mono" title={item.runId}>{item.runId}</span>
														{#if item.caption}
															<span class="shrink-0">{item.caption}</span>
														{/if}
													</div>
												</div>
											{/each}
										</div>
									</div>
								{:else}
									{@render summaryTable(
										t('myaaw.observability.highlights'),
										[
											t('myaaw.observability.metric'),
											t('myaaw.observability.run'),
											t('myaaw.observability.value'),
											t('myaaw.observability.comparison')
										],
										highlights.map((item) => [item.label, item.runId, item.value, item.caption])
									)}
								{/if}
							{/if}
						</div>
					{:else if mode === 'sessions'}
						{#if !loading && sessions.length === 0}
							<EmptyState
								title={t('myaaw.observability.empty')}
								description={t('myaaw.observability.empty_desc')}
								bloub="curieux"
								icon={Activity}
							/>
						{/if}
						{#each sessions as item (item.conversation_id)}
							<button
								class={[
									'w-full rounded-md border p-2 text-left transition-colors list-row',
									session?.conversation_id === item.conversation_id ? 'bg-accent' : 'hover:bg-muted'
								]}
								aria-current={session?.conversation_id === item.conversation_id
									? 'true'
									: undefined}
								onclick={() => openSession(item)}
							>
								<div class="flex items-center justify-between gap-2">
									<span class="truncate font-mono text-[11px]">
										{item.conversation_id}
									</span>
									<Badge variant="outline">{item.turns}</Badge>
								</div>
								<div
									class="mt-1 flex items-center justify-between text-[10px] text-muted-foreground"
								>
									<span>{item.channel ?? '—'} · {formatWhen(item.last_active_at)}</span>
									<span>
										{formatTokens(item.input_tokens + item.output_tokens)} tok · {formatCost(
											item.cost_usd
										)}
									</span>
								</div>
							</button>
						{/each}
					{:else}
						{#if !loading && runs.length === 0}
							<EmptyState
								title={t('myaaw.observability.empty')}
								description={t('myaaw.observability.empty_desc')}
								bloub="curieux"
								icon={Activity}
							/>
						{/if}
						{#each runs as run (run.id)}
							<button
								class={[
									'w-full rounded-md border p-2 text-left transition-colors list-row',
									detail?.run.id === run.id ? 'bg-accent' : 'hover:bg-muted'
								]}
								aria-current={detail?.run.id === run.id ? 'true' : undefined}
								onclick={() => openRun(run)}
							>
								<div class="flex items-center justify-between gap-2">
									<span class="truncate font-mono text-[11px]">{run.id}</span>
									<Badge variant={run.status === 'completed' ? 'secondary' : 'destructive'}>
										{run.status}
									</Badge>
								</div>
								<div
									class="mt-1 flex items-center justify-between text-[10px] text-muted-foreground"
								>
									<span>{run.model ?? run.agent ?? '—'} · {run.trigger ?? 'user'}</span>
									<span>{formatMs(run.total_ms)} · {formatCost(run.model_cost_usd)}</span>
								</div>
							</button>
						{/each}
					{/if}
				</CardContent>
				{#if mode === 'runs'}
					<div class="flex shrink-0 items-center justify-between gap-2 border-t px-4 pt-3">
						<Button
							variant="outline"
							size="xs"
							disabled={offset === 0}
							onclick={() => (offset = Math.max(0, offset - limit))}
						>
							<ChevronLeft class="h-3.5 w-3.5" />
						</Button>
						{#if runs.length > 0}
							<span class="text-[10px] text-muted-foreground">
								{offset + 1}–{offset + runs.length}
							</span>
						{/if}
						<Button
							variant="outline"
							size="xs"
							disabled={runs.length < limit}
							onclick={() => (offset = offset + limit)}
						>
							<ChevronRight class="h-3.5 w-3.5" />
						</Button>
					</div>
				{/if}
			</Card>

			{#if mode !== 'summary'}
				<Card class="flex min-h-0 flex-col lg:col-span-3">
					<CardHeader class="shrink-0">
						<CardTitle class="text-sm">
							{mode === 'sessions'
								? t('myaaw.observability.session')
								: t('myaaw.observability.trajectory')}
						</CardTitle>
						<CardDescription class="truncate">
							{#if mode === 'sessions'}
								{#if session}
									{session.conversation_id} · {session.turns} ·
									{formatWhen(session.started_at)}
								{:else}
									{t('myaaw.observability.select_session')}
								{/if}
							{:else if detail}
								{detail.run.id} · {detail.run.model ?? '—'} · {formatMs(detail.run.total_ms)} ·
								{formatCost(detail.run.model_cost_usd)}
							{:else}
								{t('myaaw.observability.select_run')}
							{/if}
						</CardDescription>
					</CardHeader>
					<CardContent class="min-h-0 flex-1 space-y-1 overflow-y-auto">
						{#if mode === 'sessions'}
							{#if sessionLoading}
								<p class="text-sm text-muted-foreground">{t('common.loading')}</p>
							{:else if session}
								{#each session.runs ?? [] as turn (turn.id)}
									<div class="rounded-md border">
										<div class="flex items-center justify-between gap-2 px-2 py-1.5 text-[11px]">
											<span class="truncate font-mono">{turn.id}</span>
											<Badge variant={turn.status === 'completed' ? 'secondary' : 'destructive'}>
												{turn.status}
											</Badge>
										</div>
										<div
											class="flex flex-wrap items-center gap-x-2 gap-y-0.5 border-t px-2 py-1 text-[10px] text-muted-foreground"
										>
											<span>{turn.model ?? '—'} · {turn.trigger ?? 'user'}</span>
											<span>{formatMs(turn.total_ms)} · {formatCost(turn.model_cost_usd)}</span>
											{#if turn.input_tokens || turn.output_tokens}
												<span>
													in {formatTokens(turn.input_tokens)} · out {formatTokens(
														turn.output_tokens
													)}
												</span>
											{/if}
										</div>
										<div class="space-y-1 border-t p-2">
											{#if answerOf(turn.spans)}
												<div class="mb-2">
													{@render answerBlock(answerOf(turn.spans))}
												</div>
											{/if}
											{#each spanRows(turn.spans) as row (row.span.id)}
												{@render spanRow(row)}
											{:else}
												<p class="text-[10px] text-muted-foreground">
													{t('myaaw.observability.no_spans')}
												</p>
											{/each}
										</div>
									</div>
								{/each}
							{:else}
								<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
									{t('myaaw.observability.select_session')}
								</p>
							{/if}
						{:else if detailLoading}
							<p class="text-sm text-muted-foreground">{t('common.loading')}</p>
						{:else if detail}
							{#if answerOf(detail.spans)}
								<div class="mb-2">
									{@render answerBlock(answerOf(detail.spans))}
								</div>
							{/if}
							{#each rows as row (row.span.id)}
								{@render spanRow(row)}
							{/each}
							<Separator class="my-2" />
							<p class="text-[10px] text-muted-foreground">
								{t('myaaw.observability.spans', { count: rows.length })}
							</p>
						{:else}
							<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
								{t('myaaw.observability.select_run')}
							</p>
						{/if}
					</CardContent>
				</Card>
			{/if}
		</div>
	{/if}
</div>
