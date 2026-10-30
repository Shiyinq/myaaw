import { agentPath, client } from './client';
import type { AgentConfig, ApprovalsConfig } from '$lib/types';

export interface ConfigPatch {
	bot?: Record<string, unknown>;
	heartbeat?: Record<string, unknown>;
	cron?: { active?: boolean };
	transcriber?: Record<string, unknown>;
	synthesizer?: Record<string, unknown>;
	observability?: Record<string, unknown>;
	approvals?: Partial<ApprovalsConfig>;
}

export const config = {
	get: (agent: string) => client<AgentConfig>(agentPath(agent, '/config')),
	patch: (agent: string, payload: ConfigPatch) =>
		client<AgentConfig>(agentPath(agent, '/config'), { method: 'PATCH', body: payload }),
	reset: (agent: string) =>
		client<AgentConfig>(agentPath(agent, '/config/reset'), { method: 'POST' }),
	resetFields: (agent: string, payload: Record<string, string[] | []>) =>
		client<AgentConfig>(agentPath(agent, '/config/reset'), { method: 'PATCH', body: payload })
};
