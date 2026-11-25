<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { agentsStore } from '$lib/stores/agents.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { providersStore } from '$lib/stores/providers.svelte';
	import { serverStore } from '$lib/stores/server.svelte';
	import { skillsStore } from '$lib/stores/skills.svelte';
	import { toolsStore } from '$lib/stores/tools.svelte';
	import { agentSelection, toastStore, topBarTitle } from '$lib/stores';
	import { BLOUB_DEFAULT_COLOR, colorOf, type BloubColor } from '$lib/bloub';
	import { getErrorMessage } from '$lib/utils/api';
	import { hasErrors, validate, type FieldErrors } from '$lib/utils/validation';
	import { agentSetupSchema, type AgentSetupForm } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import CapabilityPicker from '$lib/components/onboard/CapabilityPicker.svelte';
	import AgentColorPicker from '$lib/components/onboard/AgentColorPicker.svelte';
	import Section from '$lib/components/onboard/Section.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import ChatGPTConnection from '$lib/components/common/ChatGPTConnection.svelte';
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
	import {
		invalidateRuntimeOptions,
		loadRuntimeOptions as loadAgentRuntimeOptions,
		probeRuntimeOptions as probeRuntimeOptionsApi
	} from '$lib/stores/runtimeOptions';
	import type {
		CapabilityEntry,
		CapabilitiesView,
		DetectedRuntime,
		ModelListView,
		OnboardRequest,
		ProviderOption,
		ProviderConnection,
		RuntimeOptionItem,
		RuntimeOptions
	} from '$lib/types';

	const { t } = useTranslation();

	type ChannelMode = 'none' | 'telegram' | 'discord' | 'both';
	type TelegramMode = 'polling' | 'webhook';
	type BotType = 'private' | 'public';

	/** What the agent already had, so only changed sections are written. */
	interface SetupSnapshot {
		runtime: string;
		provider: string;
		skills: string;
		tools: string;
		channels: string;
		access: string;
		heartbeat: string;
		color: string;
	}

	const selectClass =
		'h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 disabled:opacity-60';

	let name = $state('');
	let runtimes = $state<DetectedRuntime[]>([]);
	let runtimePreset = $state('myaaw');
	let runtimeCommand = $state('');
	let runtimeArgs = $state('');
	let runtimeModel = $state('');
	let runtimeThinking = $state('');
	let runtimeSessionMode = $state('');
	// Live options discovered from the selected CLI (models, thinking levels,
	// native modes, approval routing); nothing here is hardcoded.
	let runtimeOptions = $state<RuntimeOptions | null>(null);
	let loadingRuntimeOptions = $state(false);
	let runtimeProbeToken = 0;
	let runtimeAuthSource = $state<'cli_login' | 'api_key'>('cli_login');
	let runtimeApiKeyEnv = $state('');
	let runtimeApiKey = $state('');
	let runtimeApiKeySaved = $state(false);
	let runtimePermissionMode = $state<'bridge' | 'auto' | 'read_only'>('bridge');
	let runtimeMcpBridge = $state(true);
	let isExternalRuntime = $derived(runtimePreset !== 'myaaw');
	let activeRuntimePreset = $derived(runtimes.find((entry) => entry.id === runtimePreset) ?? null);

	// Saved CLI identity on reconfigure so unchanged agents probe via
	// GET /agents/:name/runtime/options (preserving masked API keys & workspace).
	let savedRuntimeProbeKey = $state('');

	const parseRuntimeArgs = (raw: string) =>
		raw
			.split(',')
			.map((v) => v.trim())
			.filter(Boolean);

	function tOr(key: string, fallback: string): string {
		const translated = t(key);
		return translated === key ? fallback : translated;
	}

	function runtimePresetLabel(id: string, fallback: string): string {
		if (id === 'myaaw') return t('myaaw.onboard.runtime_builtin');
		if (id === 'custom_acp') return t('myaaw.onboard.runtime_custom_acp');
		return fallback;
	}

	function runtimePresetDescription(preset: DetectedRuntime | null): string {
		if (!preset) return '';
		return tOr(`myaaw.onboard.runtime_preset_${preset.id}_desc`, preset.description ?? '');
	}

	function runtimePresetToSNotice(preset: DetectedRuntime | null): string {
		if (!preset?.tos_notice) return '';
		return tOr(`myaaw.onboard.runtime_preset_${preset.id}_tos`, preset.tos_notice);
	}

	function runtimeApprovalLabel(item: RuntimeOptionItem): string {
		return tOr(`myaaw.chat.approval_runtime_${item.value}`, item.label);
	}

	function runtimeApprovalDescription(item: RuntimeOptionItem | undefined): string {
		if (!item?.description) return '';
		return tOr(`myaaw.chat.approval_runtime_${item.value}_desc`, item.description);
	}

	function formatThinkingLabel(level: string): string {
		const normalized = level.trim().toLowerCase();
		if (normalized === 'off' || normalized === 'none') return t('myaaw.chat.thinking_off');
		if (normalized === 'xhigh') return 'XHigh';
		return level
			.trim()
			.replace(/[_-]+/g, ' ')
			.replace(/\b\w/g, (ch) => ch.toUpperCase());
	}

	async function probeRuntimeOptions(refresh: boolean) {
		const token = ++runtimeProbeToken;
		loadingRuntimeOptions = true;
		try {
			const args = parseRuntimeArgs(runtimeArgs);
			const currentProbeKey = [
				runtimePreset,
				runtimeCommand.trim(),
				args.join(','),
				runtimeAuthSource,
				runtimeApiKeyEnv.trim(),
				runtimeApiKey.trim()
			].join('|');
			const opts =
				reconfigure && agentParam && savedRuntimeProbeKey === currentProbeKey
					? await loadAgentRuntimeOptions(agentParam, refresh)
					: await probeRuntimeOptionsApi(
							{
								preset: runtimePreset,
								type: activeRuntimePreset?.type || undefined,
								command: runtimeCommand.trim() || undefined,
								args: args.length > 0 ? args : undefined,
								model: runtimeModel.trim() || undefined,
								auth_source: runtimeAuthSource,
								api_key_env: runtimeApiKeyEnv.trim() || undefined,
								api_key: runtimeApiKey.trim() || undefined
							},
							refresh
						);
			if (token !== runtimeProbeToken) return;
			runtimeOptions = opts;
		} catch (err) {
			if (token !== runtimeProbeToken) return;
			runtimeOptions = null;
			if (refresh) toastStore.error(getErrorMessage(err));
		} finally {
			if (token === runtimeProbeToken) loadingRuntimeOptions = false;
		}
	}

	// Re-probe the CLI whenever the engine, binary, or arguments change.
	$effect(() => {
		const external = isExternalRuntime;
		const command = runtimeCommand.trim();
		void runtimePreset;
		void runtimeArgs;
		if (!external || !command) {
			runtimeProbeToken++;
			runtimeOptions = null;
			loadingRuntimeOptions = false;
			return;
		}
		const timer = setTimeout(() => void probeRuntimeOptions(false), 450);
		return () => clearTimeout(timer);
	});

	let runtimeModelGroups = $derived.by(() => {
		const grouped = new Map<string, RuntimeOptionItem[]>();
		for (const item of runtimeOptions?.models.options ?? []) {
			const key = item.group ?? '';
			const bucket = grouped.get(key);
			if (bucket) bucket.push(item);
			else grouped.set(key, [item]);
		}
		return Array.from(grouped.entries()).map(([group, items]) => ({ group, items }));
	});
	let runtimeModelKnown = $derived(
		(runtimeOptions?.models.options ?? []).some((item) => item.value === runtimeModel)
	);
	let runtimeThinkingOptions = $derived.by((): RuntimeOptionItem[] => {
		if (!runtimeOptions) return [];
		const models = runtimeOptions.models.options ?? [];
		if (models.some((m) => (m.thinking_levels?.length ?? 0) > 0)) {
			const selected = runtimeModel || runtimeOptions.models.current || '';
			const match = models.find((m) => m.value === selected);
			if (match) {
				return (match.thinking_levels ?? []).map((lvl) => ({
					value: lvl,
					label: formatThinkingLabel(lvl)
				}));
			}
		}
		return runtimeOptions.thinking.options ?? [];
	});
	let runtimeApprovalOptions = $derived(runtimeOptions?.approval_modes.options ?? []);
	let runtimeSessionModeOptions = $derived(runtimeOptions?.session_modes.options ?? []);

	// Clear stale thinking level if the newly selected model does not support it.
	$effect(() => {
		if (!runtimeThinking || !runtimeOptions) return;
		const opts = runtimeThinkingOptions;
		if (opts.length === 0 || !opts.some((item) => item.value === runtimeThinking)) {
			runtimeThinking = '';
		}
	});

	let presetId = $state('openai');
	let providerType = $state('openai');
	let providerName = $state('');
	let baseUrl = $state('');
	let apiKey = $state('');
	let model = $state('');
	let savedProvider = $state<{ name: string; keyConfigured: boolean } | null>(null);
	let credentialId = $state('');
	let connection = $state<ProviderConnection | undefined>();
	let connecting = $state(false);
	let connectionVersion = $state(0);
	let isChatGPT = $derived(providerType === 'openai_chatgpt');
	let connected = $derived(
		!!credentialId &&
			!!connection?.configured &&
			['connected', 'refresh_required'].includes(connection.status)
	);

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

	/** Palette key for this agent's face. Legacy agents resolve to ink. */
	let agentColor = $state<BloubColor>(BLOUB_DEFAULT_COLOR);

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
	let resolvedWorkspacePath = $derived(
		`~/.myaaw/agents/${(agentParam || name).trim() || '<name>'}/home/workspace`
	);

	// Editing an existing agent must not introduce itself as a new one. The shell
	// reads this override on every page, and the cleanup drops it on the way out.
	$effect(() => {
		if (!reconfigure) {
			topBarTitle.clear();
			return;
		}
		topBarTitle.set(
			t('myaaw.onboard.reconfigure_title', { name: name || agentParam }),
			t('myaaw.onboard.reconfigure_subtitle')
		);
		return () => topBarTitle.clear();
	});
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
			id: 'openai_chatgpt',
			type: 'openai_chatgpt',
			label: 'ChatGPT Subscription'
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
			label: option.type === 'openai_chatgpt' ? 'ChatGPT Subscription' : option.label,
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
	let canDiscover = $derived(
		!connecting &&
			(isChatGPT ? connected : baseUrl.trim() !== '' && (apiKey.trim() !== '' || useSavedKey))
	);

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
		runtime: [
			runtimePreset,
			runtimeCommand.trim(),
			runtimeArgs.trim(),
			runtimeModel.trim(),
			runtimeThinking,
			runtimeSessionMode,
			runtimeAuthSource,
			runtimeApiKeyEnv.trim(),
			runtimeApiKey.trim(),
			runtimePermissionMode,
			runtimeMcpBridge
		].join('|'),
		provider: [
			providerType,
			providerName.trim(),
			baseUrl.trim(),
			model.trim(),
			apiKey.trim(),
			credentialId
		].join('|'),
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
		),
		color: agentColor
	});
	let changed = $derived({
		runtime: !!initial && initial.runtime !== snapshot.runtime,
		provider: !!initial && initial.provider !== snapshot.provider,
		skills: !!initial && initial.skills !== snapshot.skills,
		tools: !!initial && initial.tools !== snapshot.tools,
		channels: !!initial && initial.channels !== snapshot.channels,
		access: !!initial && initial.access !== snapshot.access,
		heartbeat: !!initial && initial.heartbeat !== snapshot.heartbeat,
		color: !!initial && initial.color !== snapshot.color
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

	function clearConnection() {
		connectionVersion += 1;
		credentialId = '';
		connection = undefined;
		connecting = false;
		clearDiscovered();
	}

	async function initialise() {
		const target = agentParam;
		clearConnection();
		loading = true;
		loadError = '';
		errors = {};
		discovered = null;
		manualModel = false;
		try {
			try {
				const [capsRes, runtimesRes] = await Promise.all([
					agentsStore.fetchCapabilities(),
					agentsStore.fetchRuntimes().catch(() => ({ runtimes: [] }))
				]);
				capabilities = capsRes;
				runtimes = runtimesRes.runtimes ?? [];
			} catch (err) {
				loadError = `${t('myaaw.onboard.load_failed')} ${getErrorMessage(err)}`;
			}

			if (target !== agentParam) return;
			if (target) {
				await prefill(target);
			} else {
				resetForm();
			}
		} finally {
			loading = false;
		}
	}

	function applyRuntimePreset(id: string) {
		runtimePreset = id;
		// Options belong to the previous CLI; clear choices so a stale model or
		// mode is never sent to a different engine.
		runtimeModel = '';
		runtimeThinking = '';
		runtimeSessionMode = '';
		runtimeOptions = null;
		const preset = runtimes.find((entry) => entry.id === id);
		if (!preset || preset.id === 'myaaw') {
			runtimeCommand = '';
			runtimeArgs = '';
			runtimeApiKeyEnv = '';
			runtimeAuthSource = 'cli_login';
			return;
		}
		runtimeCommand = preset.default_command ?? '';
		runtimeArgs = (preset.default_args ?? []).join(', ');
		runtimeApiKeyEnv = preset.default_api_key_env ?? '';
		runtimeAuthSource = preset.default_auth === 'api_key' ? 'api_key' : 'cli_login';
		if (runtimeAuthSource === 'cli_login') {
			botType = 'private';
		}
	}

	/** Mirrors the wizard defaults: no skills, every built-in tool enabled. */
	function resetForm() {
		name = '';
		runtimePreset = 'myaaw';
		runtimeCommand = '';
		runtimeArgs = '';
		runtimeModel = '';
		runtimeThinking = '';
		runtimeSessionMode = '';
		runtimeAuthSource = 'cli_login';
		runtimeApiKeyEnv = '';
		runtimeApiKey = '';
		runtimeApiKeySaved = false;
		runtimePermissionMode = 'bridge';
		runtimeMcpBridge = true;
		savedRuntimeProbeKey = '';
		credentialId = '';
		connection = undefined;
		connecting = false;
		connectionVersion += 1;
		savedProvider = null;
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
		agentColor = BLOUB_DEFAULT_COLOR;

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
		connectionVersion += 1;
		credentialId = '';
		connection = undefined;
		connecting = false;
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
				configStore.fetchConfig(target),
				skillsStore.fetchList(target),
				toolsStore.fetchView(target),
				agentsStore.get(target)
			]);

			if (target !== agentParam) return;
			name = target;
			serverRunning = info.running;
			agentColor = colorOf(info.agent_color);

			if (config.runtime && config.runtime.type && config.runtime.type !== 'myaaw') {
				runtimePreset = config.runtime.preset || config.runtime.type;
				runtimeCommand = config.runtime.command ?? '';
				runtimeArgs = (config.runtime.args ?? []).join(', ');
				runtimeModel = config.runtime.model ?? '';
				runtimeThinking = config.runtime.thinking_level ?? '';
				runtimeSessionMode = config.runtime.session_mode ?? '';
				runtimeAuthSource = config.runtime.auth_source === 'api_key' ? 'api_key' : 'cli_login';
				runtimeApiKeyEnv = config.runtime.api_key_env ?? '';
				runtimeApiKey = '';
				runtimeApiKeySaved = config.runtime.api_key?.configured ?? false;
				runtimePermissionMode =
					config.runtime.permission_mode === 'auto'
						? 'auto'
						: config.runtime.permission_mode === 'read_only'
							? 'read_only'
							: 'bridge';
				runtimeMcpBridge = config.runtime.mcp_bridge ?? true;
				savedRuntimeProbeKey = [
					runtimePreset,
					runtimeCommand.trim(),
					parseRuntimeArgs(runtimeArgs).join(','),
					runtimeAuthSource,
					runtimeApiKeyEnv.trim(),
					''
				].join('|');
			} else {
				savedRuntimeProbeKey = '';
				runtimePreset = 'myaaw';
				runtimeCommand = '';
				runtimeArgs = '';
				runtimeModel = '';
				runtimeThinking = '';
				runtimeSessionMode = '';
				runtimeAuthSource = 'cli_login';
				runtimeApiKeyEnv = '';
				runtimeApiKey = '';
				runtimeApiKeySaved = false;
				runtimePermissionMode = 'bridge';
				runtimeMcpBridge = true;
			}

			const provider =
				config.providers.find((item) => item.name === config.default_provider) ??
				config.providers[0];
			providerType =
				provider?.type === 'openai_chatgpt'
					? 'openai_chatgpt'
					: provider?.type === 'gemini'
						? 'gemini'
						: 'openai';
			credentialId = provider?.credential_id ?? '';
			connection = provider?.connection;
			connectionVersion += 1;
			providerName = provider?.name ?? '';
			baseUrl = provider?.base_url ?? '';
			model = provider?.default_model ?? '';
			apiKey = '';
			discovered = null;
			manualModel = false;
			savedProvider = provider
				? { name: provider.name, keyConfigured: provider.api_key?.configured ?? false }
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
		const discoveryKey = [agentParam, name, providerType, baseUrl, apiKey, credentialId].join('|');
		discovering = true;
		try {
			const view = isChatGPT
				? await providersStore.modelsForCredentials({
						type: 'openai_chatgpt',
						credential_id: credentialId
					})
				: useSavedKey && savedProvider
					? await providersStore.models(agentParam, savedProvider.name)
					: await providersStore.modelsForCredentials({
							type: providerType,
							base_url: baseUrl.trim() || undefined,
							api_key: apiKey.trim() || undefined
						});
			if (
				discoveryKey !== [agentParam, name, providerType, baseUrl, apiKey, credentialId].join('|')
			)
				return;
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

		if (!initial || changed.runtime) {
			if (isExternalRuntime) {
				const parsedArgs = parseRuntimeArgs(runtimeArgs);
				payload.runtime = {
					preset: runtimePreset,
					type: activeRuntimePreset?.type || undefined,
					command: runtimeCommand.trim() || undefined,
					args: parsedArgs.length > 0 ? parsedArgs : undefined,
					working_dir: '',
					// Empty strings are sent on purpose so "CLI Default" clears a saved choice.
					model: runtimeModel.trim(),
					thinking_level: runtimeThinking,
					session_mode: runtimeSessionMode,
					auth_source: runtimeAuthSource,
					api_key_env: runtimeApiKeyEnv.trim() || undefined,
					api_key: runtimeApiKey.trim() || undefined,
					permission_mode: runtimePermissionMode,
					mcp_bridge: runtimeMcpBridge
				};
			} else {
				payload.runtime = { preset: 'myaaw', type: 'myaaw' };
			}
		}

		if (!isExternalRuntime && (!initial || changed.provider)) {
			payload.provider = {
				type: data.provider_type || providerType,
				name: data.provider_name || undefined,
				...(isChatGPT
					? { auth_mode: 'oauth' as const, credential_id: credentialId }
					: { base_url: data.base_url || undefined, api_key: data.api_key?.trim() || undefined }),
				model: data.model || model
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
		// The colour is registry metadata, not agent config: send it only when it
		// actually changed, so an untouched legacy agent stays untouched.
		if (!initial || changed.color) {
			payload.agent_color = agentColor;
		}
		return payload;
	}

	async function submit() {
		if (connecting) return;
		if (!isExternalRuntime && isChatGPT && !connected) {
			toastStore.error(t('myaaw.chatgpt.connect_required'));
			return;
		}
		const target = (agentParam || name).trim();
		const result = validate(agentSetupSchema, {
			name: target,
			provider_type: isExternalRuntime ? undefined : providerType,
			provider_name: isExternalRuntime ? undefined : providerName,
			base_url: isExternalRuntime || isChatGPT ? undefined : baseUrl,
			api_key: isExternalRuntime || isChatGPT ? undefined : apiKey,
			model: isExternalRuntime ? undefined : model,
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
		if (isExternalRuntime) {
			if (runtimePreset === 'custom_acp' && runtimeCommand.trim() === '') {
				fieldErrors.runtime_command = 'required';
			}
			if (runtimeAuthSource === 'cli_login' && botType === 'public') {
				toastStore.error(t('myaaw.onboard.runtime_public_cli_error'));
				openAccess = true;
				return;
			}
		} else {
			if (!isChatGPT && !initial && apiKey.trim() === '') fieldErrors.api_key = 'api_key_required';
			if (model.trim() === '') fieldErrors.model = 'required';
		}
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
			const response = await agentsStore.onboard(target, buildPayload(result.data));
			invalidateRuntimeOptions(target);
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
				if (await askConfirm({ message: t('myaaw.onboard.restart_confirm', { name: target }) })) {
					await serverStore.restart(target);
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
			disabled={loading || connecting || (!isExternalRuntime && isChatGPT && !connected)}
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
				<!-- The colour comes first: the face is the agent's identity, so it is
				     chosen before the name the user gives it. -->
				<div class="space-y-2">
					<div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
						<Label>{t('myaaw.onboard.agent_color_title')}</Label>
						<span class="text-[11px] text-muted-foreground">
							{t('myaaw.onboard.agent_color_hint')}
						</span>
					</div>
					<AgentColorPicker bind:value={agentColor} />
				</div>

				<Separator />

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid content-start gap-1.5">
						<Label for="onboard-name">{t('myaaw.onboard.agent_name')}</Label>
						<Input
							id="onboard-name"
							bind:value={name}
							oninput={clearConnection}
							placeholder="research"
							disabled={reconfigure}
						/>
						<FieldError code={errors.name} />
					</div>
					<div class="grid content-start gap-1.5">
						<Label for="onboard-runtime-preset">{t('myaaw.onboard.runtime_engine')}</Label>
						<select
							id="onboard-runtime-preset"
							class={selectClass}
							value={runtimePreset}
							onchange={(event) => applyRuntimePreset(event.currentTarget.value)}
						>
							{#if runtimes.length > 0}
								{#each runtimes as rt (rt.id)}
									<option value={rt.id}>
										{runtimePresetLabel(rt.id, rt.label)}{rt.id !== 'myaaw' && rt.default_command
											? rt.installed
												? ` (${t('myaaw.onboard.runtime_installed')})`
												: ` (${t('myaaw.onboard.runtime_not_installed')})`
											: ''}
									</option>
								{/each}
							{:else}
								<option value="myaaw">{t('myaaw.onboard.runtime_builtin')}</option>
								<option value="antigravity">Google Antigravity (agy)</option>
								<option value="claude_code">Claude Code (claude)</option>
								<option value="omp">Oh My Pi (omp)</option>
								<option value="pi">Pi Coding Agent (pi)</option>
								<option value="codex">OpenAI Codex CLI (codex)</option>
								<option value="gemini_cli">Gemini CLI (gemini)</option>
								<option value="opencode">OpenCode (opencode)</option>
								<option value="custom_acp">{t('myaaw.onboard.runtime_custom_acp')}</option>
							{/if}
						</select>
						{#if runtimePresetDescription(activeRuntimePreset)}
							<p class="text-[11px] text-muted-foreground">
								{runtimePresetDescription(activeRuntimePreset)}
								{#if isExternalRuntime && activeRuntimePreset?.installed && activeRuntimePreset.version}
									· <span class="font-mono text-emerald-600 dark:text-emerald-400"
										>{activeRuntimePreset.version}</span
									>
								{/if}
							</p>
						{/if}
					</div>
				</div>

				{#if isExternalRuntime}
					<Separator />
					<div class="space-y-4">
						<div class="grid gap-4 sm:grid-cols-2">
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-command">{t('myaaw.onboard.runtime_binary')}</Label>
								<Input
									id="onboard-runtime-command"
									bind:value={runtimeCommand}
									placeholder={activeRuntimePreset?.default_command || 'agy'}
								/>
								<FieldError code={errors.runtime_command} />
							</div>
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-args">{t('myaaw.onboard.runtime_args')}</Label>
								<Input
									id="onboard-runtime-args"
									bind:value={runtimeArgs}
									placeholder={(activeRuntimePreset?.default_args ?? []).join(', ') || 'acp'}
								/>
							</div>
						</div>

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-workdir">{t('myaaw.onboard.runtime_workdir')}</Label>
								<Input
									id="onboard-runtime-workdir"
									value={resolvedWorkspacePath}
									readonly
									disabled
									class="font-mono text-xs"
								/>
								<p class="text-[11px] text-muted-foreground">
									{t('myaaw.onboard.runtime_workdir_hint')}
								</p>
							</div>
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-model">{t('myaaw.onboard.model')}</Label>
								<select
									id="onboard-runtime-model"
									class={selectClass}
									value={runtimeModel}
									disabled={loadingRuntimeOptions && !runtimeOptions}
									onchange={(event) => (runtimeModel = event.currentTarget.value)}
								>
									<option value="">
										{t('myaaw.onboard.runtime_cli_default')}{runtimeOptions?.models.current
											? ` (${runtimeOptions.models.current})`
											: ''}
									</option>
									{#if runtimeModel && !runtimeModelKnown}
										<option value={runtimeModel}>{runtimeModel}</option>
									{/if}
									{#each runtimeModelGroups as grp (grp.group)}
										{#if grp.group}
											<optgroup label={grp.group}>
												{#each grp.items as item (item.value)}
													<option value={item.value}>{item.label}</option>
												{/each}
											</optgroup>
										{:else}
											{#each grp.items as item (item.value)}
												<option value={item.value}>{item.label}</option>
											{/each}
										{/if}
									{/each}
								</select>
								<p class="text-[11px] text-muted-foreground">
									{#if loadingRuntimeOptions}
										{t('myaaw.onboard.runtime_models_loading')}
									{:else if runtimeOptions?.warning}
										{t('myaaw.onboard.runtime_models_error', { error: runtimeOptions.warning })}
									{:else if runtimeOptions && runtimeOptions.models.options.length === 0}
										{t('myaaw.onboard.runtime_models_empty')}
									{:else}
										{t('myaaw.onboard.runtime_models_live')}
									{/if}
									{#if runtimeCommand.trim()}
										<button
											type="button"
											class="underline"
											onclick={() => void probeRuntimeOptions(true)}
										>
											{t('common.refresh')}
										</button>
									{/if}
								</p>
							</div>
						</div>

						{#if runtimeThinkingOptions.length > 0 || runtimeSessionModeOptions.length > 0}
							<div class="grid gap-4 sm:grid-cols-2">
								{#if runtimeThinkingOptions.length > 0}
									<div class="grid content-start gap-1.5">
										<Label for="onboard-runtime-thinking">{t('myaaw.providers.thinking')}</Label>
										<select
											id="onboard-runtime-thinking"
											class={selectClass}
											value={runtimeThinking}
											onchange={(event) => (runtimeThinking = event.currentTarget.value)}
										>
											<option value="">
												{t('myaaw.onboard.runtime_cli_default')}{runtimeOptions?.thinking.current
													? ` (${runtimeOptions.thinking.current})`
													: ''}
											</option>
											{#each runtimeThinkingOptions as item (item.value)}
												<option value={item.value}>{item.label}</option>
											{/each}
										</select>
									</div>
								{/if}
								{#if runtimeSessionModeOptions.length > 0}
									<div class="grid content-start gap-1.5">
										<Label for="onboard-runtime-session-mode"
											>{t('myaaw.onboard.runtime_mode')}</Label
										>
										<select
											id="onboard-runtime-session-mode"
											class={selectClass}
											value={runtimeSessionMode}
											onchange={(event) => (runtimeSessionMode = event.currentTarget.value)}
										>
											<option value="">
												{t('myaaw.onboard.runtime_cli_default')}{runtimeOptions?.session_modes
													.current
													? ` (${runtimeOptions.session_modes.current})`
													: ''}
											</option>
											{#each runtimeSessionModeOptions as item (item.value)}
												<option value={item.value}>{item.label}</option>
											{/each}
										</select>
									</div>
								{/if}
							</div>
						{/if}

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-auth">{t('myaaw.onboard.runtime_auth_mode')}</Label>
								<select
									id="onboard-runtime-auth"
									class={selectClass}
									value={runtimeAuthSource}
									onchange={(event) => {
										runtimeAuthSource = event.currentTarget.value as 'cli_login' | 'api_key';
										if (runtimeAuthSource === 'cli_login') botType = 'private';
									}}
								>
									{#if !activeRuntimePreset || activeRuntimePreset.capabilities.supports_cli_auth}
										<option value="cli_login">{t('myaaw.onboard.runtime_auth_cli_login')}</option>
									{/if}
									<option value="api_key">{t('myaaw.onboard.runtime_auth_api_key')}</option>
								</select>
							</div>
							<div class="grid content-start gap-1.5">
								<Label for="onboard-runtime-permission">{t('myaaw.onboard.runtime_approvals')}</Label>
								<select
									id="onboard-runtime-permission"
									class={selectClass}
									value={runtimePermissionMode}
									disabled={runtimeApprovalOptions.length === 0}
									onchange={(event) =>
										(runtimePermissionMode = event.currentTarget.value as
											| 'bridge'
											| 'auto'
											| 'read_only')}
								>
									{#each runtimeApprovalOptions as item (item.value)}
										<option value={item.value}>{runtimeApprovalLabel(item)}</option>
									{:else}
										<option value={runtimePermissionMode}>
											{loadingRuntimeOptions
												? t('common.loading')
												: runtimeOptions
													? t('myaaw.onboard.runtime_approvals_managed_by_cli')
													: t('myaaw.onboard.runtime_set_binary_first')}
										</option>
									{/each}
								</select>
								{#if runtimeApprovalDescription(runtimeApprovalOptions.find((o) => o.value === runtimePermissionMode))}
									<p class="text-[11px] text-muted-foreground">
										{runtimeApprovalDescription(
											runtimeApprovalOptions.find((o) => o.value === runtimePermissionMode)
										)}
									</p>
								{/if}
							</div>
						</div>

						{#if runtimeAuthSource === 'api_key'}
							<div class="grid gap-4 sm:grid-cols-2">
								<div class="grid content-start gap-1.5">
									<Label for="onboard-runtime-apikey-env"
										>{t('myaaw.onboard.runtime_api_key_env')}</Label
									>
									<Input
										id="onboard-runtime-apikey-env"
										bind:value={runtimeApiKeyEnv}
										placeholder={activeRuntimePreset?.default_api_key_env || 'OPENAI_API_KEY'}
									/>
								</div>
								<div class="grid content-start gap-1.5">
									<Label for="onboard-runtime-apikey">{t('myaaw.onboard.runtime_api_key')}</Label>
									<Input
										id="onboard-runtime-apikey"
										type="password"
										bind:value={runtimeApiKey}
										placeholder={runtimeApiKeySaved
											? t('myaaw.onboard.api_key_saved')
											: t('myaaw.onboard.runtime_api_key_placeholder')}
									/>
								</div>
							</div>
						{/if}

						<label class="flex items-center gap-2 text-xs text-foreground">
							<Checkbox bind:checked={runtimeMcpBridge} />
							<span>
								{t('myaaw.onboard.runtime_mcp_bridge')} (<code class="font-mono">cron</code>,
								<code class="font-mono">a2a_contact</code>,
								<code class="font-mono">extra_messages</code>,
								{t('myaaw.onboard.runtime_mcp_bridge_custom_tools')})
							</span>
						</label>

						{#if runtimePresetToSNotice(activeRuntimePreset)}
							<div
								class="rounded-md border border-border/70 bg-muted/40 px-3 py-2 text-[11px] text-muted-foreground"
							>
								{runtimePresetToSNotice(activeRuntimePreset)}
								{#if runtimeAuthSource === 'cli_login'}
									{t('myaaw.onboard.runtime_remote_login_hint')}
									<a href="/app/terminal" class="underline"
										>{t('myaaw.onboard.runtime_open_terminal')}</a
									>.
								{/if}
							</div>
						{/if}
					</div>
				{:else}
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid content-start gap-1.5">
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
						<div class="grid content-start gap-1.5">
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
					</div>

					{#if !isChatGPT}
						<div class="grid gap-4 sm:grid-cols-2">
							<div class="grid content-start gap-1.5">
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
					{/if}
					{#if isChatGPT}
						{#key `${agentParam}:${name.trim()}:${connectionVersion}`}
							<ChatGPTConnection
								agent={(agentParam || name).trim()}
								providerId={reconfigure && savedProvider?.name === providerName.trim()
									? savedProvider.name
									: undefined}
								bind:credentialId
								bind:connection
								bind:busy={connecting}
								onchange={clearDiscovered}
							/>
						{/key}
					{/if}

					<div class="grid gap-4 sm:grid-cols-2">
						{#if !isChatGPT}
							<div class="grid content-start gap-1.5">
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
						{/if}
						<div class="grid content-start gap-1.5">
							<Label for="onboard-model">{t('myaaw.onboard.model')}</Label>
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
									placeholder={isChatGPT
										? t('myaaw.chatgpt.model_hint')
										: (activePreset?.default_model ?? 'gpt-4o')}
								/>
							{/if}
							<FieldError code={errors.model} />
						</div>
					</div>

					<!-- Discovery needs the credentials above, so it comes right after them
					     and keeps the two fields above on the same baseline. -->
					<div
						class="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground"
					>
						<Button
							variant="outline"
							size="sm"
							loading={discovering}
							disabled={!canDiscover}
							onclick={() => void discover()}
						>
							<Wand2 class="mr-2 h-4 w-4" />
							{t('myaaw.onboard.model_discover')}
						</Button>
						{#if !canDiscover}
							<span
								>{t(
									isChatGPT ? 'myaaw.chatgpt.connect_required' : 'myaaw.onboard.model_discover_hint'
								)}</span
							>
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
				{/if}
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
					<div class="grid content-start gap-1.5">
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
					<div class="grid content-start gap-1.5">
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
					<div class="grid content-start gap-1.5">
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
				<div class="grid content-start gap-1.5">
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
				<div class="grid content-start gap-1.5">
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
					<div class="grid content-start gap-1.5">
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
					<div class="grid content-start gap-1.5">
						<Label for="onboard-heartbeat-to">{t('myaaw.onboard.heartbeat_to')}</Label>
						<Input id="onboard-heartbeat-to" bind:value={heartbeatTo} placeholder="123456789" />
						<FieldError code={errors.heartbeat_to} />
					</div>
					<div class="grid content-start gap-1.5">
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
							{#if isExternalRuntime}
								{t('myaaw.onboard.summary_provider', {
									type: activeRuntimePreset
										? runtimePresetLabel(activeRuntimePreset.id, activeRuntimePreset.label)
										: runtimePreset,
									model: runtimeModel || t('myaaw.onboard.runtime_cli_default')
								})}
							{:else}
								{t('myaaw.onboard.summary_provider', {
									type: providerType,
									model: model || '—'
								})}
							{/if}
						</p>
					</div>
					{#if reconfigure}
						<Badge variant="outline" class="shrink-0">
							{changed.runtime || changed.provider || changed.color
								? t('myaaw.onboard.changing')
								: t('myaaw.onboard.unchanged')}
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
							{botType === 'public'
								? t('myaaw.onboard.bot_public')
								: t('myaaw.onboard.bot_private')} · {heartbeatSummary}
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
