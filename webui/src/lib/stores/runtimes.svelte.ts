import { RUNTIMES_MANAGE_STALE_TIME } from '$lib/constants/query';
import { agents as agentsApi } from '$lib/apis/agents';
import { apiBase } from '$lib/stores/apiBase.svelte';
import { agentsStore } from '$lib/stores/agents.svelte';
import { invalidateRuntimeOptions } from '$lib/stores/runtimeOptions';
import { createQueryStore } from './query.svelte';
import type { SSEEvent } from '$lib/utils/sse';
import type {
	ManagedRuntimesResponse,
	ManagedRuntimeView,
	RuntimeAuthSessionInfo,
	RuntimeDefaultsInput
} from '$lib/types';

let forceRefreshing = $state(false);
let forceError = $state('');
let probingAuth = $state(false);

const managed = createQueryStore<Record<string, never>, ManagedRuntimesResponse>({
	key: () => `runtimes:manage:${apiBase.value}`,
	fetcher: async () => {
		// On a cold store, start the full probe_auth=1 request immediately. If the
		// backend cache is already warm, it resolves in a few ms with just 1 HTTP
		// request. Only if it takes >150ms (cold backend) do we fire a fast
		// probe_auth=0 request to paint the cards right away while auth resolves.
		const cold = managed.peek({}) === undefined;
		if (!cold) {
			return agentsApi.managedRuntimes(true, false);
		}
		let fullDone = false;
		const fallbackTimer = setTimeout(() => {
			if (fullDone || managed.peek({}) !== undefined) return;
			probingAuth = true;
			void agentsApi
				.managedRuntimes(false, false)
				.then((fast) => {
					if (!fullDone && managed.peek({}) === undefined) {
						managed.write({}, fast);
					}
				})
				.catch(() => {
					// Ignore fast-phase errors; full request reports any real failure.
				});
		}, 150);
		try {
			const full = await agentsApi.managedRuntimes(true, false);
			fullDone = true;
			clearTimeout(fallbackTimer);
			return full;
		} finally {
			fullDone = true;
			clearTimeout(fallbackTimer);
			probingAuth = false;
		}
	},
	staleTime: RUNTIMES_MANAGE_STALE_TIME
});

function patchOne(updated: ManagedRuntimeView) {
	managed.setData((prev) => {
		if (!prev) return prev;
		return {
			...prev,
			runtimes: prev.runtimes.map((item) => (item.id === updated.id ? updated : item))
		};
	});
	// Keep /app/new runtime discovery and chat composer model lists in sync
	// immediately after installs, updates, logins, logouts, or default edits.
	agentsStore.runtimes.invalidate({});
	invalidateRuntimeOptions();
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
	get probingAuth() {
		return probingAuth;
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
			agentsStore.runtimes.invalidate({});
			invalidateRuntimeOptions();
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
