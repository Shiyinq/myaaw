import { agentPath, client } from './client';
import type { CustomToolInput, MCPServerInput, OKResponse, ToolsView } from '$lib/types';

export const tools = {
	list: (agent: string) => client<ToolsView>(agentPath(agent, '/tools')),
	toggleBuiltin: (agent: string, name: string, enabled: boolean) =>
		client<OKResponse>(agentPath(agent, `/tools/builtin/${encodeURIComponent(name)}`), {
			method: 'PATCH',
			body: { enabled }
		}),
	createCustom: (agent: string, payload: CustomToolInput) =>
		client<OKResponse>(agentPath(agent, '/tools/custom'), { method: 'POST', body: payload }),
	patchCustom: (agent: string, name: string, payload: Partial<CustomToolInput>) =>
		client<OKResponse>(agentPath(agent, `/tools/custom/${encodeURIComponent(name)}`), {
			method: 'PATCH',
			body: payload
		}),
	removeCustom: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, `/tools/custom/${encodeURIComponent(name)}`), {
			method: 'DELETE'
		}),
	createMCP: (agent: string, payload: MCPServerInput) =>
		client<OKResponse>(agentPath(agent, '/tools/mcp'), { method: 'POST', body: payload }),
	patchMCP: (agent: string, name: string, payload: Partial<MCPServerInput>) =>
		client<OKResponse>(agentPath(agent, `/tools/mcp/${encodeURIComponent(name)}`), {
			method: 'PATCH',
			body: payload
		}),
	removeMCP: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, `/tools/mcp/${encodeURIComponent(name)}`), {
			method: 'DELETE'
		})
};
