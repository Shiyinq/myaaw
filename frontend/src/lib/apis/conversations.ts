import { agentPath, client } from './client';
import type { ConversationDetail, ConversationPage, OKResponse } from '$lib/types';

export const conversations = {
	list: (agent: string, params: { search?: string; limit?: number; offset?: number } = {}) => {
		const query = new URLSearchParams();
		if (params.search) query.set('search', params.search);
		if (params.limit !== undefined) query.set('limit', String(params.limit));
		if (params.offset !== undefined) query.set('offset', String(params.offset));
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<ConversationPage>(agentPath(agent, `/conversations${suffix}`));
	},
	get: (agent: string, id: string, params: { limit?: number; offset?: number } = {}) => {
		const query = new URLSearchParams();
		if (params.limit !== undefined) query.set('limit', String(params.limit));
		if (params.offset !== undefined) query.set('offset', String(params.offset));
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<ConversationDetail>(
			agentPath(agent, `/conversations/${encodeURIComponent(id)}${suffix}`)
		);
	},
	rename: (agent: string, id: string, title: string) =>
		client<{ ok: boolean; id: string; title: string }>(
			agentPath(agent, `/conversations/${encodeURIComponent(id)}`),
			{ method: 'PATCH', body: { title } }
		),
	remove: (agent: string, id: string) =>
		client<OKResponse>(agentPath(agent, `/conversations/${encodeURIComponent(id)}`), {
			method: 'DELETE'
		})
};
