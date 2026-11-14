import { tools as toolsApi } from '$lib/apis/tools';
import type { CustomToolInput, MCPServerInput, ToolsView } from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, ToolsView>({
	key: (args) => `tools:${args.agent}`,
	fetcher: (args) => toolsApi.list(args.agent)
});

/** Patch the cached view in place so a toggle paints before the refetch lands. */
function patchView(agent: string, updater: (view: ToolsView) => ToolsView) {
	const current = query.data;
	if (!current) return;
	query.write({ agent }, updater(current));
}

export const toolsStore = {
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
	fetchView(agent: string) {
		return toolsApi.list(agent);
	},
	refresh() {
		return query.refresh();
	},
	async toggleBuiltin(agent: string, name: string, enabled: boolean) {
		patchView(agent, (view) => ({
			...view,
			builtin: view.builtin.map((tool) => (tool.name === name ? { ...tool, enabled } : tool))
		}));
		try {
			await toolsApi.toggleBuiltin(agent, name, enabled);
		} catch (error) {
			await query.refresh({ agent });
			throw error;
		}
	},
	async createCustom(agent: string, payload: CustomToolInput) {
		await toolsApi.createCustom(agent, payload);
		await query.refresh({ agent });
	},
	async patchCustom(agent: string, name: string, payload: Partial<CustomToolInput>) {
		patchView(agent, (view) => ({
			...view,
			custom: view.custom.map((tool) => (tool.name === name ? { ...tool, ...payload } : tool))
		}));
		try {
			await toolsApi.patchCustom(agent, name, payload);
		} catch (error) {
			await query.refresh({ agent });
			throw error;
		}
	},
	async removeCustom(agent: string, name: string) {
		await toolsApi.removeCustom(agent, name);
		await query.refresh({ agent });
	},
	async createMCP(agent: string, payload: MCPServerInput) {
		await toolsApi.createMCP(agent, payload);
		await query.refresh({ agent });
	},
	async patchMCP(agent: string, name: string, payload: Partial<MCPServerInput>) {
		patchView(agent, (view) => ({
			...view,
			mcp_servers: view.mcp_servers.map((server) =>
				server.name === name
					? {
							...server,
							command: payload.command ?? server.command,
							args: payload.args ?? server.args,
							enabled: payload.enabled ?? server.enabled
						}
					: server
			)
		}));
		try {
			await toolsApi.patchMCP(agent, name, payload);
		} catch (error) {
			await query.refresh({ agent });
			throw error;
		}
	},
	async removeMCP(agent: string, name: string) {
		await toolsApi.removeMCP(agent, name);
		await query.refresh({ agent });
	}
};
