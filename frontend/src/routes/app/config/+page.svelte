<script lang="ts">
	import { config as configApi } from '$lib/apis/config';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';

	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Tabs from '$lib/components/ui/tabs';
	import { RefreshCw, RotateCcw, Save, Undo2 } from 'lucide-svelte';
	import { Activity, Bot, HeartPulse, ShieldCheck } from 'lucide-svelte';
	import type { AgentConfig } from '$lib/types';

	const { t } = useTranslation();

	let tab = $state('bot');
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadKey = '';

	let botType = $state('private');
	let ownerIds = $state('');
	let stream = $state(true);
	let watermark = $state(false);
	let maxIterations = $state(100);
	let warningIterations = $state(90);
	let contextMaxTokens = $state(64000);
	let maxToolOutputBytes = $state(32768);
	let toolTimeout = $state(300);
	let runTimeout = $state(900);
	let streamTimeout = $state(180);
	let requestRetries = $state(1);
	let queueWorkers = $state(4);
	let repeatToolCallLimit = $state(3);
	let computerUse = $state(false);
	let blockedTools = $state('');

	let hbActive = $state(false);
	let hbChannel = $state('');
	let hbTo = $state('');
	let hbEvery = $state('30m');

	let obsEnabled = $state(true);
	let obsRetention = $state(30);
	let obsMaxChars = $state(16384);
	let obsStoreContent = $state(true);
	let obsRedact = $state('');
	let obsModelPrices = $state('{}');
	let obsToolPrices = $state('{}');

	let apMode = $state('manual');
	let apTimeout = $state(60);
	let apUnattended = $state('allow');
	let apSubagents = $state('inherit');
	let apAllow = $state('');
	let apDeny = $state('');

	let agent = $derived(agentSelection.selected);

	const resettable = ['bot', 'heartbeat', 'observability', 'approvals'] as const;
	type ResettableSection = (typeof resettable)[number];

	function apply(cfgValue: AgentConfig) {
		const bot = cfgValue.bot;
		botType = bot.type;
		ownerIds = (bot.owner_ids ?? []).join(', ');
		stream = bot.stream;
		watermark = bot.watermark;
		maxIterations = bot.max_iterations;
		warningIterations = bot.warning_iterations;
		contextMaxTokens = bot.context_max_tokens;
		maxToolOutputBytes = bot.max_tool_output_bytes;
		toolTimeout = bot.tool_timeout_seconds;
		runTimeout = bot.run_timeout_seconds;
		streamTimeout = bot.stream_timeout_seconds;
		requestRetries = bot.request_retries;
		queueWorkers = bot.queue_workers;
		repeatToolCallLimit = bot.repeat_tool_call_limit ?? 3;
		computerUse = bot.computer_use?.enabled ?? false;
		blockedTools = (bot.blocked_tools ?? []).join(', ');

		hbActive = cfgValue.heartbeat.active;
		hbChannel = cfgValue.heartbeat.channel;
		hbTo = cfgValue.heartbeat.to;
		hbEvery = cfgValue.heartbeat.every;

		const obs = cfgValue.observability;
		obsEnabled = obs?.enabled ?? true;
		obsRetention = obs?.retention_days ?? 30;
		obsMaxChars = obs?.max_content_chars ?? 16384;
		obsStoreContent = obs?.store_content ?? true;
		obsRedact = (obs?.redact_patterns ?? []).join('\n');
		obsModelPrices = JSON.stringify(obs?.model_prices ?? {}, null, 2);
		obsToolPrices = JSON.stringify(obs?.tool_prices ?? {}, null, 2);

		const approvals = cfgValue.approvals;
		apMode = approvals?.mode ?? 'manual';
		apTimeout = approvals?.timeout_seconds ?? 60;
		apUnattended = approvals?.unattended ?? 'allow';
		apSubagents = approvals?.subagents ?? 'inherit';
		apAllow = (approvals?.allow ?? []).join('\n');
		apDeny = (approvals?.deny ?? []).join('\n');
	}

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			apply(await configApi.get(currentAgent));
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

	function splitList(value: string) {
		return value
			.split(/[\n,]/)
			.map((part) => part.trim())
			.filter(Boolean);
	}

	async function resetSection(section: ResettableSection) {
		if (!agent) return;
		if (!(await askConfirm({ message: t('myaaw.config.reset_confirm'), variant: 'destructive' })))
			return;
		saving = true;
		try {
			await configApi.resetFields(agent, section === 'bot' ? { bot: [] } : { [section]: true });
			toastStore.success(t('myaaw.config.reset_done'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function resetAll() {
		if (!agent) return;
		if (
			!(await askConfirm({ message: t('myaaw.config.reset_all_confirm'), variant: 'destructive' }))
		)
			return;
		saving = true;
		try {
			await configApi.reset(agent);
			toastStore.success(t('myaaw.config.reset_done'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	function resetCurrent() {
		const section = resettable.find((item) => item === tab);
		if (section) void resetSection(section);
	}

	async function resetField(section: string, field: string) {
		if (!agent) return;
		saving = true;
		try {
			await configApi.resetFields(agent, { [section]: [field] });
			toastStore.success(t('myaaw.config.reset_done'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function save() {
		if (!agent) return;
		saving = true;
		try {
			await configApi.patch(agent, {
				bot: {
					type: botType,
					owner_ids: splitList(ownerIds),
					stream,
					watermark,
					max_iterations: Number(maxIterations),
					warning_iterations: Number(warningIterations),
					context_max_tokens: Number(contextMaxTokens),
					max_tool_output_bytes: Number(maxToolOutputBytes),
					tool_timeout_seconds: Number(toolTimeout),
					run_timeout_seconds: Number(runTimeout),
					stream_timeout_seconds: Number(streamTimeout),
					request_retries: Number(requestRetries),
					queue_workers: Number(queueWorkers),
					repeat_tool_call_limit: Number(repeatToolCallLimit),
					computer_use: { enabled: computerUse },
					blocked_tools: splitList(blockedTools)
				},
				heartbeat: {
					active: hbActive,
					channel: hbChannel,
					to: hbTo,
					every: hbEvery
				},
				observability: {
					enabled: obsEnabled,
					retention_days: Number(obsRetention),
					max_content_chars: Number(obsMaxChars),
					store_content: obsStoreContent,
					redact_patterns: splitList(obsRedact),
					model_prices: JSON.parse(obsModelPrices || '{}'),
					tool_prices: JSON.parse(obsToolPrices || '{}')
				},
				approvals: {
					mode: apMode,
					timeout_seconds: Number(apTimeout),
					unattended: apUnattended,
					subagents: apSubagents,
					allow: splitList(apAllow),
					deny: splitList(apDeny)
				}
			});
			toastStore.success(t('myaaw.config.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}
</script>

{#snippet resetIcon(section: string, field: string)}
	<button
		type="button"
		class="shrink-0 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
		title={t('myaaw.config.reset_field')}
		aria-label={t('myaaw.config.reset_field')}
		disabled={saving}
		onclick={() => resetField(section, field)}
	>
		<Undo2 class="h-3.5 w-3.5" />
	</button>
{/snippet}

{#snippet fieldLabel(id: string, key: string, section: string, field: string)}
	<div class="flex items-center justify-between gap-2">
		<Label for={id}>{t(key)}</Label>
		{@render resetIcon(section, field)}
	</div>
{/snippet}

{#snippet botLabel(id: string, key: string, field: string)}
	{@render fieldLabel(id, key, 'bot', field)}
{/snippet}

<TopBarActions id="config-actions">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
		<TopBarButton
			label={t('myaaw.config.reset_all')}
			icon={RotateCcw}
			onclick={resetAll}
			variant="outline"
		/>
		<TopBarButton
			label={t('common.save')}
			icon={Save}
			onclick={save}
			loading={saving}
			variant="default"
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

		<Tabs.Root bind:value={tab} class="space-y-4">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<Tabs.List variant="line" class="flex-wrap justify-start">
					<Tabs.Trigger value="bot">
						<Bot />
						{t('myaaw.config.tab_bot')}
					</Tabs.Trigger>
					<Tabs.Trigger value="heartbeat">
						<HeartPulse />
						{t('myaaw.config.tab_heartbeat')}
					</Tabs.Trigger>
					<Tabs.Trigger value="observability">
						<Activity />
						{t('myaaw.config.tab_observability')}
					</Tabs.Trigger>
					<Tabs.Trigger value="approvals">
						<ShieldCheck />
						{t('myaaw.config.tab_approvals')}
					</Tabs.Trigger>
				</Tabs.List>
				<Button variant="ghost" size="sm" onclick={resetCurrent} disabled={saving}>
					<RotateCcw class="mr-1.5 h-3 w-3" />
					{t('myaaw.config.reset_section')}
				</Button>
			</div>

			<Tabs.Content value="bot">
				<Card>
					<CardHeader>
						<CardTitle>{t('myaaw.config.bot_title')}</CardTitle>
						<CardDescription>{t('myaaw.config.bot_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="grid gap-4 md:grid-cols-2">
						<div class="grid gap-1">
							{@render botLabel('bot-type', 'myaaw.config.bot_type', 'type')}
							<select
								id="bot-type"
								bind:value={botType}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="private">private</option>
								<option value="public">public</option>
							</select>
						</div>
						<div class="grid gap-1">
							{@render botLabel('bot-owners', 'myaaw.config.owner_ids', 'owner_ids')}
							<Input id="bot-owners" bind:value={ownerIds} placeholder="123456789, 987654321" />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-max-iterations',
								'myaaw.config.max_iterations',
								'max_iterations'
							)}
							<Input id="bot-max-iterations" type="number" bind:value={maxIterations} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-warning-iterations',
								'myaaw.config.warning_iterations',
								'warning_iterations'
							)}
							<Input id="bot-warning-iterations" type="number" bind:value={warningIterations} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-context-tokens',
								'myaaw.config.context_max_tokens',
								'context_max_tokens'
							)}
							<Input id="bot-context-tokens" type="number" bind:value={contextMaxTokens} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-tool-output',
								'myaaw.config.max_tool_output_bytes',
								'max_tool_output_bytes'
							)}
							<Input id="bot-tool-output" type="number" bind:value={maxToolOutputBytes} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-tool-timeout',
								'myaaw.config.tool_timeout_seconds',
								'tool_timeout_seconds'
							)}
							<Input id="bot-tool-timeout" type="number" bind:value={toolTimeout} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-run-timeout',
								'myaaw.config.run_timeout_seconds',
								'run_timeout_seconds'
							)}
							<Input id="bot-run-timeout" type="number" bind:value={runTimeout} />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-stream-timeout',
								'myaaw.config.stream_timeout_seconds',
								'stream_timeout_seconds'
							)}
							<Input id="bot-stream-timeout" type="number" bind:value={streamTimeout} />
						</div>
						<div class="grid gap-1">
							{@render botLabel('bot-retries', 'myaaw.config.request_retries', 'request_retries')}
							<Input id="bot-retries" type="number" bind:value={requestRetries} />
						</div>
						<div class="grid gap-1">
							{@render botLabel('bot-workers', 'myaaw.config.queue_workers', 'queue_workers')}
							<Input id="bot-workers" type="number" bind:value={queueWorkers} min="1" max="16" />
						</div>
						<div class="grid gap-1">
							{@render botLabel(
								'bot-repeat-limit',
								'myaaw.config.repeat_tool_call_limit',
								'repeat_tool_call_limit'
							)}
							<Input id="bot-repeat-limit" type="number" bind:value={repeatToolCallLimit} min="0" />
							<span class="text-xs text-muted-foreground">
								{t('myaaw.config.repeat_tool_call_limit_hint')}
							</span>
						</div>
						<div class="grid gap-1">
							{@render botLabel('bot-blocked', 'myaaw.config.blocked_tools', 'blocked_tools')}
							<Input id="bot-blocked" bind:value={blockedTools} placeholder="bash, python" />
						</div>
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={stream} class="h-4 w-4 accent-primary" />
								{t('myaaw.config.stream')}
							</label>
							{@render resetIcon('bot', 'stream')}
						</div>
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={watermark} class="h-4 w-4 accent-primary" />
								{t('myaaw.config.watermark')}
							</label>
							{@render resetIcon('bot', 'watermark')}
						</div>
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={computerUse} class="h-4 w-4 accent-primary" />
								{t('myaaw.config.computer_use')}
							</label>
							{@render resetIcon('bot', 'computer_use')}
						</div>
					</CardContent>
				</Card>
			</Tabs.Content>

			<Tabs.Content value="heartbeat">
				<Card>
					<CardHeader>
						<CardTitle>{t('myaaw.config.heartbeat_title')}</CardTitle>
						<CardDescription>{t('myaaw.config.heartbeat_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="grid gap-4 md:grid-cols-2">
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={hbActive} class="h-4 w-4 accent-primary" />
								{t('myaaw.config.active')}
							</label>
							{@render resetIcon('heartbeat', 'active')}
						</div>
						<div class="grid gap-1">
							{@render fieldLabel('hb-every', 'myaaw.config.every', 'heartbeat', 'every')}
							<Input id="hb-every" bind:value={hbEvery} placeholder="30m" />
						</div>
						<div class="grid gap-1">
							{@render fieldLabel('hb-channel', 'myaaw.config.channel', 'heartbeat', 'channel')}
							<select
								id="hb-channel"
								bind:value={hbChannel}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="">none</option>
								<option value="telegram">telegram</option>
								<option value="discord">discord</option>
							</select>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel('hb-to', 'myaaw.config.to', 'heartbeat', 'to')}
							<Input id="hb-to" bind:value={hbTo} />
						</div>
					</CardContent>
				</Card>
			</Tabs.Content>

			<Tabs.Content value="observability">
				<Card>
					<CardHeader>
						<CardTitle>{t('myaaw.config.obs_title')}</CardTitle>
						<CardDescription>{t('myaaw.config.obs_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="grid gap-4 md:grid-cols-2">
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={obsEnabled} class="h-4 w-4 accent-primary" />
								{t('myaaw.config.obs_enabled')}
							</label>
							{@render resetIcon('observability', 'enabled')}
						</div>
						<div class="flex items-center justify-between gap-2">
							<label class="flex items-center gap-2 text-sm">
								<input
									type="checkbox"
									bind:checked={obsStoreContent}
									class="h-4 w-4 accent-primary"
								/>
								{t('myaaw.config.obs_store_content')}
							</label>
							{@render resetIcon('observability', 'store_content')}
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'obs-retention',
								'myaaw.config.obs_retention',
								'observability',
								'retention_days'
							)}
							<Input id="obs-retention" type="number" bind:value={obsRetention} min="0" />
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'obs-max-chars',
								'myaaw.config.obs_max_chars',
								'observability',
								'max_content_chars'
							)}
							<Input id="obs-max-chars" type="number" bind:value={obsMaxChars} />
						</div>
						<div class="grid gap-1 md:col-span-2">
							{@render fieldLabel(
								'obs-redact',
								'myaaw.config.obs_redact',
								'observability',
								'redact_patterns'
							)}
							<textarea
								id="obs-redact"
								bind:value={obsRedact}
								rows="3"
								placeholder={'(?i)bearer\\s+[a-z0-9._~+/=-]{16,}'}
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'obs-model-prices',
								'myaaw.config.model_prices',
								'observability',
								'model_prices'
							)}
							<textarea
								id="obs-model-prices"
								bind:value={obsModelPrices}
								rows="8"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'obs-tool-prices',
								'myaaw.config.tool_prices',
								'observability',
								'tool_prices'
							)}
							<textarea
								id="obs-tool-prices"
								bind:value={obsToolPrices}
								rows="8"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
					</CardContent>
				</Card>
			</Tabs.Content>

			<Tabs.Content value="approvals">
				<Card>
					<CardHeader>
						<CardTitle>{t('myaaw.config.approvals_title')}</CardTitle>
						<CardDescription>{t('myaaw.config.approvals_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="grid gap-4 md:grid-cols-2">
						<div class="grid gap-1">
							{@render fieldLabel('ap-mode', 'myaaw.approvals.mode', 'approvals', 'mode')}
							<select
								id="ap-mode"
								bind:value={apMode}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="manual">manual</option>
								<option value="off">off</option>
							</select>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'ap-timeout',
								'myaaw.approvals.timeout',
								'approvals',
								'timeout_seconds'
							)}
							<Input id="ap-timeout" type="number" bind:value={apTimeout} />
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'ap-unattended',
								'myaaw.approvals.unattended',
								'approvals',
								'unattended'
							)}
							<select
								id="ap-unattended"
								bind:value={apUnattended}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="allow">allow</option>
								<option value="deny">deny</option>
							</select>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel(
								'ap-subagents',
								'myaaw.approvals.subagents',
								'approvals',
								'subagents'
							)}
							<select
								id="ap-subagents"
								bind:value={apSubagents}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="inherit">inherit</option>
								<option value="allow">allow</option>
								<option value="deny">deny</option>
							</select>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel('ap-allow', 'myaaw.approvals.allow', 'approvals', 'allow')}
							<textarea
								id="ap-allow"
								bind:value={apAllow}
								rows="5"
								placeholder="filesystem:read_file"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							{@render fieldLabel('ap-deny', 'myaaw.approvals.deny', 'approvals', 'deny')}
							<textarea
								id="ap-deny"
								bind:value={apDeny}
								rows="5"
								placeholder="computer:key"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
					</CardContent>
				</Card>
			</Tabs.Content>
		</Tabs.Root>

		<p class="text-xs text-muted-foreground">{t('myaaw.config.restart_hint')}</p>
	{/if}
</div>
