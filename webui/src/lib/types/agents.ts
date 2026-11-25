import type { AgentConfig, RuntimeCapabilities } from './config';
import type { ChatRunInfo } from './chat';
import type { ConversationSummary } from './conversations';

export interface AgentInfo {
	name: string;
	default: boolean;
	running: boolean;
	port?: number;
	base_url?: string;
	/** RFC3339 UTC instant the agent was created (used for sorting). */
	created_at?: string;
	/** Bloub palette key; absent from an older control plane, read it with `colorOf()`. */
	agent_color?: string;
	runtime_type?: string;
	runtime_preset?: string;
}

export interface DetectedRuntime {
	id: string;
	label: string;
	description: string;
	type: string;
	default_command?: string;
	default_args?: string[];
	default_api_key_env?: string;
	default_auth?: string;
	capabilities: RuntimeCapabilities;
	tos_notice?: string;
	installed: boolean;
	binary_path?: string;
	version?: string;
}

export interface RuntimesResponse {
	runtimes: DetectedRuntime[];
}

export interface OnboardRuntime {
	type?: string;
	preset?: string;
	command?: string;
	args?: string[];
	working_dir?: string;
	additional_dirs?: string[];
	model?: string;
	auth_source?: string;
	api_key_env?: string;
	api_key?: string;
	permission_mode?: string;
	mcp_bridge?: boolean;
	idle_timeout_seconds?: number;
}

export interface AgentsResponse {
	agents: AgentInfo[];
}

export interface BotSidebarSummary {
	agent: string;
	latest_conversation_id?: string;
	latest_title?: string;
	latest_preview?: string;
	latest_updated_at?: string;
	latest_read_at?: string | null;
	recent_conversations?: ConversationSummary[];
	active_runs?: ChatRunInfo[];
}

export interface BotsSummaryResponse {
	bots: BotSidebarSummary[];
}

export interface OnboardProvider {
	type: string;
	auth_mode?: 'oauth';
	credential_id?: string;
	name?: string;
	base_url?: string;
	api_key?: string;
	model?: string;
}

export interface OnboardChannels {
	telegram_token?: string;
	telegram_mode?: string;
	telegram_webhook_url?: string;
	discord_token?: string;
	disable_telegram?: boolean;
	disable_discord?: boolean;
}

export interface OnboardHeartbeat {
	active?: boolean;
	channel?: string;
	to?: string;
	every?: string;
}

export interface OnboardBot {
	type?: string;
	owner_ids?: string[];
}

export interface OnboardRequest {
	runtime?: OnboardRuntime;
	provider?: OnboardProvider;
	skills?: string[];
	disable_tools?: string[];
	channels?: OnboardChannels;
	heartbeat?: OnboardHeartbeat;
	bot?: OnboardBot;
	/** Palette key stored in the registry; omitted keeps the current colour. */
	agent_color?: string;
}

export interface OnboardResponse {
	created: boolean;
	config: AgentConfig;
	warnings: string[] | null;
}

/** One built-in skill or tool the onboarding wizard can install or enable. */
export interface CapabilityEntry {
	name: string;
	description?: string;
}

/**
 * One provider preset the wizard offers, with the endpoint and model it
 * prefills. Presets can share a wire `type`: DeepSeek speaks the OpenAI
 * protocol and is stored as `type: openai` with its own endpoint.
 */
export interface ProviderOption {
	id: string;
	type: string;
	label: string;
	default_base_url?: string;
	default_model?: string;
}

/** Global onboarding catalog: a not-yet-created agent has no skills or tools. */
export interface CapabilitiesView {
	providers: ProviderOption[];
	skills: CapabilityEntry[];
	tools: CapabilityEntry[];
}
