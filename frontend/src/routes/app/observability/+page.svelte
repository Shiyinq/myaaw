<script lang="ts">
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
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
	import type { ObservabilitySummary, RunDetail, RunSummary, Span } from '$lib/types';

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

	let agent = $derived(agentSelection.selected);

	let rows = $derived.by(() => {
		const spans = detail?.spans ?? [];
		const byId = new Map(spans.map((span) => [span.id, span]));
		return [...spans]
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
	});

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const [summaryResult, runsResult] = await Promise.all([
				observabilityApi.summary(currentAgent, 7),
				observabilityApi.runs(currentAgent, {
					limit,
					offset,
					status: status || undefined,
					trigger: trigger || undefined
				})
			]);
			summary = summaryResult;
			runs = runsResult.runs ?? [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}:${offset}:${status}:${trigger}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

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
		if (!window.confirm(t('myaaw.observability.purge_confirm'))) return;
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

	function preview(span: Span) {
		if (span.kind === 'llm' && span.output_json) {
			try {
				const parsed = JSON.parse(span.output_json) as { content?: string; thought?: string };
				const parts = [
					parsed.thought ? `thinking: ${parsed.thought}` : '',
					parsed.content ? `text: ${parsed.content}` : ''
				].filter(Boolean);
				if (parts.length > 0) return truncate(parts.join('\n'));
			} catch {
				// fall through to the raw payload
			}
		}
		return truncate(span.input_json ?? span.output_json ?? '');
	}

	function truncate(value: string) {
		const oneLine = value.replace(/\s+/g, ' ').trim();
		return oneLine.length > 220 ? `${oneLine.slice(0, 220)}…` : oneLine;
	}
</script>

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
	class="flex h-full min-h-0 flex-col gap-3 overflow-y-auto p-4 lg:gap-4 lg:overflow-hidden lg:p-6"
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

		<div class="grid gap-3 lg:min-h-0 lg:flex-1 lg:grid-cols-5 lg:gap-4 lg:overflow-visible">
			<Card class="flex min-h-0 flex-col lg:col-span-2">
				<CardHeader class="shrink-0 space-y-2">
					<CardTitle class="flex items-center gap-2 text-sm">
						<Activity class="h-4 w-4 shrink-0" />
						{t('myaaw.observability.recent')}
					</CardTitle>
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
				</CardHeader>
				<CardContent class="min-h-0 flex-1 space-y-1 overflow-y-auto">
					{#if !loading && runs.length === 0}
						<EmptyState
							title={t('myaaw.observability.empty')}
							description={t('myaaw.observability.empty_desc')}
							icon={Activity}
						/>
					{/if}
					{#each runs as run (run.id)}
						<button
							class={[
								'w-full rounded-md border p-2 text-left transition-colors',
								detail?.run.id === run.id ? 'bg-accent' : 'hover:bg-muted'
							]}
							onclick={() => openRun(run)}
						>
							<div class="flex items-center justify-between gap-2">
								<span class="truncate font-mono text-[11px]">{run.id}</span>
								<Badge variant={run.status === 'completed' ? 'secondary' : 'destructive'}>
									{run.status}
								</Badge>
							</div>
							<div class="mt-1 flex items-center justify-between text-[10px] text-muted-foreground">
								<span>{run.model ?? run.agent ?? '—'} · {run.trigger ?? 'user'}</span>
								<span>{formatMs(run.total_ms)} · {formatCost(run.model_cost_usd)}</span>
							</div>
						</button>
					{/each}
				</CardContent>
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
			</Card>

			<Card class="flex min-h-0 flex-col lg:col-span-3">
				<CardHeader class="shrink-0">
					<CardTitle class="text-sm">{t('myaaw.observability.trajectory')}</CardTitle>
					<CardDescription class="truncate">
						{#if detail}
							{detail.run.id} · {detail.run.model ?? '—'} · {formatMs(detail.run.total_ms)} ·
							{formatCost(detail.run.model_cost_usd)}
						{:else}
							{t('myaaw.observability.select_run')}
						{/if}
					</CardDescription>
				</CardHeader>
				<CardContent class="min-h-0 flex-1 space-y-1 overflow-y-auto">
					{#if detailLoading}
						<p class="text-sm text-muted-foreground">{t('common.loading')}</p>
					{:else if detail}
						{#each rows as row (row.span.id)}
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
											in {formatTokens(row.span.input_tokens)} · out {formatTokens(
												row.span.output_tokens
											)}
										</div>
									{/if}
									{#if preview(row.span)}
										<p class="mt-1 break-words text-muted-foreground">{preview(row.span)}</p>
									{/if}
								</div>
							</div>
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
		</div>
	{/if}
</div>
