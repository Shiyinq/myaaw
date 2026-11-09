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

export interface ConfigResetRequest {
	bot?: string[];
	/** `true` resets the whole section, a list resets only those fields. */
	heartbeat?: boolean | string[];
	cron?: boolean | string[];
	transcriber?: boolean | string[];
	synthesizer?: boolean | string[];
	observability?: boolean | string[];
	approvals?: boolean | string[];
}

export const config = {
	get: (agent: string) => client<AgentConfig>(agentPath(agent, '/config')),
	patch: (agent: string, payload: ConfigPatch) =>
		client<AgentConfig>(agentPath(agent, '/config'), { method: 'PATCH', body: payload }),
	reset: (agent: string) =>
		client<AgentConfig>(agentPath(agent, '/config/reset'), { method: 'POST' }),
	/** Resets the listed sections; `bot: []` resets the whole bot section. */
	resetFields: (agent: string, payload: ConfigResetRequest) =>
		client<AgentConfig>(agentPath(agent, '/config/reset'), { method: 'PATCH', body: payload })
};
