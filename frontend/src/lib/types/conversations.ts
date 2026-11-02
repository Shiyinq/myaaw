export interface ContentPart {
	type: string;
	text?: string;
	image_url?: { url?: string };
}

export interface ToolCall {
	id?: string;
	type?: string;
	function?: {
		name?: string;
		arguments?: string | Record<string, unknown>;
	};
}

export interface Message {
	role: string;
	content: string | ContentPart[];
	images?: string[];
	tool_calls?: ToolCall[];
	tool_call_id?: string;
	is_error?: boolean;
	name?: string;
	thought?: string;
	action?: string;
	action_input?: string;
	/** Stable transcript-entry id, used to address uploaded files. */
	id?: string;
	uploads?: Upload[];
	/** UTC timestamp of the stored message, serialised by the agent. */
	created_at?: string;
	/** How long the turn took, set on the final assistant message. */
	duration_ms?: number;
}

/** A file a user attached to a message; persisted on the agent. */
export interface Upload {
	name: string;
	path: string;
	mime_type?: string;
	size?: number;
}

export interface ConversationSummary {
	id: string;
	user_id: number;
	title: string;
	active: boolean;
	message_count: number;
	created_at: string;
	updated_at: string;
}

export interface ConversationPage {
	conversations: ConversationSummary[];
	total: number;
	limit: number;
	offset: number;
	has_more: boolean;
}

/** One calendar day in the activity histogram. */
export interface ActivityDay {
	date: string;
	count: number;
}

/** Per-day count of conversations created by the signed-in user. */
export interface ConversationActivity {
	from: number;
	to: number;
	total: number;
	days: ActivityDay[];
}

/** One of the user's questions, located in the whole stored transcript. */
export interface ConversationTurn {
	index: number;
	text: string;
}

export interface ConversationDetail {
	id: string;
	user_id: number;
	title: string;
	active: boolean;
	created_at: string;
	updated_at: string;
	messages: Message[];
	total_messages: number;
	limit: number;
	offset: number;
	has_more: boolean;
	/** Every question of the conversation, not only the returned messages. */
	turns: ConversationTurn[];
}
