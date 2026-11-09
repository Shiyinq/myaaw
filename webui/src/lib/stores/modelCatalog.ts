import { providers as providersApi } from '$lib/apis/providers';
import { getErrorMessage } from '$lib/utils/api';

export interface CatalogProvider {
	name: string;
	type: string;
	model: string;
}

export interface ModelGroup {
	name: string;
	type: string;
	models: string[];
	warning?: string;
}

interface CacheEntry {
	signature: string;
	groups: ModelGroup[];
}

// Live model discovery is a network round trip per provider, so the result is
// cached for the lifetime of the page load. Moving between routes keeps this
// module alive, while a full refresh clears it. A provider appearing,
// disappearing or changing type invalidates the entry.
const cache = new Map<string, CacheEntry>();

function signature(providers: CatalogProvider[]): string {
	return providers
		.map((provider) => `${provider.name}:${provider.type}`)
		.sort()
		.join('|');
}

/** Keeps the configured model selectable even when the live list omits it. */
function ensureCurrent(groups: ModelGroup[], providers: CatalogProvider[]): ModelGroup[] {
	const current = new Map(providers.map((provider) => [provider.name, provider.model]));
	return groups.map((group) => {
		const model = current.get(group.name) ?? '';
		if (!model || group.models.includes(model)) return group;
		return { ...group, models: [model, ...group.models] };
	});
}

export async function loadModelGroups(
	agent: string,
	providers: CatalogProvider[]
): Promise<ModelGroup[]> {
	const key = signature(providers);
	const hit = cache.get(agent);
	if (hit && hit.signature === key) return ensureCurrent(hit.groups, providers);

	const groups = await Promise.all(
		providers.map(async (provider): Promise<ModelGroup> => {
			try {
				const view = await providersApi.models(agent, provider.name);
				const models = view.models ?? [];
				if (provider.model && !models.includes(provider.model)) {
					models.unshift(provider.model);
				}
				return {
					name: provider.name,
					type: provider.type,
					models,
					warning: view.warning
				};
			} catch (err) {
				// A provider whose key or connection is broken still shows its
				// configured model, so the menu never hides the current choice.
				return {
					name: provider.name,
					type: provider.type,
					models: provider.model ? [provider.model] : [],
					warning: getErrorMessage(err)
				};
			}
		})
	);
	cache.set(agent, { signature: key, groups });
	return groups;
}

/** Drops one agent's cache, or every agent's when `agent` is empty. */
export function clearModelCatalog(agent?: string) {
	if (agent) cache.delete(agent);
	else cache.clear();
}
