<script lang="ts">
	import { goto } from '$app/navigation';
	import { agentSelection } from '$lib/stores';
	import { server as serverApi } from '$lib/apis/server';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		Activity,
		Bot,
		CircleCheck,
		CircleSlash,
		RefreshCw,
		Server,
		MessageSquare,
		Play
	} from 'lucide-svelte';
	import type { ObservabilitySummary, RunSummary, ServerStatus } from '$lib/types';

	const { t } = useTranslation();

	let status = $state<ServerStatus | null>(null);
	let summary = $state<ObservabilitySummary | null>(null);
	let runs = $state<RunSummary[]>([]);
	let loading = $state(false);
	let error = $state('');
	let loadedFor = '';

	let totalAgents = $derived(agentSelection.agents.length);
	let runningAgents = $derived(agentSelection.agents.filter((agent) => agent.running).length);

	async function loadFor(agent: string) {
		loading = true;
		error = '';
		const [statusResult, summaryResult, runsResult] = await Promise.allSettled([
			serverApi.status(agent),
			observabilityApi.summary(agent, 7),
			observabilityApi.runs(agent, { limit: 5 })
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
		const agent = agentSelection.selected;
		const key = `${agent}:${agentSelection.agents.length}`;
		if (!agent || key === loadedFor) return;
		loadedFor = key;
		void loadFor(agent);
	});

	async function refresh() {
		await agentSelection.load(true);
		const agent = agentSelection.selected;
		if (!agent) return;
		loadedFor = `${agent}:${agentSelection.agents.length}`;
		await loadFor(agent);
	}

	function formatCost(value?: number) {
		return `$${(value ?? 0).toFixed(4)}`;
	}

	function formatTime(timestamp?: number) {
		if (!timestamp) return '—';
		return new Date(timestamp).toLocaleString();
	}
</script>

<div class="space-y-6">
	<PageHeader title={t('myaaw.home.title')} description={t('myaaw.home.subtitle')}>
		<Button variant="outline" size="sm" onclick={refresh} loading={loading}>
			<RefreshCw class="mr-2 h-4 w-4" />
			{t('common.refresh')}
		</Button>
	</PageHeader>

	{#if error}
		<div class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
			{error}
		</div>
	{/if}

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<Card>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.agents')}</CardTitle>
				<Bot class="h-4 w-4 text-muted-foreground" />
			</CardHeader>
			<CardContent>
				{#if loading}
					<Skeleton class="h-8 w-16" />
				{:else}
					<div class="text-2xl font-bold">{totalAgents}</div>
				{/if}
			</CardContent>
		</Card>

		<Card>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.running')}</CardTitle>
				<Activity class="h-4 w-4 text-muted-foreground" />
			</CardHeader>
			<CardContent>
				{#if loading}
					<Skeleton class="h-8 w-16" />
				{:else}
					<div class="text-2xl font-bold">{runningAgents}</div>
				{/if}
			</CardContent>
		</Card>

		<Card>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.runs')}</CardTitle>
				<MessageSquare class="h-4 w-4 text-muted-foreground" />
			</CardHeader>
			<CardContent>
				{#if loading}
					<Skeleton class="h-8 w-16" />
				{:else}
					<div class="text-2xl font-bold">{summary?.runs ?? 0}</div>
					{#if summary && !summary.cost_complete}
						<p class="mt-1 text-[10px] text-muted-foreground">
							{t('myaaw.home.cost_incomplete')}
						</p>
					{/if}
				{/if}
			</CardContent>
		</Card>

		<Card>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.cost')}</CardTitle>
				<CircleCheck class="h-4 w-4 text-muted-foreground" />
			</CardHeader>
			<CardContent>
				{#if loading}
					<Skeleton class="h-8 w-16" />
				{:else}
					<div class="text-2xl font-bold">{formatCost(summary?.cost_usd)}</div>
				{/if}
			</CardContent>
		</Card>
	</div>

	<div class="grid gap-4 lg:grid-cols-3">
		<Card class="lg:col-span-2">
			<CardHeader class="flex flex-row items-center justify-between">
				<div>
					<CardTitle>{t('myaaw.home.agents')}</CardTitle>
					<CardDescription>{t('myaaw.home.agents_desc')}</CardDescription>
				</div>
				<Button variant="outline" size="sm" onclick={() => goto('/app/server')}>
					<Server class="mr-2 h-4 w-4" />
					{t('myaaw.nav.server')}
				</Button>
			</CardHeader>
			<CardContent class="space-y-2">
				{#if agentSelection.agents.length === 0}
					<EmptyState
						title={t('myaaw.home.no_agents')}
						description={t('myaaw.home.no_agents_desc')}
						icon={Bot}
					/>
				{:else}
					{#each agentSelection.agents as agent (agent.name)}
						<div
							class="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="flex items-center gap-2">
								<span class="font-medium">{agent.name}</span>
								{#if agent.default}
									<Badge variant="secondary">{t('myaaw.agent.default')}</Badge>
								{/if}
								{#if agent.running}
									<Badge variant="outline" class="text-emerald-600 dark:text-emerald-400">
										<CircleCheck class="mr-1 h-3 w-3" />
										{t('myaaw.agent.running')}
									</Badge>
								{:else}
									<Badge variant="outline" class="text-muted-foreground">
										<CircleSlash class="mr-1 h-3 w-3" />
										{t('myaaw.agent.stopped')}
									</Badge>
								{/if}
							</div>
							<div class="flex items-center gap-2">
								{#if agent.port}
									<span class="font-mono text-xs text-muted-foreground">:{agent.port}</span>
								{/if}
								<Button variant="outline" size="sm" onclick={() => goto('/app/chat')}>
									<MessageSquare class="mr-2 h-4 w-4" />
									{t('myaaw.home.open_chat')}
								</Button>
							</div>
						</div>
					{/each}
				{/if}
			</CardContent>
		</Card>

		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.home.recent')}</CardTitle>
				<CardDescription>
					{status?.running
						? `${t('myaaw.server.running')} · :${status?.port ?? '—'}`
						: t('myaaw.server.stopped')}
				</CardDescription>
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
								<span class="truncate text-sm font-medium">{run.model ?? run.agent ?? run.id}</span>
								<Badge variant={run.status === 'completed' ? 'secondary' : 'destructive'}>
									{run.status}
								</Badge>
							</div>
							<div class="mt-1 flex items-center justify-between text-[11px] text-muted-foreground">
								<span>{formatTime(run.started_at)}</span>
								<span>{formatCost(run.model_cost_usd)}</span>
							</div>
						</div>
					{/each}
				{/if}
				<Separator />
				<Button
					variant="ghost"
					class="w-full justify-start"
					onclick={() => goto('/app/chat')}
				>
					<Play class="mr-2 h-4 w-4" />
					{t('myaaw.home.open_chat')}
				</Button>
			</CardContent>
		</Card>
	</div>
</div>
