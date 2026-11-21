import type { AskQuote, ChatFile, TimelineItem } from './chat';

export type DiscussionMode = 'autonomous' | 'all' | 'mention';

export type GroupSenderType = 'user' | 'agent' | 'system';

export interface GroupMember {
	name: string;
	agent_color: string;
	running: boolean;
	last_seen_message_id?: string;
	conversation_id?: string;
}

export interface ReplyPreview {
	id: string;
	sender_type: GroupSenderType;
	sender_name: string;
	excerpt: string;
}

export interface GroupMessage {
	id: string;
	group_id: string;
	sender_type: GroupSenderType;
	sender_name: string;
	agent_color?: string;
	content: string;
	timeline?: TimelineItem[];
	reply_to_id?: string;
	reply_preview?: ReplyPreview;
	quotes?: AskQuote[];
	created_at: string;
}

export interface GroupPendingApproval {
	approval_id: string;
	group_id: string;
	agent: string;
	agent_color?: string;
	conversation_id?: string;
	run_id?: string;
	tool_name: string;
	action?: string;
	args_summary?: string;
	args?: string;
	subagent?: string;
}

export interface GroupRunState {
	running: boolean;
	paused: boolean;
	pause_reason: '' | 'max_turns' | string;
	current_agent: string;
	turn_count: number;
	max_turns: number;
	queue: string[];
	pending_approvals?: GroupPendingApproval[];
}

export interface Group {
	id: string;
	name: string;
	description: string;
	members: GroupMember[];
	discussion_mode: DiscussionMode;
	max_turns: number;
	created_at: string;
	updated_at: string;
}

export interface GroupLastMessage {
	id: string;
	sender_type: GroupSenderType;
	sender_name: string;
	content: string;
	created_at: string;
}

export interface GroupSummary {
	id: string;
	name: string;
	description: string;
	members: GroupMember[];
	member_count: number;
	discussion_mode: DiscussionMode;
	max_turns: number;
	running: boolean;
	current_agent: string;
	pending_approval_count?: number;
	last_message?: GroupLastMessage | null;
	created_at: string;
	updated_at: string;
}

export interface GroupDetailParams {
	limit?: number;
	offset?: number;
	before_id?: string;
}

export interface GroupDetail {
	group: Group;
	messages: GroupMessage[];
	run_state: GroupRunState;
	total_messages?: number;
	offset?: number;
	limit?: number;
	has_more?: boolean;
}

export interface CreateGroupRequest {
	name: string;
	description?: string;
	members: string[];
	discussion_mode?: DiscussionMode;
	max_turns?: number;
}

export interface UpdateGroupRequest {
	name?: string;
	description?: string;
	members?: string[];
	discussion_mode?: DiscussionMode;
	max_turns?: number;
}

export interface SendGroupMessageRequest {
	text: string;
	reply_to_id?: string;
	quotes?: AskQuote[];
	images?: string[];
	files?: ChatFile[];
}

export interface SendGroupMessageResponse {
	message: GroupMessage;
	run_state: GroupRunState;
}

export interface StopGroupResponse {
	stopped: boolean;
	run_state: GroupRunState;
}

export interface ContinueGroupRequest {
	extra_turns?: number;
}

export interface ContinueGroupResponse {
	continued: boolean;
	run_state: GroupRunState;
}

export type GroupStreamEventName =
	| 'status'
	| 'turn_start'
	| 'thinking'
	| 'chunk'
	| 'tool_start'
	| 'tool_end'
	| 'message'
	| 'turn_end'
	| 'run_end'
	| 'error';

export interface GroupStatusEvent {
	group_id: string;
	run_state: GroupRunState;
}

export interface GroupTurnStartEvent {
	group_id: string;
	agent: string;
	agent_color: string;
	turn: number;
	max_turns: number;
	request_id: string;
}

export interface GroupThinkingEvent {
	group_id: string;
	agent: string;
	text: string;
}

export interface GroupChunkEvent {
	group_id: string;
	agent: string;
	text: string;
}

export interface GroupToolStartEvent {
	group_id: string;
	agent: string;
	id: string;
	name: string;
	arguments: string;
}

export interface GroupToolEndEvent {
	group_id: string;
	agent: string;
	id: string;
	name: string;
	result: string;
	is_error: boolean;
}

export interface GroupMessageEvent {
	group_id: string;
	message: GroupMessage;
}

export interface GroupTurnEndEvent {
	group_id: string;
	agent: string;
	turn: number;
	max_turns: number;
	message_id: string;
	stopped: boolean;
	passed: boolean;
	done_signal: boolean;
}

export interface GroupRunEndEvent {
	group_id: string;
	reason: 'completed' | 'max_turns' | 'stopped' | 'interrupted' | 'no_online_members' | 'error';
	turn_count: number;
	max_turns: number;
	paused: boolean;
	run_state: GroupRunState;
}

export interface GroupErrorEvent {
	group_id: string;
	agent?: string;
	error: string;
}
