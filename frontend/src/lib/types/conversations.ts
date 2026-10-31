export interface ContentPart {
	type: string;
	text?: string;
	image_url?: { url?: string };
}

export interface Message {
	role: string;
	content: string | ContentPart[];
	images?: string[];
	tool_calls?: unknown[];
	tool_call_id?: string;
	thought?: string;
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
