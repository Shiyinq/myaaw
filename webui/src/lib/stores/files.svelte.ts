import { files as filesApi } from '$lib/apis/files';
import type { FileContent, FileListResponse, SearchResponse } from '$lib/types';
import { createQueryStore } from './query.svelte';

function dirOf(path: string): string {
	const index = path.lastIndexOf('/');
	return index <= 0 ? '/' : path.slice(0, index);
}

const list = createQueryStore<{ agent: string; path: string }, FileListResponse>({
	key: (args) => `files:list:${args.agent}:${args.path}`,
	fetcher: (args) => filesApi.list(args.agent, args.path)
});

const read = createQueryStore<{ agent: string; path: string }, FileContent>({
	key: (args) => `files:read:${args.agent}:${args.path}`,
	fetcher: (args) => filesApi.read(args.agent, args.path)
});

export const filesStore = {
	list,
	read,
	get data() {
		return list.data;
	},
	get loading() {
		return list.loading;
	},
	get refreshing() {
		return list.refreshing;
	},
	get error() {
		return list.error;
	},
	get status() {
		return list.status;
	},
	ensure(agent: string, path = '') {
		return list.ensure({ agent, path });
	},
	refresh() {
		return list.refresh();
	},
	ensureFile(agent: string, path: string) {
		return read.ensure({ agent, path });
	},
	/** Direct directory listing for callers that aggregate multiple paths without mutating the browser query. */
	listDir(agent: string, path = '', conversationId = ''): Promise<FileListResponse> {
		return filesApi.list(agent, path, conversationId);
	},
	/** Direct file read for modal previews outside the Files page editor state. */
	readFile(agent: string, path: string, conversationId = ''): Promise<FileContent> {
		return filesApi.read(agent, path, conversationId);
	},
	async search(
		agent: string,
		query: string,
		path = '/home',
		conversationId = ''
	): Promise<SearchResponse> {
		return filesApi.search(agent, query, path, conversationId);
	},
	async write(
		agent: string,
		path: string,
		content: string,
		dir = dirOf(path),
		conversationId = ''
	) {
		await filesApi.write(agent, path, content, conversationId);
		if (!conversationId) {
			read.invalidate({ agent, path });
			await list.refresh({ agent, path: dir });
		}
	},
	async mkdir(agent: string, path: string, dir = dirOf(path), conversationId = '') {
		await filesApi.mkdir(agent, path, conversationId);
		if (!conversationId) {
			await list.refresh({ agent, path: dir });
		}
	},
	async rename(agent: string, from: string, to: string, dir = dirOf(from), conversationId = '') {
		await filesApi.rename(agent, from, to, conversationId);
		if (!conversationId) {
			read.invalidate({ agent, path: from });
			read.invalidate({ agent, path: to });
			await list.refresh({ agent, path: dir });
		}
	},
	async remove(
		agent: string,
		path: string,
		dir = dirOf(path),
		recursive = false,
		conversationId = ''
	) {
		await filesApi.remove(agent, path, recursive, conversationId);
		if (!conversationId) {
			read.invalidate({ agent, path });
			await list.refresh({ agent, path: dir });
		}
	},
	rawUrl(
		agent: string,
		path: string,
		options: { download?: boolean; conversationId?: string } = {}
	) {
		return filesApi.rawUrl(agent, path, options);
	},
	download(agent: string, path: string, conversationId = '') {
		return filesApi.download(agent, path, conversationId);
	},
	downloadUrl(url: string, name: string) {
		return filesApi.downloadUrl(url, name);
	}
};
