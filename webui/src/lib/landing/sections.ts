import type { BloubColor } from '$lib/bloub';

export const REPO_URL = 'https://github.com/Shiyinq/myaaw';
export const RELEASES_URL = `${REPO_URL}/releases`;
export const DOCS_URL = `${REPO_URL}/tree/main/docs`;

export interface LandingSection {
	id: string;
	num: string;
	navKey: string;
}

export const LANDING_SECTIONS: readonly LandingSection[] = [
	{ id: 'runtimes', num: '01', navKey: 'landing.nav.runtimes' },
	{ id: 'workspace', num: '02', navKey: 'landing.nav.workspace' },
	{ id: 'fleet', num: '03', navKey: 'landing.nav.fleet' },
	{ id: 'doors', num: '04', navKey: 'landing.nav.doors' },
	{ id: 'rules', num: '05', navKey: 'landing.nav.rules' },
	{ id: 'closing', num: '06', navKey: 'landing.nav.install' }
];

export const MARQUEE_PRIMARY: readonly string[] = [
	'claude code',
	'codex',
	'antigravity',
	'opencode',
	'deepseek',
	'pi',
	'oh my pi',
	'custom engines',
	'shared tools',
	'thinking depth',
	'local history'
];

export const MARQUEE_SECONDARY: readonly string[] = [
	'group rooms',
	'built-in git',
	'split terminals',
	'interactive cards',
	'file gallery',
	'telegram',
	'discord',
	'team handoffs',
	'sub-tasks',
	'check-ins',
	'schedules'
];

export type RuntimeCategory = 'native' | 'vendor' | 'oss' | 'custom';

export type RuntimePresetId =
	| 'myaaw'
	| 'claude_code'
	| 'codex'
	| 'antigravity'
	| 'opencode'
	| 'deepseek_harness'
	| 'pi'
	| 'omp'
	| 'custom_acp';

export interface RuntimePreset {
	id: RuntimePresetId;
	name: string;
	category: RuntimeCategory;
	protocol: string;
	command: string;
	summaryKey: string;
	bridgeSupport: boolean;
}

export const RUNTIME_PRESETS: readonly RuntimePreset[] = [
	{
		id: 'myaaw',
		name: 'Myaaw',
		category: 'native',
		protocol: 'Built-in',
		command: 'myaaw',
		summaryKey: 'landing.runtimes.presets.myaaw',
		bridgeSupport: false
	},
	{
		id: 'claude_code',
		name: 'Claude Code',
		category: 'vendor',
		protocol: 'Official CLI',
		command: 'claude',
		summaryKey: 'landing.runtimes.presets.claude_code',
		bridgeSupport: true
	},
	{
		id: 'codex',
		name: 'Codex CLI',
		category: 'vendor',
		protocol: 'Official CLI',
		command: 'codex app-server',
		summaryKey: 'landing.runtimes.presets.codex',
		bridgeSupport: true
	},
	{
		id: 'antigravity',
		name: 'Antigravity CLI',
		category: 'vendor',
		protocol: 'Official CLI',
		command: 'agy acp',
		summaryKey: 'landing.runtimes.presets.antigravity',
		bridgeSupport: true
	},
	{
		id: 'opencode',
		name: 'OpenCode',
		category: 'oss',
		protocol: 'Open-source',
		command: 'opencode acp',
		summaryKey: 'landing.runtimes.presets.opencode',
		bridgeSupport: true
	},
	{
		id: 'deepseek_harness',
		name: 'DeepSeek Harness',
		category: 'oss',
		protocol: 'Open-source',
		command: 'dsh --profile acp',
		summaryKey: 'landing.runtimes.presets.deepseek_harness',
		bridgeSupport: true
	},
	{
		id: 'pi',
		name: 'Pi Coding Agent',
		category: 'oss',
		protocol: 'Open-source',
		command: 'pi --mode rpc',
		summaryKey: 'landing.runtimes.presets.pi',
		bridgeSupport: true
	},
	{
		id: 'omp',
		name: 'Oh My Pi',
		category: 'oss',
		protocol: 'Open-source',
		command: 'omp --mode rpc',
		summaryKey: 'landing.runtimes.presets.omp',
		bridgeSupport: true
	},
	{
		id: 'custom_acp',
		name: 'Custom Engine',
		category: 'custom',
		protocol: 'Custom',
		command: '<your-assistant-cli>',
		summaryKey: 'landing.runtimes.presets.custom_acp',
		bridgeSupport: true
	}
];

