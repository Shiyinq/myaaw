<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		ArrowRight,
		Brain,
		Clock,
		FileText,
		Layers,
		Loader2,
		MessageSquarePlus,
		MessagesSquare,
		Play,
		Puzzle,
		RefreshCw,
		Sparkles
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
	import { agentSelection, toastStore } from '$lib/stores';
	import { previewSnippet } from '$lib/stores/botPreviews.svelte';
	import { contextStore } from '$lib/stores/context.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { cronStore } from '$lib/stores/cron.svelte';
	import { filesStore } from '$lib/stores/files.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatChatTimestamp } from '$lib/utils/time';
	import type { ConversationSummary, CronJob, FileEntry } from '$lib/types';

	const { t, locale } = useTranslation();

	let starting = $state(false);
	let refreshing = $state(false);
	let soulExcerpt = $state('');
	let memoryExcerpt = $state('');
	let routines = $state<CronJob[]>([]);
	let recentFiles = $state<FileEntry[]>([]);
	let latestChat = $state<ConversationSummary | null>(null);

	const agent = $derived(agentSelection.selected);
	const currentAgent = $derived(agentSelection.current);
	const isRunning = $derived(Boolean(currentAgent?.running));

	let loadedFor = '';
	$effect(() => {
		const target = agent;
		if (!target || target === loadedFor) return;
		loadedFor = target;
		untrack(() => void loadSanctuary(target));
	});

	async function loadSanctuary(target: string) {
		try {
			const [soulRes, memoryRes, , filesRes, chatsRes] = await Promise.all([
				contextStore.fetchFile(target, 'SOUL.md').catch(() => null),
				contextStore.fetchFile(target, 'MEMORY.md').catch(() => null),
				cronStore.ensure(target).catch(() => null),
				filesStore.listDir(target, '/home/workspace').catch(() => null),
				conversationsStore.fetchList(target, { limit: 1 }).catch(() => null)
			]);
			if (agent !== target) return;

			soulExcerpt = previewSnippet(soulRes?.content ?? '');
			memoryExcerpt = previewSnippet(memoryRes?.content ?? '');
			routines = cronStore.list.peek({ agent: target })?.jobs ?? [];
			recentFiles = (filesRes?.entries ?? []).filter((e) => !e.name.startsWith('.')).slice(0, 4);
			latestChat = chatsRes?.conversations?.[0] ?? null;
		} catch {
			// Offline daemon; cards keep their empty states.
		}
	}

	async function refresh() {
		if (!agent || refreshing) return;
		refreshing = true;
		try {
			await Promise.all([agentSelection.load(true), loadSanctuary(agent)]);
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
				if (agent) void loadSanctuary(agent);
			}, 1500);
		} catch (err) {
			agentSelection.clearOptimisticRunning(agent);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			starting = false;
		}
	}
</script>

