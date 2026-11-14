import { approvals as approvalsApi } from '$lib/apis/approvals';
import type { ApprovalsView } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, ApprovalsView>({
	key: (args) => `approvals:${args.agent}`,
	fetcher: (args) => approvalsApi.get(args.agent)
});

export const approvalsStore = {
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
	async revokeKey(agent: string, key: string, scope: { channel?: string; user?: string } = {}) {
		await approvalsApi.revokeKey(agent, key, scope);
		await query.refresh({ agent });
	},
	async revokeSession(
		agent: string,
		key: string,
		conversation: string,
		scope: { channel?: string; user?: string } = {}
	) {
		await approvalsApi.revokeSession(agent, key, conversation, scope);
		await query.refresh({ agent });
	},
	async clear(agent: string) {
		await approvalsApi.clear(agent);
		await query.refresh({ agent });
	}
};
