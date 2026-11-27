import { browser } from '$app/environment';
import { terminalStore } from './terminal.svelte';

export interface TerminalMuxHandlers {
	onData: (bytes: Uint8Array) => void;
	onClosed: () => void;
	onFallback: () => void;
}

interface MuxFrame {
	id: string;
	data?: string;
	closed?: boolean;
}

interface AgentMuxState {
	subs: Map<string, TerminalMuxHandlers>;
	controller: AbortController | null;
	debounceTimer: ReturnType<typeof setTimeout> | null;
	retryTimer: ReturnType<typeof setTimeout> | null;
	generation: number;
	unsupported: boolean;
}

const agents = new Map<string, AgentMuxState>();

function abortAllStreams() {
	for (const state of agents.values()) {
		state.generation += 1;
		if (state.debounceTimer) {
			clearTimeout(state.debounceTimer);
			state.debounceTimer = null;
		}
		if (state.retryTimer) {
			clearTimeout(state.retryTimer);
			state.retryTimer = null;
		}
		state.controller?.abort();
		state.controller = null;
	}
}

if (browser) {
	window.addEventListener('pagehide', abortAllStreams);
}

function decodeBase64(encoded: string): Uint8Array {
	const binary = atob(encoded);
	const out = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i += 1) {
		out[i] = binary.charCodeAt(i);
	}
	return out;
}

function getOrCreate(agent: string): AgentMuxState {
	let state = agents.get(agent);
	if (!state) {
		state = {
			subs: new Map(),
			controller: null,
			debounceTimer: null,
			retryTimer: null,
			generation: 0,
			unsupported: false
		};
		agents.set(agent, state);
	}
	return state;
}

function scheduleReconnect(agent: string, state: AgentMuxState) {
	if (state.debounceTimer) clearTimeout(state.debounceTimer);
	state.debounceTimer = setTimeout(() => {
		state.debounceTimer = null;
		void connect(agent, state);
	}, 20);
}

function scheduleRetry(agent: string, state: AgentMuxState, gen: number) {
	if (state.generation !== gen || state.subs.size === 0 || state.unsupported) return;
	if (state.retryTimer) clearTimeout(state.retryTimer);
	state.retryTimer = setTimeout(() => {
		state.retryTimer = null;
		if (state.generation === gen && state.subs.size > 0 && !state.unsupported) {
			void connect(agent, state);
		}
	}, 1000);
}

function dispatchEventBlock(state: AgentMuxState, block: string) {
	const lines = block.split(/\r?\n/);
	const dataLines: string[] = [];
	for (const line of lines) {
		if (line.startsWith('data:')) {
			dataLines.push(line.slice(5).trimStart());
		}
	}
	if (dataLines.length === 0) return;

	let frame: MuxFrame;
	try {
		frame = JSON.parse(dataLines.join('\n')) as MuxFrame;
	} catch {
		return;
	}
	if (!frame || typeof frame.id !== 'string') return;
	const sub = state.subs.get(frame.id);
	if (!sub) return;

	if (frame.data) {
		try {
			sub.onData(decodeBase64(frame.data));
		} catch {
			// Ignore malformed base64 chunks.
		}
	}
	if (frame.closed) {
		state.subs.delete(frame.id);
		sub.onClosed();
	}
}

async function connect(agent: string, state: AgentMuxState) {
	state.generation += 1;
	const gen = state.generation;

	if (state.retryTimer) {
		clearTimeout(state.retryTimer);
		state.retryTimer = null;
	}
	state.controller?.abort();
	state.controller = null;

	const ids = Array.from(state.subs.keys());
	if (ids.length === 0) {
		if (!state.unsupported) agents.delete(agent);
		return;
	}

	const controller = new AbortController();
	state.controller = controller;

	let response: Response;
	try {
		response = await terminalStore.streamMux(agent, ids, controller.signal);
	} catch {
		if (!controller.signal.aborted) {
			scheduleRetry(agent, state, gen);
		}
		return;
	}

	if (state.generation !== gen) return;

	if (response.status === 404) {
		// Older backend without /terminal-stream: transparently fall back to legacy streams.
		state.unsupported = true;
		const existing = Array.from(state.subs.values());
		state.subs.clear();
		for (const sub of existing) {
			sub.onFallback();
		}
		return;
	}

	if (!response.ok || !response.body) {
		scheduleRetry(agent, state, gen);
		return;
	}

	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';

	try {
		while (true) {
			const { value, done } = await reader.read();
			if (done) break;
			if (state.generation !== gen) break;
			buffer += decoder.decode(value, { stream: true });

			let boundary = buffer.indexOf('\n\n');
			while (boundary !== -1) {
				const block = buffer.slice(0, boundary);
				buffer = buffer.slice(boundary + 2);
				if (block.trim()) dispatchEventBlock(state, block);
				boundary = buffer.indexOf('\n\n');
			}
		}
	} catch {
		// Aborted intentionally on subscribe/unsubscribe or transient network drop.
	}

	if (state.generation === gen && state.subs.size > 0 && !controller.signal.aborted) {
		scheduleRetry(agent, state, gen);
	}
}

/**
 * Multiplexes all open terminal streams for an agent over a single HTTP/1.1 SSE
 * connection so opening up to 8 terminal tabs never exhausts the browser's
 * 6-connection-per-origin limit or blocks keystrokes/API requests.
 */
export const terminalMux = {
	subscribe(agent: string, sessionId: string, handlers: TerminalMuxHandlers): () => void {
		const state = getOrCreate(agent);
		if (state.unsupported) {
			handlers.onFallback();
			return () => {};
		}

		state.subs.set(sessionId, handlers);
		scheduleReconnect(agent, state);

		return () => {
			const current = agents.get(agent);
			if (!current) return;
			if (current.subs.get(sessionId) === handlers) {
				current.subs.delete(sessionId);
				if (current.subs.size === 0) {
					// Immediately close the SSE connection when the last terminal detaches
					// so the backend marks its sessions detached right away.
					current.generation += 1;
					if (current.debounceTimer) {
						clearTimeout(current.debounceTimer);
						current.debounceTimer = null;
					}
					if (current.retryTimer) {
						clearTimeout(current.retryTimer);
						current.retryTimer = null;
					}
					current.controller?.abort();
					current.controller = null;
					if (!current.unsupported) agents.delete(agent);
				} else {
					scheduleReconnect(agent, current);
				}
			}
		};
	}
};