<TopBarActions id="focus-home-actions">
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
	<!-- Focus companion spotlight -->
	<Card class="overflow-hidden">
		<CardContent class="flex flex-col items-center gap-3 px-6 py-7 text-center">
			<div class="relative">
				<Bloub mood="attentif" size={84} tight color={agentSelection.color} />
				<span
					aria-hidden="true"
					class={[
						'absolute right-1 bottom-1 h-3.5 w-3.5 rounded-full ring-2 ring-card',
						isRunning ? 'bg-emerald-500' : 'bg-muted-foreground/40'
					]}
				></span>
			</div>
			<div class="space-y-1">
				<h2 class="text-2xl font-bold tracking-tight">{agent || t('myaaw.agent.none')}</h2>
				<p class="text-xs text-muted-foreground">{t('myaaw.home.focus_subtitle')}</p>
			</div>

			<div class="mt-2 flex flex-wrap items-center justify-center gap-2">
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
				<Button size="sm" onclick={() => void goto('/app/chat')} disabled={!agent}>
					<MessagesSquare class="mr-1.5 h-3.5 w-3.5" />
					{t('myaaw.home.open_chat')}
				</Button>
				<Button
					size="sm"
					variant="outline"
					onclick={() => void goto('/app/chat?new=1')}
					disabled={!agent}
				>
					<MessageSquarePlus class="mr-1.5 h-3.5 w-3.5" />
					{t('myaaw.chat.new')}
				</Button>
			</div>

			{#if latestChat}
				<button
					type="button"
					onclick={() => latestChat && void goto(`/app/chat/${encodeURIComponent(latestChat.id)}`)}
					class="mt-2 inline-flex max-w-md items-center gap-2 rounded-full border bg-muted/40 px-3.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
				>
					<MessagesSquare class="h-3.5 w-3.5 shrink-0" />
					<span class="truncate font-medium text-foreground">
						{latestChat.title || latestChat.id.slice(0, 12)}
					</span>
					<span class="shrink-0">
						· {formatChatTimestamp(latestChat.updated_at, locale.value, t('myaaw.chat.yesterday'))}
					</span>
				</button>
			{/if}
		</CardContent>
	</Card>

	<!-- Soul & Memory cards -->
	<div class="grid gap-4 sm:grid-cols-2">
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="flex items-center gap-2 text-sm">
					<Sparkles class="h-4 w-4 text-muted-foreground" />
					{t('myaaw.focus.context_soul')}
				</CardTitle>
				<CardDescription>SOUL.md</CardDescription>
			</CardHeader>
			<CardContent>
				<p class="line-clamp-3 text-xs text-muted-foreground">
					{soulExcerpt || t('myaaw.focus.context_empty')}
				</p>
			</CardContent>
		</Card>

		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="flex items-center gap-2 text-sm">
					<Brain class="h-4 w-4 text-muted-foreground" />
					{t('myaaw.focus.context_memory')}
				</CardTitle>
				<CardDescription>MEMORY.md</CardDescription>
			</CardHeader>
			<CardContent>
				<p class="line-clamp-3 text-xs text-muted-foreground">
					{memoryExcerpt || t('myaaw.focus.context_empty')}
				</p>
			</CardContent>
		</Card>
	</div>

	<!-- Routines & Recent Artifacts -->
	<div class="grid gap-4 sm:grid-cols-2">
		<Card>
			<CardHeader class="flex flex-row items-center justify-between pb-2">
				<div>
					<CardTitle class="flex items-center gap-2 text-sm">
						<Clock class="h-4 w-4 text-muted-foreground" />
						{t('myaaw.bots.routines')}
					</CardTitle>
					<CardDescription>{t('myaaw.schedule.subtitle')}</CardDescription>
				</div>
				<Button variant="ghost" size="sm" onclick={() => void goto('/app/schedule')}>
					{t('myaaw.bots.manage')}
					<ArrowRight class="ml-1 h-3.5 w-3.5" />
				</Button>
			</CardHeader>
			<CardContent class="space-y-1.5">
				{#if routines.length === 0}
					<p class="py-3 text-xs text-muted-foreground">{t('myaaw.bots.routines_empty')}</p>
				{:else}
					{#each routines.slice(0, 4) as job (job.id)}
						<a
							href="/app/schedule"
							class="flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-xs transition-colors hover:bg-muted/60"
						>
							<span class="truncate font-medium">{job.name}</span>
							<span class="shrink-0 font-mono text-[11px] text-muted-foreground">
								{job.enabled ? job.schedule.expr : t('myaaw.bots.routine_paused')}
							</span>
						</a>
					{/each}
				{/if}
			</CardContent>
		</Card>

		<Card>
			<CardHeader class="flex flex-row items-center justify-between pb-2">
				<div>
					<CardTitle class="flex items-center gap-2 text-sm">
						<Layers class="h-4 w-4 text-muted-foreground" />
						{t('myaaw.nav.artifacts')}
					</CardTitle>
					<CardDescription>{t('myaaw.artifacts.subtitle')}</CardDescription>
				</div>
				<Button variant="ghost" size="sm" onclick={() => void goto('/app/artifacts')}>
					<ArrowRight class="h-3.5 w-3.5" />
				</Button>
			</CardHeader>
			<CardContent class="space-y-1.5">
				{#if recentFiles.length === 0}
					<p class="py-3 text-xs text-muted-foreground">{t('myaaw.artifacts.empty')}</p>
				{:else}
					{#each recentFiles as file (file.path)}
						<a
							href="/app/artifacts"
							class="flex items-center gap-2 rounded-lg border px-3 py-2 text-xs transition-colors hover:bg-muted/60"
						>
							<FileText class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
							<span class="min-w-0 flex-1 truncate font-medium">{file.name}</span>
						</a>
					{/each}
				{/if}
				<div class="pt-1">
					<a
						href="/app/plugins"
						class="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
					>
						<Puzzle class="h-3.5 w-3.5" />
						{t('myaaw.plugins.title')}
					</a>
				</div>
			</CardContent>
		</Card>
	</div>
</div>
