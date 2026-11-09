import { browser } from '$app/environment';
import {
	API_BASE_KEY,
	API_CONNECTIONS_KEY,
	DEFAULT_API_BASE,
	isLoopbackApiBase,
	normalizeApiBase
} from '$lib/constants';
import { apiBase } from './apiBase.svelte';

export interface ApiConnection {
	id: string;
	/** Empty means the same-origin proxy (the local control plane). */
	url: string;
	label: string;
}

function makeId(): string {
	return Math.random().toString(36).slice(2, 10);
}

export function connectionLabel(url: string): string {
	const normalized = normalizeApiBase(url);
	if (normalized === DEFAULT_API_BASE) return 'Local';
	try {
		return new URL(normalized).host;
	} catch {
		return normalized;
	}
}

function loadConnections(): ApiConnection[] {
	if (!browser) return [];
	try {
		const raw = localStorage.getItem(API_CONNECTIONS_KEY);
		if (!raw) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed
			.filter(
				(entry): entry is ApiConnection =>
					typeof entry === 'object' &&
					entry !== null &&
					typeof (entry as ApiConnection).url === 'string' &&
					typeof (entry as ApiConnection).label === 'string'
			)
			.map((entry) => ({
				id: typeof entry.id === 'string' && entry.id ? entry.id : makeId(),
				url: normalizeApiBase(entry.url),
				label: entry.label || connectionLabel(entry.url)
			}));
	} catch {
		return [];
	}
}

let list = $state<ApiConnection[]>(loadConnections());

function persist() {
	if (!browser) return;
	localStorage.setItem(API_CONNECTIONS_KEY, JSON.stringify(list));
}

function dedupe(entries: ApiConnection[]): ApiConnection[] {
	const seen = new Set<string>();
	const out: ApiConnection[] = [];
	for (const entry of entries) {
		const url = normalizeApiBase(entry.url);
		if (seen.has(url)) continue;
		seen.add(url);
		out.push({
			id: entry.id || makeId(),
			url,
			label: entry.label || connectionLabel(url)
		});
	}
	return out;
}

export const connections = {
	get list() {
		return list;
	},
	/**
	 * Seed the saved connections once, from the server's view of this install.
	 * A configured loopback URL is folded into "Local" instead of duplicating it.
	 */
	seedFromServer(apiUrl: string, binaryInstalled: boolean) {
		if (!browser) return;

		const configured = normalizeApiBase(apiUrl || '');
		if (localStorage.getItem(API_CONNECTIONS_KEY) === null) {
			const defaults: ApiConnection[] = [];
			if (binaryInstalled) {
				defaults.push({ id: 'local', url: DEFAULT_API_BASE, label: 'Local' });
			}
			if (configured !== '' && !isLoopbackApiBase(configured)) {
				defaults.push({ id: 'configured', url: configured, label: connectionLabel(configured) });
			}
			list = dedupe(defaults);
			persist();
		}

		// No explicit choice yet: follow the configured remote, otherwise keep the
		// same-origin proxy (which already covers the local control plane).
		if (
			localStorage.getItem(API_BASE_KEY) === null &&
			configured !== '' &&
			!isLoopbackApiBase(configured)
		) {
			apiBase.set(configured);
		}
	},
	add(label: string, url: string): ApiConnection {
		const normalized = normalizeApiBase(url);
		const existing = list.find((entry) => entry.url === normalized);
		if (existing) return existing;

		const entry: ApiConnection = {
			id: makeId(),
			url: normalized,
			label: label.trim() || connectionLabel(normalized)
		};
		list = [...list, entry];
		persist();
		return entry;
	},
	remove(id: string) {
		const entry = list.find((item) => item.id === id);
		if (!entry || entry.url === DEFAULT_API_BASE) {
			// Local is the fallback control plane and must stay available.
			return;
		}
		list = list.filter((item) => item.id !== id);
		persist();
	}
};
