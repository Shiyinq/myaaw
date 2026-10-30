export interface Message {
	role: string;
	content: string;
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
