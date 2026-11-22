<script lang="ts">
	import { cronStore } from '$lib/stores/cron.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import CreateScheduleDialog from '$lib/components/schedule/CreateScheduleDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		CalendarClock,
		ChevronLeft,
		ChevronRight,
		Clock,
		History,
		Loader2,
		Play,
		Plus,
		RefreshCw,
		Send,
		Trash2
	} from 'lucide-svelte';
	import type { CronExecution, CronJob, CronJobExecution } from '$lib/types';

	const { t } = useTranslation();

	let createDialogOpen = $state(false);

	let historyFor = $state('');
	let history = $state<CronExecution[]>([]);

	const RUN_PAGE_SIZE = 15;
	let recentRuns = $state<CronJobExecution[]>([]);
	let runsTotal = $state(0);
	let runsOffset = $state(0);
	let runsLoading = $state(false);
	let runDetail = $state<CronJobExecution | null>(null);
	let runDetailOpen = $state(false);
	let jobDetail = $state<CronJob | null>(null);
	let jobDetailOpen = $state(false);
	let jobTab = $state('jobs');
	let runningId = $state('');

	let savingScheduler = $state(false);

	let agent = $derived(agentSelection.selected);
	useQuery(cronStore.list, () => (agent ? { agent } : null));
	useQuery(configStore.query, () => (agent ? { agent } : null));

	let jobs = $derived(cronStore.jobs);
	let enabledCount = $derived(jobs.filter((job) => job.enabled).length);
	let loading = $derived(cronStore.list.loading);
	let error = $derived(cronStore.list.error);
	let schedulerActive = $derived(configStore.data?.cron?.active ?? true);

	let runsKey = '';
	$effect(() => {
		if (!agent) return;
		if (runsKey === agent) return;
		runsKey = agent;
		void loadRuns(0);
	});

	const runsHasMore = $derived(runsOffset + recentRuns.length < runsTotal);
	const runsRange = $derived.by(() => {
		if (recentRuns.length === 0) return '';
		return t('myaaw.schedule.runs_range', {
			from: runsOffset + 1,
			to: runsOffset + recentRuns.length,
			total: runsTotal
		});
	});

	// The agent reads cron.active when it loads its config, so the switch only
	// takes effect after a server restart.
	async function setScheduler(next: boolean) {
		if (!agent) return;
		savingScheduler = true;
		try {
			await configStore.patch(agent, { cron: { active: next } });
			toastStore.success(t('myaaw.schedule.scheduler_saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			savingScheduler = false;
		}
	}

	async function remove(id: string) {
		if (!agent) return;
		if (
			!(await askConfirm({ message: t('myaaw.schedule.delete_confirm'), variant: 'destructive' }))
		)
			return;
		try {
			await cronStore.remove(agent, id);
			if (historyFor === id) {
				historyFor = '';
				history = [];
			}
			toastStore.success(t('myaaw.schedule.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function run(job: CronJob) {
		if (!agent || runningId) return;
		runningId = job.id;
		try {
			await cronStore.run(agent, job.id);
			toastStore.success(t('myaaw.schedule.ran'));
			await loadRuns(0);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			runningId = '';
		}
	}

	async function toggleHistory(job: CronJob) {
		if (!agent) return;
		if (historyFor === job.id) {
			historyFor = '';
			history = [];
			return;
		}
		historyFor = job.id;
		try {
			const result = await cronStore.fetchHistory(agent, job.id);
			history = result.history ?? [];
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	// Paging runs through the agent-wide log, newest page first.
	async function loadRuns(offset: number) {
		if (!agent || runsLoading) return;
		runsLoading = true;
		try {
			const page = await cronStore.fetchRuns(agent, RUN_PAGE_SIZE, Math.max(0, offset));
			recentRuns = page.history ?? [];
			runsTotal = page.total ?? 0;
			runsOffset = page.offset ?? Math.max(0, offset);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			runsLoading = false;
		}
	}

	function showRun(entry: CronJobExecution) {
		runDetail = entry;
		runDetailOpen = true;
	}

	function showJob(job: CronJob) {
		jobDetail = job;
		jobDetailOpen = true;
	}

	function jobLabel(id: string) {
		const known = jobs.find((job) => job.id === id);
		return known ? known.name : t('myaaw.schedule.removed_job', { id: id.slice(0, 8) });
	}

	function kindLabel(value: string) {
		if (value === 'cron') return t('myaaw.schedule.kind_recurring');
		if (value === 'every') return t('myaaw.schedule.kind_interval');
		if (value === 'at') return t('myaaw.schedule.kind_once');
		return value;
	}
</script>

<TopBarActions id="cron-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => {
				void cronStore.list.refresh();
				void configStore.refresh();
				void loadRuns(0);
			}}
			loading={loading || cronStore.list.refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto w-full max-w-5xl space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<!-- Compact master scheduler bar -->
		<div
			class="flex flex-wrap items-center justify-between gap-4 rounded-xl border bg-card px-4 py-3 shadow-xs"
		>
			<div class="flex min-w-0 items-start gap-3">
				<span
					aria-hidden="true"
					class={[
						'mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full',
						schedulerActive ? 'bg-emerald-500' : 'bg-muted-foreground/40'
					]}
				></span>
				<div class="min-w-0">
					<div class="flex flex-wrap items-center gap-2">
						<p class="text-sm font-semibold text-foreground">{t('myaaw.schedule.scheduler')}</p>
						<Badge variant={schedulerActive ? 'secondary' : 'outline'} class="text-[10px]">
							{schedulerActive ? t('myaaw.schedule.enabled') : t('myaaw.schedule.disabled')}
						</Badge>
					</div>
					<p class="mt-0.5 text-xs text-muted-foreground">
						{t('myaaw.schedule.scheduler_desc')}
					</p>
					<p class="mt-0.5 text-[11px] text-muted-foreground/80">
						{t('myaaw.schedule.scheduler_hint')}
					</p>
				</div>
			</div>

			<div class="flex shrink-0 items-center gap-3">
				<span class="hidden text-xs text-muted-foreground sm:inline">
					{t('myaaw.schedule.scheduler_active')}
				</span>
				<Switch
					checked={schedulerActive}
					disabled={savingScheduler}
					label={t('myaaw.schedule.scheduler_active')}
					onchange={(next) => setScheduler(next)}
				/>
			</div>
		</div>

		<!-- Main schedules & recent runs panel -->
		<div class="rounded-xl border bg-card shadow-xs">
			<Tabs.Root bind:value={jobTab}>
				<div class="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
					<Tabs.List variant="line" class="flex-wrap justify-start">
						<Tabs.Trigger value="jobs" class="gap-1.5">
							<CalendarClock class="h-3.5 w-3.5" />
							{t('myaaw.schedule.tab_jobs')}
							<span
								class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] leading-none font-medium text-muted-foreground tabular-nums"
							>
								{enabledCount}/{jobs.length}
							</span>
						</Tabs.Trigger>
						<Tabs.Trigger value="history" class="gap-1.5">
							<History class="h-3.5 w-3.5" />
							{t('myaaw.schedule.tab_history')}
							{#if runsTotal > 0}
								<span
									class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] leading-none font-medium text-muted-foreground tabular-nums"
								>
									{runsTotal}
								</span>
							{/if}
						</Tabs.Trigger>
					</Tabs.List>

					<Button size="sm" onclick={() => (createDialogOpen = true)} class="gap-1.5 rounded-lg">
						<Plus class="h-3.5 w-3.5" />
						{t('myaaw.schedule.create')}
					</Button>
				</div>

				<div class="p-4 sm:p-5">
					<Tabs.Content value="jobs" class="mt-0 space-y-2.5">
						{#if !loading && jobs.length === 0}
							<EmptyState
								title={t('myaaw.schedule.empty')}
								description={t('myaaw.schedule.empty_desc')}
								bloub="somnolent"
								icon={CalendarClock}
								class="min-h-64"
							/>
						{:else}
							{#each jobs as job (job.id)}
								<div
									class="space-y-2.5 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20"
								>
									<div class="flex items-start justify-between gap-3">
										<button
											type="button"
											class="min-w-0 flex-1 rounded-md text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
											onclick={() => showJob(job)}
											title={t('myaaw.schedule.detail_desc')}
										>
											<div class="flex flex-wrap items-center gap-2">
												<span
													aria-hidden="true"
													class={[
														'h-2 w-2 shrink-0 rounded-full',
														job.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/40'
													]}
												></span>
												<span class="text-sm font-semibold text-foreground">{job.name}</span>
												<Badge variant={job.enabled ? 'secondary' : 'outline'} class="text-[10px]">
													{job.enabled ? t('myaaw.schedule.enabled') : t('myaaw.schedule.disabled')}
												</Badge>
												<span
													class="inline-flex items-center gap-1 rounded-md border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
												>
													<Clock class="h-2.5 w-2.5" />
													{kindLabel(job.schedule.kind)} · {job.schedule.expr}
													{#if job.schedule.tz}
														· {job.schedule.tz}
													{/if}
												</span>
											</div>

											<p class="mt-1.5 line-clamp-2 text-xs text-muted-foreground">
												{job.payload.content}
											</p>

											{#if job.delivery.channel || job.delivery.to}
												<div
													class="mt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground"
												>
													<Send class="h-3 w-3 shrink-0 opacity-70" />
													<span class="font-medium capitalize text-foreground/80">
														{job.delivery.channel || '—'}
													</span>
													<span>→</span>
													<span class="font-mono">{job.delivery.to || '—'}</span>
													{#if job.delivery.thread_id}
														<span
															>· {t('myaaw.schedule.delivery_topic')} {job.delivery.thread_id}</span
														>
													{/if}
													{#if job.delivery.chat_type}
														<span>· {job.delivery.chat_type}</span>
													{/if}
													{#if job.delivery.session_id}
														<span>
															· {t('myaaw.schedule.delivery_session')}
															<span class="font-mono">{job.delivery.session_id.slice(0, 8)}</span>
														</span>
													{/if}
												</div>
											{/if}
										</button>

										<div class="flex shrink-0 items-center gap-1">
											<Button
												size="icon-sm"
												variant="outline"
												disabled={runningId !== '' && runningId !== job.id}
												onclick={() => run(job)}
												title={t('myaaw.schedule.run_now')}
												aria-label={t('myaaw.schedule.run_now')}
											>
												{#if runningId === job.id}
													<Loader2 class="h-3.5 w-3.5 animate-spin" />
												{:else}
													<Play class="h-3.5 w-3.5" />
												{/if}
											</Button>
											<Button
												size="icon-sm"
												variant={historyFor === job.id ? 'secondary' : 'ghost'}
												onclick={() => toggleHistory(job)}
												title={t('myaaw.schedule.tab_history')}
												aria-label={t('myaaw.schedule.tab_history')}
											>
												<History class="h-3.5 w-3.5" />
											</Button>
											<Button
												size="icon-sm"
												variant="ghost"
												onclick={() => remove(job.id)}
												title={t('common.delete')}
												aria-label={t('common.delete')}
											>
												<Trash2 class="h-3.5 w-3.5 text-destructive" />
											</Button>
										</div>
									</div>

									{#if historyFor === job.id}
										<div
											class="max-h-52 space-y-1 overflow-y-auto rounded-lg border bg-muted/30 p-2 text-[11px]"
										>
											{#if history.length === 0}
												<p class="px-1 py-1 text-muted-foreground">
													{t('myaaw.schedule.no_history')}
												</p>
											{/if}
											{#each history as entry, index (index)}
												<button
													type="button"
													class="flex w-full items-start gap-2 rounded-md px-1.5 py-1 text-left transition-colors hover:bg-background/80 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
													onclick={() => showRun({ ...entry, job_id: job.id })}
												>
													<Badge
														variant={entry.status === 'success' ? 'secondary' : 'destructive'}
														class="text-[9px]"
													>
														{entry.status}
													</Badge>
													<span class="shrink-0 font-mono text-muted-foreground">
														{new Date(entry.ts).toLocaleString()}
													</span>
													<span class="min-w-0 truncate text-foreground/90">{entry.result}</span>
												</button>
											{/each}
										</div>
									{/if}
								</div>
							{/each}
						{/if}
					</Tabs.Content>

					<Tabs.Content value="history" class="mt-0 space-y-3">
						{#if recentRuns.length === 0 && runsTotal === 0}
							<EmptyState
								title={t('myaaw.schedule.recent_runs_empty')}
								description={t('myaaw.schedule.recent_runs_empty_desc')}
								bloub="somnolent"
								icon={History}
								class="min-h-64"
							/>
						{:else}
							<div class="space-y-1.5">
								{#each recentRuns as run (run.job_id + run.ts)}
									<button
										type="button"
										class="flex w-full flex-wrap items-center gap-x-2.5 gap-y-1 rounded-xl border bg-background/40 px-3 py-2.5 text-left text-xs transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
										onclick={() => showRun(run)}
									>
										<Badge
											variant={run.status === 'success' ? 'secondary' : 'destructive'}
											class="text-[10px]"
										>
											{run.status}
										</Badge>
										<span class="shrink-0 font-mono text-[11px] text-muted-foreground">
											{new Date(run.ts).toLocaleString()}
										</span>
										<span class="min-w-0 flex-1 truncate font-medium text-foreground">
											{run.name || jobLabel(run.job_id)}
										</span>
										{#if run.channel}
											<Badge variant="outline" class="text-[10px] capitalize">
												{run.channel}
											</Badge>
										{/if}
										<span class="min-w-0 basis-full truncate text-[11px] text-muted-foreground">
											{run.message || run.result}
										</span>
									</button>
								{/each}
							</div>

							<div class="flex items-center justify-between gap-2 pt-1">
								<p class="text-xs text-muted-foreground">{runsRange}</p>
								<div class="flex shrink-0 gap-1">
									<Button
										size="icon-sm"
										variant="outline"
										disabled={runsOffset === 0 || runsLoading}
										onclick={() => loadRuns(runsOffset - RUN_PAGE_SIZE)}
										aria-label={t('myaaw.schedule.page_prev')}
									>
										<ChevronLeft class="h-3.5 w-3.5" />
									</Button>
									<Button
										size="icon-sm"
										variant="outline"
										disabled={!runsHasMore || runsLoading}
										onclick={() => loadRuns(runsOffset + RUN_PAGE_SIZE)}
										aria-label={t('myaaw.schedule.page_next')}
									>
										<ChevronRight class="h-3.5 w-3.5" />
									</Button>
								</div>
							</div>
						{/if}
					</Tabs.Content>
				</div>
			</Tabs.Root>
		</div>
	{/if}

	<CreateScheduleDialog
		open={createDialogOpen}
		agent={agent ?? ''}
		onOpenChange={(next) => (createDialogOpen = next)}
	/>

	<!-- Every field the job stores. The card is only a summary, and a job created
	     from chat can carry a topic or a session nobody typed here. -->
	<Dialog.Root bind:open={jobDetailOpen}>
		<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
			<Dialog.Header>
				<Dialog.Title class="pr-6">{jobDetail?.name || ''}</Dialog.Title>
				<Dialog.Description>{t('myaaw.schedule.detail_desc')}</Dialog.Description>
			</Dialog.Header>

			{#if jobDetail}
				<dl class="grid grid-cols-[minmax(0,6rem)_1fr] gap-x-3 gap-y-2 text-xs">
					<dt class="text-muted-foreground">{t('myaaw.schedule.field_id')}</dt>
					<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.id}</dd>

					{#if jobDetail.name}
						<dt class="text-muted-foreground">{t('myaaw.schedule.name')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.name}</dd>
					{/if}

					{#if jobDetail.agent_id}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_agent')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.agent_id}</dd>
					{/if}

					<dt class="text-muted-foreground">{t('myaaw.schedule.kind')}</dt>
					<dd class="min-w-0 font-mono text-[11px] wrap-break-word">
						{kindLabel(jobDetail.schedule.kind)} · {jobDetail.schedule.expr}
						{#if jobDetail.schedule.tz}· {jobDetail.schedule.tz}{/if}
					</dd>

					{#if jobDetail.payload.kind}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_payload')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.payload.kind}</dd>
					{/if}

					{#if jobDetail.payload.content}
						<dt class="text-muted-foreground">{t('myaaw.schedule.message')}</dt>
						<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{jobDetail.payload.content}</dd>
					{/if}

					{#if jobDetail.delivery.mode}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_mode')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.mode}</dd>
					{/if}

					{#if jobDetail.delivery.channel}
						<dt class="text-muted-foreground">{t('myaaw.schedule.channel')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.channel}</dd>
					{/if}

					{#if jobDetail.delivery.to}
						<dt class="text-muted-foreground">{t('myaaw.schedule.to')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.to}</dd>
					{/if}

					{#if jobDetail.delivery.chat_type}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_chat_type')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.chat_type}</dd>
					{/if}

					{#if jobDetail.delivery.thread_id}
						<dt class="text-muted-foreground">{t('myaaw.schedule.thread_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.thread_id}</dd>
					{/if}

					{#if jobDetail.delivery.session_id}
						<dt class="text-muted-foreground">{t('myaaw.schedule.session_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.session_id}</dd>
					{/if}

					<dt class="text-muted-foreground">{t('myaaw.schedule.field_status')}</dt>
					<dd class="min-w-0 wrap-break-word">
						{jobDetail.enabled ? t('myaaw.schedule.enabled') : t('myaaw.schedule.disabled')}
					</dd>

					{#if jobDetail.created_at}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_created')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.created_at}</dd>
					{/if}

					{#if jobDetail.updated_at}
						<dt class="text-muted-foreground">{t('myaaw.schedule.field_updated')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.updated_at}</dd>
					{/if}
				</dl>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (jobDetailOpen = false)}>
					{t('myaaw.schedule.run_close')}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>

	<!-- One run, opened from the Recent runs list. -->
	<Dialog.Root bind:open={runDetailOpen}>
		<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
			<Dialog.Header>
				<Dialog.Title class="flex flex-wrap items-center gap-2 pr-6">
					{runDetail ? runDetail.name || jobLabel(runDetail.job_id) : ''}
					{#if runDetail}
						<Badge
							variant={runDetail.status === 'success' ? 'secondary' : 'destructive'}
							class="text-[10px]"
						>
							{runDetail.status}
						</Badge>
					{/if}
				</Dialog.Title>
				<Dialog.Description>{t('myaaw.schedule.run_detail_desc')}</Dialog.Description>
			</Dialog.Header>

			{#if runDetail}
				<div class="space-y-3">
					<dl class="grid grid-cols-[minmax(0,6rem)_1fr] gap-x-3 gap-y-2 text-xs">
						<dt class="text-muted-foreground">{t('myaaw.schedule.run_time')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.ts}</dd>

						<dt class="text-muted-foreground">{t('myaaw.schedule.run_job_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.job_id}</dd>

						{#if runDetail.name}
							<dt class="text-muted-foreground">{t('myaaw.schedule.name')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.name}</dd>
						{/if}

						<dt class="text-muted-foreground">{t('myaaw.schedule.run_status')}</dt>
						<dd class="min-w-0 wrap-break-word">{runDetail.status}</dd>

						{#if runDetail.message}
							<dt class="text-muted-foreground">{t('myaaw.schedule.message')}</dt>
							<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{runDetail.message}</dd>
						{/if}

						{#if runDetail.channel}
							<dt class="text-muted-foreground">{t('myaaw.schedule.channel')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.channel}</dd>
						{/if}

						{#if runDetail.to}
							<dt class="text-muted-foreground">{t('myaaw.schedule.to')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.to}</dd>
						{/if}

						{#if runDetail.chat_type}
							<dt class="text-muted-foreground">{t('myaaw.schedule.field_chat_type')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.chat_type}</dd>
						{/if}

						{#if runDetail.thread_id}
							<dt class="text-muted-foreground">{t('myaaw.schedule.thread_id')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.thread_id}</dd>
						{/if}

						{#if runDetail.session_id}
							<dt class="text-muted-foreground">{t('myaaw.schedule.session_id')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.session_id}</dd>
						{/if}

						<dt class="text-muted-foreground">{t('myaaw.schedule.run_result')}</dt>
						<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{runDetail.result || '—'}</dd>
					</dl>

					{#if !runDetail.name && !runDetail.message && !runDetail.channel}
						<p class="rounded-md bg-muted/40 p-2 text-[11px] text-muted-foreground">
							{t('myaaw.schedule.run_detail_old')}
						</p>
					{/if}
				</div>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (runDetailOpen = false)}>
					{t('myaaw.schedule.run_close')}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
</div>
