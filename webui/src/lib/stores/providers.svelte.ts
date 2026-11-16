import { providers as providersApi } from '$lib/apis/providers';
import type {
	CreateProviderRequest,
	ModelListView,
	ProviderView,
	ProvidersView,
	UpdateProviderRequest
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const list = createQueryStore<{ agent: string }, ProvidersView>({
	key: (args) => `providers:${args.agent}`,
	fetcher: (args) => providersApi.list(args.agent)
});

export const providersStore = {
	list,
	get data() {
		return list.data;
	},
	get providers(): ProviderView[] {
		return list.data?.providers ?? [];
	},
	get defaultProvider(): string {
		return list.data?.default_provider ?? '';
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
	async create(agent: string, payload: CreateProviderRequest) {
		await providersApi.create(agent, payload);
		await list.refresh({ agent });
	},
	async update(agent: string, name: string, payload: UpdateProviderRequest) {
		await providersApi.update(agent, name, payload);
		await list.refresh({ agent });
	},
	async remove(agent: string, name: string) {
		await providersApi.remove(agent, name);
		await list.refresh({ agent });
	},
	async setDefault(agent: string, name: string) {
		await providersApi.setDefault(agent, name);
		list.setData((previous) => (previous ? { ...previous, default_provider: name } : previous));
		await list.refresh({ agent });
	},
	async disconnect(agent: string, name: string) {
		await providersApi.disconnect(agent, name);
		await list.refresh({ agent });
	},
	connectChatGPT(agent: string, credentialId?: string) {
		return providersApi.connectChatGPT(agent, credentialId);
	},
	chatGPTStatus(agent: string, session: string, signal?: AbortSignal) {
		return providersApi.chatGPTStatus(agent, session, signal);
	},
	cancelChatGPT(agent: string, session: string) {
		return providersApi.cancelChatGPT(agent, session);
	},
	modelsForCredentials(payload: Parameters<typeof providersApi.modelsForCredentials>[0]) {
		return providersApi.modelsForCredentials(payload);
	},
	models(agent: string, name: string): Promise<ModelListView> {
		return providersApi.models(agent, name);
	}
};
