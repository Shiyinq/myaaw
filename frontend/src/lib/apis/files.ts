import { agentPath, client } from './client';
import type { FileContent, FileListResponse, OKResponse, SearchResponse } from '$lib/types';

export const files = {
	list: (agent: string, path = '') =>
		client<FileListResponse>(agentPath(agent, `/fs?path=${encodeURIComponent(path)}`)),
	read: (agent: string, path: string) =>
		client<FileContent>(agentPath(agent, `/fs/read?path=${encodeURIComponent(path)}`)),
	search: (agent: string, query: string, path = '/home') =>
		client<SearchResponse>(
			agentPath(agent, `/fs/search?q=${encodeURIComponent(query)}&path=${encodeURIComponent(path)}`)
		),
	write: (agent: string, path: string, content: string) =>
		client<OKResponse>(agentPath(agent, '/fs/write'), {
			method: 'POST',
			body: { path, content }
		}),
	mkdir: (agent: string, path: string) =>
		client<OKResponse>(agentPath(agent, '/fs/mkdir'), { method: 'POST', body: { path } }),
	rename: (agent: string, from: string, to: string) =>
		client<OKResponse>(agentPath(agent, '/fs/rename'), {
			method: 'POST',
			body: { from, to }
		}),
	remove: (agent: string, path: string, recursive = false) =>
		client<OKResponse>(
			agentPath(agent, `/fs?path=${encodeURIComponent(path)}&recursive=${recursive}`),
			{ method: 'DELETE' }
		)
};
