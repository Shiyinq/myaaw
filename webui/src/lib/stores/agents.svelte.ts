import { CAPABILITIES_STALE_TIME } from '$lib/constants/query';
import { agents as agentsApi } from '$lib/apis/agents';
import type { AgentInfo, CapabilitiesView, OnboardRequest, OnboardResponse } from '$lib/types';
import { createQueryStore } from './query.svelte';
import { agentSelection } from './selectedAgent.svelte';

const capabilities = createQueryStore<Record<string, never>, CapabilitiesView>({
	key: () => 'capabilities',
	fetcher: () => agentsApi.capabilities(),
	staleTime: CAPABILITIES_STALE_TIME
});

export const agentsStore = {
	capabilities,
	get capabilitiesData() {
		return capabilities.data;
	},
	get capabilitiesLoading() {
		return capabilities.loading;
	},
	get capabilitiesError() {
		return capabilities.error;
	},
	ensureCapabilities() {
		return capabilities.ensure({});
	},
	fetchCapabilities() {
		return agentsApi.capabilities();
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
