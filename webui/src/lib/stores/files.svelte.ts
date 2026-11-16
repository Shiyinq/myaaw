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
	async search(agent: string, query: string, path = '/home'): Promise<SearchResponse> {
		return filesApi.search(agent, query, path);
	},
	async write(agent: string, path: string, content: string, dir = dirOf(path)) {
		await filesApi.write(agent, path, content);
		read.invalidate({ agent, path });
		await list.refresh({ agent, path: dir });
	},
	async mkdir(agent: string, path: string, dir = dirOf(path)) {
		await filesApi.mkdir(agent, path);
		await list.refresh({ agent, path: dir });
	},
	async rename(agent: string, from: string, to: string, dir = dirOf(from)) {
		await filesApi.rename(agent, from, to);
		read.invalidate({ agent, path: from });
		read.invalidate({ agent, path: to });
		await list.refresh({ agent, path: dir });
	},
	async remove(agent: string, path: string, dir = dirOf(path), recursive = false) {
		await filesApi.remove(agent, path, recursive);
		read.invalidate({ agent, path });
		await list.refresh({ agent, path: dir });
	},
	rawUrl(agent: string, path: string, options: { download?: boolean } = {}) {
		return filesApi.rawUrl(agent, path, options);
	},
	download(agent: string, path: string) {
		return filesApi.download(agent, path);
	},
	downloadUrl(url: string, name: string) {
		return filesApi.downloadUrl(url, name);
	}
};
