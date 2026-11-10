export interface Usage {
	prompt_tokens: number;
	completion_tokens: number;
	total_tokens: number;
}

/**
 * How full the model's context window is for the newest request. `tokens` is the
 * exact input count the provider reported; the breakdown rows are the
 * pre-request estimate and are shown with a ~.
 */
export interface ContextUsage {
	tokens: number;
	window: number;
	system_tokens: number;
	tool_tokens: number;
	message_tokens: number;
}

export interface Attachment {
	file_name: string;
	mime_type?: string;
	data_base64: string;
}

export interface ChatRequest {
	text: string;
	/** Lets the client stop this run with `POST /chat/stop`. */
	request_id?: string;
	conversation_id?: string;
	images?: string[];
	files?: ChatFile[];
	/** Rewinds the transcript: 0 regenerates the last turn, N edits an earlier one. */
	rewind_user_offset?: number;
}

/** An uploaded document, sent base64 and persisted on the agent. */
export interface ChatFile {
	name: string;
	mime_type?: string;
	data_base64: string;
}

export interface TimelineItem {
	kind: string;
	text?: string;
	id?: string;
	name?: string;
	arguments?: string;
	result?: string;
	is_error?: boolean;
	attempt?: number;
	tokens_before?: number;
	tokens_after?: number;
	dropped_messages?: number;
	subagent?: string;
	args?: string;
}

export interface ApprovalPrompt {
	approval_id: string;
	tool_name: string;
	action?: string;
	args_summary?: string;
	/** Full raw argument JSON, when the source carries it (the chat stream does). */
	args?: string;
	subagent?: string;
}

export type ApprovalDecision = 'allow_once' | 'allow_session' | 'allow_always' | 'deny';

export interface ChatResponse {
	text: string;
	conversation_id?: string;
	run_id?: string;
	timeline?: TimelineItem[];
	usage?: Usage;
	attachments?: Attachment[];
	status?: 'approval_required' | string;
	approval?: ApprovalPrompt;
}

/** An in-flight (or recently finished) run a client can attach to. */
export interface ChatRunInfo {
	request_id: string;
	conversation_id?: string;
	streaming: boolean;
	started_at: string;
	finished_at?: string;
}

export type ChatStreamEvent =
	| 'run'
	| 'thought'
	| 'chunk'
	| 'conversation'
	| 'tool'
	| 'tool_result'
	| 'retry'
	| 'approval_required'
	| 'approval_resolved'
	| 'compaction'
	| 'context'
	| 'done'
	| 'error';
