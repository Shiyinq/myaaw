import { CONFIG_STALE_TIME } from '$lib/constants/query';
import { config as configApi, type ConfigPatch, type ConfigResetRequest } from '$lib/apis/config';
import type { AgentConfig } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, AgentConfig>({
	key: (args) => `config:${args.agent}`,
	fetcher: (args) => configApi.get(args.agent),
	staleTime: CONFIG_STALE_TIME
});

/**
 * One agent's editable configuration. The cache keeps the last value on screen
 * while a background refetch reconciles it, so opening the page again never
 * blanks the form.
 */
export const configStore = {
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
	get updatedAt() {
		return query.updatedAt;
	},
	ensure(agent: string) {
		return query.ensure({ agent });
	},
	/** One-shot read for a page that keeps its own copy (the chat header). */
	fetchConfig(agent: string) {
		return configApi.get(agent);
	},
	refresh() {
		return query.refresh();
	},
	async patch(agent: string, payload: ConfigPatch) {
		const next = await configApi.patch(agent, payload);
		query.write({ agent }, next);
		return next;
	},
	async reset(agent: string) {
		const next = await configApi.reset(agent);
		query.write({ agent }, next);
		return next;
	},
	async resetFields(agent: string, payload: ConfigResetRequest) {
		const next = await configApi.resetFields(agent, payload);
		query.write({ agent }, next);
		return next;
	}
};
