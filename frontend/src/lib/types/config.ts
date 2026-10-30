export interface SecretState {
	configured: boolean;
	masked?: string;
}

export interface ModelPrice {
	input: number;
	cache_hit?: number;
	output: number;
	off_peak?: {
		input?: number;
		cache_hit?: number;
		output?: number;
	};
	peak_windows?: string[];
}

export interface ObservabilityConfig {
	enabled?: boolean;
	retention_days?: number;
	max_content_chars?: number;
	store_content?: boolean;
	redact_patterns?: string[];
	tool_prices?: Record<string, { per_call: number }>;
	model_prices?: Record<string, ModelPrice>;
}

export interface ApprovalsConfig {
	mode: string;
	timeout_seconds: number;
	allow: string[];
	deny: string[];
	unattended: string;
	subagents: string;
}

export interface ProviderView {
	name: string;
	type: string;
	base_url?: string;
	default_model?: string;
	api_key: SecretState;
}

export interface ProvidersView {
	default_provider: string;
	providers: ProviderView[];
}

export interface ChannelView {
	name: string;
	active: boolean;
	mode?: string;
	token: SecretState;
	ngrok_active?: boolean;
	ngrok_auth_token?: SecretState;
}

export interface ChannelsView {
	channels: ChannelView[];
}

export interface BotConfig {
	owner_ids: string[];
	type: string;
	watermark: boolean;
	stream: boolean;
	max_iterations: number;
	warning_iterations: number;
	context_max_tokens: number;
	max_tool_output_bytes: number;
	repeat_tool_call_limit: number;
	tool_timeout_seconds: number;
	run_timeout_seconds: number;
	stream_timeout_seconds: number;
	request_retries: number;
	queue_workers: number;
	computer_use: { enabled: boolean };
	blocked_tools: string[];
}

export interface HeartbeatConfig {
	active: boolean;
	every: string;
	to: string;
	channel: string;
}

export interface CronConfig {
	active: boolean;
}

export interface ServiceSecretView {
	provider?: string;
	api_key: SecretState;
}

export interface AgentConfig {
	default_provider: string;
	providers: ProviderView[];
	channels: ChannelView[];
	bot: BotConfig;
	heartbeat: HeartbeatConfig;
	cron: CronConfig;
	transcriber: ServiceSecretView;
	synthesizer: ServiceSecretView;
	observability?: ObservabilityConfig;
	approvals?: ApprovalsConfig;
}
