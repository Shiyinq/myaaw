<script lang="ts">
	import ClassicHomeSkeleton from './ClassicHomeSkeleton.svelte';
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		ArrowRight,
		Cpu,
		Loader2,
		MessageSquarePlus,
		MessagesSquare,
		Pin,
		Play,
		RefreshCw,
		SlidersHorizontal,
		Sparkles,
		Wrench
	} from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { agentSelection, conversationHistory, toastStore } from '$lib/stores';
	import { configStore } from '$lib/stores/config.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { skillsStore } from '$lib/stores/skills.svelte';
	import { toolsStore } from '$lib/stores/tools.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { ConversationSummary } from '$lib/types';

	const { t, locale } = useTranslation();

	let starting = $state(false);
	let refreshing = $state(false);
	let activeModel = $state('');
	let activeProvider = $state('');
	let skillsActive = $state(0);
	let skillsTotal = $state(0);
	let toolsActive = $state(0);
	let mcpCount = $state(0);
	let fallbackChats = $state<ConversationSummary[]>([]);
	let summaryLoadedFor = $state('');

	const agent = $derived(agentSelection.selected);
	const currentAgent = $derived(agentSelection.current);
	const isRunning = $derived(Boolean(currentAgent?.running));
	const initialLoading = $derived(
		!agentSelection.loaded || (Boolean(agent) && summaryLoadedFor !== agent)
	);

	const recentChats = $derived.by(() => {
		const pinned = conversationHistory.pinned.items;
		const recents = conversationHistory.recents.items;
		const combined = [...pinned, ...recents];
		if (combined.length > 0) return combined.slice(0, 6);
		return fallbackChats.slice(0, 6);
	});

	let loadedFor = '';
	$effect(() => {
		const target = agent;
		if (!target || target === loadedFor) return;
		loadedFor = target;
		untrack(() => void loadSummary(target));
	});

	async function loadSummary(target: string) {
		try {
			const [cfg, skills, tools, chats] = await Promise.all([
				configStore.fetchConfig(target).catch(() => null),
				skillsStore.fetchList(target).catch(() => null),
				toolsStore.fetchView(target).catch(() => null),
				conversationsStore.fetchList(target, { limit: 6 }).catch(() => null)
			]);
			if (agent !== target) return;

			if (cfg) {
				const defName = cfg.default_provider ?? '';
				const found = (cfg.providers ?? []).find((p) => p.name === defName) ?? cfg.providers?.[0];
				activeProvider = found?.name ?? defName;
				activeModel = found?.default_model ?? '';
			} else {
				activeProvider = '';
				activeModel = '';
			}

			if (skills?.skills) {
				skillsTotal = skills.skills.length;
				skillsActive = skills.skills.filter((s) => s.enabled).length;
			} else {
				skillsTotal = 0;
				skillsActive = 0;
			}

			if (tools) {
				const builtinOn = (tools.builtin ?? []).filter((entry) => entry.enabled).length;
				const customOn = (tools.custom ?? []).filter((entry) => entry.enabled !== false).length;
				toolsActive = builtinOn + customOn;
				mcpCount = (tools.mcp_servers ?? []).length;
			} else {
				toolsActive = 0;
				mcpCount = 0;
			}

			fallbackChats = chats?.conversations ?? [];
		} catch {
			// Offline daemon; cards keep their safe defaults.
		} finally {
			if (agent === target) summaryLoadedFor = target;
		}
	}

	async function refresh() {
		if (!agent || refreshing) return;
		refreshing = true;
		try {
			await Promise.all([
				agentSelection.load(true),
				loadSummary(agent),
				Promise.resolve(conversationHistory.refresh())
			]);
		} finally {
			refreshing = false;
		}
	}

	async function startServer() {
		if (!agent || starting) return;
		starting = true;
		agentSelection.setRunning(agent, true);
		try {
			await serverStore.start(agent);
			agentSelection.setRunning(agent, true);
			toastStore.success(t('myaaw.server.start_done'));
			setTimeout(() => {
				if (agent) void loadSummary(agent);
			}, 1500);
		} catch (err) {
			agentSelection.clearOptimisticRunning(agent);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			starting = false;
		}
	}

	function startNewChat() {
		void goto('/app/chat?new=1');
	}

	function openConversation(id: string) {
		void goto(`/app/chat/${encodeURIComponent(id)}`);
	}
</script>

