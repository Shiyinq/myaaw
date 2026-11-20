import { browser } from '$app/environment';
import { agents as agentsApi } from '$lib/apis/agents';
import { colorOf, type BloubColor } from '$lib/bloub';
import { SELECTED_AGENT_KEY } from '$lib/constants';
import { getErrorMessage } from '$lib/utils/api';
import type { AgentInfo } from '$lib/types';

let agents = $state<AgentInfo[]>([]);
let selected = $state(browser ? (localStorage.getItem(SELECTED_AGENT_KEY) ?? '') : '');
let loading = $state(false);
let loaded = $state(false);
let error = $state('');

/**
 * Short-lived optimistic override per agent after start/stop/restart: the daemon
 * process takes ~1s to bind or release its port after the control-plane call
 * returns, so an immediate `/agents` reload must not flip the badge back.
 */
const optimisticRunning = new Map<string, { running: boolean; until: number }>();
const reconcileTimers = new Map<string, ReturnType<typeof setTimeout>>();

function applyOptimistic(list: AgentInfo[]): AgentInfo[] {
	const now = Date.now();
	return list.map((agent) => {
		const entry = optimisticRunning.get(agent.name);
		if (!entry) return agent;
		if (now >= entry.until || agent.running === entry.running) {
			optimisticRunning.delete(agent.name);
			return agent;
		}
		return { ...agent, running: entry.running };
	});
}

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
	get loaded() {
		return loaded;
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
		const merged = applyOptimistic(next);
		agents = merged;
		loaded = true;
		const exists = merged.some((agent) => agent.name === selected);
		if (!exists) {
			const fallback = merged.find((agent) => agent.default) ?? merged[0];
			select(fallback?.name ?? '');
		}
	},
	// Optimistic patch so the UI reflects a start/stop request immediately and
	// holds that state while the daemon finishes binding/releasing its port.
	setRunning(name: string, running: boolean) {
		optimisticRunning.set(name, { running, until: Date.now() + 2500 });
		agents = agents.map((agent) => (agent.name === name ? { ...agent, running } : agent));
		const existingTimer = reconcileTimers.get(name);
		if (existingTimer) clearTimeout(existingTimer);
		const timer = setTimeout(() => {
			reconcileTimers.delete(name);
			void agentSelection.load(true);
		}, 1600);
		reconcileTimers.set(name, timer);
	},
	clearOptimisticRunning(name: string) {
		optimisticRunning.delete(name);
		const existingTimer = reconcileTimers.get(name);
		if (existingTimer) {
			clearTimeout(existingTimer);
			reconcileTimers.delete(name);
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
			loaded = true;
		}
	}
};
