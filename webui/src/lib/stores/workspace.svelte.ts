import { workspace as workspaceApi } from '$lib/apis/workspace';
import type {
	GitActionResult,
	GitBranchesResponse,
	GitCommitDetailResponse,
	GitCommitEntry,
	GitDiffResponse,
	GitLogPageResponse,
	GitMultiDiffResponse,
	GitStashEntry,
	GitStatusResponse,
	WorkspaceBrowseResponse,
	WorkspaceRootsResponse
} from '$lib/types';

/**
 * Host directory browsing and per-conversation Git Source Control live behind
 * a single store facade so components never import `$lib/apis` directly and
 * backend transports can be swapped in one place.
 */
export const workspaceStore = {
	roots(agent?: string): Promise<WorkspaceRootsResponse> {
		return workspaceApi.roots(agent);
	},
	browse(agent: string, path = '', showHidden = false): Promise<WorkspaceBrowseResponse> {
		return workspaceApi.browse(agent, path, showHidden);
	},
	mkdir(agent: string, parent: string, name: string): Promise<{ path: string }> {
		return workspaceApi.mkdir(agent, parent, name);
	},
	gitStatus(agent: string, conversationId: string): Promise<GitStatusResponse> {
		return workspaceApi.gitStatus(agent, conversationId);
	},
	gitLog(
		agent: string,
		conversationId: string,
		limit = 25,
		offset = 0
	): Promise<GitLogPageResponse> {
		return workspaceApi.gitLog(agent, conversationId, limit, offset);
	},
	gitCommitDetail(
		agent: string,
		conversationId: string,
		hash: string
	): Promise<GitCommitDetailResponse> {
		return workspaceApi.gitCommitDetail(agent, conversationId, hash);
	},
	gitDiff(
		agent: string,
		conversationId: string,
		path: string,
		options: { staged?: boolean; untracked?: boolean; commit?: string } = {}
	): Promise<GitDiffResponse> {
		return workspaceApi.gitDiff(agent, conversationId, path, options);
	},
	gitDiffs(
		agent: string,
		conversationId: string,
		options: { scope?: 'changes' | 'staged' | 'all' | 'commit'; commit?: string } = {}
	): Promise<GitMultiDiffResponse> {
		return workspaceApi.gitDiffs(agent, conversationId, options);
	},
	gitStage(
		agent: string,
		conversationId: string,
		payload: { paths?: string[]; all?: boolean }
	): Promise<{ ok: boolean }> {
		return workspaceApi.gitStage(agent, conversationId, payload);
	},
	gitUnstage(
		agent: string,
		conversationId: string,
		payload: { paths?: string[]; all?: boolean }
	): Promise<{ ok: boolean }> {
		return workspaceApi.gitUnstage(agent, conversationId, payload);
	},
	gitDiscard(
		agent: string,
		conversationId: string,
		payload: { paths?: string[]; all?: boolean; include_untracked?: boolean }
	): Promise<{ ok: boolean }> {
		return workspaceApi.gitDiscard(agent, conversationId, payload);
	},
	gitCommit(
		agent: string,
		conversationId: string,
		payload: { message: string; amend?: boolean }
	): Promise<{ commit: GitCommitEntry }> {
		return workspaceApi.gitCommit(agent, conversationId, payload);
	},
	gitBranches(agent: string, conversationId: string): Promise<GitBranchesResponse> {
		return workspaceApi.gitBranches(agent, conversationId);
	},
	gitCheckout(
		agent: string,
		conversationId: string,
		payload: { branch: string; create?: boolean; from?: string }
	): Promise<GitStatusResponse> {
		return workspaceApi.gitCheckout(agent, conversationId, payload);
	},
	gitFetch(agent: string, conversationId: string): Promise<GitActionResult> {
		return workspaceApi.gitFetch(agent, conversationId);
	},
	gitPull(agent: string, conversationId: string): Promise<GitActionResult> {
		return workspaceApi.gitPull(agent, conversationId);
	},
	gitPush(agent: string, conversationId: string, setUpstream = false): Promise<GitActionResult> {
		return workspaceApi.gitPush(agent, conversationId, setUpstream);
	},
	gitStashes(agent: string, conversationId: string): Promise<{ stashes: GitStashEntry[] }> {
		return workspaceApi.gitStashes(agent, conversationId);
	},
	gitStashSave(
		agent: string,
		conversationId: string,
		payload: { message?: string; include_untracked?: boolean } = {}
	): Promise<GitActionResult> {
		return workspaceApi.gitStashSave(agent, conversationId, payload);
	},
	gitStashApply(agent: string, conversationId: string, ref: string): Promise<GitActionResult> {
		return workspaceApi.gitStashApply(agent, conversationId, ref);
	},
	gitStashPop(agent: string, conversationId: string, ref: string): Promise<GitActionResult> {
		return workspaceApi.gitStashPop(agent, conversationId, ref);
	},
	gitStashDrop(agent: string, conversationId: string, ref: string): Promise<GitActionResult> {
		return workspaceApi.gitStashDrop(agent, conversationId, ref);
	}
};
