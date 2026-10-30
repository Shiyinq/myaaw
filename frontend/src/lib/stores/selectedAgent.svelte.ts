import { browser } from '$app/environment';
import { agents as agentsApi } from '$lib/apis/agents';
import { SELECTED_AGENT_KEY } from '$lib/constants';
import { getErrorMessage } from '$lib/utils/api';
import type { AgentInfo } from '$lib/types';

let agents = $state<AgentInfo[]>([]);
let selected = $state(browser ? (localStorage.getItem(SELECTED_AGENT_KEY) ?? '') : '');
let loading = $state(false);
let error = $state('');

function select(name: string) {
	selected = name;
	if (browser) {
		if (name) {
			localStorage.setItem(SELECTED_AGENT_KEY, name);
		} else {
			localStorage.removeItem(SELECTED_AGENT_KEY);
		}
	}
}

export const agentSelection = {
	get agents() {
		return agents;
	},
	get selected() {
		return selected;
	},
	get loading() {
		return loading;
	},
	get error() {
		return error;
	},
	get current(): AgentInfo | undefined {
		return agents.find((agent) => agent.name === selected) ?? agents.find((agent) => agent.default);
	},
	setAgents(next: AgentInfo[]) {
		agents = next;
		const exists = next.some((agent) => agent.name === selected);
		if (!exists) {
			const fallback = next.find((agent) => agent.default) ?? next[0];
			select(fallback?.name ?? '');
		}
	},
	select,
	async load(force = false) {
		if (loading) return;
		if (!force && agents.length > 0) return;
		loading = true;
		error = '';
		try {
			const { agents: list } = await agentsApi.list();
			agentSelection.setAgents(list);
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}
};
