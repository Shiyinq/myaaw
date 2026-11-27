import { agentPath, client } from './client';
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

function gitBase(agent: string, conversationId: string, suffix: string): string {
	return agentPath(agent, `/conversations/${encodeURIComponent(conversationId)}/git${suffix}`);
}

export const workspace = {
	roots: (agent?: string) => {
		const q = agent ? `?agent=${encodeURIComponent(agent)}` : '';
		return client<WorkspaceRootsResponse>(`/workspace/roots${q}`);
	},
	browse: (agent: string, path = '', showHidden = false) => {
		const query = new URLSearchParams();
		if (agent) query.set('agent', agent);
		if (path) query.set('path', path);
		if (showHidden) query.set('hidden', '1');
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<WorkspaceBrowseResponse>(`/workspace/browse${suffix}`);
	},
	mkdir: (agent: string, parent: string, name: string) =>
		client<{ path: string }>(agentPath(agent, '/workspace/mkdir'), {
			method: 'POST',
			body: { parent, name }
		}),
	gitStatus: (agent: string, conversationId: string) =>
		client<GitStatusResponse>(gitBase(agent, conversationId, '/status')),
	gitLog: (agent: string, conversationId: string, limit = 25, offset = 0) =>
		client<GitLogPageResponse>(
			gitBase(agent, conversationId, `/log?limit=${limit}&offset=${offset}`)
		),
	gitCommitDetail: (agent: string, conversationId: string, hash: string) =>
		client<GitCommitDetailResponse>(
			gitBase(agent, conversationId, `/commits/${encodeURIComponent(hash)}`)
		),
	gitDiff: (
		agent: string,
		conversationId: string,
		path: string,
		options: { staged?: boolean; untracked?: boolean; commit?: string } = {}
	) => {
		const query = new URLSearchParams({ path });
		if (options.commit) query.set('commit', options.commit);
		if (options.staged) query.set('staged', '1');
		if (options.untracked) query.set('untracked', '1');
		return client<GitDiffResponse>(gitBase(agent, conversationId, `/diff?${query.toString()}`));
	},
	gitDiffs: (
		agent: string,
		conversationId: string,
		options: { scope?: 'changes' | 'staged' | 'all' | 'commit'; commit?: string } = {}
	) => {
		const query = new URLSearchParams();
		if (options.commit) query.set('commit', options.commit);
		else if (options.scope) query.set('scope', options.scope);
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<GitMultiDiffResponse>(gitBase(agent, conversationId, `/diffs${suffix}`));
	},
	gitStage: (agent: string, conversationId: string, payload: { paths?: string[]; all?: boolean }) =>
		client<{ ok: boolean }>(gitBase(agent, conversationId, '/stage'), {
			method: 'POST',
			body: payload
		}),
	gitUnstage: (
		agent: string,
		conversationId: string,
		payload: { paths?: string[]; all?: boolean }
	) =>
		client<{ ok: boolean }>(gitBase(agent, conversationId, '/unstage'), {
			method: 'POST',
			body: payload
		}),
	gitDiscard: (
		agent: string,
		conversationId: string,
		payload: { paths?: string[]; all?: boolean; include_untracked?: boolean }
	) =>
		client<{ ok: boolean }>(gitBase(agent, conversationId, '/discard'), {
			method: 'POST',
			body: payload
		}),
	gitCommit: (
		agent: string,
		conversationId: string,
		payload: { message: string; amend?: boolean }
	) =>
		client<{ commit: GitCommitEntry }>(gitBase(agent, conversationId, '/commit'), {
			method: 'POST',
			body: payload
		}),
	gitBranches: (agent: string, conversationId: string) =>
		client<GitBranchesResponse>(gitBase(agent, conversationId, '/branches')),
	gitCheckout: (
		agent: string,
		conversationId: string,
		payload: { branch: string; create?: boolean; from?: string }
	) =>
		client<GitStatusResponse>(gitBase(agent, conversationId, '/checkout'), {
			method: 'POST',
			body: payload
		}),
	gitFetch: (agent: string, conversationId: string) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/fetch'), { method: 'POST' }),
	gitPull: (agent: string, conversationId: string) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/pull'), { method: 'POST' }),
	gitPush: (agent: string, conversationId: string, setUpstream = false) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/push'), {
			method: 'POST',
			body: { set_upstream: setUpstream }
		}),
	gitStashes: (agent: string, conversationId: string) =>
		client<{ stashes: GitStashEntry[] }>(gitBase(agent, conversationId, '/stashes')),
	gitStashSave: (
		agent: string,
		conversationId: string,
		payload: { message?: string; include_untracked?: boolean } = {}
	) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/stashes'), {
			method: 'POST',
			body: payload
		}),
	gitStashApply: (agent: string, conversationId: string, ref: string) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/stashes/apply'), {
			method: 'POST',
			body: { ref }
		}),
	gitStashPop: (agent: string, conversationId: string, ref: string) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/stashes/pop'), {
			method: 'POST',
			body: { ref }
		}),
	gitStashDrop: (agent: string, conversationId: string, ref: string) =>
		client<GitActionResult>(gitBase(agent, conversationId, '/stashes/drop'), {
			method: 'POST',
			body: { ref }
		})
};
