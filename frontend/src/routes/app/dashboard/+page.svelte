<script lang="ts">
	import { goto } from '$app/navigation';
	import { agentSelection } from '$lib/stores';
	import { server as serverApi } from '$lib/apis/server';
	import { contacts as contactsApi } from '$lib/apis/contacts';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { skills as skillsApi } from '$lib/apis/skills';
	import { tools as toolsApi } from '$lib/apis/tools';
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
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Bot, MessageSquare, MessagesSquare, RefreshCw, Server } from 'lucide-svelte';
	import type { ConversationSummary, ObservabilitySummary, ServerStatus } from '$lib/types';

	const { t } = useTranslation();

	let status = $state<ServerStatus | null>(null);
	let summary = $state<ObservabilitySummary | null>(null);
	let chats = $state<ConversationSummary[]>([]);
	let chatTotal = $state<number | null>(null);
	let skillTotal = $state<number | null>(null);
	let skillEnabled = $state<number | null>(null);
	let toolTotal = $state<number | null>(null);
	let toolBuiltin = $state<number | null>(null);
	let toolCustom = $state<number | null>(null);
	let toolMcp = $state<number | null>(null);
	let contactTotal = $state<number | null>(null);

	let loading = $state(false);
	let error = $state('');
	let loadedFor = '';

	let agent = $derived(agentSelection.selected);

	async function loadFor(currentAgent: string) {
		loading = true;
		error = '';
		const [statusResult, summaryResult, chatsResult, skillsResult, toolsResult, contactsResult] =
			await Promise.allSettled([
				serverApi.status(currentAgent),
				observabilityApi.summary(currentAgent, 7),
				conversationsApi.list(currentAgent, { limit: 5 }),
				skillsApi.list(currentAgent),
				toolsApi.list(currentAgent),
				contactsApi.list(currentAgent)
			]);

		status = statusResult.status === 'fulfilled' ? statusResult.value : null;
		summary = summaryResult.status === 'fulfilled' ? summaryResult.value : null;

		if (chatsResult.status === 'fulfilled') {
			chats = chatsResult.value.conversations ?? [];
			chatTotal = chatsResult.value.total ?? chats.length;
		} else {
			chats = [];
			chatTotal = null;
		}

		if (skillsResult.status === 'fulfilled') {
			const list = skillsResult.value.skills ?? [];
			skillTotal = list.length;
			skillEnabled = list.filter((skill) => skill.enabled).length;
		} else {
			skillTotal = null;
			skillEnabled = null;
		}

		if (toolsResult.status === 'fulfilled') {
			const view = toolsResult.value;
			const builtin = (view.builtin ?? []).filter((tool) => tool.enabled).length;
			const custom = (view.custom ?? []).filter((tool) => tool.enabled).length;
			const mcp = (view.mcp_servers ?? []).filter((tool) => tool.enabled).length;
			toolBuiltin = builtin;
			toolCustom = custom;
			toolMcp = mcp;
			toolTotal = builtin + custom + mcp;
		} else {
			toolBuiltin = null;
			toolCustom = null;
			toolMcp = null;
			toolTotal = null;
		}

		contactTotal =
			contactsResult.status === 'fulfilled' ? (contactsResult.value.contacts ?? []).length : null;

		const failed = [
			statusResult,
			summaryResult,
			chatsResult,
			skillsResult,
			toolsResult,
			contactsResult
		].find((result) => result.status === 'rejected');
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

	function newChat() {
		void goto('/app/chat?new=1');
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

	const relativeUnits: [Intl.RelativeTimeFormatUnit, number][] = [
		['year', 31536000],
		['month', 2592000],
		['week', 604800],
		['day', 86400],
		['hour', 3600],
		['minute', 60]
	];

	function relativeTime(timestamp?: string) {
		if (!timestamp) return '—';
		const then = new Date(timestamp).getTime();
		if (Number.isNaN(then)) return '—';
		const seconds = Math.round((then - Date.now()) / 1000);
		const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
		for (const [unit, size] of relativeUnits) {
			if (Math.abs(seconds) >= size) return formatter.format(Math.round(seconds / size), unit);
		}
		return formatter.format(seconds, 'second');
	}

	function countLabel(value: number | null) {
		return value === null ? '—' : value.toLocaleString();
	}
</script>

<TopBarActions id="agent-dashboard-refresh">
	{#snippet children()}
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

		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.home.total_chats')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{countLabel(chatTotal)}</div>
					<p class="truncate text-[10px] text-muted-foreground">
						{#if chats.length > 0}
							{t('myaaw.home.last_chat', { when: relativeTime(chats[0].updated_at) })}
						{:else}
							&nbsp;
						{/if}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.home.total_skills')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{countLabel(skillTotal)}</div>
					<p class="truncate text-[10px] text-muted-foreground">
						{#if skillTotal !== null}
							{t('myaaw.home.skills_hint', {
								enabled: skillEnabled ?? 0,
								total: skillTotal
							})}
						{:else}
							&nbsp;
						{/if}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.home.total_tools')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{countLabel(toolTotal)}</div>
					<p class="truncate text-[10px] text-muted-foreground">
						{#if toolTotal !== null}
							{t('myaaw.home.tools_hint', {
								builtin: toolBuiltin ?? 0,
								custom: toolCustom ?? 0,
								mcp: toolMcp ?? 0
							})}
						{:else}
							&nbsp;
						{/if}
					</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium">{t('myaaw.home.total_contacts')}</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{countLabel(contactTotal)}</div>
					<p class="truncate text-[10px] text-muted-foreground">
						{#if contactTotal !== null}
							{t('myaaw.home.contacts_hint')}
						{:else}
							&nbsp;
						{/if}
					</p>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.home.recent_chats')}</CardTitle>
				<CardDescription>{t('myaaw.home.recent_chats_desc')}</CardDescription>
			</CardHeader>
			<CardContent class="space-y-2">
				{#if loading}
					{#each Array.from({ length: 3 }) as _, index (index)}
						<Skeleton class="h-12 w-full" />
					{/each}
				{:else if chats.length === 0}
					<div class="flex flex-col items-center gap-2 py-6 text-center">
						<MessagesSquare class="h-6 w-6 text-muted-foreground" />
						<p class="text-sm text-muted-foreground">{t('myaaw.home.no_chats')}</p>
						<Button variant="outline" size="sm" onclick={newChat}>
							<MessageSquare class="mr-2 h-4 w-4" />
							{t('myaaw.home.start_chat')}
						</Button>
					</div>
				{:else}
					{#each chats as chat (chat.id)}
						<a
							href={`/app/chat/${chat.id}`}
							class="block rounded-md border p-2 transition-colors hover:bg-muted/50"
						>
							<div class="flex items-center justify-between gap-2">
								<span class="truncate text-sm font-medium">
									{chat.title || chat.id.slice(0, 12)}
								</span>
								<Badge variant="secondary" class="shrink-0">
									{relativeTime(chat.updated_at)}
								</Badge>
							</div>
							<p class="mt-0.5 text-[11px] text-muted-foreground">
								{t('myaaw.conversations.messages', { count: chat.message_count })}
							</p>
						</a>
					{/each}
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>
