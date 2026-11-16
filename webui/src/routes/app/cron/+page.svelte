<script lang="ts">
	import { cronStore } from '$lib/stores/cron.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { cronJobSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
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
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		CalendarClock,
		ChevronLeft,
		ChevronRight,
		History,
		Loader2,
		Play,
		Plus,
		RefreshCw,
		Trash2
	} from 'lucide-svelte';
	import type {
		ConversationSummary,
		CronChannel,
		CronChatType,
		CronExecution,
		CronJob,
		CronJobExecution
	} from '$lib/types';

	const { t, locale } = useTranslation();

	let saving = $state(false);

	let name = $state('');
	let kind = $state('cron');
	let expr = $state('0 7 * * *');
	let tz = $state('Asia/Jakarta');
	let message = $state('');
	let channel = $state<CronChannel>('telegram');
	let to = $state('');
	let threadId = $state('');
	let sessionId = $state('');
	let chatType = $state<CronChatType>('guild');
	let enabled = $state(true);
	let errors = $state<FieldErrors>({});

	// Session history for the picker. Only sessions of the selected channel are
	// listed, so a web target never offers a Telegram DM transcript. Old rows
	// without a channel are left out on purpose.
	let sessionChoices = $state<ConversationSummary[]>([]);
	let sessionChoicesLoading = $state(false);
	let sessionsKey = '';

	const isTelegram = $derived(channel === 'telegram');
	const isDiscord = $derived(channel === 'discord');
	const needsSession = $derived(channel === 'api' || channel === 'cli');

	// The preset is a frontend convenience: it renders the cron expression the
	// backend already understands, so nothing new is stored.
	let preset = $state('custom');
	let presetMinutes = $state(15);
	let presetTime = $state('07:00');
	let presetWeekday = $state('1');
	let presetDay = $state('1');

	let historyFor = $state('');
	let history = $state<CronExecution[]>([]);
	// The run log grows forever, so the tab shows one page at a time. The list is
	// capped in height for the same reason: a long history must not stretch the
	// card or hide the form next to it.
	const RUN_PAGE_SIZE = 10;
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
		return t('myaaw.cron.runs_range', {
			from: runsOffset + 1,
			to: runsOffset + recentRuns.length,
			total: runsTotal
		});
	});

	const weekdays = $derived(
		Array.from({ length: 7 }, (_, index) => ({
			value: String(index),
			label: new Intl.DateTimeFormat(locale.value, {
				weekday: 'long',
				timeZone: 'UTC'
			}).format(new Date(Date.UTC(2023, 0, 1 + index)))
		}))
	);

	function presetTimeParts() {
		const [hour, minute] = presetTime.split(':').map((part) => Number.parseInt(part, 10));
		return {
			hour: Number.isFinite(hour) ? Math.min(23, Math.max(0, hour)) : 0,
			minute: Number.isFinite(minute) ? Math.min(59, Math.max(0, minute)) : 0
		};
	}

	/** The cron expression a preset expands to; empty for the custom mode. */
	const presetCron = $derived.by(() => {
		const { hour, minute } = presetTimeParts();
		switch (preset) {
			case 'hourly':
				return '0 * * * *';
			case 'daily':
				return `${minute} ${hour} * * *`;
			case 'weekdays':
				return `${minute} ${hour} * * 1-5`;
			case 'weekly':
				return `${minute} ${hour} * * ${presetWeekday}`;
			case 'monthly':
				return `${minute} ${hour} ${presetDay} * *`;
			case 'minutes': {
				const every = Math.min(59, Math.max(1, Number(presetMinutes) || 1));
				return `*/${every} * * * *`;
			}
			default:
				return '';
		}
	});

	// The agent reads cron.active when it loads its config, so the switch only
	// takes effect after a server restart.
	async function setScheduler(next: boolean) {
		if (!agent) return;
		savingScheduler = true;
		try {
			await configStore.patch(agent, { cron: { active: next } });
			toastStore.success(t('myaaw.cron.scheduler_saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			savingScheduler = false;
		}
	}

	async function loadSessions(currentAgent: string, currentChannel: CronChannel) {
		sessionChoicesLoading = true;
		try {
			const page = await conversationsStore.fetchList(currentAgent, {
				limit: 50,
				channel: currentChannel
			});
			sessionChoices = page.conversations ?? [];
		} catch {
			sessionChoices = [];
		} finally {
			sessionChoicesLoading = false;
		}
	}

	$effect(() => {
		const currentAgent = agent;
		const key = `${currentAgent}:${channel}`;
		if (!currentAgent || key === sessionsKey) return;
		sessionsKey = key;
		void loadSessions(currentAgent, channel);
	});

	// A channel change starts a new target: stale room fields would point at the
	// previous platform.
	function changeChannel(next: CronChannel) {
		channel = next;
		to = '';
		threadId = '';
		sessionId = '';
		chatType = 'guild';
		errors = {};
	}

	// Picking a past session also fills what its scope already knows: a Telegram
	// topic id, or whether a Discord target is a guild channel or a DM.
	function pickSession(id: string) {
		const choice = sessionChoices.find((candidate) => candidate.id === id);
		if (!choice) {
			sessionId = '';
			return;
		}
		sessionId = choice.id;
		if (isTelegram) {
			const parts = (choice.scope ?? '').split(':');
			threadId = parts.length >= 3 ? parts[2] : '';
		}
		if (isDiscord) {
			chatType = (choice.scope ?? '').startsWith('dc:') ? 'guild' : 'dm';
		}
	}

	async function create() {
		if (!agent) return;
		// A preset is converted to cron syntax here, so the backend only ever sees
		// a plain expression.
		const submitted = kind === 'cron' && preset !== 'custom' ? presetCron : expr;
		const result = validate(cronJobSchema, {
			name,
			schedule_kind: kind,
			expr: submitted,
			tz,
			message,
			channel,
			to,
			thread_id: isTelegram ? threadId.trim() : '',
			session_id: sessionId.trim(),
			chat_type: isDiscord ? chatType : undefined
		});
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		saving = true;
		try {
			await cronStore.add(agent, {
				name: result.data.name,
				schedule_kind: result.data.schedule_kind,
				expr: result.data.expr,
				tz: result.data.tz,
				message: result.data.message,
				channel: result.data.channel,
				to: result.data.to,
				thread_id: isTelegram ? result.data.thread_id || undefined : undefined,
				session_id: result.data.session_id || undefined,
				chat_type: isDiscord ? result.data.chat_type : undefined,
				enabled
			});
			name = '';
			message = '';
			to = '';
			threadId = '';
			sessionId = '';
			chatType = 'guild';
			toastStore.success(t('myaaw.cron.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function remove(id: string) {
		if (!agent) return;
		if (!(await askConfirm({ message: t('myaaw.cron.delete_confirm'), variant: 'destructive' })))
			return;
		try {
			await cronStore.remove(agent, id);
			if (historyFor === id) {
				historyFor = '';
				history = [];
			}
			toastStore.success(t('myaaw.cron.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function run(job: CronJob) {
		if (!agent || runningId) return;
		runningId = job.id;
		try {
			await cronStore.run(agent, job.id);
			toastStore.success(t('myaaw.cron.ran'));
			// The agent writes the run entry before answering, so the newest page can
			// be reloaded right away.
			await loadRuns(0);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			runningId = '';
		}
	}

	async function showHistory(job: CronJob) {
		if (!agent) return;
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

	// The card is a summary; the dialog repeats every field the job stores.
	function showJob(job: CronJob) {
		jobDetail = job;
		jobDetailOpen = true;
	}

	// Runs keep their job id after the job row is gone, so a one-time job stays
	// readable both by name (while it exists) and by a short id afterwards.
	function jobLabel(id: string) {
		const known = jobs.find((job) => job.id === id);
		return known ? known.name : t('myaaw.cron.removed_job', { id: id.slice(0, 8) });
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

<div class="space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
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

		<div class="grid items-start gap-4 lg:grid-cols-5">
			<div class="min-w-0 space-y-4 lg:col-span-3">
				<Card>
					<CardHeader class="pb-2">
						<CardTitle class="text-sm font-medium">{t('myaaw.cron.scheduler')}</CardTitle>
						<CardDescription>{t('myaaw.cron.scheduler_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="space-y-1">
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={schedulerActive}
								disabled={savingScheduler}
								onchange={(event) => setScheduler(event.currentTarget.checked)}
								class="h-4 w-4 accent-primary"
							/>
							{t('myaaw.cron.scheduler_active')}
						</label>
						<p class="text-xs text-muted-foreground">{t('myaaw.cron.scheduler_hint')}</p>
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<CardTitle class="flex items-center gap-2">
							<CalendarClock class="h-4 w-4" />
							{t('myaaw.cron.jobs')}
						</CardTitle>
						<CardDescription>{t('myaaw.cron.jobs_desc')}</CardDescription>
					</CardHeader>
					<CardContent>
						<Tabs.Root bind:value={jobTab} class="space-y-3">
							<Tabs.List variant="line" class="flex-wrap justify-start">
								<Tabs.Trigger value="jobs">{t('myaaw.cron.tab_jobs')}</Tabs.Trigger>
								<Tabs.Trigger value="history">{t('myaaw.cron.tab_history')}</Tabs.Trigger>
							</Tabs.List>
							<Tabs.Content value="jobs" class="space-y-2">
								{#if !loading && jobs.length === 0}
									<EmptyState
										title={t('myaaw.cron.empty')}
										description={t('myaaw.cron.empty_desc')}
										bloub="somnolent"
										icon={CalendarClock}
										class="min-h-55"
									/>
								{/if}
								{#each jobs as job (job.id)}
									<div class="space-y-2 rounded-lg border p-3">
										<div class="flex items-start justify-between gap-2">
											<button
												type="button"
												class="min-w-0 flex-1 rounded-md text-left transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
												onclick={() => showJob(job)}
												title={t('myaaw.cron.detail_desc')}
											>
												<div class="flex flex-wrap items-center gap-2">
													<span class="font-medium">{job.name}</span>
													<Badge variant={job.enabled ? 'secondary' : 'outline'}>
														{job.enabled ? t('myaaw.cron.enabled') : t('myaaw.cron.disabled')}
													</Badge>
													<Badge variant="outline" class="font-mono text-[10px]">
														{job.schedule.kind} · {job.schedule.expr}
													</Badge>
												</div>
												<p class="mt-1 truncate text-[11px] text-muted-foreground">
													{job.payload.content}
												</p>
												{#if job.delivery.channel || job.delivery.to}
													<p class="text-[11px] text-muted-foreground">
														{job.delivery.channel || '—'} → {job.delivery.to || '—'}
														{#if job.delivery.thread_id}
															· {t('myaaw.cron.delivery_topic')} {job.delivery.thread_id}
														{/if}
														{#if job.delivery.chat_type}
															· {job.delivery.chat_type}
														{/if}
														{#if job.delivery.session_id}
															· {t('myaaw.cron.delivery_session')}
															<span class="font-mono">{job.delivery.session_id.slice(0, 8)}</span>
														{/if}
													</p>
												{/if}
											</button>
											<div class="flex shrink-0 gap-1">
												<Button
													size="icon-sm"
													variant="outline"
													disabled={runningId !== '' && runningId !== job.id}
													onclick={() => run(job)}
													aria-label={t('myaaw.cron.run_now')}
												>
													{#if runningId === job.id}
														<Loader2 class="h-3.5 w-3.5 animate-spin" />
													{:else}
														<Play class="h-3.5 w-3.5" />
													{/if}
												</Button>
												<Button size="icon-sm" variant="ghost" onclick={() => showHistory(job)}>
													<History class="h-3.5 w-3.5" />
												</Button>
												<Button size="icon-sm" variant="ghost" onclick={() => remove(job.id)}>
													<Trash2 class="h-3.5 w-3.5 text-destructive" />
												</Button>
											</div>
										</div>

										{#if historyFor === job.id}
											<div
												class="max-h-48 space-y-0.5 overflow-y-auto rounded-md bg-muted/30 p-2 text-[11px]"
											>
												{#if history.length === 0}
													<p class="text-muted-foreground">{t('myaaw.cron.no_history')}</p>
												{/if}
												{#each history as entry, index (index)}
													<button
														type="button"
														class="flex w-full items-start gap-2 rounded-sm px-1 py-0.5 text-left hover:bg-background/70 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
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
														<span class="min-w-0 truncate">{entry.result}</span>
													</button>
												{/each}
											</div>
										{/if}
									</div>
								{/each}
							</Tabs.Content>

							<Tabs.Content value="history" class="space-y-2">
								{#if recentRuns.length === 0 && runsTotal === 0}
									<EmptyState
										title={t('myaaw.cron.recent_runs_empty')}
										description={t('myaaw.cron.recent_runs_empty_desc')}
										bloub="somnolent"
										icon={History}
										class="min-h-55"
									/>
								{:else}
									<div class="max-h-80 overflow-y-auto overscroll-contain pr-1">
										<div class="space-y-1">
											{#each recentRuns as run (run.job_id + run.ts)}
												<button
													type="button"
													class="flex w-full flex-wrap items-center gap-x-2 gap-y-1 rounded-md border p-2 text-left text-[11px] transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
													onclick={() => showRun(run)}
												>
													<Badge
														variant={run.status === 'success' ? 'secondary' : 'destructive'}
														class="text-[9px]"
													>
														{run.status}
													</Badge>
													<span class="shrink-0 font-mono text-muted-foreground">
														{new Date(run.ts).toLocaleString()}
													</span>
													<span class="min-w-0 flex-1 truncate font-medium">
														{run.name || jobLabel(run.job_id)}
													</span>
													<span class="min-w-0 basis-full truncate text-muted-foreground">
														{run.message || run.result}
													</span>
												</button>
											{/each}
										</div>
									</div>
									<div class="flex items-center justify-between gap-2">
										<p class="text-[11px] text-muted-foreground">{runsRange}</p>
										<div class="flex shrink-0 gap-1">
											<Button
												size="icon-sm"
												variant="outline"
												disabled={runsOffset === 0 || runsLoading}
												onclick={() => loadRuns(runsOffset - RUN_PAGE_SIZE)}
												aria-label={t('myaaw.cron.page_prev')}
											>
												<ChevronLeft class="h-3.5 w-3.5" />
											</Button>
											<Button
												size="icon-sm"
												variant="outline"
												disabled={!runsHasMore || runsLoading}
												onclick={() => loadRuns(runsOffset + RUN_PAGE_SIZE)}
												aria-label={t('myaaw.cron.page_next')}
											>
												<ChevronRight class="h-3.5 w-3.5" />
											</Button>
										</div>
									</div>
								{/if}
							</Tabs.Content>
						</Tabs.Root>
					</CardContent>
				</Card>
			</div>

			<Card class="min-w-0 lg:col-span-2">
				<CardHeader>
					<CardTitle>{t('myaaw.cron.create')}</CardTitle>
					<CardDescription>{t('myaaw.cron.create_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						id="cron-create-form"
						class="space-y-5"
						onsubmit={(event) => {
							event.preventDefault();
							void create();
						}}
					>
						<section class="space-y-3">
							<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.cron.section_schedule')}
							</p>
							<div class="grid gap-1">
								<Label for="cron-name">{t('myaaw.cron.name')}</Label>
								<Input id="cron-name" bind:value={name} placeholder="Morning reminder" />
								<FieldError code={errors.name} />
							</div>
							<div class="grid grid-cols-2 gap-2">
								<div class="grid gap-1">
									<Label for="cron-kind">{t('myaaw.cron.kind')}</Label>
									<select
										id="cron-kind"
										bind:value={kind}
										class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
									>
										<option value="cron">cron</option>
										<option value="every">every</option>
										<option value="at">at</option>
									</select>
								</div>
								<div class="grid gap-1">
									<Label for="cron-tz">{t('myaaw.cron.tz')}</Label>
									<Input id="cron-tz" bind:value={tz} />
								</div>
							</div>
							{#if kind === 'cron'}
								<div class="grid gap-1">
									<Label for="cron-preset">{t('myaaw.cron.preset')}</Label>
									<select
										id="cron-preset"
										bind:value={preset}
										class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
									>
										<option value="custom">{t('myaaw.cron.preset_custom')}</option>
										<option value="minutes">{t('myaaw.cron.preset_minutes')}</option>
										<option value="hourly">{t('myaaw.cron.preset_hourly')}</option>
										<option value="daily">{t('myaaw.cron.preset_daily')}</option>
										<option value="weekdays">{t('myaaw.cron.preset_weekdays')}</option>
										<option value="weekly">{t('myaaw.cron.preset_weekly')}</option>
										<option value="monthly">{t('myaaw.cron.preset_monthly')}</option>
									</select>
								</div>

								{#if preset === 'minutes'}
									<div class="grid gap-1">
										<Label for="cron-minutes">{t('myaaw.cron.minutes')}</Label>
										<Input
											id="cron-minutes"
											type="number"
											min="1"
											max="59"
											bind:value={presetMinutes}
										/>
									</div>
								{:else if preset === 'daily' || preset === 'weekdays' || preset === 'weekly' || preset === 'monthly'}
									<div class="grid grid-cols-2 gap-2">
										<div class="grid gap-1">
											<Label for="cron-time">{t('myaaw.cron.time')}</Label>
											<Input id="cron-time" type="time" bind:value={presetTime} />
										</div>
										{#if preset === 'weekly'}
											<div class="grid gap-1">
												<Label for="cron-weekday">{t('myaaw.cron.weekday')}</Label>
												<select
													id="cron-weekday"
													bind:value={presetWeekday}
													class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
												>
													{#each weekdays as day (day.value)}
														<option value={day.value}>{day.label}</option>
													{/each}
												</select>
											</div>
										{:else if preset === 'monthly'}
											<div class="grid gap-1">
												<Label for="cron-day">{t('myaaw.cron.day_of_month')}</Label>
												<Input
													id="cron-day"
													type="number"
													min="1"
													max="31"
													bind:value={presetDay}
												/>
											</div>
										{/if}
									</div>
								{/if}

								<div class="grid gap-1">
									<Label for="cron-expr">{t('myaaw.cron.expr')}</Label>
									{#if preset === 'custom'}
										<Input id="cron-expr" bind:value={expr} placeholder="0 7 * * *" />
									{:else}
										<Input id="cron-expr" value={presetCron} disabled />
									{/if}
									<FieldError code={errors.expr} />
								</div>
							{:else}
								<div class="grid gap-1">
									<Label for="cron-expr">{t('myaaw.cron.expr')}</Label>
									<Input id="cron-expr" bind:value={expr} placeholder="0 7 * * *" />
									<FieldError code={errors.expr} />
								</div>
							{/if}
						</section>

						<section class="space-y-3">
							<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.cron.section_message')}
							</p>
							<div class="grid gap-1">
								<Label for="cron-message">{t('myaaw.cron.message')}</Label>
								<textarea
									id="cron-message"
									bind:value={message}
									rows="3"
									placeholder="Good morning!"
									class="w-full rounded-md border border-input bg-transparent p-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								></textarea>
								<FieldError code={errors.message} />
							</div>
						</section>

						<section class="space-y-3">
							<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.cron.section_delivery')}
							</p>
							<div class="grid items-start gap-2 sm:grid-cols-2">
								<div class="grid gap-1">
									<Label for="cron-channel">{t('myaaw.cron.channel')}</Label>
									<select
										id="cron-channel"
										value={channel}
										onchange={(event) => changeChannel(event.currentTarget.value as CronChannel)}
										class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
									>
										<option value="telegram">{t('myaaw.cron.channel_telegram')}</option>
										<option value="discord">{t('myaaw.cron.channel_discord')}</option>
										<option value="api">{t('myaaw.cron.channel_api')}</option>
										<option value="cli">{t('myaaw.cron.channel_cli')}</option>
									</select>
								</div>
								<div class="grid gap-1">
									<Label for="cron-to">{t('myaaw.cron.to')}</Label>
									<Input id="cron-to" bind:value={to} placeholder="123456789" />
									{#if isTelegram}
										<p class="text-[11px] text-muted-foreground">
											{t('myaaw.cron.to_telegram_hint')}
										</p>
									{:else if isDiscord}
										<p class="text-[11px] text-muted-foreground">
											{t('myaaw.cron.to_discord_hint')}
										</p>
									{:else}
										<p class="text-[11px] text-muted-foreground">
											{t('myaaw.cron.to_api_hint')}
										</p>
									{/if}
								</div>
							</div>

							{#if isTelegram}
								<div class="grid gap-1">
									<Label for="cron-thread">{t('myaaw.cron.thread_id')}</Label>
									<Input id="cron-thread" bind:value={threadId} placeholder="77" />
									<p class="text-[11px] text-muted-foreground">{t('myaaw.cron.thread_id_hint')}</p>
									<FieldError code={errors.thread_id} />
								</div>
							{:else if isDiscord}
								<div class="grid gap-1">
									<Label for="cron-chat-type">{t('myaaw.cron.chat_type')}</Label>
									<select
										id="cron-chat-type"
										bind:value={chatType}
										class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
									>
										<option value="guild">{t('myaaw.cron.chat_type_guild')}</option>
										<option value="dm">{t('myaaw.cron.chat_type_dm')}</option>
									</select>
									<p class="text-[11px] text-muted-foreground">{t('myaaw.cron.chat_type_hint')}</p>
								</div>
							{/if}

							<div class="grid gap-1">
								<Label for="cron-session">{t('myaaw.cron.session_id')}</Label>
								<select
									id="cron-session-pick"
									value={sessionChoices.some((choice) => choice.id === sessionId) ? sessionId : ''}
									onchange={(event) => pickSession(event.currentTarget.value)}
									disabled={sessionChoicesLoading}
									class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								>
									<option value="">
										{sessionChoices.length === 0
											? t('myaaw.cron.session_none')
											: t('myaaw.cron.session_history')}
									</option>
									{#each sessionChoices as choice (choice.id)}
										<option value={choice.id}>
											{choice.title || choice.id.slice(0, 8)} · {choice.id.slice(0, 8)}
										</option>
									{/each}
								</select>
								<Input id="cron-session" bind:value={sessionId} placeholder="0f4e8b3a-…" />
								<p class="text-[11px] text-muted-foreground">
									{needsSession
										? t('myaaw.cron.session_id_required_hint')
										: t('myaaw.cron.session_id_optional_hint')}
								</p>
								<FieldError code={errors.session_id} />
							</div>
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={enabled} class="h-4 w-4 accent-primary" />
								{t('myaaw.cron.enabled')}
							</label>
						</section>
						<Button type="submit" class="w-full" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.cron.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	{/if}

	<!-- Every field the job stores. The card is only a summary, and a job created
	     from chat can carry a topic or a session nobody typed here. -->
	<Dialog.Root bind:open={jobDetailOpen}>
		<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
			<Dialog.Header>
				<Dialog.Title class="pr-6">{jobDetail?.name || ''}</Dialog.Title>
				<Dialog.Description>{t('myaaw.cron.detail_desc')}</Dialog.Description>
			</Dialog.Header>

			{#if jobDetail}
				<dl class="grid grid-cols-[minmax(0,6rem)_1fr] gap-x-3 gap-y-2 text-xs">
					<dt class="text-muted-foreground">{t('myaaw.cron.field_id')}</dt>
					<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.id}</dd>

					{#if jobDetail.name}
						<dt class="text-muted-foreground">{t('myaaw.cron.name')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.name}</dd>
					{/if}

					{#if jobDetail.agent_id}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_agent')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.agent_id}</dd>
					{/if}

					<dt class="text-muted-foreground">{t('myaaw.cron.kind')}</dt>
					<dd class="min-w-0 font-mono text-[11px] wrap-break-word">
						{jobDetail.schedule.kind} · {jobDetail.schedule.expr}
						{#if jobDetail.schedule.tz}· {jobDetail.schedule.tz}{/if}
					</dd>

					{#if jobDetail.payload.kind}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_payload')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.payload.kind}</dd>
					{/if}

					{#if jobDetail.payload.content}
						<dt class="text-muted-foreground">{t('myaaw.cron.message')}</dt>
						<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{jobDetail.payload.content}</dd>
					{/if}

					{#if jobDetail.delivery.mode}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_mode')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.mode}</dd>
					{/if}

					{#if jobDetail.delivery.channel}
						<dt class="text-muted-foreground">{t('myaaw.cron.channel')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.channel}</dd>
					{/if}

					{#if jobDetail.delivery.to}
						<dt class="text-muted-foreground">{t('myaaw.cron.to')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.to}</dd>
					{/if}

					{#if jobDetail.delivery.chat_type}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_chat_type')}</dt>
						<dd class="min-w-0 wrap-break-word">{jobDetail.delivery.chat_type}</dd>
					{/if}

					{#if jobDetail.delivery.thread_id}
						<dt class="text-muted-foreground">{t('myaaw.cron.thread_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.thread_id}</dd>
					{/if}

					{#if jobDetail.delivery.session_id}
						<dt class="text-muted-foreground">{t('myaaw.cron.session_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.delivery.session_id}</dd>
					{/if}

					<dt class="text-muted-foreground">{t('myaaw.cron.field_status')}</dt>
					<dd class="min-w-0 wrap-break-word">
						{jobDetail.enabled ? t('myaaw.cron.enabled') : t('myaaw.cron.disabled')}
					</dd>

					{#if jobDetail.created_at}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_created')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.created_at}</dd>
					{/if}

					{#if jobDetail.updated_at}
						<dt class="text-muted-foreground">{t('myaaw.cron.field_updated')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{jobDetail.updated_at}</dd>
					{/if}
				</dl>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (jobDetailOpen = false)}>
					{t('myaaw.cron.run_close')}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>

	<!-- One run, opened from the Recent runs list. Each entry carries a snapshot of
	     the job, so a run of a job that was since removed still says what it
	     delivered; older entries only have the id and result. -->
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
				<Dialog.Description>{t('myaaw.cron.run_detail_desc')}</Dialog.Description>
			</Dialog.Header>

			{#if runDetail}
				<div class="space-y-3">
					<dl class="grid grid-cols-[minmax(0,6rem)_1fr] gap-x-3 gap-y-2 text-xs">
						<dt class="text-muted-foreground">{t('myaaw.cron.run_time')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.ts}</dd>

						<dt class="text-muted-foreground">{t('myaaw.cron.run_job_id')}</dt>
						<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.job_id}</dd>

						{#if runDetail.name}
							<dt class="text-muted-foreground">{t('myaaw.cron.name')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.name}</dd>
						{/if}

						<dt class="text-muted-foreground">{t('myaaw.cron.run_status')}</dt>
						<dd class="min-w-0 wrap-break-word">{runDetail.status}</dd>

						{#if runDetail.message}
							<dt class="text-muted-foreground">{t('myaaw.cron.message')}</dt>
							<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{runDetail.message}</dd>
						{/if}

						{#if runDetail.channel}
							<dt class="text-muted-foreground">{t('myaaw.cron.channel')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.channel}</dd>
						{/if}

						{#if runDetail.to}
							<dt class="text-muted-foreground">{t('myaaw.cron.to')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.to}</dd>
						{/if}

						{#if runDetail.chat_type}
							<dt class="text-muted-foreground">{t('myaaw.cron.field_chat_type')}</dt>
							<dd class="min-w-0 wrap-break-word">{runDetail.chat_type}</dd>
						{/if}

						{#if runDetail.thread_id}
							<dt class="text-muted-foreground">{t('myaaw.cron.thread_id')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.thread_id}</dd>
						{/if}

						{#if runDetail.session_id}
							<dt class="text-muted-foreground">{t('myaaw.cron.session_id')}</dt>
							<dd class="min-w-0 font-mono text-[11px] break-all">{runDetail.session_id}</dd>
						{/if}

						<dt class="text-muted-foreground">{t('myaaw.cron.run_result')}</dt>
						<dd class="min-w-0 whitespace-pre-wrap wrap-break-word">{runDetail.result || '—'}</dd>
					</dl>

					{#if !runDetail.name && !runDetail.message && !runDetail.channel}
						<p class="rounded-md bg-muted/40 p-2 text-[11px] text-muted-foreground">
							{t('myaaw.cron.run_detail_old')}
						</p>
					{/if}
				</div>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (runDetailOpen = false)}>
					{t('myaaw.cron.run_close')}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
</div>
