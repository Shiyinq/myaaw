<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { Span } from '$lib/types';

	interface Props {
		spans: Span[] | null | undefined;
		showCount?: boolean;
	}

	let { spans, showCount = true }: Props = $props();
	const { t } = useTranslation();

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

	let rows = $derived(spanRows(spans));
	let answer = $derived(answerOf(spans));

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
		<div class="min-w-0 rounded-md border bg-muted/40 p-2 text-[11px]">
			<div class="mb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
				{t('myaaw.observability.answer')}
			</div>
			<p class="whitespace-pre-wrap wrap-anywhere">{text}</p>
		</div>
	{/if}
{/snippet}

{#snippet spanRow(row: SpanRow)}
	<div
		class="flex min-w-0 items-start gap-2 rounded-md border p-2 text-[11px]"
		data-slot="run-span"
		data-kind={row.span.kind}
		data-depth={row.depth}
		style={`margin-left: ${row.depth * 16}px`}
	>
		<Badge
			variant="outline"
			class="max-w-1/2 shrink-0 font-mono text-[9px] whitespace-normal wrap-anywhere"
		>
			{row.span.kind}
		</Badge>
		<div class="min-w-0 flex-1 wrap-anywhere">
			<div class="flex min-w-0 flex-wrap items-center justify-between gap-x-2 gap-y-0.5">
				<span class="min-w-0 flex-1 truncate font-medium">{row.span.name ?? row.span.kind}</span>
				<span class="max-w-full shrink-0 text-muted-foreground">
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
				<p class="mt-1 text-muted-foreground">
					{#if part.label}
						<span class="font-medium text-foreground/80">{part.label}:</span>
					{/if}
					{truncate(part.text)}
				</p>
			{/each}
		</div>
	</div>
{/snippet}

<div class="min-w-0 space-y-1" data-slot="run-trajectory">
	{#if answer}
		<div class="mb-2">
			{@render answerBlock(answer)}
		</div>
	{/if}
	{#each rows as row (row.span.id)}
		{@render spanRow(row)}
	{:else}
		{#if !showCount}
			<p class="text-[10px] text-muted-foreground">
				{t('myaaw.observability.no_spans')}
			</p>
		{/if}
	{/each}
	{#if showCount}
		<Separator class="my-2" />
		<p class="text-[10px] text-muted-foreground">
			{t('myaaw.observability.spans', { count: rows.length })}
		</p>
	{/if}
</div>
