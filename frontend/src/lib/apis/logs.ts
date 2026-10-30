import { agentPath, client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type { LogsResponse } from '$lib/types';

export const logs = {
	list: (agent: string) => client<LogsResponse>(agentPath(agent, '/logs')),
	stream: (agent: string, file: string, onEvent: (event: SSEEvent) => void, signal?: AbortSignal) =>
		streamSSE(agentPath(agent, `/logs/stream?file=${encodeURIComponent(file)}`), {
			method: 'GET',
			onEvent,
			signal
		})
};
