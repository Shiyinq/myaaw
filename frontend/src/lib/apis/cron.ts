import { agentPath, client } from './client';
import type {
	CronHistoryResponse,
	CronJob,
	CronJobInput,
	CronJobsResponse,
	OKResponse
} from '$lib/types';

export const cron = {
	list: (agent: string) => client<CronJobsResponse>(agentPath(agent, '/cron')),
	add: (agent: string, payload: CronJobInput) =>
		client<CronJob>(agentPath(agent, '/cron'), { method: 'POST', body: payload }),
	remove: (agent: string, id: string) =>
		client<OKResponse>(agentPath(agent, `/cron/${encodeURIComponent(id)}`), { method: 'DELETE' }),
	run: (agent: string, id: string) =>
		client<OKResponse>(agentPath(agent, `/cron/${encodeURIComponent(id)}/run`), {
			method: 'POST'
		}),
	history: (agent: string, id: string, limit = 20) =>
		client<CronHistoryResponse>(
			agentPath(agent, `/cron/${encodeURIComponent(id)}/history?limit=${limit}`)
		)
};