<TopBarActions id="classic-home-actions">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={refresh}
			loading={refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto flex w-full max-w-4xl flex-col gap-5">
	{#if initialLoading}
		<ClassicHomeSkeleton />
	{:else}
		<Card class="overflow-hidden">
			<CardContent class="flex flex-col gap-5 p-6 sm:flex-row sm:items-center sm:justify-between">
				<div class="flex items-center gap-4">
					<div
						class="relative grid h-16 w-16 shrink-0 place-items-center rounded-2xl bg-muted/60 ring-1 ring-border/50"
					>
						<Bloub mood="attentif" size={54} tight color={agentSelection.color} />
						<span
							aria-hidden="true"
							class={[
								'absolute -right-0.5 -bottom-0.5 h-3 w-3 rounded-full ring-2 ring-card',
								isRunning ? 'bg-emerald-500' : 'bg-muted-foreground/40'
							]}
						></span>
					</div>
					<div class="min-w-0 space-y-1">
						<div class="flex flex-wrap items-center gap-2">
							<h2 class="truncate text-xl font-bold tracking-tight">
								{agent || t('myaaw.agent.none')}
							</h2>
							<span
								class={[
									'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium',
									isRunning
										? 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300'
										: 'bg-muted text-muted-foreground'
								]}
							>
								{isRunning ? t('myaaw.agent.running') : t('myaaw.agent.stopped')}
							</span>
						</div>
						<p class="text-xs text-muted-foreground">{t('myaaw.home.classic_subtitle')}</p>
						{#if activeModel || activeProvider}
							<p class="inline-flex items-center gap-1.5 pt-0.5 text-xs text-muted-foreground">
								<Cpu class="h-3.5 w-3.5 shrink-0" />
								<span class="font-mono text-[11px]">
									{activeProvider}{activeModel ? ` · ${activeModel}` : ''}
								</span>
							</p>
						{/if}
					</div>
				</div>

				<div class="flex flex-wrap items-center gap-2">
					{#if !isRunning && agent}
						<Button size="sm" variant="outline" disabled={starting} onclick={startServer}>
							{#if starting}
								<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
							{:else}
								<Play class="mr-1.5 h-3.5 w-3.5" />
							{/if}
							{t('myaaw.server.start')}
						</Button>
					{/if}
					<Button size="sm" onclick={startNewChat} disabled={!agent}>
						<MessageSquarePlus class="mr-1.5 h-3.5 w-3.5" />
						{t('myaaw.chat.new')}
					</Button>
				</div>
			</CardContent>
		</Card>

		<div class="grid gap-3 sm:grid-cols-3">
			<a
				href="/app/plugins"
				class="group flex items-center justify-between rounded-xl border bg-card p-4 transition-colors hover:bg-muted/50"
			>
				<div class="space-y-1">
					<span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
						<Sparkles class="h-3.5 w-3.5" />
						{t('myaaw.home.total_skills')}
					</span>
					<p class="text-lg font-bold tabular-nums">
						{t('myaaw.home.skills_hint', { enabled: skillsActive, total: skillsTotal })}
					</p>
				</div>
				<ArrowRight
					class="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
				/>
			</a>

			<a
				href="/app/plugins"
				class="group flex items-center justify-between rounded-xl border bg-card p-4 transition-colors hover:bg-muted/50"
			>
				<div class="space-y-1">
					<span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
						<Wrench class="h-3.5 w-3.5" />
						{t('myaaw.home.total_tools')}
					</span>
					<p class="text-lg font-bold tabular-nums">
						{toolsActive}
						{#if mcpCount > 0}
							<span class="text-xs font-normal text-muted-foreground">· {mcpCount} MCP</span>
						{/if}
					</p>
				</div>
				<ArrowRight
					class="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
				/>
			</a>

			<a
				href={agent ? `/app/new?agent=${encodeURIComponent(agent)}` : '/app/new'}
				class="group flex items-center justify-between rounded-xl border bg-card p-4 transition-colors hover:bg-muted/50"
			>
				<div class="space-y-1">
					<span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
						<SlidersHorizontal class="h-3.5 w-3.5" />
						{t('myaaw.onboard.reconfigure')}
					</span>
					<p class="truncate text-sm font-semibold">{agent || '—'}</p>
				</div>
				<ArrowRight
					class="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
				/>
			</a>
		</div>

		<Card>
			<CardHeader class="flex flex-row items-center justify-between pb-3">
				<div>
					<CardTitle class="text-base">{t('myaaw.home.recent_chats')}</CardTitle>
					<CardDescription>{t('myaaw.home.recent_chats_desc')}</CardDescription>
				</div>
				<Button variant="ghost" size="sm" onclick={() => void goto('/app/chat')}>
					{t('myaaw.home.open_chat')}
					<ArrowRight class="ml-1.5 h-3.5 w-3.5" />
				</Button>
			</CardHeader>
			<CardContent>
				{#if recentChats.length === 0}
					<div
						class="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed py-8 text-center"
					>
						<Bloub mood="timide" size={44} tight color={agentSelection.color} />
						<p class="text-sm text-muted-foreground">{t('myaaw.home.no_chats')}</p>
						<Button size="sm" variant="outline" class="mt-1" onclick={startNewChat}>
							<MessageSquarePlus class="mr-1.5 h-3.5 w-3.5" />
							{t('myaaw.home.start_chat')}
						</Button>
					</div>
				{:else}
					<div class="grid gap-2 sm:grid-cols-2">
						{#each recentChats as chat (chat.id)}
							<button
								type="button"
								onclick={() => openConversation(chat.id)}
								class="group flex items-start gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-muted/60"
							>
								<span
									class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground"
								>
									{#if chat.pinned}
										<Pin class="h-3.5 w-3.5" />
									{:else}
										<MessagesSquare class="h-3.5 w-3.5" />
									{/if}
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-foreground">
										{chat.title || chat.id.slice(0, 12)}
									</span>
									<span class="mt-0.5 block truncate text-xs text-muted-foreground">
										{formatChatTimestamp(chat.updated_at, locale.value, t('myaaw.chat.yesterday'))} ·
										{t('myaaw.conversations.messages', { count: chat.message_count })}
									</span>
								</span>
							</button>
						{/each}
					</div>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>
