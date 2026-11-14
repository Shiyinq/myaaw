import { terminal as terminalApi } from '$lib/apis/terminal';
import type { TerminalInfo, TerminalStatus } from '$lib/types';

/**
 * The terminal is a live PTY, not a cached read: every call is an explicit
 * action. It still lives in a store so no component imports `lib/apis` directly
 * and the streaming/cleanup contract stays in one place.
 */
export const terminalStore = {
	status(agent: string): Promise<TerminalStatus> {
		return terminalApi.status(agent);
	},
	get(agent: string, id: string): Promise<TerminalInfo> {
		return terminalApi.get(agent, id);
	},
	open(agent: string, cols: number, rows: number): Promise<TerminalInfo> {
		return terminalApi.open(agent, cols, rows);
	},
	input(agent: string, id: string, data: Uint8Array) {
		return terminalApi.input(agent, id, data);
	},
	resize(agent: string, id: string, cols: number, rows: number) {
		return terminalApi.resize(agent, id, cols, rows);
	},
	close(agent: string, id: string) {
		return terminalApi.close(agent, id);
	},
	heartbeat(agent: string, id: string) {
		return terminalApi.heartbeat(agent, id);
	},
	stream(agent: string, id: string, signal: AbortSignal) {
		return terminalApi.stream(agent, id, signal);
	}
};
