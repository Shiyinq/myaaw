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
		signal?: AbortSignal
	) => streamSSE(agentPath(agent, '/chat/stream'), { body: payload, onEvent, signal }),
	decide: (agent: string, approvalId: string, decision: ApprovalDecision) =>
		client<OKResponse>(agentPath(agent, `/approvals/${encodeURIComponent(approvalId)}/decision`), {
			method: 'POST',
			body: { decision }
		})
};
