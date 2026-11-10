import { agentPath } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';

/** One `conversation.updated` event: a background turn was persisted. */
export interface ConversationUpdatedEvent {
	agent: string;
	conversation_id: string;
	trigger: 'cron' | 'heartbeat' | 'subagent' | string;
	at: string;
}

export const conversationEvents = {
	/**
	 * Streams conversation updates for one agent. The connection is long-lived: a
	 * lost stream (agent restart, network blip) reconnects with backoff instead of
	 * ending, so a background turn is never missed for longer than the retry.
	 */
	stream: (agent: string, onEvent: (event: SSEEvent) => void, signal?: AbortSignal) =>
		streamSSE(agentPath(agent, '/events'), {
			method: 'GET',
			signal,
			onEvent,
			retry: {
				attempts: 100,
				baseDelayMs: 1000,
				maxDelayMs: 15_000
			}
		})
};
