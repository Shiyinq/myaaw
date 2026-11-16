import { browser } from '$app/environment';
import { agentPath, client } from '$lib/apis/client';

interface SignedEntry {
	suffix: string;
	exp: number;
}

interface SignResponse {
	expires_at: number;
	urls: Record<string, string>;
}

/** Signed query suffixes keyed by agent and path. Reactive, so rawUrl re-renders. */
const cache = $state<Record<string, SignedEntry>>({});
/** Signing outcome per agent: absent while pending, then ok or failed. */
const status = $state<Record<string, 'ok' | 'failed'>>({});
/** Paths each page asked for, re-signed when the tab becomes visible again. */
const watched = new Map<string, Set<string>>();
let listening = false;

/** True only when signing is unavailable, so callers may fall back to the token. */
export function mediaFallbackAllowed(agent: string): boolean {
	return status[agent] === 'failed';
}

function key(agent: string, path: string): string {
	return `${agent}\u0000${path}`;
}

function expiryMs(entry: SignedEntry): number {
	return entry.exp * 1000;
}

/** Returns a valid signed suffix, or an empty string when the caller must fall back. */
export function signedSuffix(agent: string, path: string): string {
	const entry = cache[key(agent, path)];
	if (!entry) return '';
	if (expiryMs(entry) - Date.now() < 60_000) return '';
	return entry.suffix;
}

function listenOnce() {
	if (listening || !browser) return;
	listening = true;
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState !== 'visible') return;
		for (const [agent, paths] of watched) {
			void ensureSigned(agent, [...paths]);
		}
	});
}

/** Signs the paths that do not have a usable signature yet, in one request. */
export async function ensureSigned(agent: string, paths: string[]): Promise<void> {
	if (!browser || !agent) return;
	const unique = [...new Set(paths.filter((path) => path && path.startsWith('/')))];
	if (unique.length === 0) return;

	listenOnce();
	const entry = watched.get(agent) ?? new Set<string>();
	unique.forEach((path) => entry.add(path));
	watched.set(agent, entry);

	const wanted = unique.filter((path) => !signedSuffix(agent, path));
	if (wanted.length === 0) return;

	try {
		const response = await client<SignResponse>(agentPath(agent, '/fs/sign'), {
			method: 'POST',
			body: { paths: wanted }
		});
		const exp = Number(response?.expires_at ?? 0);
		for (const [path, suffix] of Object.entries(response?.urls ?? {})) {
			if (!suffix) continue;
			cache[key(agent, path)] = { suffix, exp };
		}
		status[agent] = 'ok';
	} catch {
		// Keep the session-token fallback only when signing is truly unavailable.
		status[agent] = 'failed';
	}
}
