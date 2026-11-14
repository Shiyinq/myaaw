import { context as contextApi } from '$lib/apis/context';
import type { ContextContent, ContextFilesResponse } from '$lib/types';
import { createQueryStore } from './query.svelte';

const list = createQueryStore<{ agent: string }, ContextFilesResponse>({
	key: (args) => `context:list:${args.agent}`,
	fetcher: (args) => contextApi.list(args.agent)
});

const read = createQueryStore<{ agent: string; path: string }, ContextContent>({
	key: (args) => `context:read:${args.agent}:${args.path}`,
	fetcher: (args) => contextApi.read(args.agent, args.path)
});

export const contextStore = {
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
	ensure(agent: string) {
		return list.ensure({ agent });
	},
	refresh() {
		return list.refresh();
	},
	ensureFile(agent: string, path: string) {
		return read.ensure({ agent, path });
	},
	/** Plain reads for surfaces that are not the active query (focus panel). */
	fetchList(agent: string) {
		return contextApi.list(agent);
	},
	fetchFile(agent: string, path: string) {
		return contextApi.read(agent, path);
	},
	async write(agent: string, path: string, content: string) {
		const saved = await contextApi.write(agent, path, content);
		read.write({ agent, path }, saved);
		await list.refresh({ agent });
		return saved;
	}
};
