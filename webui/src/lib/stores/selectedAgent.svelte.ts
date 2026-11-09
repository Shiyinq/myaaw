import { browser } from '$app/environment';
import { agents as agentsApi } from '$lib/apis/agents';
import { colorOf, type BloubColor } from '$lib/bloub';
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
	/**
	 * Palette key for the selected agent, already resolved to ink for agents that
	 * never picked one. Every agent-scoped surface reads this instead of working
	 * out the default on its own.
	 */
	get color(): BloubColor {
		const entry =
			agents.find((agent) => agent.name === selected) ?? agents.find((agent) => agent.default);
		return colorOf(entry?.agent_color);
	},
	setAgents(next: AgentInfo[]) {
		agents = next;
		const exists = next.some((agent) => agent.name === selected);
		if (!exists) {
			const fallback = next.find((agent) => agent.default) ?? next[0];
			select(fallback?.name ?? '');
		}
	},
	// Optimistic patch so the UI reflects a start/stop request immediately.
	setRunning(name: string, running: boolean) {
		agents = agents.map((agent) => (agent.name === name ? { ...agent, running } : agent));
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
