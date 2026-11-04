export interface CronSchedule {
	kind: 'cron' | 'every' | 'at' | string;
	expr: string;
	tz: string;
}

export interface CronPayload {
	kind: string;
	content: string;
}

/** Platform that receives the reply. */
export type CronChannel = 'telegram' | 'discord' | 'api' | 'cli';

/** Discord only: a DM recipient is a user id, a guild target is a channel id. */
export type CronChatType = 'dm' | 'guild';

export interface CronDelivery {
	mode: string;
	channel: CronChannel | string;
	to: string;
	thread_id?: string;
	session_id?: string;
	chat_type?: CronChatType | string;
}

export interface CronJob {
	id: string;
	name: string;
	agent_id: string;
	schedule: CronSchedule;
	payload: CronPayload;
	delivery: CronDelivery;
	enabled: boolean;
	created_at: string;
	updated_at: string;
}

export interface CronJobsResponse {
	jobs: CronJob[] | null;
}

export interface CronJobInput {
	name: string;
	schedule_kind: 'cron' | 'every' | 'at' | string;
	expr: string;
	tz?: string;
	message: string;
	channel: CronChannel;
	to: string;
	/** Telegram forum message_thread_id. */
	thread_id?: string;
	/** Exact conversation id; required for api/cli. */
	session_id?: string;
	/** Discord only. */
	chat_type?: CronChatType;
	enabled?: boolean;
}

export interface CronExecution {
	ts: string;
	status: string;
	result: string;
}

export interface CronHistoryResponse {
	history: CronExecution[] | null;
}
