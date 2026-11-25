import { CAPABILITIES_STALE_TIME, RUNTIMES_MANAGE_STALE_TIME } from '$lib/constants/query';
import { agents as agentsApi } from '$lib/apis/agents';
import { apiBase } from '$lib/stores/apiBase.svelte';
import type {
	AgentInfo,
	CapabilitiesView,
	OnboardRequest,
	OnboardResponse,
	RuntimesResponse
} from '$lib/types';
import { createQueryStore } from './query.svelte';
import { agentSelection } from './selectedAgent.svelte';

const capabilities = createQueryStore<Record<string, never>, CapabilitiesView>({
	key: () => `capabilities:${apiBase.value}`,
	fetcher: () => agentsApi.capabilities(),
	staleTime: CAPABILITIES_STALE_TIME
});

const runtimes = createQueryStore<Record<string, never>, RuntimesResponse>({
	key: () => `runtimes:${apiBase.value}`,
	fetcher: () => agentsApi.runtimes(),
	staleTime: RUNTIMES_MANAGE_STALE_TIME
});

export const agentsStore = {
	capabilities,
	runtimes,
	get capabilitiesData() {
		return capabilities.data;
	},
	get capabilitiesLoading() {
		return capabilities.loading;
	},
	get capabilitiesError() {
		return capabilities.error;
	},
	get runtimesData() {
		return runtimes.data;
	},
	ensureCapabilities() {
		return capabilities.ensure({});
	},
	ensureRuntimes() {
		return runtimes.ensure({});
	},
	fetchCapabilities() {
		return agentsApi.capabilities();
	},
	fetchRuntimes() {
		return agentsApi.runtimes();
	},
	/** Fetch the fleet and push it into the shared selection store. */
	async reload(): Promise<AgentInfo[]> {
		const { agents } = await agentsApi.list();
		agentSelection.setAgents(agents);
		return agents;
	},
	get(name: string) {
		return agentsApi.get(name);
	},
	async setDefault(name: string) {
		await agentsApi.setDefault(name);
		await agentSelection.load(true);
	},
	async remove(name: string) {
		await agentsApi.remove(name);
		await agentSelection.load(true);
	},
	onboard(name: string, payload: OnboardRequest): Promise<OnboardResponse> {
		return agentsApi.onboard(name, payload);
	}
};
