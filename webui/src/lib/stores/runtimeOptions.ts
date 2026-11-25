import { agents as agentsApi } from '$lib/apis/agents';
import type { RuntimeOptions } from '$lib/types';

const cache = new Map<string, RuntimeOptions>();
const inflight = new Map<string, Promise<RuntimeOptions>>();

export function peekRuntimeOptions(agent: string): RuntimeOptions | null {
	return cache.get(agent) ?? null;
}

export async function loadRuntimeOptions(agent: string, refresh = false): Promise<RuntimeOptions> {
	if (!refresh) {
		const cached = cache.get(agent);
		if (cached) return cached;
		const pending = inflight.get(agent);
		if (pending) return pending;
	}

	const req = agentsApi
		.runtimeOptions(agent, refresh)
		.then((opts) => {
			cache.set(agent, opts);
			return opts;
		})
		.finally(() => {
			if (inflight.get(agent) === req) {
				inflight.delete(agent);
			}
		});

	inflight.set(agent, req);
	return req;
}
