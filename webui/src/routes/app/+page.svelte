<script lang="ts">
	import { goto } from '$app/navigation';
	import { agents as agentsApi } from '$lib/apis/agents';
	import { server as serverApi } from '$lib/apis/server';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import BackupsPanel from '$lib/components/backup/BackupsPanel.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardAction,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		Bot,
		Check,
		CircleCheck,
		CircleSlash,
		Ellipsis,
		Loader2,
		Play,
		Plus,
		RefreshCw,
		RotateCw,
		Settings2,
		Square,
		Star,
		Trash2
	} from 'lucide-svelte';

	const { t } = useTranslation();

	let loading = $state(false);
	let toggling = $state<string | null>(null);
	// Which fleet-wide action is running, if any. One at a time, so the daemons
	// never race for the same port.
	let bulkBusy = $state<'' | 'start' | 'restart' | 'stop'>('');

	let agents = $derived(agentSelection.agents);
	let runningCount = $derived(agents.filter((agent) => agent.running).length);
	let defaultAgent = $derived(agents.find((agent) => agent.default)?.name ?? '—');

	// Newest first by default; the dropdown flips to oldest. Timestamps are UTC
	// RFC3339, so lexicographic order matches chronological order. The default
	// agent stays pinned to the top.
	let sortNewest = $state(true);
	let sortedAgents = $derived(
		[...agents].sort((a, b) => {
			if (a.default !== b.default) return a.default ? -1 : 1;
			const at = a.created_at ?? '';
			const bt = b.created_at ?? '';
			if (!at && !bt) return 0;
			if (!at) return 1;
			if (!bt) return -1;
			const cmp = at < bt ? -1 : at > bt ? 1 : 0;
			return sortNewest ? -cmp : cmp;
		})
	);

	async function refresh() {
		loading = true;
		try {
			await agentSelection.load(true);
		} finally {
			loading = false;
		}
	}

	// Opening an agent also selects it, so the agent dashboard shows its data.
	function openChat(name: string) {
		agentSelection.select(name);
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
		if (
			!(await askConfirm({
				message: t('myaaw.home.agent_delete_confirm', { name }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await agentsApi.remove(name);
			await agentSelection.load(true);
			toastStore.success(t('myaaw.home.agent_deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function toggleServer(name: string, running: boolean) {
		if (toggling || bulkBusy) return;
		toggling = name;
		// Flip the badge right away; the daemon takes a moment to bind its port.
		agentSelection.setRunning(name, !running);
		try {
			if (running) await serverApi.stop(name);
			else await serverApi.start(name);
			toastStore.success(t(running ? 'myaaw.server.stop_done' : 'myaaw.server.start_done'));
			await waitForServerState(name, !running);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await agentSelection.load(true);
		} finally {
			toggling = null;
		}
	}

	// The daemon reports as running only once its port accepts connections, which
	// can take a few seconds after a start request. Poll until the list agrees.
	async function waitForServerState(name: string, running: boolean) {
		for (let attempt = 0; attempt < 20; attempt += 1) {
			await new Promise((resolve) => setTimeout(resolve, 300));
			try {
				const { agents: list } = await agentsApi.list();
				agentSelection.setAgents(list);
				if (list.some((agent) => agent.name === name && agent.running === running)) return;
			} catch {
				// Transient failures are expected while the daemon boots; keep polling.
			}
		}
	}

	async function restartServer(name: string) {
		if (toggling || bulkBusy) return;
		toggling = name;
		try {
			await serverApi.restart(name);
			toastStore.success(t('myaaw.server.restart_done'));
			await waitForServerState(name, true);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await agentSelection.load(true);
		} finally {
			toggling = null;
		}
	}

	/**
	 * Starts what is stopped, or restarts/stops what is running, for the whole fleet.
	 * Every badge flips before the first request goes out, so the labels on the cards
	 * answer the click immediately instead of after the daemons finish booting.
	 */
	async function bulkAction(kind: 'start' | 'restart' | 'stop') {
		if (bulkBusy || toggling) return;
		const targets = agents.filter((agent) => (kind === 'start' ? !agent.running : agent.running));
		if (targets.length === 0) return;
		if (
			kind === 'stop' &&
			!(await askConfirm({
				message: t('myaaw.home.stop_all_confirm', { count: targets.length }),
				variant: 'destructive'
			}))
		) {
			return;
		}

		bulkBusy = kind;
		const running = kind !== 'stop';
		for (const agent of targets) agentSelection.setRunning(agent.name, running);

		const failed: string[] = [];
		for (const agent of targets) {
			try {
				if (kind === 'start') await serverApi.start(agent.name);
				else if (kind === 'restart') await serverApi.restart(agent.name);
				else await serverApi.stop(agent.name);
			} catch {
				failed.push(agent.name);
			}
		}

		await waitForStates(
			targets.filter((agent) => !failed.includes(agent.name)).map((agent) => agent.name),
			running
		);
		if (failed.length > 0) await agentSelection.load(true);

		bulkBusy = '';
		if (failed.length > 0) {
			toastStore.error(t('myaaw.server.bulk_failed', { names: failed.join(', ') }));
		}
		const done = targets.length - failed.length;
		if (done > 0) {
			const key =
				kind === 'start' ? 'bulk_started' : kind === 'restart' ? 'bulk_restarted' : 'bulk_stopped';
			toastStore.success(t(`myaaw.server.${key}`, { count: done }));
		}
	}

	/** Polls until every named agent reports the expected state, then gives up. */
	async function waitForStates(names: string[], running: boolean) {
		const pending = new Set(names);
		for (let attempt = 0; attempt < 20 && pending.size > 0; attempt += 1) {
			await new Promise((resolve) => setTimeout(resolve, 300));
			try {
				const { agents: list } = await agentsApi.list();
				agentSelection.setAgents(list);
				for (const agent of list) {
					if (pending.has(agent.name) && agent.running === running) pending.delete(agent.name);
				}
			} catch {
				// Transient failures are expected while the daemons boot; keep polling.
			}
		}
	}

	function openOnboard() {
		void goto('/app/new');
	}

	// First run: nothing to show until an agent exists, so go straight to the
	// creation form instead of leaving an empty fleet.
	$effect(() => {
		if (!agentSelection.loaded || agentSelection.loading || agentSelection.error) return;
		if (agentSelection.agents.length === 0) {
			void goto('/app/new');
		}
	});

	function reconfigureAgent(name: string) {
		void goto(`/app/new?agent=${encodeURIComponent(name)}`);
	}
</script>

<TopBarActions id="fleet-actions">
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

<div class="flex flex-col gap-4">
	<div class="grid gap-3 sm:grid-cols-3">
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.agents')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="text-2xl font-bold">{agents.length}</div>
			</CardContent>
		</Card>
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.running')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="text-2xl font-bold">{runningCount}</div>
			</CardContent>
		</Card>
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.fleet.default_agent')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="truncate font-mono text-lg font-semibold">{defaultAgent}</div>
			</CardContent>
		</Card>
	</div>

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Bot class="h-4 w-4 shrink-0" />
				{t('myaaw.home.agents')}
			</CardTitle>
			<CardDescription>{t('myaaw.home.agents_desc')}</CardDescription>
			<CardAction>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button
								{...props}
								variant="ghost"
								size="icon-sm"
								aria-label={t('myaaw.home.fleet_actions')}
								title={t('myaaw.home.fleet_actions')}
							>
								<Ellipsis class="h-4 w-4" />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end" class="w-60">
						<DropdownMenu.Item onclick={() => (sortNewest = true)}>
							<Check class={sortNewest ? 'mr-2 h-4 w-4' : 'mr-2 h-4 w-4 opacity-0'} />
							{t('myaaw.home.sort_newest')}
						</DropdownMenu.Item>
						<DropdownMenu.Item onclick={() => (sortNewest = false)}>
							<Check class={!sortNewest ? 'mr-2 h-4 w-4' : 'mr-2 h-4 w-4 opacity-0'} />
							{t('myaaw.home.sort_oldest')}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item
							disabled={bulkBusy !== '' || runningCount === agents.length}
							onclick={() => void bulkAction('start')}
						>
							{#if bulkBusy === 'start'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<Play class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.start_all')}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							disabled={bulkBusy !== '' || runningCount === 0}
							onclick={() => void bulkAction('restart')}
						>
							{#if bulkBusy === 'restart'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<RotateCw class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.restart_all')}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							disabled={bulkBusy !== '' || runningCount === 0}
							onclick={() => void bulkAction('stop')}
						>
							{#if bulkBusy === 'stop'}
								<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							{:else}
								<Square class="mr-2 h-4 w-4" />
							{/if}
							{t('myaaw.server.stop_all')}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</CardAction>
		</CardHeader>
		<CardContent>
			{#if agents.length === 0}
				<EmptyState
					title={t('myaaw.home.no_agents')}
					description={t('myaaw.home.no_agents_desc')}
					bloub="curieux"
					icon={Bot}
				/>
			{:else}
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
					{#each sortedAgents as agent (agent.name)}
						<div
							class="relative rounded-xl border bg-card transition-colors hover:border-ring/40 hover:bg-muted/40"
						>
							<button
								class="flex w-full flex-col items-center gap-1.5 p-5 text-center"
								onclick={() => openChat(agent.name)}
								title={t('myaaw.home.open_chat')}
							>
								<Bloub mood="attentif" size={40} tight color={colorOf(agent.agent_color)} />
								<span class="mt-1 w-full truncate text-sm font-semibold">{agent.name}</span>
								<span class="flex flex-wrap items-center justify-center gap-1">
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
								</span>
								{#if agent.port}
									<span class="font-mono text-[10px] text-muted-foreground">
										:{agent.port}
									</span>
								{/if}
							</button>
							<div class="absolute top-1 right-1">
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
									<DropdownMenu.Content align="end" class="w-48">
										<DropdownMenu.Item
											disabled={!!toggling || bulkBusy !== ''}
											onclick={() => toggleServer(agent.name, agent.running)}
										>
											{#if agent.running}
												<Square class="mr-2 h-4 w-4" />
											{:else}
												<Play class="mr-2 h-4 w-4" />
											{/if}
											{agent.running ? t('myaaw.server.stop') : t('myaaw.server.start')}
										</DropdownMenu.Item>
										{#if agent.running}
											<DropdownMenu.Item
												disabled={!!toggling || bulkBusy !== ''}
												onclick={() => void restartServer(agent.name)}
											>
												{#if toggling === agent.name}
													<Loader2 class="mr-2 h-4 w-4 animate-spin" />
												{:else}
													<RotateCw class="mr-2 h-4 w-4" />
												{/if}
												{t('myaaw.server.restart')}
											</DropdownMenu.Item>
										{/if}
										<DropdownMenu.Separator />
										<DropdownMenu.Item onclick={() => reconfigureAgent(agent.name)}>
											<Settings2 class="mr-2 h-4 w-4" />
											{t('myaaw.onboard.reconfigure')}
										</DropdownMenu.Item>
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
				</div>
			{/if}
		</CardContent>
	</Card>

	<!-- Fleet-wide backup management: create archives for any agent and manage
	     every archive. Agent-scoped backups live on /app/backups. -->
	<BackupsPanel />
</div>
