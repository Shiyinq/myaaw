<script lang="ts">
	import { cron as cronApi } from '$lib/apis/cron';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { cronJobSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
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
	import { Separator } from '$lib/components/ui/separator';
	import { CalendarClock, History, Play, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { CronExecution, CronJob } from '$lib/types';

	const { t } = useTranslation();

	let jobs = $state<CronJob[]>([]);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadKey = '';

	let name = $state('');
	let kind = $state('cron');
	let expr = $state('0 7 * * *');
	let tz = $state('Asia/Jakarta');
	let message = $state('');
	let channel = $state('telegram');
	let to = $state('');
	let enabled = $state(true);
	let errors = $state<FieldErrors>({});

	let historyFor = $state('');
	let history = $state<CronExecution[]>([]);

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const result = await cronApi.list(currentAgent);
			jobs = result.jobs ?? [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	async function create() {
		if (!agent) return;
		const result = validate(cronJobSchema, {
			name,
			schedule_kind: kind,
			expr,
			tz,
			message,
			channel,
			to
		});
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		saving = true;
		try {
			await cronApi.add(agent, {
				name: result.data.name,
				schedule_kind: result.data.schedule_kind,
				expr: result.data.expr,
				tz: result.data.tz,
				message: result.data.message,
				channel: result.data.channel || undefined,
				to: result.data.to || undefined,
				enabled
			});
			name = '';
			message = '';
			to = '';
			toastStore.success(t('myaaw.cron.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function remove(id: string) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.cron.delete_confirm'))) return;
		try {
			await cronApi.remove(agent, id);
			if (historyFor === id) {
				historyFor = '';
				history = [];
			}
			await load(agent);
			toastStore.success(t('myaaw.cron.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function run(job: CronJob) {
		if (!agent) return;
		try {
			await cronApi.run(agent, job.id);
			toastStore.success(t('myaaw.cron.ran'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function showHistory(job: CronJob) {
		if (!agent) return;
		historyFor = job.id;
		try {
			const result = await cronApi.history(agent, job.id, 20);
			history = result.history ?? [];
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="cron-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
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

		<div class="grid gap-4 lg:grid-cols-5">
			<Card class="lg:col-span-3">
				<CardHeader>
					<CardTitle class="flex items-center gap-2">
						<CalendarClock class="h-4 w-4" />
						{t('myaaw.cron.jobs')}
					</CardTitle>
					<CardDescription>{t('myaaw.cron.jobs_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-2">
					{#if !loading && jobs.length === 0}
						<EmptyState
							title={t('myaaw.cron.empty')}
							description={t('myaaw.cron.empty_desc')}
							icon={CalendarClock}
						/>
					{/if}
					{#each jobs as job (job.id)}
						<div class="space-y-2 rounded-lg border p-3">
							<div class="flex items-start justify-between gap-2">
								<div class="min-w-0">
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
										</p>
									{/if}
								</div>
								<div class="flex shrink-0 gap-1">
									<Button size="icon-sm" variant="outline" onclick={() => run(job)}>
										<Play class="h-3.5 w-3.5" />
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
								<div class="rounded-md bg-muted/30 p-2 text-[11px]">
									{#if history.length === 0}
										<p class="text-muted-foreground">{t('myaaw.cron.no_history')}</p>
									{/if}
									{#each history as entry, index (index)}
										<div class="flex items-start gap-2 py-0.5">
											<Badge
												variant={entry.status === 'success' ? 'secondary' : 'destructive'}
												class="text-[9px]"
											>
												{entry.status}
											</Badge>
											<span class="font-mono text-muted-foreground">
												{new Date(entry.ts).toLocaleString()}
											</span>
											<span class="truncate">{entry.result}</span>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-2">
				<CardHeader>
					<CardTitle>{t('myaaw.cron.create')}</CardTitle>
					<CardDescription>{t('myaaw.cron.create_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-2"
						onsubmit={(event) => {
							event.preventDefault();
							void create();
						}}
					>
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
						<div class="grid gap-1">
							<Label for="cron-expr">{t('myaaw.cron.expr')}</Label>
							<Input id="cron-expr" bind:value={expr} placeholder="0 7 * * *" />
							<FieldError code={errors.expr} />
						</div>
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
						<div class="grid grid-cols-2 gap-2">
							<div class="grid gap-1">
								<Label for="cron-channel">{t('myaaw.cron.channel')}</Label>
								<select
									id="cron-channel"
									bind:value={channel}
									class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								>
									<option value="telegram">telegram</option>
									<option value="discord">discord</option>
									<option value="">none</option>
								</select>
							</div>
							<div class="grid gap-1">
								<Label for="cron-to">{t('myaaw.cron.to')}</Label>
								<Input id="cron-to" bind:value={to} placeholder="123456789" />
							</div>
						</div>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={enabled} class="h-4 w-4 accent-primary" />
							{t('myaaw.cron.enabled')}
						</label>
						<Separator />
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.cron.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
