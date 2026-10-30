import { API_BASE } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
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
		),
	/**
	 * Same-origin URL for the raw bytes. <img>, <video>, <iframe> and
	 * <a download> cannot send an Authorization header, so the session token
	 * travels as a query parameter; the proxy keeps everything on this origin.
	 */
	rawUrl: (agent: string, path: string, options: { download?: boolean } = {}) => {
		const query = new URLSearchParams({ path });
		if (accessToken.value) query.set('token', accessToken.value);
		if (options.download) query.set('download', '1');
		return `${API_BASE}${agentPath(agent, `/fs/raw?${query.toString()}`)}`;
	},
	download: (agent: string, path: string) => {
		const anchor = document.createElement('a');
		anchor.href = files.rawUrl(agent, path, { download: true });
		anchor.download = path.split('/').pop() || 'download';
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
	}
};
