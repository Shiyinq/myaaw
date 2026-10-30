import { agentPath, client, request } from './client';
import type { OKResponse, TerminalInfo, TerminalStatus } from '$lib/types';

export const terminal = {
	status: (agent: string) => client<TerminalStatus>(agentPath(agent, '/terminal')),
	open: (agent: string, cols: number, rows: number) =>
		client<TerminalInfo>(agentPath(agent, '/terminal'), {
			method: 'POST',
			body: { cols, rows }
		}),
	/** Raw keystrokes; the handler writes them straight into the PTY. */
	input: (agent: string, id: string, data: Uint8Array) =>
		request(agentPath(agent, `/terminal/${id}/input`), {
			method: 'POST',
			body: data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer,
			headers: { 'Content-Type': 'application/octet-stream' }
		}),
	resize: (agent: string, id: string, cols: number, rows: number) =>
		client<OKResponse>(agentPath(agent, `/terminal/${id}/resize`), {
			method: 'POST',
			body: { cols, rows }
		}),
	close: (agent: string, id: string, keepalive = false) =>
		client<OKResponse>(agentPath(agent, `/terminal/${id}`), { method: 'DELETE', keepalive }),
	/** Keeps the session from being reaped while the tab stays open. */
	heartbeat: (agent: string, id: string) =>
		client<OKResponse>(agentPath(agent, `/terminal/${id}/heartbeat`), { method: 'POST' }),
	/** Long-lived chunked response with the PTY output. */
	stream: (agent: string, id: string, signal: AbortSignal) =>
		request(agentPath(agent, `/terminal/${id}/stream`), { signal })
};
