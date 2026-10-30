<script lang="ts">
	import { goto } from '$app/navigation';
	import { agentSelection, toastStore } from '$lib/stores';
	import { agents as agentsApi } from '$lib/apis/agents';
	import { server as serverApi } from '$lib/apis/server';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { onboardSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		Activity,
		Bot,
		CircleCheck,
		CircleSlash,
		Ellipsis,
		RefreshCw,
		Server,
		MessageSquare,
		Play,
		Plus,
		Star,
		Trash2
	} from 'lucide-svelte';
	import type { ObservabilitySummary, RunSummary, ServerStatus } from '$lib/types';

	const { t } = useTranslation();

	let status = $state<ServerStatus | null>(null);
	let summary = $state<ObservabilitySummary | null>(null);
	let runs = $state<RunSummary[]>([]);
	let loading = $state(false);
	let error = $state('');
	let loadedFor = '';

	let onboardOpen = $state(false);
	let onboarding = $state(false);
	let onboardErrors = $state<FieldErrors>({});
	let newName = $state('');
	let newType = $state('openai');
	let newBaseUrl = $state('');
	let newApiKey = $state('');
	let newModel = $state('');

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

	// Opening a chat from the agent list also switches the selected agent, so the
	// chat page (and every other page) shows that agent's data.
	function openChat(name?: string) {
		if (name) agentSelection.select(name);
		void goto('/app/chat');
	}

	async function setDefaultAgent(name: string) {
		try {
			await agentsApi.setDefault(name);
			await agentSelection.load(true);
			toastStore.success(t('myaaw.home.agent_default_set', { name }));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeAgent(name: string) {
		if (!window.confirm(t('myaaw.home.agent_delete_confirm', { name }))) return;
		try {
			await agentsApi.remove(name);
			await agentSelection.load(true);
			toastStore.success(t('myaaw.home.agent_deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function openOnboard() {
		onboardErrors = {};
		onboardOpen = true;
	}

	async function createAgent() {
		const result = validate(onboardSchema, {
			name: newName,
			provider_type: newType,
			base_url: newBaseUrl,
			api_key: newApiKey,
			model: newModel
		});
		if (!result.ok) {
			onboardErrors = result.errors;
			return;
		}
		onboardErrors = {};
		onboarding = true;
		try {
			const response = await agentsApi.onboard(result.data.name, {
				provider: {
					type: result.data.provider_type,
					base_url: result.data.base_url || undefined,
					api_key: result.data.api_key || undefined,
					model: result.data.model || undefined
				}
			});
			onboardOpen = false;
			newName = '';
			newBaseUrl = '';
			newApiKey = '';
			newModel = '';
			toastStore.success(t('myaaw.home.agent_created', { name: result.data.name }));
			for (const warning of response.warnings ?? []) {
				toastStore.warning(warning);
			}
			await agentSelection.load(true);
			agentSelection.select(result.data.name);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			onboarding = false;
		}
	}

	function formatCost(value?: number) {
		return `$${(value ?? 0).toFixed(4)}`;
	}

	function formatTime(timestamp?: number) {
		if (!timestamp) return '—';
		return new Date(timestamp).toLocaleString();
	}
</script>

<TopBarActions id="dashboard-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('myaaw.home.new_agent')}
			icon={Plus}
			onclick={openOnboard}
			variant="default"
		/>
		<TopBarButton label={t('common.refresh')} icon={RefreshCw} onclick={refresh} {loading} />
	{/snippet}
</TopBarActions>

<div class="space-y-6">
	{#if error}
		<div
			class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
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
								<Button variant="outline" size="sm" onclick={() => openChat(agent.name)}>
									<MessageSquare class="mr-2 h-4 w-4" />
									{t('myaaw.home.open_chat')}
								</Button>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}
											<Button
												{...props}
												variant="ghost"
												size="icon-sm"
												aria-label={t('myaaw.home.agent_actions')}
											>
												<Ellipsis class="h-4 w-4" />
											</Button>
										{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										{#if !agent.default}
											<DropdownMenu.Item onclick={() => setDefaultAgent(agent.name)}>
												<Star class="mr-2 h-4 w-4" />
												{t('myaaw.home.set_default')}
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
										{/if}
										<DropdownMenu.Item
											variant="destructive"
											onclick={() => removeAgent(agent.name)}
										>
											<Trash2 class="mr-2 h-4 w-4" />
											{t('myaaw.home.delete_agent')}
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
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
					onclick={() => openChat(agentSelection.selected)}
				>
					<Play class="mr-2 h-4 w-4" />
					{t('myaaw.home.open_chat')}
				</Button>
			</CardContent>
		</Card>
	</div>
</div>

<Dialog.Root bind:open={onboardOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{t('myaaw.home.new_agent')}</Dialog.Title>
			<Dialog.Description>{t('myaaw.home.new_agent_desc')}</Dialog.Description>
		</Dialog.Header>
		<form
			class="space-y-3"
			onsubmit={(event) => {
				event.preventDefault();
				void createAgent();
			}}
		>
			<div class="grid gap-1">
				<Label for="agent-name">{t('myaaw.home.agent_name')}</Label>
				<Input id="agent-name" bind:value={newName} placeholder="research" />
				<FieldError code={onboardErrors.name} />
			</div>
			<div class="grid gap-1">
				<Label for="agent-provider-type">{t('myaaw.providers.type')}</Label>
				<select
					id="agent-provider-type"
					bind:value={newType}
					class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
				>
					<option value="openai">openai</option>
					<option value="gemini">gemini</option>
				</select>
			</div>
			<div class="grid gap-1">
				<Label for="agent-base-url">{t('myaaw.providers.base_url')}</Label>
				<Input id="agent-base-url" bind:value={newBaseUrl} placeholder="https://api.deepseek.com" />
				<FieldError code={onboardErrors.base_url} />
			</div>
			<div class="grid gap-1">
				<Label for="agent-api-key">{t('myaaw.providers.api_key')}</Label>
				<Input id="agent-api-key" type="password" bind:value={newApiKey} placeholder="sk-..." />
			</div>
			<div class="grid gap-1">
				<Label for="agent-model">{t('myaaw.providers.model')}</Label>
				<Input id="agent-model" bind:value={newModel} placeholder="deepseek-chat" />
			</div>
			<Dialog.Footer class="gap-2">
				<Button type="button" variant="ghost" onclick={() => (onboardOpen = false)}>
					{t('common.cancel')}
				</Button>
				<Button type="submit" loading={onboarding}>
					<Plus class="mr-2 h-4 w-4" />
					{t('myaaw.home.create_agent')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
