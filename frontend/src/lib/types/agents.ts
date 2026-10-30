import type { AgentConfig } from './config';

export interface AgentInfo {
	name: string;
	default: boolean;
	running: boolean;
	port?: number;
	base_url?: string;
}

export interface AgentsResponse {
	agents: AgentInfo[];
}

export interface OnboardProvider {
	type: string;
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
	provider?: OnboardProvider;
	skills?: string[];
	disable_tools?: string[];
	channels?: OnboardChannels;
	heartbeat?: OnboardHeartbeat;
	bot?: OnboardBot;
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
