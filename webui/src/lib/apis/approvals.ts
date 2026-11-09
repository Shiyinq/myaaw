import { agentPath, client } from './client';
import type { ApprovalsView, OKResponse } from '$lib/types';

export const approvals = {
	get: (agent: string) => client<ApprovalsView>(agentPath(agent, '/approvals')),
	revokeKey: (agent: string, key: string, scope: { channel?: string; user?: string } = {}) => {
		const query = new URLSearchParams();
		if (scope.channel) query.set('channel', scope.channel);
		if (scope.user) query.set('user', scope.user);
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<OKResponse>(
			agentPath(agent, `/approvals/grants/${encodeURIComponent(key)}${suffix}`),
			{ method: 'DELETE' }
		);
	},
	revokeSession: (
		agent: string,
		key: string,
		conversation: string,
		scope: { channel?: string; user?: string } = {}
	) => {
		const query = new URLSearchParams();
		if (scope.channel) query.set('channel', scope.channel);
		if (scope.user) query.set('user', scope.user);
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<OKResponse>(
			agentPath(
				agent,
				`/approvals/grants/${encodeURIComponent(key)}/sessions/${encodeURIComponent(conversation)}${suffix}`
			),
			{ method: 'DELETE' }
		);
	},
	clear: (agent: string) =>
		client<OKResponse>(agentPath(agent, '/approvals/grants'), { method: 'DELETE' })
};
