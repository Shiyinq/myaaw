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
	telegram_owner_ids?: string[];
	discord_token?: string;
	discord_owner_ids?: string[];
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
