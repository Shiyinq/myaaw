export interface Usage {
	prompt_tokens: number;
	completion_tokens: number;
	total_tokens: number;
}

export interface Attachment {
	file_name: string;
	mime_type?: string;
	data_base64: string;
}

export interface ChatRequest {
	text: string;
	conversation_id?: string;
	images?: string[];
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

export type ChatStreamEvent =
	| 'thought'
	| 'chunk'
	| 'tool'
	| 'tool_result'
	| 'retry'
	| 'approval_required'
	| 'approval_resolved'
	| 'compaction'
	| 'done'
	| 'error';
