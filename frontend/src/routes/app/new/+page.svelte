<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { agents as agentsApi } from '$lib/apis/agents';
	import { config as configApi } from '$lib/apis/config';
	import { providers as providersApi } from '$lib/apis/providers';
	import { server as serverApi } from '$lib/apis/server';
	import { skills as skillsApi } from '$lib/apis/skills';
	import { tools as toolsApi } from '$lib/apis/tools';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { hasErrors, validate, type FieldErrors } from '$lib/utils/validation';
	import { agentSetupSchema, type AgentSetupForm } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import CapabilityPicker from '$lib/components/onboard/CapabilityPicker.svelte';
	import Section from '$lib/components/onboard/Section.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Plus, Save, Wand2, X } from 'lucide-svelte';
	import type {
		CapabilityEntry,
		CapabilitiesView,
		ModelListView,
		OnboardRequest,
		ProviderOption
	} from '$lib/types';

	const { t } = useTranslation();

	type ChannelMode = 'none' | 'telegram' | 'discord' | 'both';
	type TelegramMode = 'polling' | 'webhook';
	type BotType = 'private' | 'public';

	/** What the agent already had, so only changed sections are written. */
	interface SetupSnapshot {
		provider: string;
		skills: string;
		tools: string;
		channels: string;
		access: string;
		heartbeat: string;
	}

	const selectClass =
		'h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 disabled:opacity-60';

	let name = $state('');
	let presetId = $state('openai');
	let providerType = $state('openai');
	let providerName = $state('');
	let baseUrl = $state('');
	let apiKey = $state('');
	let model = $state('');
	let savedProvider = $state<{ name: string; keyConfigured: boolean } | null>(null);

	let discovered = $state<ModelListView | null>(null);
	let discovering = $state(false);
	let manualModel = $state(false);

	let skillSelection = $state<Record<string, boolean>>({});
	let toolSelection = $state<Record<string, boolean>>({});

	let channelMode = $state<ChannelMode>('none');
	let telegramMode = $state<TelegramMode>('polling');
	let telegramToken = $state('');
	let webhookUrl = $state('');
	let discordToken = $state('');
	let telegramConfigured = $state(false);
	let discordConfigured = $state(false);

	let botType = $state<BotType>('private');
	let ownerIds = $state('');

	let heartbeatActive = $state(false);
	let heartbeatChannel = $state<'telegram' | 'discord'>('telegram');
	let heartbeatTo = $state('');
	let heartbeatEvery = $state('30m');

	let capabilities = $state<CapabilitiesView | null>(null);
	let initial = $state<SetupSnapshot | null>(null);
	let serverRunning = $state(false);

	let errors = $state<FieldErrors>({});
	let loading = $state(true);
	let saving = $state(false);
	let loadError = $state('');

	let openCapabilities = $state(false);
	let openChannels = $state(false);
	let openAccess = $state(false);

	let agentParam = $derived(page.url.searchParams.get('agent') ?? '');
	let reconfigure = $derived(agentParam !== '');
	// Presets that share a wire type (DeepSeek is OpenAI-compatible) are offered
	// separately; the payload still sends the real type. These are also the
	// fallback when an older control plane does not describe providers at all.
	const fallbackPresets: ProviderOption[] = [
		{
			id: 'openai',
			type: 'openai',
			label: 'OpenAI (or compatible)',
			default_base_url: 'https://api.openai.com',
			default_model: 'gpt-4o'
		},
		{
			id: 'deepseek',
			type: 'openai',
			label: 'DeepSeek',
			default_base_url: 'https://api.deepseek.com',
			default_model: 'deepseek-chat'
		},
		{
			id: 'gemini',
			type: 'gemini',
			label: 'Gemini',
			default_base_url: 'https://generativelanguage.googleapis.com',
			default_model: 'models/gemini-3-flash-preview'
		}
	];
	let providerChoices = $derived.by(() => {
		const presets = (capabilities?.providers ?? []).map((option, index) => ({
			...option,
			// Older control planes omit `id`; fall back to the wire type so every
			// option stays unique and the select keeps working.
			id: option.id || option.type || `preset-${index}`
		}));
		const known = new Set(presets.map((option) => option.id));
		for (const preset of fallbackPresets) {
			if (!known.has(preset.id)) presets.push(preset);
		}
		return presets;
	});
	let activePreset = $derived(
		providerChoices.find((option) => option.id === presetId) ?? providerChoices[0]
	);
	let wantsTelegram = $derived(channelMode === 'telegram' || channelMode === 'both');
	let wantsDiscord = $derived(channelMode === 'discord' || channelMode === 'both');

	// A saved provider can be queried with its stored key, so reconfiguring a
	// model does not require retyping the secret.
	let useSavedKey = $derived(
		reconfigure &&
			apiKey.trim() === '' &&
			!!savedProvider &&
			savedProvider.keyConfigured &&
			providerName.trim() === savedProvider.name
	);
	let canDiscover = $derived(baseUrl.trim() !== '' && (apiKey.trim() !== '' || useSavedKey));

	// The heartbeat can only use channels this agent has, like the terminal's
	// heartbeat step, which lists configured channels only.
	let telegramAvailable = $derived(wantsTelegram || telegramConfigured);
	let discordAvailable = $derived(wantsDiscord || discordConfigured);
	let heartbeatOptions = $derived([
		...(telegramAvailable
			? [{ value: 'telegram' as const, label: t('myaaw.onboard.channel_telegram') }]
			: []),
		...(discordAvailable
			? [{ value: 'discord' as const, label: t('myaaw.onboard.channel_discord') }]
			: [])
	]);

	let capabilitySkills = $derived<CapabilityEntry[]>(capabilities?.skills ?? []);
	let capabilityTools = $derived<CapabilityEntry[]>(capabilities?.tools ?? []);

	let selectedSkills = $derived(
		Object.keys(skillSelection)
			.filter((key) => skillSelection[key])
			.sort()
	);
	let disabledTools = $derived(
		Object.keys(toolSelection)
			.filter((key) => !toolSelection[key])
			.sort()
	);
	let enabledToolCount = $derived(
		Object.keys(toolSelection).filter((key) => toolSelection[key]).length
	);

	let snapshot = $derived<SetupSnapshot>({
		provider: [providerType, providerName.trim(), baseUrl.trim(), model.trim(), apiKey.trim()].join(
			'|'
		),
		skills: selectedSkills.join(','),
		tools: disabledTools.join(','),
		channels: [
			channelMode,
			telegramMode,
			telegramToken.trim(),
			webhookUrl.trim(),
			discordToken.trim()
		].join('|'),
		access: [botType, ownerIds.trim()].join('|'),
		heartbeat: [heartbeatActive, heartbeatChannel, heartbeatTo.trim(), heartbeatEvery.trim()].join(
			'|'
		)
	});
	let changed = $derived({
		provider: !!initial && initial.provider !== snapshot.provider,
		skills: !!initial && initial.skills !== snapshot.skills,
		tools: !!initial && initial.tools !== snapshot.tools,
		channels: !!initial && initial.channels !== snapshot.channels,
		access: !!initial && initial.access !== snapshot.access,
		heartbeat: !!initial && initial.heartbeat !== snapshot.heartbeat
	});

	let capabilitiesSummary = $derived(
		`${t('myaaw.onboard.summary_skills', { count: selectedSkills.length })} · ${t(
			'myaaw.onboard.summary_tools',
			{ count: enabledToolCount, total: capabilityTools.length }
		)}`
	);
	let channelSummary = $derived(
		channelMode === 'none'
			? t('myaaw.onboard.summary_channels')
			: channelMode === 'both'
				? t('myaaw.onboard.channel_both')
				: channelMode === 'telegram'
					? t('myaaw.onboard.channel_telegram')
					: t('myaaw.onboard.channel_discord')
	);
	let heartbeatSummary = $derived(
		heartbeatActive
			? t('myaaw.onboard.summary_heartbeat', { every: heartbeatEvery || '30m' })
			: t('myaaw.onboard.summary_no_heartbeat')
	);
	let submitLabel = $derived(reconfigure ? t('myaaw.onboard.save') : t('myaaw.onboard.create'));

	let initialisedFor: string | null = null;

	$effect(() => {
		if (initialisedFor === agentParam) return;
		initialisedFor = agentParam;
		void initialise();
	});

	// Keep the heartbeat target on a channel this agent actually has.
	$effect(() => {
		if (heartbeatOptions.length === 0) {
			heartbeatActive = false;
			return;
		}
		if (!heartbeatOptions.some((option) => option.value === heartbeatChannel)) {
			heartbeatChannel = heartbeatOptions[0].value;
		}
	});

	async function initialise() {
		loading = true;
		loadError = '';
		errors = {};
		discovered = null;
		manualModel = false;
		try {
			try {
				capabilities = await agentsApi.capabilities();
			} catch (err) {
				loadError = `${t('myaaw.onboard.load_failed')} ${getErrorMessage(err)}`;
			}

			if (agentParam) {
				await prefill(agentParam);
			} else {
				resetForm();
			}
		} finally {
			loading = false;
		}
	}

	/** Mirrors the wizard defaults: no skills, every built-in tool enabled. */
	function resetForm() {
		name = '';
		presetId = '';
		providerName = '';
		apiKey = '';
		channelMode = 'none';
		telegramMode = 'polling';
		telegramToken = '';
		webhookUrl = '';
		discordToken = '';
		telegramConfigured = false;
		discordConfigured = false;
		botType = 'private';
		ownerIds = '';
		heartbeatActive = false;
		heartbeatChannel = 'telegram';
		heartbeatTo = '';
		heartbeatEvery = '30m';

		const skills: Record<string, boolean> = {};
		for (const entry of capabilitySkills) skills[entry.name] = false;
		skillSelection = skills;

		const tools: Record<string, boolean> = {};
		for (const entry of capabilityTools) tools[entry.name] = true;
		toolSelection = tools;

		applyPreset('openai', true);
		initial = null;
	}

	/** Applies a provider preset: wire type, endpoint and default model. */
	function applyPreset(id: string, resetName = false) {
		const previous = providerChoices.find((option) => option.id === presetId);
		const next = providerChoices.find((option) => option.id === id) ?? providerChoices[0];
		presetId = next.id;
		providerType = next.type;
		baseUrl = next.default_base_url ?? '';
		model = next.default_model ?? '';
		discovered = null;
		manualModel = false;
		// Only replace the name when the user has not typed one of their own.
		const current = providerName.trim();
		if (resetName || current === '' || current === previous?.id) {
			providerName = next.id;
		}
	}

	async function prefill(target: string) {
		try {
			const [config, skillList, toolList, info] = await Promise.all([
				configApi.get(target),
				skillsApi.list(target),
				toolsApi.list(target),
				agentsApi.get(target)
			]);

			name = target;
			serverRunning = info.running;

			const provider =
				config.providers.find((item) => item.name === config.default_provider) ??
				config.providers[0];
			providerType = provider?.type === 'gemini' ? 'gemini' : 'openai';
			providerName = provider?.name ?? '';
			baseUrl = provider?.base_url ?? '';
			model = provider?.default_model ?? '';
			apiKey = '';
			discovered = null;
			manualModel = false;
			savedProvider = provider
				? { name: provider.name, keyConfigured: provider.api_key.configured }
				: null;

			// Match the stored endpoint to a preset so the select shows the right
			// choice; a custom endpoint falls back to the generic type preset.
			const matchedPreset =
				providerChoices.find(
					(option) => option.type === providerType && option.default_base_url === baseUrl
				) ??
				providerChoices.find((option) => option.id === providerType) ??
				providerChoices[0];
			presetId = matchedPreset.id;

			const skills: Record<string, boolean> = {};
			for (const entry of capabilitySkills) skills[entry.name] = false;
			for (const skill of skillList.skills) skills[skill.name] = skill.enabled;
			skillSelection = skills;

			const tools: Record<string, boolean> = {};
			for (const entry of capabilityTools) tools[entry.name] = true;
			for (const tool of toolList.builtin) tools[tool.name] = tool.enabled;
			toolSelection = tools;

			const telegram = config.channels.find((channel) => channel.name === 'telegram');
			const discord = config.channels.find((channel) => channel.name === 'discord');
			telegramConfigured = telegram?.token.configured ?? false;
			discordConfigured = discord?.token.configured ?? false;
			channelMode =
				telegram?.active && discord?.active
					? 'both'
					: telegram?.active
						? 'telegram'
						: discord?.active
							? 'discord'
							: 'none';
			telegramMode = telegram?.mode === 'webhook' ? 'webhook' : 'polling';
			telegramToken = '';
			webhookUrl = '';
			discordToken = '';

			botType = config.bot.type === 'public' ? 'public' : 'private';
			ownerIds = (config.bot.owner_ids ?? []).join(', ');

			heartbeatActive = config.heartbeat.active;
			heartbeatChannel = config.heartbeat.channel === 'discord' ? 'discord' : 'telegram';
			heartbeatTo = config.heartbeat.to ?? '';
			heartbeatEvery = config.heartbeat.every || '30m';

			initial = snapshot;
		} catch (err) {
			toastStore.error(t('myaaw.onboard.prefill_failed', { error: getErrorMessage(err) }));
		}
	}

	async function discover() {
		if (!canDiscover) return;
		discovering = true;
		try {
			const view =
				useSavedKey && savedProvider
					? await providersApi.models(agentParam, savedProvider.name)
					: await providersApi.modelsForCredentials({
							type: providerType,
							base_url: baseUrl.trim() || undefined,
							api_key: apiKey.trim() || undefined
						});
			discovered = view;
			manualModel = false;
			if (view.models.length > 0 && !view.models.includes(model)) {
				model = view.models[0];
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			discovering = false;
		}
	}

	/** The discovered list is tied to the credentials that produced it. */
	function clearDiscovered() {
		discovered = null;
		manualModel = false;
	}

	function toggleManualModel() {
		manualModel = !manualModel;
		if (!manualModel && discovered && discovered.models.length > 0) {
			if (!discovered.models.includes(model)) model = discovered.models[0];
		}
	}

	function buildPayload(data: AgentSetupForm): OnboardRequest {
		const payload: OnboardRequest = {};

		if (!initial || changed.provider) {
			payload.provider = {
				type: data.provider_type,
				name: data.provider_name || undefined,
				base_url: data.base_url || undefined,
				api_key: data.api_key?.trim() || undefined,
				model: data.model
			};
		}
		if (!initial || changed.skills) {
			payload.skills = selectedSkills;
		}
		if (!initial || changed.tools) {
			payload.disable_tools = disabledTools;
		}
		// A brand-new agent with no channels keeps the template defaults.
		if ((initial || channelMode !== 'none') && (!initial || changed.channels)) {
			payload.channels = {
				telegram_token: wantsTelegram ? telegramToken.trim() || undefined : undefined,
				telegram_mode: wantsTelegram ? telegramMode : undefined,
				telegram_webhook_url:
					wantsTelegram && telegramMode === 'webhook' ? webhookUrl.trim() || undefined : undefined,
				discord_token: wantsDiscord ? discordToken.trim() || undefined : undefined,
				disable_telegram: !wantsTelegram,
				disable_discord: !wantsDiscord
			};
		}
		// A brand-new agent with the heartbeat off keeps the template defaults.
		if ((initial || heartbeatActive) && (!initial || changed.heartbeat)) {
			payload.heartbeat = heartbeatActive
				? {
						active: true,
						channel: heartbeatChannel,
						to: heartbeatTo.trim(),
						every: heartbeatEvery.trim() || '30m'
					}
				: { active: false };
		}
		if (!initial || changed.access) {
			const owners = ownerIds
				.split(',')
				.map((value) => value.trim())
				.filter(Boolean);
			payload.bot = { type: botType, owner_ids: owners.length > 0 ? owners : undefined };
		}
		return payload;
	}

	async function submit() {
		const target = (agentParam || name).trim();
		const result = validate(agentSetupSchema, {
			name: target,
			provider_type: providerType,
			provider_name: providerName,
			base_url: baseUrl,
			api_key: apiKey,
			model,
			telegram_token: telegramToken,
			webhook_url: webhookUrl,
			heartbeat_to: heartbeatTo,
			owner_ids: ownerIds,
			heartbeat_every: heartbeatEvery
		});
		if (!result.ok) {
			errors = result.errors;
			return;
		}

		const fieldErrors: FieldErrors = {};
		if (!initial && apiKey.trim() === '') fieldErrors.api_key = 'api_key_required';
		if (wantsTelegram && telegramToken.trim() === '' && !telegramConfigured) {
			fieldErrors.telegram_token = 'required';
		}
		if (wantsDiscord && discordToken.trim() === '' && !discordConfigured) {
			fieldErrors.discord_token = 'required';
		}
		if (wantsTelegram && telegramMode === 'webhook' && webhookUrl.trim() === '') {
			fieldErrors.webhook_url = 'required';
		}
		if (heartbeatActive && heartbeatTo.trim() === '') fieldErrors.heartbeat_to = 'required';
		if (hasErrors(fieldErrors)) {
			errors = fieldErrors;
			// Surface the section that owns the failing field.
			if (fieldErrors.telegram_token || fieldErrors.discord_token || fieldErrors.webhook_url) {
				openChannels = true;
			}
			if (fieldErrors.heartbeat_to) openAccess = true;
			return;
		}

		errors = {};
		saving = true;
		try {
			const response = await agentsApi.onboard(target, buildPayload(result.data));
			toastStore.success(
				reconfigure
					? t('myaaw.onboard.updated', { name: target })
					: t('myaaw.onboard.created', { name: target })
			);
			for (const warning of response.warnings ?? []) {
				toastStore.warning(warning);
			}

			await agentSelection.load(true);
			agentSelection.select(target);

			if (reconfigure && serverRunning) {
				if (window.confirm(t('myaaw.onboard.restart_confirm', { name: target }))) {
					await serverApi.restart(target);
					toastStore.success(t('myaaw.onboard.restart_done'));
				}
			}
			void goto('/app/dashboard');
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}
</script>

<TopBarActions id="onboard-actions">
	{#snippet children()}
		<TopBarButton label={t('common.cancel')} icon={X} onclick={() => goto('/app')} />
		<TopBarButton
			label={submitLabel}
			icon={reconfigure ? Save : Plus}
			onclick={() => void submit()}
			variant="default"
			loading={saving}
			disabled={loading}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto w-full max-w-3xl space-y-4 p-4 lg:p-6">
	{#if loadError}
		<div
			class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{loadError}
		</div>
	{/if}

	{#if loading}
		{#each Array.from({ length: 3 }) as _, index (index)}
			<Skeleton class="h-32 w-full" />
		{/each}
	{:else}
		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.onboard.identity')}</CardTitle>
				<CardDescription>{t('myaaw.onboard.identity_desc')}</CardDescription>
			</CardHeader>
			<CardContent class="space-y-4">
				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid gap-1.5">
						<Label for="onboard-name">{t('myaaw.onboard.agent_name')}</Label>
						<Input
							id="onboard-name"
							bind:value={name}
							placeholder="research"
							disabled={reconfigure}
						/>
						<FieldError code={errors.name} />
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-provider-type">{t('myaaw.onboard.provider_type')}</Label>
						<select
							id="onboard-provider-type"
							class={selectClass}
							value={presetId}
							onchange={(event) => applyPreset(event.currentTarget.value)}
						>
							{#each providerChoices as option (option.id)}
								<option value={option.id}>{option.label}</option>
							{/each}
						</select>
					</div>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid gap-1.5">
						<Label for="onboard-provider-name">{t('myaaw.onboard.provider_name')}</Label>
						<Input
							id="onboard-provider-name"
							bind:value={providerName}
							placeholder={activePreset?.id ?? providerType}
						/>
						<p class="text-[11px] text-muted-foreground">
							{t('myaaw.onboard.provider_name_hint')}
						</p>
						<FieldError code={errors.provider_name} />
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-base-url">{t('myaaw.providers.base_url')}</Label>
						<Input
							id="onboard-base-url"
							bind:value={baseUrl}
							placeholder={activePreset?.default_base_url ?? 'https://api.openai.com'}
							oninput={clearDiscovered}
						/>
						<FieldError code={errors.base_url} />
					</div>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid gap-1.5">
						<Label for="onboard-api-key">{t('myaaw.providers.api_key')}</Label>
						<Input
							id="onboard-api-key"
							type="password"
							bind:value={apiKey}
							placeholder={reconfigure ? t('myaaw.onboard.api_key_saved') : 'sk-...'}
							oninput={clearDiscovered}
						/>
						<FieldError code={errors.api_key} />
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-model">{t('myaaw.onboard.model')}</Label>
						<div class="flex gap-2">
							{#if discovered && discovered.models.length > 0 && !manualModel}
								<select id="onboard-model" class={selectClass} bind:value={model}>
									{#each discovered.models as option}
										<option value={option}>{option}</option>
									{/each}
								</select>
							{:else}
								<Input
									id="onboard-model"
									bind:value={model}
									placeholder={activePreset?.default_model ?? 'gpt-4o'}
								/>
							{/if}
							<Button
								variant="outline"
								size="sm"
								class="shrink-0"
								loading={discovering}
								disabled={!canDiscover}
								onclick={() => void discover()}
							>
								<Wand2 class="mr-2 h-4 w-4" />
								{t('myaaw.onboard.model_discover')}
							</Button>
						</div>
						<div class="flex flex-wrap items-center gap-x-2 text-[11px] text-muted-foreground">
							{#if !canDiscover}
								<span>{t('myaaw.onboard.model_discover_hint')}</span>
							{:else if discovered}
								<span>
									{discovered.source === 'live'
										? t('myaaw.onboard.model_live')
										: t('myaaw.onboard.model_fallback')}
								</span>
								{#if discovered.warning}
									<span class="truncate">{discovered.warning}</span>
								{/if}
								{#if discovered.models.length > 0}
									<button type="button" class="underline" onclick={toggleManualModel}>
										{manualModel ? t('myaaw.onboard.model_list') : t('myaaw.onboard.model_manual')}
									</button>
								{/if}
							{/if}
						</div>
						<FieldError code={errors.model} />
					</div>
				</div>
			</CardContent>
		</Card>

		<Section
			title={t('myaaw.onboard.capabilities')}
			summary={capabilitiesSummary}
			description={t('myaaw.onboard.capabilities_desc')}
			bind:open={openCapabilities}
		>
			<div class="space-y-2">
				<div class="flex flex-wrap items-baseline justify-between gap-x-3">
					<Label>{t('myaaw.onboard.skills')}</Label>
					<span class="text-[11px] text-muted-foreground">
						{t('myaaw.onboard.skills_desc')}
					</span>
				</div>
				<CapabilityPicker items={capabilitySkills} selection={skillSelection} emptyLabel="—" />
			</div>
			<Separator />
			<div class="space-y-2">
				<div class="flex flex-wrap items-baseline justify-between gap-x-3">
					<Label>{t('myaaw.onboard.tools')}</Label>
					<span class="text-[11px] text-muted-foreground">
						{t('myaaw.onboard.tools_desc')}
					</span>
				</div>
				<CapabilityPicker items={capabilityTools} selection={toolSelection} emptyLabel="—" />
			</div>
		</Section>

		<Section
			title={t('myaaw.onboard.channels')}
			summary={channelSummary}
			description={t('myaaw.onboard.channels_desc')}
			bind:open={openChannels}
		>
			<div class="grid gap-1.5 sm:max-w-xs">
				<Label for="onboard-channel-mode">{t('myaaw.onboard.channels')}</Label>
				<select
					id="onboard-channel-mode"
					class={selectClass}
					value={channelMode}
					onchange={(event) => (channelMode = event.currentTarget.value as ChannelMode)}
				>
					<option value="none">{t('myaaw.onboard.channel_none')}</option>
					<option value="telegram">{t('myaaw.onboard.channel_telegram')}</option>
					<option value="discord">{t('myaaw.onboard.channel_discord')}</option>
					<option value="both">{t('myaaw.onboard.channel_both')}</option>
				</select>
			</div>

			{#if wantsTelegram}
				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid gap-1.5">
						<Label for="onboard-telegram-mode">{t('myaaw.onboard.telegram_mode')}</Label>
						<select
							id="onboard-telegram-mode"
							class={selectClass}
							value={telegramMode}
							onchange={(event) => (telegramMode = event.currentTarget.value as TelegramMode)}
						>
							<option value="polling">{t('myaaw.onboard.polling')}</option>
							<option value="webhook">{t('myaaw.onboard.webhook')}</option>
						</select>
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-telegram-token">{t('myaaw.onboard.telegram_token')}</Label>
						<Input
							id="onboard-telegram-token"
							type="password"
							bind:value={telegramToken}
							placeholder={telegramConfigured ? t('myaaw.onboard.token_saved') : '123456:ABC-DEF'}
						/>
						<FieldError code={errors.telegram_token} />
					</div>
				</div>
				{#if telegramMode === 'webhook'}
					<div class="grid gap-1.5">
						<Label for="onboard-webhook-url">{t('myaaw.onboard.webhook_url')}</Label>
						<Input
							id="onboard-webhook-url"
							bind:value={webhookUrl}
							placeholder="https://agent.example.com"
						/>
						<p class="text-[11px] text-muted-foreground">
							{t('myaaw.onboard.webhook_url_hint')}
						</p>
						<FieldError code={errors.webhook_url} />
					</div>
				{/if}
			{/if}

			{#if wantsDiscord}
				<div class="grid gap-1.5 sm:max-w-md">
					<Label for="onboard-discord-token">{t('myaaw.onboard.discord_token')}</Label>
					<Input
						id="onboard-discord-token"
						type="password"
						bind:value={discordToken}
						placeholder={discordConfigured ? t('myaaw.onboard.token_saved') : 'MTIz...'}
					/>
					<FieldError code={errors.discord_token} />
				</div>
			{/if}
		</Section>

		<Section
			title={t('myaaw.onboard.access')}
			summary={heartbeatSummary}
			description={t('myaaw.onboard.access_desc')}
			bind:open={openAccess}
		>
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="grid gap-1.5">
					<Label for="onboard-bot-type">{t('myaaw.onboard.bot_type')}</Label>
					<select
						id="onboard-bot-type"
						class={selectClass}
						value={botType}
						onchange={(event) => (botType = event.currentTarget.value as BotType)}
					>
						<option value="private">{t('myaaw.onboard.bot_private')}</option>
						<option value="public">{t('myaaw.onboard.bot_public')}</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="onboard-owner-ids">{t('myaaw.onboard.owner_ids')}</Label>
					<Input id="onboard-owner-ids" bind:value={ownerIds} placeholder="123456789, 987654321" />
					<p class="text-[11px] text-muted-foreground">
						{t('myaaw.onboard.owner_ids_hint')}
					</p>
					<FieldError code={errors.owner_ids} />
				</div>
			</div>
			<Separator />
			<label
				class={[
					'flex items-start gap-2.5',
					heartbeatOptions.length === 0 ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
				]}
			>
				<Checkbox
					class="mt-0.5"
					bind:checked={heartbeatActive}
					disabled={heartbeatOptions.length === 0}
				/>
				<span class="min-w-0">
					<span class="block text-sm font-medium">
						{t('myaaw.onboard.heartbeat_enable')}
					</span>
					<span class="block text-[11px] text-muted-foreground">
						{heartbeatOptions.length === 0
							? t('myaaw.onboard.heartbeat_needs_channel')
							: t('myaaw.onboard.heartbeat_hint')}
					</span>
				</span>
			</label>
			{#if heartbeatActive}
				<div class="grid gap-4 sm:grid-cols-3">
					<div class="grid gap-1.5">
						<Label for="onboard-heartbeat-channel">
							{t('myaaw.onboard.heartbeat_channel')}
						</Label>
						<select
							id="onboard-heartbeat-channel"
							class={selectClass}
							value={heartbeatChannel}
							onchange={(event) =>
								(heartbeatChannel = event.currentTarget.value as 'telegram' | 'discord')}
						>
							{#each heartbeatOptions as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
						<p class="text-[11px] text-muted-foreground">
							{t('myaaw.onboard.heartbeat_channel_hint')}
						</p>
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-heartbeat-to">{t('myaaw.onboard.heartbeat_to')}</Label>
						<Input id="onboard-heartbeat-to" bind:value={heartbeatTo} placeholder="123456789" />
						<FieldError code={errors.heartbeat_to} />
					</div>
					<div class="grid gap-1.5">
						<Label for="onboard-heartbeat-every">{t('myaaw.onboard.heartbeat_every')}</Label>
						<Input id="onboard-heartbeat-every" bind:value={heartbeatEvery} placeholder="30m" />
						<FieldError code={errors.heartbeat_every} />
					</div>
				</div>
			{/if}
		</Section>

		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.onboard.review')}</CardTitle>
				<CardDescription>
					{reconfigure ? t('myaaw.onboard.review_desc_save') : t('myaaw.onboard.review_desc')}
				</CardDescription>
			</CardHeader>
			<CardContent class="space-y-3 text-sm">
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<p class="font-medium">{t('myaaw.onboard.identity')}</p>
						<p class="truncate text-xs text-muted-foreground">
							{name || '—'} ·
							{t('myaaw.onboard.summary_provider', {
								type: providerType,
								model: model || '—'
							})}
						</p>
					</div>
					{#if reconfigure}
						<Badge variant="outline" class="shrink-0">
							{changed.provider ? t('myaaw.onboard.changing') : t('myaaw.onboard.unchanged')}
						</Badge>
					{/if}
				</div>
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<p class="font-medium">{t('myaaw.onboard.capabilities')}</p>
						<p class="truncate text-xs text-muted-foreground">{capabilitiesSummary}</p>
					</div>
					{#if reconfigure}
						<Badge variant="outline" class="shrink-0">
							{changed.skills || changed.tools
								? t('myaaw.onboard.changing')
								: t('myaaw.onboard.unchanged')}
						</Badge>
					{/if}
				</div>
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<p class="font-medium">{t('myaaw.onboard.channels')}</p>
						<p class="truncate text-xs text-muted-foreground">{channelSummary}</p>
					</div>
					{#if reconfigure}
						<Badge variant="outline" class="shrink-0">
							{changed.channels ? t('myaaw.onboard.changing') : t('myaaw.onboard.unchanged')}
						</Badge>
					{/if}
				</div>
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<p class="font-medium">{t('myaaw.onboard.access')}</p>
						<p class="truncate text-xs text-muted-foreground">
							{botType} · {heartbeatSummary}
						</p>
					</div>
					{#if reconfigure}
						<Badge variant="outline" class="shrink-0">
							{changed.access || changed.heartbeat
								? t('myaaw.onboard.changing')
								: t('myaaw.onboard.unchanged')}
						</Badge>
					{/if}
				</div>
			</CardContent>
		</Card>
	{/if}
</div>