export type WorkspaceTabId = 'files' | 'git' | 'terminal' | 'rich';

export const WORKSPACE_TABS: readonly WorkspaceTabId[] = ['files', 'git', 'terminal', 'rich'];

export type ShellModeId = 'bots' | 'studio' | 'focus' | 'classic';

export interface ShellMode {
	id: ShellModeId;
	titleKey: string;
	taglineKey: string;
	descKey: string;
	isDefault?: boolean;
}

export const SHELL_MODES: readonly ShellMode[] = [
	{
		id: 'bots',
		titleKey: 'landing.fleet.modes.bots.title',
		taglineKey: 'landing.fleet.modes.bots.tagline',
		descKey: 'landing.fleet.modes.bots.desc',
		isDefault: true
	},
	{
		id: 'studio',
		titleKey: 'landing.fleet.modes.studio.title',
		taglineKey: 'landing.fleet.modes.studio.tagline',
		descKey: 'landing.fleet.modes.studio.desc'
	},
	{
		id: 'focus',
		titleKey: 'landing.fleet.modes.focus.title',
		taglineKey: 'landing.fleet.modes.focus.tagline',
		descKey: 'landing.fleet.modes.focus.desc'
	},
	{
		id: 'classic',
		titleKey: 'landing.fleet.modes.classic.title',
		taglineKey: 'landing.fleet.modes.classic.tagline',
		descKey: 'landing.fleet.modes.classic.desc'
	}
];

export interface FleetAgent {
	name: string;
	isDefault?: boolean;
	runtimeType: string;
	runtimePreset: string;
	runtimeLabel: string;
	sectionKey: string;
	previewKey: string;
	timeKey: string;
	color: BloubColor;
	initialRunning: boolean;
	initialStreaming?: boolean;
	unreads?: number;
}

export const FLEET_AGENTS: readonly FleetAgent[] = [
	{
		name: 'myaaw',
		isDefault: true,
		runtimeType: 'myaaw',
		runtimePreset: '',
		runtimeLabel: 'Built-in',
		sectionKey: 'landing.fleet.cards.myaaw_section',
		previewKey: 'landing.fleet.cards.myaaw_preview',
		timeKey: 'landing.fleet.cards.myaaw_time',
		color: 'encre',
		initialRunning: true,
		unreads: 2
	},
	{
		name: 'myaadex',
		runtimeType: 'codex',
		runtimePreset: 'codex',
		runtimeLabel: 'Codex CLI',
		sectionKey: 'landing.fleet.cards.codex_section',
		previewKey: 'landing.fleet.cards.codex_preview',
		timeKey: 'landing.fleet.cards.codex_time',
		color: 'rouge',
		initialRunning: true,
		initialStreaming: true
	},
	{
		name: 'nightshift',
		runtimeType: 'claude_code',
		runtimePreset: 'claude_code',
		runtimeLabel: 'Claude Code',
		sectionKey: 'landing.fleet.cards.nightshift_section',
		previewKey: 'landing.fleet.cards.nightshift_preview',
		timeKey: 'landing.fleet.cards.nightshift_time',
		color: 'rose',
		initialRunning: false
	}
];

export type InstallMethodId = 'cli' | 'stack' | 'webui';

export interface InstallMethod {
	id: InstallMethodId;
	labelKey: string;
	descKey: string;
	command: string;
}

export const INSTALL_METHODS: readonly InstallMethod[] = [
	{
		id: 'cli',
		labelKey: 'landing.closing.methods.cli.label',
		descKey: 'landing.closing.methods.cli.desc',
		command: 'curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/install.sh | bash'
	},
	{
		id: 'stack',
		labelKey: 'landing.closing.methods.stack.label',
		descKey: 'landing.closing.methods.stack.desc',
		command:
			'curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/install.sh | bash -s -- --webui'
	},
	{
		id: 'webui',
		labelKey: 'landing.closing.methods.webui.label',
		descKey: 'landing.closing.methods.webui.desc',
		command:
			'curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh | bash'
	}
];

