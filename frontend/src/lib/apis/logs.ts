import { agentPath, client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type { LogsResponse } from '$lib/types';

export interface LogStreamOptions {
	signal?: AbortSignal;
	/** Called before each automatic reconnect, so callers can drop buffered lines. */
	onReconnect?: (attempt: number) => void;
}

export const logs = {
	list: (agent: string) => client<LogsResponse>(agentPath(agent, '/logs')),
	// The server replays the whole file on every connection, so a reconnect is
	// safe as long as the caller resets its buffer in `onReconnect`.
	stream: (
		agent: string,
		file: string,
		onEvent: (event: SSEEvent) => void,
		options: LogStreamOptions = {}
	) =>
		streamSSE(agentPath(agent, `/logs/stream?file=${encodeURIComponent(file)}`), {
			method: 'GET',
			onEvent,
			signal: options.signal,
			retry: {
				attempts: 5,
				baseDelayMs: 1000,
				maxDelayMs: 8000,
				onRetry: (attempt) => options.onReconnect?.(attempt)
			}
		})
};
