<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		agentSelection,
		contactsStore,
		conversationsStore,
		observabilityStore,
		skillsStore,
		toolsStore
	} from '$lib/stores';
	import { serverStore } from '$lib/stores/server.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import ActivityHeatmap from '$lib/components/dashboard/ActivityHeatmap.svelte';
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

	const { t } = useTranslation();

	// Newest month first: the grid fills left to right, top to bottom.
	let activityMonths = $derived.by(() => {
		const today = new Date();
		return Array.from(
			{ length: 3 },
			(_, index) => new Date(today.getFullYear(), today.getMonth() - index, 1)
		);
	});

	let agent = $derived(agentSelection.selected);

	useQuery(serverStore.query, () => (agent ? { agent } : null));
	useQuery(observabilityStore.summary, () => (agent ? { agent, days: 7 } : null));
	useQuery(conversationsStore.list, () => (agent ? { agent, filters: { limit: 5 } } : null));
	useQuery(skillsStore.list, () => (agent ? { agent } : null));
	useQuery(toolsStore.query, () => (agent ? { agent } : null));
	useQuery(contactsStore.query, () => (agent ? { agent } : null));
	useQuery(conversationsStore.activity, () =>
		agent
			? {
					agent,
					from: activityMonths[activityMonths.length - 1].getTime(),
					to: Date.now()
				}
			: null
	);

	let status = $derived(serverStore.data ?? null);
	let summary = $derived(observabilityStore.summary.data ?? null);
	let chats = $derived(conversationsStore.list.data?.conversations ?? []);
	let chatTotal = $derived(
		conversationsStore.list.data?.total ?? (conversationsStore.list.data ? chats.length : null)
	);
	let skillTotal = $derived(
		skillsStore.list.data ? (skillsStore.list.data.skills ?? []).length : null
	);
	let skillEnabled = $derived(
		skillsStore.list.data
			? (skillsStore.list.data.skills ?? []).filter((item) => item.enabled).length
			: null
	);
	let toolBuiltin = $derived(
		toolsStore.query.data
			? (toolsStore.query.data.builtin ?? []).filter((item) => item.enabled).length
			: null
	);
	let toolCustom = $derived(
		toolsStore.query.data
			? (toolsStore.query.data.custom ?? []).filter((item) => item.enabled).length
			: null
	);
	let toolMcp = $derived(
		toolsStore.query.data
			? (toolsStore.query.data.mcp_servers ?? []).filter((item) => item.enabled).length
			: null
	);
	let toolTotal = $derived(
		toolBuiltin !== null && toolCustom !== null && toolMcp !== null
			? toolBuiltin + toolCustom + toolMcp
			: null
	);
	let contactTotal = $derived(
		contactsStore.query.data ? (contactsStore.query.data.contacts ?? []).length : null
	);
	let activity = $derived(conversationsStore.activity.data ?? null);
	let loading = $derived(
		serverStore.loading ||
			observabilityStore.summary.loading ||
			conversationsStore.list.loading ||
			skillsStore.list.loading ||
			toolsStore.query.loading ||
			contactsStore.query.loading ||
			conversationsStore.activity.loading
	);
	let error = $derived(
		serverStore.error ||
			observabilityStore.summary.error ||
			conversationsStore.list.error ||
			skillsStore.list.error ||
			toolsStore.query.error ||
			contactsStore.query.error ||
			conversationsStore.activity.error
	);

	async function refresh() {
		await agentSelection.load(true);
		await Promise.all([
			serverStore.refresh(),
			observabilityStore.summary.refresh(),
			conversationsStore.list.refresh(),
			skillsStore.list.refresh(),
			toolsStore.query.refresh(),
			contactsStore.query.refresh(),
			conversationsStore.activity.refresh()
		]);
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

<div
	class="h-full overflow-y-auto p-4 max-lg:space-y-4 lg:flex lg:min-h-0 lg:flex-col lg:gap-5 lg:overflow-hidden lg:p-6"
>
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

		<div class="grid shrink-0 gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
						tools: {formatCost(summary?.tool_cost_usd)}
						{#if summary && !summary.cost_complete}
							· {t('myaaw.observability.incomplete')}
						{/if}
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

		<div class="grid shrink-0 gap-3 sm:grid-cols-2 lg:grid-cols-4">
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

		<div class="grid min-h-0 gap-4 lg:flex-1 lg:grid-cols-2 lg:gap-5">
			<Card class="flex min-h-0 flex-col">
				<CardHeader>
					<CardTitle>{t('myaaw.home.recent_chats')}</CardTitle>
					<CardDescription>{t('myaaw.home.recent_chats_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
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
								class="flex flex-1 flex-col justify-center rounded-md border p-2 transition-colors hover:bg-muted/50"
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

			<Card class="flex min-h-0 flex-col">
				<CardHeader>
					<CardTitle>{t('myaaw.home.activity')}</CardTitle>
					<CardDescription>{t('myaaw.home.activity_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="flex min-h-0 flex-1 flex-col">
					<ActivityHeatmap days={activity?.days ?? []} months={activityMonths} {loading} />
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
