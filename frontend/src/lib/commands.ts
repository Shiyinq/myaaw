import type { ComponentType } from 'svelte';
import type { Icon } from 'lucide-svelte';

/**
 * One entry of the command palette. Navigation entries carry `href`; anything
 * that acts on the current screen carries `run`.
 */
export interface Command {
	id: string;
	/** Display name, from i18n unless `name` is a literal such as an agent. */
	labelKey?: string;
	name?: string;
	descriptionKey?: string;
	/** Values interpolated into `labelKey`, e.g. the agent name. */
	params?: Record<string, string | number>;
	aliases?: string[];
	group: 'navigate' | 'agent' | 'action';
	icon: ComponentType<Icon>;
	href?: string;
	run?: () => void | Promise<void>;
	destructive?: boolean;
	/** Marks the agent the shell is already using. */
	current?: boolean;
}

const DIACRITICS = /[\u0300-\u036f]/g;

/** Lowercase, accent-free text, so "pengaturan" and "Pengaturán" both match. */
export function normalize(value: string): string {
	return value.toLowerCase().normalize('NFD').replace(DIACRITICS, '');
}

/**
 * Scores how well a query matches the strings describing one command: every
 * token has to appear somewhere, and a label that starts with the token beats a
 * match buried in the description.
 */
export function scoreMatch(haystack: string[], query: string): number {
	const tokens = normalize(query).split(/\s+/).filter(Boolean);
	if (tokens.length === 0) return 1;

	let total = 0;
	for (const token of tokens) {
		let best = 0;
		for (const raw of haystack) {
			const value = normalize(raw);
			if (!value) continue;
			if (value === token) best = Math.max(best, 100);
			else if (value.startsWith(token)) best = Math.max(best, 80);
			else if (value.split(/\s+/).some((word) => word.startsWith(token))) best = Math.max(best, 70);
			else if (value.includes(token)) best = Math.max(best, 50);
		}
		if (best === 0) return 0;
		total += best;
	}
	return total;
}
