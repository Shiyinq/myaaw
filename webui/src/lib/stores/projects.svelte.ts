import { PROJECT_STATUS_STALE_TIME } from '$lib/constants/query';
import { projects as projectsApi } from '$lib/apis/projects';
import type { ProjectStatus } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string; path: string }, ProjectStatus>({
	key: (args) => `project:${args.agent}:${args.path}`,
	fetcher: (args) => projectsApi.status(args.agent, args.path),
	staleTime: PROJECT_STATUS_STALE_TIME
});

export const projectsStore = {
	query,
	get data() {
		return query.data;
	},
	get loading() {
		return query.loading;
	},
	get refreshing() {
		return query.refreshing;
	},
	get error() {
		return query.error;
	},
	get status() {
		return query.status;
	},
	ensure(agent: string, path: string) {
		return query.ensure({ agent, path });
	},
	fetchStatus(agent: string, path: string) {
		return projectsApi.status(agent, path);
	},
	refresh() {
		return query.refresh();
	},
	async build(agent: string, path: string) {
		const next = await projectsApi.build(agent, path);
		query.write({ agent, path }, next);
		return next;
	}
};