export type DoorId = 'telegram' | 'discord' | 'web' | 'cli' | 'api' | 'heartbeat';

export interface DoorItem {
	id: DoorId;
	titleKey: string;
	tagKey: string;
	bodyKey: string;
	metaKey: string;
}

export const DOOR_ITEMS: readonly DoorItem[] = [
	{
		id: 'telegram',
		titleKey: 'landing.doors.items.telegram.title',
		tagKey: 'landing.doors.items.telegram.tag',
		bodyKey: 'landing.doors.items.telegram.body',
		metaKey: 'landing.doors.items.telegram.meta'
	},
	{
		id: 'discord',
		titleKey: 'landing.doors.items.discord.title',
		tagKey: 'landing.doors.items.discord.tag',
		bodyKey: 'landing.doors.items.discord.body',
		metaKey: 'landing.doors.items.discord.meta'
	},
	{
		id: 'web',
		titleKey: 'landing.doors.items.web.title',
		tagKey: 'landing.doors.items.web.tag',
		bodyKey: 'landing.doors.items.web.body',
		metaKey: 'landing.doors.items.web.meta'
	},
	{
		id: 'cli',
		titleKey: 'landing.doors.items.cli.title',
		tagKey: 'landing.doors.items.cli.tag',
		bodyKey: 'landing.doors.items.cli.body',
		metaKey: 'landing.doors.items.cli.meta'
	},
	{
		id: 'api',
		titleKey: 'landing.doors.items.api.title',
		tagKey: 'landing.doors.items.api.tag',
		bodyKey: 'landing.doors.items.api.body',
		metaKey: 'landing.doors.items.api.meta'
	},
	{
		id: 'heartbeat',
		titleKey: 'landing.doors.items.heartbeat.title',
		tagKey: 'landing.doors.items.heartbeat.tag',
		bodyKey: 'landing.doors.items.heartbeat.body',
		metaKey: 'landing.doors.items.heartbeat.meta'
	}
];

export type CapabilityId =
	| 'providers'
	| 'extra_messages'
	| 'extensions'
	| 'delegation'
	| 'schedule'
	| 'backup';

export interface CapabilityItem {
	id: CapabilityId;
	titleKey: string;
	bodyKey: string;
	badge: 'available' | 'configured';
}

export const CAPABILITY_ITEMS: readonly CapabilityItem[] = [
	{
		id: 'providers',
		titleKey: 'landing.capabilities.providers_title',
		bodyKey: 'landing.capabilities.providers_body',
		badge: 'configured'
	},
	{
		id: 'extra_messages',
		titleKey: 'landing.capabilities.extra_messages_title',
		bodyKey: 'landing.capabilities.extra_messages_body',
		badge: 'available'
	},
	{
		id: 'extensions',
		titleKey: 'landing.capabilities.extensions_title',
		bodyKey: 'landing.capabilities.extensions_body',
		badge: 'configured'
	},
	{
		id: 'delegation',
		titleKey: 'landing.capabilities.delegation_title',
		bodyKey: 'landing.capabilities.delegation_body',
		badge: 'available'
	},
	{
		id: 'schedule',
		titleKey: 'landing.capabilities.schedule_title',
		bodyKey: 'landing.capabilities.schedule_body',
		badge: 'available'
	},
	{
		id: 'backup',
		titleKey: 'landing.capabilities.backup_title',
		bodyKey: 'landing.capabilities.backup_body',
		badge: 'available'
	}
];

export type SetupStepId = 'install' | 'stack' | 'dashboard' | 'remote';

export const SETUP_STEPS: readonly SetupStepId[] = ['install', 'stack', 'dashboard', 'remote'];

export type RuleItemId = 'item_1' | 'item_2' | 'item_3' | 'item_4' | 'item_5' | 'item_6';

export const RULE_ITEMS: readonly RuleItemId[] = [
	'item_1',
	'item_2',
	'item_3',
	'item_4',
	'item_5',
	'item_6'
];
