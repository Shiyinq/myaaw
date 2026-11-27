export interface WorkspaceRootItem {
	label: string;
	path: string;
	kind: 'agent_workspace' | 'configured' | 'host' | string;
}

export interface WorkspaceRootsResponse {
	hostname: string;
	os: string;
	allow_host_browse: boolean;
	roots: WorkspaceRootItem[];
}

export interface WorkspaceBrowseEntry {
	name: string;
	path: string;
	is_git: boolean;
	is_empty: boolean;
	markers?: string[];
}

export interface WorkspaceBrowseResponse {
	path: string;
	parent?: string;
	can_attach: boolean;
	entries: WorkspaceBrowseEntry[];
}

export interface GitChangeEntry {
	path: string;
	orig_path?: string;
	status: string;
}

export interface GitStatusResponse {
	has_git: boolean;
	branch?: string;
	head_oid?: string;
	upstream?: string;
	ahead: number;
	behind: number;
	detached?: boolean;
	clean: boolean;
	staged: GitChangeEntry[];
	unstaged: GitChangeEntry[];
	untracked: GitChangeEntry[];
}

export interface GitCommitEntry {
	hash: string;
	short_hash: string;
	subject: string;
	author: string;
	date: string;
}

export interface GitLogPageResponse {
	commits: GitCommitEntry[];
	offset: number;
	limit: number;
	has_more: boolean;
}

export interface GitCommitFileEntry {
	path: string;
	orig_path?: string;
	status: string;
	additions: number;
	deletions: number;
	binary?: boolean;
}

export interface GitCommitDetailResponse {
	commit: GitCommitEntry;
	body?: string;
	files: GitCommitFileEntry[];
}

export interface GitDiffResponse {
	path: string;
	commit?: string;
	staged: boolean;
	untracked: boolean;
	binary: boolean;
	truncated?: boolean;
	diff: string;
	additions: number;
	deletions: number;
}

export interface GitBranchEntry {
	name: string;
	current: boolean;
	upstream?: string;
	subject?: string;
}

export interface GitBranchesResponse {
	current?: string;
	detached?: boolean;
	branches: GitBranchEntry[];
	remotes: string[];
}

export interface GitStashEntry {
	ref: string;
	message: string;
	date: string;
}

export interface GitActionResult {
	output?: string;
}

export interface GitMultiDiffResponse {
	scope: 'changes' | 'staged' | 'all' | 'commit' | string;
	commit?: string;
	files: GitDiffResponse[];
	total: number;
	truncated?: boolean;
}

export interface ProjectDiffTarget {
	path: string;
	staged?: boolean;
	untracked?: boolean;
	commit?: string;
}

export interface ProjectMultiDiffTarget {
	scope: 'changes' | 'staged' | 'all' | 'commit';
	commit?: string;
	label?: string;
}

export type ProjectWorkspaceTab =
	| {
			id: string;
			kind: 'file';
			path: string;
			label: string;
	  }
	| {
			id: string;
			kind: 'diff';
			path: string;
			label: string;
			staged?: boolean;
			untracked?: boolean;
			commit?: string;
	  }
	| {
			id: string;
			kind: 'multi-diff';
			path: string;
			label: string;
			scope: 'changes' | 'staged' | 'all' | 'commit';
			commit?: string;
	  };
