import { channels as channelsApi } from '$lib/apis/channels';
import type { ChannelInput, ChannelsView } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, ChannelsView>({
	key: (args) => `channels:${args.agent}`,
	fetcher: (args) => channelsApi.list(args.agent)
});

export const channelsStore = {
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
	refresh() {
		return query.refresh();
	},
	async upsert(agent: string, channel: 'telegram' | 'discord', payload: ChannelInput) {
		await channelsApi.upsert(agent, channel, payload);
		await query.refresh({ agent });
	},
	async disable(agent: string, channel: 'telegram' | 'discord') {
		await channelsApi.disable(agent, channel);
		await query.refresh({ agent });
	}
};
