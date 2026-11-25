import { RUNTIMES_MANAGE_STALE_TIME } from '$lib/constants/query';
import { agents as agentsApi } from '$lib/apis/agents';
import { apiBase } from '$lib/stores/apiBase.svelte';
import { createQueryStore } from './query.svelte';
import type { SSEEvent } from '$lib/utils/sse';
import type {
	ManagedRuntimesResponse,
	ManagedRuntimeView,
	RuntimeAuthSessionInfo,
	RuntimeDefaultsInput
} from '$lib/types';

const managed = createQueryStore<Record<string, never>, ManagedRuntimesResponse>({
	key: () => `runtimes:manage:${apiBase.value}`,
	fetcher: () => agentsApi.managedRuntimes(true, false),
	staleTime: RUNTIMES_MANAGE_STALE_TIME
});

let forceRefreshing = $state(false);
let forceError = $state('');

function patchOne(updated: ManagedRuntimeView) {
	managed.setData((prev) => {
		if (!prev) return prev;
		return {
			...prev,
			runtimes: prev.runtimes.map((item) => (item.id === updated.id ? updated : item))
		};
	});
}

export const runtimesManageStore = {
	query: managed,
	get data() {
		return managed.data;
	},
	get loading() {
		return managed.loading;
	},
	get refreshing() {
		return managed.refreshing || forceRefreshing;
	},
	get error() {
		return forceError || managed.error;
	},
	list(probeAuth = true, refresh = false): Promise<ManagedRuntimesResponse> {
		return agentsApi.managedRuntimes(probeAuth, refresh);
	},
	/** Force a live backend probe (?refresh=1) while keeping cached cards on screen. */
	async refresh(): Promise<void> {
		if (forceRefreshing) return;
		forceRefreshing = true;
		forceError = '';
		try {
			const fresh = await agentsApi.managedRuntimes(true, true);
			managed.write({}, fresh);
		} catch (err) {
			forceError = err instanceof Error ? err.message : String(err);
		} finally {
			forceRefreshing = false;
		}
	},
	replaceOne(updated: ManagedRuntimeView) {
		patchOne(updated);
	},
	async getOne(id: string): Promise<ManagedRuntimeView> {
		const updated = await agentsApi.managedRuntime(id);
		patchOne(updated);
		return updated;
	},
	async saveDefaults(id: string, payload: RuntimeDefaultsInput): Promise<ManagedRuntimeView> {
		const updated = await agentsApi.saveRuntimeDefaults(id, payload);
		patchOne(updated);
		return updated;
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
	},
	reset() {
		forceError = '';
		managed.reset();
	}
};
