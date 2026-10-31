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
}
