import { agents as agentsApi } from '$lib/apis/agents';
import type { SSEEvent } from '$lib/utils/sse';
import type {
	ManagedRuntimesResponse,
	ManagedRuntimeView,
	RuntimeAuthSessionInfo,
	RuntimeDefaultsInput
} from '$lib/types';

export const runtimesManageStore = {
	list(probeAuth = true, refresh = false): Promise<ManagedRuntimesResponse> {
		return agentsApi.managedRuntimes(probeAuth, refresh);
	},
	getOne(id: string): Promise<ManagedRuntimeView> {
		return agentsApi.managedRuntime(id);
	},
	saveDefaults(id: string, payload: RuntimeDefaultsInput): Promise<ManagedRuntimeView> {
		return agentsApi.saveRuntimeDefaults(id, payload);
	},
	streamInstall(
		id: string,
		method: string,
		onEvent: (event: SSEEvent) => void,
		signal?: AbortSignal
	): Promise<void> {
		return agentsApi.streamRuntimeInstall(id, method, onEvent, signal);
	},
	streamUpdate(
		id: string,
		onEvent: (event: SSEEvent) => void,
		signal?: AbortSignal
	): Promise<void> {
		return agentsApi.streamRuntimeUpdate(id, onEvent, signal);
	},
	openAuthSession(
		id: string,
		payload: { mode: 'login' | 'logout'; headless: boolean; cols: number; rows: number }
	): Promise<RuntimeAuthSessionInfo> {
		return agentsApi.openRuntimeAuthSession(id, payload);
	},
	streamAuthSession(id: string, sid: string, signal: AbortSignal): Promise<Response> {
		return agentsApi.streamRuntimeAuthSession(id, sid, signal);
	},
	inputAuthSession(id: string, sid: string, data: Uint8Array): Promise<Response> {
		return agentsApi.inputRuntimeAuthSession(id, sid, data);
	},
	resizeAuthSession(id: string, sid: string, cols: number, rows: number) {
		return agentsApi.resizeRuntimeAuthSession(id, sid, cols, rows);
	},
	closeAuthSession(id: string, sid: string) {
		return agentsApi.closeRuntimeAuthSession(id, sid);
	}
};
