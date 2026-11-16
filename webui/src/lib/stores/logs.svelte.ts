import { logs as logsApi } from '$lib/apis/logs';
import type { SSEEvent } from '$lib/utils/sse';
import type { LogsResponse } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, LogsResponse>({
	key: (args) => `logs:${args.agent}`,
	fetcher: (args) => logsApi.list(args.agent)
});

export const logsStore = {
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
	refresh() {
		return query.refresh();
	},
	/** Live tail. Streaming is not a cached read, but it still belongs here. */
	stream(
		agent: string,
		file: string,
		onEvent: (event: SSEEvent) => void,
		options: Parameters<typeof logsApi.stream>[3] = {}
	) {
		return logsApi.stream(agent, file, onEvent, options);
	}
};
