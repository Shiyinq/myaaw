import { agentPath, client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type {
	ApprovalDecision,
	ChatRequest,
	ChatResponse,
	ChatRunInfo,
	OKResponse
} from '$lib/types';

export const chat = {
	send: (agent: string, payload: ChatRequest) =>
		client<ChatResponse>(agentPath(agent, '/chat'), { method: 'POST', body: payload }),
	stream: (
		agent: string,
		payload: ChatRequest,
		onEvent: (event: SSEEvent) => void,
		signal?: AbortSignal,
		onReconnect?: (attempt: number) => void
	) => {
		// The POST starts a run, so a retry is only safe while no event has been
		// received yet; retrying mid-run would duplicate the assistant reply.
		let started = false;
		return streamSSE(agentPath(agent, '/chat/stream'), {
			body: payload,
			signal,
			onEvent: (event) => {
				started = true;
				onEvent(event);
			},
			retry: {
				attempts: 2,
				baseDelayMs: 800,
				maxDelayMs: 4000,
				shouldRetry: () => !started,
				onRetry: (attempt) => onReconnect?.(attempt)
			}
		});
	},
	decide: (agent: string, approvalId: string, decision: ApprovalDecision) =>
		client<OKResponse>(agentPath(agent, `/approvals/${encodeURIComponent(approvalId)}/decision`), {
			method: 'POST',
			body: { decision }
		}),
	/**
	 * Stops a run on the agent server. Aborting the fetch only closes this tab's
	 * copy of the stream; the agent keeps running unless it is told to stop.
	 */
	stop: (agent: string, requestId: string) =>
		client<{ stopped: boolean }>(agentPath(agent, '/chat/stop'), {
			method: 'POST',
			body: { request_id: requestId }
		}),
	/** Runs still in flight on the agent server, for a tab that just opened. */
	runs: (agent: string) => client<{ runs: ChatRunInfo[] }>(agentPath(agent, '/chat/runs')),
	/**
	 * Re-attaches to a run: the server replays every buffered event and then
	 * keeps streaming, so a reloaded or reopened tab continues where it left off.
	 */
	attach: (
		agent: string,
		requestId: string,
		onEvent: (event: SSEEvent) => void,
		signal?: AbortSignal
	) =>
		streamSSE(agentPath(agent, `/chat/run/${encodeURIComponent(requestId)}/stream?after=0`), {
			method: 'GET',
			signal,
			onEvent
		})
};
