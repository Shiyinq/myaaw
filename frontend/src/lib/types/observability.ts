export interface ObservabilitySummary {
	from: number;
	to: number;
	runs: number;
	errors: number;
	error_rate: number;
	database_bytes: number;
	input_tokens: number;
	output_tokens: number;
	cache_hit_tokens: number;
	cost_usd: number;
	tool_cost_usd: number;
	cost_complete: boolean;
	unpriced_runs: number;
	unpriced_input_tokens: number;
	unpriced_output_tokens: number;
	unpriced_models?: string[];
	duration_ms_p50: number;
	duration_ms_p95: number;
	first_token_ms_p50: number;
	per_tool?: { name: string; calls: number; cost_usd?: number }[];
	per_trigger?: { trigger: string; runs: number; cost_usd?: number }[];
}

export interface RunSummary {
	id: string;
	started_at: number;
	ended_at?: number | null;
	channel?: string;
	user_id?: string;
	conversation_id?: string;
	provider?: string;
	model?: string;
	agent?: string;
	status: string;
	trigger?: string;
	input_tokens?: number;
	output_tokens?: number;
	cache_hit_tokens?: number;
	model_cost_usd?: number;
	total_ms?: number;
	first_token_ms?: number;
	steps?: number;
}

export interface RunList {
	runs: RunSummary[] | null;
	total?: number;
}

export interface Span {
	id: string;
	run_id?: string;
	parent_id?: string | null;
	kind: string;
	name?: string;
	step_index?: number;
	request_index?: number;
	started_at?: number;
	ended_at?: number | null;
	duration_ms?: number;
	first_token_ms?: number;
	generation_ms?: number;
	input_tokens?: number;
	output_tokens?: number;
	cost_usd?: number;
	status?: string;
	input_json?: string;
	output_json?: string;
}

export interface SpanNode {
	id: string;
	children?: SpanNode[] | null;
}

export interface RunDetail {
	run: RunSummary;
	spans: Span[] | null;
	tree?: SpanNode | null;
}

export interface SessionSummary {
	conversation_id: string;
	channel?: string;
	turns: number;
	started_at: number;
	last_active_at: number;
	input_tokens: number;
	output_tokens: number;
	cost_usd: number;
	error_count: number;
}

export interface SessionList {
	sessions: SessionSummary[] | null;
}

export interface SessionTurn extends RunSummary {
	steps?: number;
	spans?: Span[] | null;
}

export interface SessionDetail extends SessionSummary {
	runs: SessionTurn[] | null;
}
