import { SERVER_STATUS_STALE_TIME } from '$lib/constants/query';
import { server as serverApi } from '$lib/apis/server';
import type { ServerStatus } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, ServerStatus>({
	key: (args) => `server:${args.agent}`,
	fetcher: (args) => serverApi.status(args.agent),
	staleTime: SERVER_STATUS_STALE_TIME
});

/**
 * The agent server's running state. Starting / stopping a server is a mutation
 * that reconciles the cached status in the background, so the badge never
 * disappears while the command runs.
 */
export const serverStore = {
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
	ensure(agent: string) {
		return query.ensure({ agent });
	},
	fetchStatus(agent: string) {
		return serverApi.status(agent);
	},
	refresh() {
		return query.refresh();
	},
	async start(agent: string) {
		await serverApi.start(agent);
		await query.refresh({ agent });
	},
	async stop(agent: string) {
		await serverApi.stop(agent);
		await query.refresh({ agent });
	},
	async restart(agent: string) {
		await serverApi.restart(agent);
		await query.refresh({ agent });
	}
};
