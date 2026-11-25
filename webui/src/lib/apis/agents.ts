import { agentPath, client, request } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type {
	AgentInfo,
	AgentsResponse,
	BotsSummaryResponse,
	CapabilitiesView,
	ManagedRuntimesResponse,
	ManagedRuntimeView,
	OKResponse,
	OnboardRequest,
	OnboardResponse,
	OnboardRuntime,
	RuntimeAuthSessionInfo,
	RuntimeDefaultsInput,
	RuntimeOptions,
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
	runtimes: () => client<RuntimesResponse>('/runtimes'),
	// Full external runtime management view (install status, auth status, global defaults).
	managedRuntimes: (probeAuth = true, refresh = false) => {
		const params = new URLSearchParams();
		if (!probeAuth) params.set('probe_auth', '0');
		if (refresh) params.set('refresh', '1');
		const qs = params.toString();
		return client<ManagedRuntimesResponse>(qs ? `/runtimes/manage?${qs}` : '/runtimes/manage');
	},
	managedRuntime: (id: string) =>
		client<ManagedRuntimeView>(`/runtimes/${encodeURIComponent(id)}/manage`),
	saveRuntimeDefaults: (id: string, payload: RuntimeDefaultsInput) =>
		client<ManagedRuntimeView>(`/runtimes/${encodeURIComponent(id)}/defaults`, {
			method: 'PUT',
			body: payload
		}),
	streamRuntimeInstall: (
		id: string,
		method: string,
		onEvent: (event: SSEEvent) => void,
		signal?: AbortSignal
	) =>
		streamSSE(`/runtimes/${encodeURIComponent(id)}/install`, {
			method: 'POST',
			body: { method },
			onEvent,
			signal
		}),
	streamRuntimeUpdate: (id: string, onEvent: (event: SSEEvent) => void, signal?: AbortSignal) =>
		streamSSE(`/runtimes/${encodeURIComponent(id)}/update`, {
			method: 'POST',
			onEvent,
			signal
		}),
	openRuntimeAuthSession: (
		id: string,
		payload: { mode: 'login' | 'logout'; headless: boolean; cols: number; rows: number }
	) =>
		client<RuntimeAuthSessionInfo>(`/runtimes/${encodeURIComponent(id)}/auth/session`, {
			method: 'POST',
			body: payload
		}),
	streamRuntimeAuthSession: (id: string, sid: string, signal: AbortSignal) =>
		request(`/runtimes/${encodeURIComponent(id)}/auth/session/${encodeURIComponent(sid)}/stream`, {
			signal
		}),
	inputRuntimeAuthSession: (id: string, sid: string, data: Uint8Array) =>
		request(`/runtimes/${encodeURIComponent(id)}/auth/session/${encodeURIComponent(sid)}/input`, {
			method: 'POST',
			body: data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer,
			headers: { 'Content-Type': 'application/octet-stream' }
		}),
	resizeRuntimeAuthSession: (id: string, sid: string, cols: number, rows: number) =>
		client<OKResponse>(
			`/runtimes/${encodeURIComponent(id)}/auth/session/${encodeURIComponent(sid)}/resize`,
			{
				method: 'POST',
				body: { cols, rows }
			}
		),
	closeRuntimeAuthSession: (id: string, sid: string) =>
		client<OKResponse>(
			`/runtimes/${encodeURIComponent(id)}/auth/session/${encodeURIComponent(sid)}`,
			{ method: 'DELETE' }
		),
	// Live dynamic models, thinking levels, session modes, and approval modes for an agent's runtime.
	runtimeOptions: (name: string, refresh = false) =>
		client<RuntimeOptions>(
			agentPath(name, refresh ? '/runtime/options?refresh=1' : '/runtime/options')
		),
	// Same discovery for a runtime selection that is not saved yet (New Agent form / Settings).
	probeRuntimeOptions: (payload: OnboardRuntime, refresh = false) =>
		client<RuntimeOptions>(refresh ? '/runtime/options?refresh=1' : '/runtime/options', {
			method: 'POST',
			body: payload
		})
};
