<script lang="ts">
	import { config as configApi } from '$lib/apis/config';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';

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
	import { RefreshCw, Save } from 'lucide-svelte';
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

<TopBarActions id="config-actions">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
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

			<Tabs.Content value="bot">
				<Card>
					<CardHeader>
						<CardTitle>{t('myaaw.config.bot_title')}</CardTitle>
						<CardDescription>{t('myaaw.config.bot_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="grid gap-4 md:grid-cols-2">
						<div class="grid gap-1">
							<Label for="bot-type">{t('myaaw.config.bot_type')}</Label>
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
							<Label for="bot-owners">{t('myaaw.config.owner_ids')}</Label>
							<Input id="bot-owners" bind:value={ownerIds} placeholder="123456789, 987654321" />
						</div>
						<div class="grid gap-1">
							<Label for="bot-max-iterations">{t('myaaw.config.max_iterations')}</Label>
							<Input id="bot-max-iterations" type="number" bind:value={maxIterations} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-warning-iterations">{t('myaaw.config.warning_iterations')}</Label>
							<Input id="bot-warning-iterations" type="number" bind:value={warningIterations} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-context-tokens">{t('myaaw.config.context_max_tokens')}</Label>
							<Input id="bot-context-tokens" type="number" bind:value={contextMaxTokens} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-tool-output">{t('myaaw.config.max_tool_output_bytes')}</Label>
							<Input id="bot-tool-output" type="number" bind:value={maxToolOutputBytes} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-tool-timeout">{t('myaaw.config.tool_timeout_seconds')}</Label>
							<Input id="bot-tool-timeout" type="number" bind:value={toolTimeout} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-run-timeout">{t('myaaw.config.run_timeout_seconds')}</Label>
							<Input id="bot-run-timeout" type="number" bind:value={runTimeout} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-stream-timeout">{t('myaaw.config.stream_timeout_seconds')}</Label>
							<Input id="bot-stream-timeout" type="number" bind:value={streamTimeout} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-retries">{t('myaaw.config.request_retries')}</Label>
							<Input id="bot-retries" type="number" bind:value={requestRetries} />
						</div>
						<div class="grid gap-1">
							<Label for="bot-workers">{t('myaaw.config.queue_workers')}</Label>
							<Input id="bot-workers" type="number" bind:value={queueWorkers} min="1" max="16" />
						</div>
						<div class="grid gap-1">
							<Label for="bot-blocked">{t('myaaw.config.blocked_tools')}</Label>
							<Input id="bot-blocked" bind:value={blockedTools} placeholder="bash, python" />
						</div>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={stream} class="h-4 w-4 accent-primary" />
							{t('myaaw.config.stream')}
						</label>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={watermark} class="h-4 w-4 accent-primary" />
							{t('myaaw.config.watermark')}
						</label>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={computerUse} class="h-4 w-4 accent-primary" />
							{t('myaaw.config.computer_use')}
						</label>
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
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={hbActive} class="h-4 w-4 accent-primary" />
							{t('myaaw.config.active')}
						</label>
						<div class="grid gap-1">
							<Label for="hb-every">{t('myaaw.config.every')}</Label>
							<Input id="hb-every" bind:value={hbEvery} placeholder="30m" />
						</div>
						<div class="grid gap-1">
							<Label for="hb-channel">{t('myaaw.config.channel')}</Label>
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
							<Label for="hb-to">{t('myaaw.config.to')}</Label>
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
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={obsEnabled} class="h-4 w-4 accent-primary" />
							{t('myaaw.config.obs_enabled')}
						</label>
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								bind:checked={obsStoreContent}
								class="h-4 w-4 accent-primary"
							/>
							{t('myaaw.config.obs_store_content')}
						</label>
						<div class="grid gap-1">
							<Label for="obs-retention">{t('myaaw.config.obs_retention')}</Label>
							<Input id="obs-retention" type="number" bind:value={obsRetention} min="0" />
						</div>
						<div class="grid gap-1">
							<Label for="obs-max-chars">{t('myaaw.config.obs_max_chars')}</Label>
							<Input id="obs-max-chars" type="number" bind:value={obsMaxChars} />
						</div>
						<div class="grid gap-1 md:col-span-2">
							<Label for="obs-redact">{t('myaaw.config.obs_redact')}</Label>
							<textarea
								id="obs-redact"
								bind:value={obsRedact}
								rows="3"
								placeholder={'(?i)bearer\\s+[a-z0-9._~+/=-]{16,}'}
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							<Label for="obs-model-prices">{t('myaaw.config.model_prices')}</Label>
							<textarea
								id="obs-model-prices"
								bind:value={obsModelPrices}
								rows="8"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							<Label for="obs-tool-prices">{t('myaaw.config.tool_prices')}</Label>
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
							<Label for="ap-mode">{t('myaaw.approvals.mode')}</Label>
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
							<Label for="ap-timeout">{t('myaaw.approvals.timeout')}</Label>
							<Input id="ap-timeout" type="number" bind:value={apTimeout} />
						</div>
						<div class="grid gap-1">
							<Label for="ap-unattended">{t('myaaw.approvals.unattended')}</Label>
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
							<Label for="ap-subagents">{t('myaaw.approvals.subagents')}</Label>
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
							<Label for="ap-allow">{t('myaaw.approvals.allow')}</Label>
							<textarea
								id="ap-allow"
								bind:value={apAllow}
								rows="5"
								placeholder="filesystem:read_file"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<div class="grid gap-1">
							<Label for="ap-deny">{t('myaaw.approvals.deny')}</Label>
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
