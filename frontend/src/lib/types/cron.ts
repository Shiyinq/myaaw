export interface CronSchedule {
	kind: 'cron' | 'every' | 'at' | string;
	expr: string;
	tz: string;
}

export interface CronPayload {
	kind: string;
	content: string;
}

export interface CronDelivery {
	mode: string;
	channel: string;
	to: string;
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
	channel?: string;
	to?: string;
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
