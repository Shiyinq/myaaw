import { agentPath, client } from './client';
import type {
	ObservabilitySummary,
	OKResponse,
	RunDetail,
	RunList,
	SessionDetail,
	SessionList
} from '$lib/types';

export interface RunFilters {
	limit?: number;
	offset?: number;
	status?: string;
	channel?: string;
	conversation?: string;
	model?: string;
	trigger?: string;
	from?: number;
	to?: number;
}

export const observability = {
	summary: (agent: string, days = 7) =>
		client<ObservabilitySummary>(agentPath(agent, `/observability/summary?days=${days}`)),
	sessions: (agent: string, limit = 50) =>
		client<SessionList>(agentPath(agent, `/observability/sessions?limit=${limit}`)),
	session: (agent: string, id: string, spans = true) =>
		client<SessionDetail>(
			agentPath(
				agent,
				`/observability/sessions/${encodeURIComponent(id)}?spans=${spans ? 'true' : 'false'}`
			)
		),
	runs: (agent: string, filters: RunFilters = {}) => {
		const query = new URLSearchParams();
		for (const [key, value] of Object.entries(filters)) {
			if (value !== undefined && value !== '') query.set(key, String(value));
		}
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<RunList>(agentPath(agent, `/observability/runs${suffix}`));
	},
	run: (agent: string, id: string) =>
		client<RunDetail>(agentPath(agent, `/observability/runs/${encodeURIComponent(id)}`)),
	purge: (agent: string, before?: number) =>
		client<OKResponse>(
			agentPath(
				agent,
				`/observability/runs${before !== undefined ? `?before=${before}` : ''}`
			),
			{ method: 'DELETE' }
		)
};
