import { chat as chatApi } from '$lib/apis/chat';
import type { ApprovalDecision, ChatRunInfo } from '$lib/types';

/**
 * Chat actions that are not part of a run's own lifecycle. The run streams live
 * in `chatRuns`; deciding an approval and listing resumable runs are one-shot
 * requests, kept here so the page never imports `lib/apis`.
 */
export const chatStore = {
	decide(agent: string, approvalId: string, decision: ApprovalDecision) {
		return chatApi.decide(agent, approvalId, decision);
	},
	stop(agent: string, requestId: string) {
		return chatApi.stop(agent, requestId);
	},
	runs(agent: string): Promise<{ runs: ChatRunInfo[] }> {
		return chatApi.runs(agent);
	}
};
