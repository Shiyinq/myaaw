import { agentPath, client } from './client';
import type { ContextContent, ContextFilesResponse } from '$lib/types';

export const context = {
	list: (agent: string) => client<ContextFilesResponse>(agentPath(agent, '/context')),
	read: (agent: string, path: string) =>
		client<ContextContent>(agentPath(agent, `/context/file?path=${encodeURIComponent(path)}`)),
	write: (agent: string, path: string, content: string) =>
		client<ContextContent>(agentPath(agent, '/context/file'), {
			method: 'PUT',
			body: { path, content }
		})
};
