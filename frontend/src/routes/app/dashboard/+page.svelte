<script lang="ts">
	import { goto } from '$app/navigation';
	import { agentSelection } from '$lib/stores';
	import { server as serverApi } from '$lib/apis/server';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
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
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Activity, Bot, MessageSquare, Play, RefreshCw, ScrollText, Server } from 'lucide-svelte';
	import type { ObservabilitySummary, RunSummary, ServerStatus } from '$lib/types';

	const { t } = useTranslation();

	let status = $state<ServerStatus | null>(null);
	let summary = $state<ObservabilitySummary | null>(null);
	let runs = $state<RunSummary[]>([]);
	let loading = $state(false);
	let error = $state('');
	let loadedFor = '';

	let agent = $derived(agentSelection.selected);

	async function loadFor(currentAgent: string) {
		loading = true;
		error = '';
		const [statusResult, summaryResult, runsResult] = await Promise.allSettled([
			serverApi.status(currentAgent),
			observabilityApi.summary(currentAgent, 7),
			observabilityApi.runs(currentAgent, { limit: 5 })
		]);

		status = statusResult.status === 'fulfilled' ? statusResult.value : null;
		summary = summaryResult.status === 'fulfilled' ? summaryResult.value : null;
		runs = runsResult.status === 'fulfilled' ? (runsResult.value.runs ?? []) : [];

		const failed = [statusResult, summaryResult, runsResult].find(
			(result) => result.status === 'rejected'
		);
		if (failed && failed.status === 'rejected') {
			error = getErrorMessage(failed.reason);
		}
		loading = false;
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}:${agentSelection.agents.length}`;
		if (!currentAgent || key === loadedFor) return;
		loadedFor = key;
		void loadFor(currentAgent);
	});

	async function refresh() {
		await agentSelection.load(true);
		const currentAgent = agentSelection.selected;
		if (!currentAgent) return;
		loadedFor = `${currentAgent}:${agentSelection.agents.length}`;
		await loadFor(currentAgent);
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

	function formatTime(timestamp?: number) {
		if (!timestamp) return '—';
		const millis = timestamp > 1e12 ? timestamp : timestamp * 1000;
		return new Date(millis).toLocaleString();
	}
</script>

<TopBarActions id="agent-dashboard-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('myaaw.home.open_chat')}
			icon={MessageSquare}
			onclick={() => goto('/app/chat')}
			variant="default"
		/>
		<TopBarButton label={t('common.refresh')} icon={RefreshCw} onclick={refresh} {loading} />
	{/snippet}
</TopBarActions>

<div class="space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="flex flex-col items-center gap-3 py-10 text-center">
				<Bot class="h-6 w-6 text-muted-foreground" />
				<p class="text-sm text-muted-foreground">{t('myaaw.agent.none')}</p>
				<Button variant="outline" size="sm" onclick={() => goto('/app')}>
					{t('myaaw.nav.fleet')}
				</Button>
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.home.runs')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{summary?.runs ?? 0}</div>
					<p class="text-[10px] text-muted-foreground">
						{t('myaaw.observability.errors')}: {summary?.errors ?? 0}
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
					<CardTitle class="text-sm font-medium">{t('myaaw.home.cost')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{formatCost(summary?.cost_usd)}</div>
					<p class="text-[10px] text-muted-foreground">
						{t('myaaw.observability.tokens')}: {formatTokens(summary?.input_tokens)}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.nav.server')}</CardTitle>
				</CardHeader>
				<CardContent class="space-y-2">
					<div class="flex items-center gap-2">
						{#if status?.running}
							<Badge variant="outline" class="text-emerald-600 dark:text-emerald-400">
								{t('myaaw.agent.running')}
							</Badge>
						{:else}
							<Badge variant="outline" class="text-muted-foreground">
								{t('myaaw.agent.stopped')}
							</Badge>
						{/if}
						{#if status?.port}
							<span class="font-mono text-xs text-muted-foreground">:{status.port}</span>
						{/if}
					</div>
					<Button variant="outline" size="xs" onclick={() => goto('/app/server')}>
						<Server class="mr-1.5 h-3 w-3" />
						{t('myaaw.nav.server')}
					</Button>
				</CardContent>
			</Card>
		</div>

		<div class="grid gap-4 lg:grid-cols-3">
			<Card class="lg:col-span-2">
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle>{t('myaaw.home.recent')}</CardTitle>
						<CardDescription>{agent}</CardDescription>
					</div>
					<Button variant="outline" size="sm" onclick={() => goto('/app/observability')}>
						<Activity class="mr-2 h-4 w-4" />
						{t('myaaw.nav.observability')}
					</Button>
				</CardHeader>
				<CardContent class="space-y-2">
					{#if loading}
						{#each Array.from({ length: 4 }) as _, index (index)}
							<Skeleton class="h-12 w-full" />
						{/each}
					{:else if runs.length === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.home.no_runs')}</p>
					{:else}
						{#each runs as run (run.id)}
							<div class="rounded-md border p-2">
								<div class="flex items-center justify-between gap-2">
									<span class="truncate text-sm font-medium">
										{run.model ?? run.agent ?? run.id}
									</span>
									<Badge variant={run.status === 'completed' ? 'secondary' : 'destructive'}>
										{run.status}
									</Badge>
								</div>
								<div
									class="mt-1 flex items-center justify-between text-[11px] text-muted-foreground"
								>
									<span>{formatTime(run.started_at)}</span>
									<span>{formatCost(run.model_cost_usd)}</span>
								</div>
							</div>
						{/each}
					{/if}
					<Separator />
					<Button class="w-full justify-start" variant="ghost" onclick={() => goto('/app/chat')}>
						<Play class="mr-2 h-4 w-4" />
						{t('myaaw.home.open_chat')}
					</Button>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>{t('myaaw.home.quick_actions')}</CardTitle>
					<CardDescription>{agent}</CardDescription>
				</CardHeader>
				<CardContent class="flex flex-col gap-1">
					<Button variant="ghost" class="justify-start" onclick={() => goto('/app/chat')}>
						<MessageSquare class="mr-2 h-4 w-4" />
						{t('myaaw.nav.chat')}
					</Button>
					<Button variant="ghost" class="justify-start" onclick={() => goto('/app/observability')}>
						<Activity class="mr-2 h-4 w-4" />
						{t('myaaw.nav.observability')}
					</Button>
					<Button variant="ghost" class="justify-start" onclick={() => goto('/app/logs')}>
						<ScrollText class="mr-2 h-4 w-4" />
						{t('myaaw.nav.logs')}
					</Button>
					<Button variant="ghost" class="justify-start" onclick={() => goto('/app/server')}>
						<Server class="mr-2 h-4 w-4" />
						{t('myaaw.nav.server')}
					</Button>
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
