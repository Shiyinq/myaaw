import { agentPath, client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type { ApprovalDecision, ChatRequest, ChatResponse, OKResponse } from '$lib/types';

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
		})
};
