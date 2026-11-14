import { cron as cronApi } from '$lib/apis/cron';
import type {
	CronGlobalHistoryResponse,
	CronHistoryResponse,
	CronJob,
	CronJobInput,
	CronJobsResponse
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const list = createQueryStore<{ agent: string }, CronJobsResponse>({
	key: (args) => `cron:list:${args.agent}`,
	fetcher: (args) => cronApi.list(args.agent)
});

const history = createQueryStore<{ agent: string; id: string }, CronHistoryResponse>({
	key: (args) => `cron:history:${args.agent}:${args.id}`,
	fetcher: (args) => cronApi.history(args.agent, args.id, 20)
});

const historyAll = createQueryStore<
	{ agent: string; limit: number; offset: number },
	CronGlobalHistoryResponse
>({
	key: (args) => `cron:runs:${args.agent}:${args.limit}:${args.offset}`,
	fetcher: (args) => cronApi.historyAll(args.agent, { limit: args.limit, offset: args.offset })
});

export const cronStore = {
	list,
	history,
	historyAll,
	get data() {
		return list.data;
	},
	get jobs(): CronJob[] {
		return list.data?.jobs ?? [];
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
	ensureHistory(agent: string, id: string) {
		return history.ensure({ agent, id });
	},
	/** Paged reads that the page accumulates locally. */
	fetchHistory(agent: string, id: string) {
		return cronApi.history(agent, id, 20);
	},
	fetchRuns(agent: string, limit: number, offset: number) {
		return cronApi.historyAll(agent, { limit, offset });
	},
	ensureRuns(agent: string, limit = 20, offset = 0) {
		return historyAll.ensure({ agent, limit, offset });
	},
	async add(agent: string, payload: CronJobInput) {
		const job = await cronApi.add(agent, payload);
		await list.refresh({ agent });
		return job;
	},
	async remove(agent: string, id: string) {
		await cronApi.remove(agent, id);
		history.reset({ agent, id });
		await list.refresh({ agent });
	},
	async run(agent: string, id: string) {
		await cronApi.run(agent, id);
	}
};
