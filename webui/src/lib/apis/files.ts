import { getApiBase } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { ensureSigned, mediaFallbackAllowed, signedSuffix } from '$lib/stores/mediaSign.svelte';
import { agentPath, client } from './client';
import type { FileContent, FileListResponse, OKResponse, SearchResponse } from '$lib/types';

/**
 * Fetches the bytes and saves them with a filename. A plain `<a download>` is
 * ignored for cross-origin URLs (a remote dashboard), which would open the file
 * instead of saving it.
 */
async function saveUrl(url: string, name: string) {
	try {
		const response = await fetch(url);
		if (!response.ok) throw new Error(String(response.status));
		const objectUrl = URL.createObjectURL(await response.blob());
		const anchor = document.createElement('a');
		anchor.href = objectUrl;
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
		setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
	} catch {
		// Fall back to navigation when the download cannot be fetched (CORS,
		// offline, or a data URL the browser refuses to fetch).
		const anchor = document.createElement('a');
		anchor.href = url;
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
	}
}

export const files = {
	list: (agent: string, path = '', conversationId = '') => {
		const q = new URLSearchParams({ path });
		if (conversationId) q.set('conversation_id', conversationId);
		return client<FileListResponse>(agentPath(agent, `/fs?${q.toString()}`));
	},
	read: (agent: string, path: string, conversationId = '') => {
		const q = new URLSearchParams({ path });
		if (conversationId) q.set('conversation_id', conversationId);
		return client<FileContent>(agentPath(agent, `/fs/read?${q.toString()}`));
	},
	search: (agent: string, query: string, path = '/home', conversationId = '') => {
		const q = new URLSearchParams({ q: query, path });
		if (conversationId) q.set('conversation_id', conversationId);
		return client<SearchResponse>(agentPath(agent, `/fs/search?${q.toString()}`));
	},
	write: (agent: string, path: string, content: string, conversationId = '') =>
		client<OKResponse>(agentPath(agent, '/fs/write'), {
			method: 'POST',
			body: { path, content, ...(conversationId ? { conversation_id: conversationId } : {}) }
		}),
	mkdir: (agent: string, path: string, conversationId = '') =>
		client<OKResponse>(agentPath(agent, '/fs/mkdir'), {
			method: 'POST',
			body: { path, ...(conversationId ? { conversation_id: conversationId } : {}) }
		}),
	rename: (agent: string, from: string, to: string, conversationId = '') =>
		client<OKResponse>(agentPath(agent, '/fs/rename'), {
			method: 'POST',
			body: { from, to, ...(conversationId ? { conversation_id: conversationId } : {}) }
		}),
	remove: (agent: string, path: string, recursive = false, conversationId = '') => {
		const q = new URLSearchParams({ path, recursive: String(recursive) });
		if (conversationId) q.set('conversation_id', conversationId);
		return client<OKResponse>(agentPath(agent, `/fs?${q.toString()}`), { method: 'DELETE' });
	},
	/**
	 * URL for the raw bytes. Media elements cannot send an Authorization header,
	 * so a short-lived per-path signature is used when /fs/sign has answered;
	 * the session token remains only as the cold-cache fallback.
	 */
	rawUrl: (
		agent: string,
		path: string,
		options: { download?: boolean; conversationId?: string } = {}
	) => {
		const convId = options.conversationId ?? '';
		const query = new URLSearchParams({ path });
		if (options.download) query.set('download', '1');
		const suffix = signedSuffix(agent, path, convId);
		if (suffix) {
			return `${getApiBase()}${agentPath(agent, `/fs/raw?${query.toString()}&${suffix}`)}`;
		}
		// While signing is still in flight, render a blank data URL: it issues no
		// request and never exposes the session token. The reactive cache then swaps
		// in the signed URL once /fs/sign answers.
		if (!mediaFallbackAllowed(agent)) return 'data:,';
		if (convId) query.set('conversation_id', convId);
		if (accessToken.value) query.set('token', accessToken.value);
		return `${getApiBase()}${agentPath(agent, `/fs/raw?${query.toString()}`)}`;
	},
	download: async (agent: string, path: string, conversationId = '') => {
		await ensureSigned(agent, [path], conversationId);
		await saveUrl(
			files.rawUrl(agent, path, { download: true, conversationId }),
			path.split('/').pop() || 'download'
		);
	},
	downloadUrl: saveUrl
};
