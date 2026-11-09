import { agentPath, client } from './client';
import type { OKResponse, ServerStatus } from '$lib/types';

export const server = {
	status: (agent: string) => client<ServerStatus>(agentPath(agent, '/server/status')),
	start: (agent: string) =>
		client<OKResponse>(agentPath(agent, '/server/start'), { method: 'POST' }),
	stop: (agent: string) => client<OKResponse>(agentPath(agent, '/server/stop'), { method: 'POST' }),
	restart: (agent: string) =>
		client<OKResponse>(agentPath(agent, '/server/restart'), { method: 'POST' })
};
