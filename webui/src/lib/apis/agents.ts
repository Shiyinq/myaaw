import { agentPath, client } from './client';
import type {
	AgentInfo,
	AgentsResponse,
	BotsSummaryResponse,
	CapabilitiesView,
	OKResponse,
	OnboardRequest,
	OnboardResponse,
	RuntimesResponse
} from '$lib/types';

export const agents = {
	list: () => client<AgentsResponse>('/agents'),
	summary: () => client<BotsSummaryResponse>('/bots/summary'),
	get: (name: string) => client<AgentInfo>(agentPath(name, '')),
	remove: (name: string) => client<OKResponse>(agentPath(name, ''), { method: 'DELETE' }),
	setDefault: (name: string) => client<OKResponse>(agentPath(name, '/default'), { method: 'POST' }),
	onboard: (name: string, payload: OnboardRequest) =>
		client<OnboardResponse>(agentPath(name, '/onboard'), { method: 'POST', body: payload }),
	// Provider types plus the skills and tools an agent can be created with.
	capabilities: () => client<CapabilitiesView>('/capabilities'),
	// Supported agent runtimes (native Myaaw + external CLIs) with host PATH detection.
	runtimes: () => client<RuntimesResponse>('/runtimes')
};
