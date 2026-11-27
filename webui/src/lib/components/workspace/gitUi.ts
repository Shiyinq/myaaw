import type { GitChangeEntry, GitStatusResponse } from '$lib/types';

export interface GitChangeItem extends GitChangeEntry {
	staged: boolean;
	untracked: boolean;
}

export interface GitChangeTreeRow {
	kind: 'dir' | 'file';
	id: string;
	name: string;
	/** Relative directory path (for dir rows) or full relative file path (for file rows). */
	path: string;
	depth: number;
	statusCode: string;
	item?: GitChangeItem;
}

interface TreeDirNode {
	name: string;
	path: string;
	statusCode: string;
	dirs: Map<string, TreeDirNode>;
	files: GitChangeItem[];
}

/**
 * Builds a flat render list of nested collapsible folder & file rows from
 * flat Git changes, matching VS Code's Source Control "View as Tree" mode.
 */
export function buildChangeTreeRows(
	items: GitChangeItem[],
	collapsedDirs: Record<string, boolean>
): GitChangeTreeRow[] {
	const root: TreeDirNode = {
		name: '',
		path: '',
		statusCode: '',
		dirs: new Map(),
		files: []
	};

	for (const item of items) {
		const clean = item.path.replace(/^\/+/, '');
		const parts = clean.split('/');
		let cursor = root;
		let acc = '';
		for (let i = 0; i < parts.length - 1; i += 1) {
			const seg = parts[i];
			acc = acc ? `${acc}/${seg}` : seg;
			let child = cursor.dirs.get(seg);
			if (!child) {
				child = {
					name: seg,
					path: acc,
					statusCode: item.status || 'M',
					dirs: new Map(),
					files: []
				};
				cursor.dirs.set(seg, child);
			} else {
				child.statusCode = pickHigherPriority(child.statusCode, item.status || 'M');
			}
			cursor = child;
		}
		cursor.files.push(item);
	}

	const rows: GitChangeTreeRow[] = [];
	const walk = (node: TreeDirNode, depth: number) => {
		const sortedDirs = [...node.dirs.values()].sort((a, b) => a.name.localeCompare(b.name));
		for (const dir of sortedDirs) {
			rows.push({
				kind: 'dir',
				id: `dir:${dir.path}`,
				name: dir.name,
				path: dir.path,
				depth,
				statusCode: dir.statusCode
			});
			if (!collapsedDirs[dir.path]) {
				walk(dir, depth + 1);
			}
		}
		const sortedFiles = [...node.files].sort((a, b) => {
			const an = a.path.split('/').pop() || a.path;
			const bn = b.path.split('/').pop() || b.path;
			return an.localeCompare(bn);
		});
		for (const file of sortedFiles) {
			rows.push({
				kind: 'file',
				id: `file:${file.staged ? 's' : file.untracked ? 'u' : 'w'}:${file.path}`,
				name: file.path.split('/').pop() || file.path,
				path: file.path,
				depth,
				statusCode: file.status || (file.untracked ? '?' : 'M'),
				item: file
			});
		}
	};

	walk(root, 0);
	return rows;
}

export function splitFilePath(relPath: string): { name: string; dir: string } {
	const clean = relPath.replace(/^\/+/, '');
	const idx = clean.lastIndexOf('/');
	if (idx < 0) return { name: clean, dir: '' };
	return { name: clean.slice(idx + 1), dir: clean.slice(0, idx) };
}

/** Returns every unique directory prefix present in a list of Git changes. */
export function collectChangeTreeDirs(items: GitChangeItem[]): string[] {
	const dirs = new Set<string>();
	for (const item of items) {
		const parts = item.path.replace(/^\/+/, '').split('/');
		let acc = '';
		for (let i = 0; i < parts.length - 1; i += 1) {
			acc = acc ? `${acc}/${parts[i]}` : parts[i];
			dirs.add(acc);
		}
	}
	return [...dirs];
}

export interface GitFileBadge {
	code: string;
	staged: boolean;
	unstaged: boolean;
	untracked: boolean;
}

export interface GitStatusMaps {
	/** Keyed by virtual `/project/<relPath>`. */
	files: Record<string, GitFileBadge>;
	/** Keyed by virtual `/project/<dir>` — stores highest-priority status code in subtree. */
	dirs: Record<string, string>;
}

const STATUS_PRIORITY: Record<string, number> = {
	U: 6,
	D: 5,
	M: 4,
	R: 3,
	A: 2,
	'?': 1
};

function pickHigherPriority(current: string | undefined, next: string): string {
	if (!current) return next;
	return (STATUS_PRIORITY[next] ?? 0) > (STATUS_PRIORITY[current] ?? 0) ? next : current;
}

/**
 * Builds fast lookup maps from `git status` so the File Explorer can highlight
 * changed files and propagate status badges up parent directories in O(N).
 */
export function buildGitStatusMaps(status: GitStatusResponse | null): GitStatusMaps {
	const files: Record<string, GitFileBadge> = {};
	const dirs: Record<string, string> = {};
	if (!status?.has_git) return { files, dirs };

	const mark = (
		relPath: string,
		code: string,
		flags: { staged?: boolean; unstaged?: boolean; untracked?: boolean }
	) => {
		const clean = relPath.replace(/^\/+/, '');
		if (!clean) return;
		const vpath = `/project/${clean}`;
		const prev = files[vpath];
		files[vpath] = {
			code: pickHigherPriority(prev?.code, code),
			staged: Boolean(prev?.staged || flags.staged),
			unstaged: Boolean(prev?.unstaged || flags.unstaged),
			untracked: Boolean(prev?.untracked || flags.untracked)
		};

		const parts = clean.split('/');
		let acc = '/project';
		for (let i = 0; i < parts.length - 1; i += 1) {
			acc = `${acc}/${parts[i]}`;
			dirs[acc] = pickHigherPriority(dirs[acc], code);
		}
	};

	for (const entry of status.staged ?? []) {
		mark(entry.path, entry.status || 'M', { staged: true });
	}
	for (const entry of status.unstaged ?? []) {
		mark(entry.path, entry.status || 'M', { unstaged: true });
	}
	for (const entry of status.untracked ?? []) {
		mark(entry.path, '?', { untracked: true });
	}

	return { files, dirs };
}

export function gitStatusLetter(code: string): string {
	if (code === '?') return 'U';
	return code || 'M';
}

export function gitStatusTextClass(code: string): string {
	switch (code) {
		case 'M':
			return 'text-amber-600 dark:text-amber-400';
		case 'A':
		case '?':
			return 'text-emerald-600 dark:text-emerald-400';
		case 'D':
			return 'text-rose-600 dark:text-rose-400';
		case 'U':
			return 'text-orange-600 dark:text-orange-400';
		default:
			return 'text-blue-600 dark:text-blue-400';
	}
}

export interface UnifiedDiffRow {
	kind: 'meta' | 'hunk' | 'context' | 'add' | 'del';
	oldLine: number | null;
	newLine: number | null;
	text: string;
}

export interface SplitDiffRow {
	kind: 'meta' | 'hunk' | 'pair';
	metaText?: string;
	leftLine: number | null;
	leftText: string;
	leftKind: 'context' | 'del' | 'empty';
	rightLine: number | null;
	rightText: string;
	rightKind: 'context' | 'add' | 'empty';
}

const HUNK_RE = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

export function parseUnifiedDiff(diff: string): UnifiedDiffRow[] {
	const rows: UnifiedDiffRow[] = [];
	let oldNum = 0;
	let newNum = 0;
	let inHunk = false;

	for (const line of diff.split('\n')) {
		if (
			line.startsWith('diff --git ') ||
			line.startsWith('index ') ||
			line.startsWith('--- ') ||
			line.startsWith('+++ ') ||
			line.startsWith('new file mode ') ||
			line.startsWith('deleted file mode ') ||
			line.startsWith('rename from ') ||
			line.startsWith('rename to ')
		) {
			inHunk = false;
			rows.push({ kind: 'meta', oldLine: null, newLine: null, text: line });
			continue;
		}
		const match = HUNK_RE.exec(line);
		if (match) {
			inHunk = true;
			oldNum = Number.parseInt(match[1], 10);
			newNum = Number.parseInt(match[2], 10);
			rows.push({ kind: 'hunk', oldLine: null, newLine: null, text: line });
			continue;
		}
		if (!inHunk) {
			if (line) rows.push({ kind: 'meta', oldLine: null, newLine: null, text: line });
			continue;
		}
		if (line.startsWith('+')) {
			rows.push({ kind: 'add', oldLine: null, newLine: newNum, text: line.slice(1) });
			newNum += 1;
		} else if (line.startsWith('-')) {
			rows.push({ kind: 'del', oldLine: oldNum, newLine: null, text: line.slice(1) });
			oldNum += 1;
		} else if (line.startsWith('\\')) {
			rows.push({ kind: 'meta', oldLine: null, newLine: null, text: line });
		} else {
			rows.push({
				kind: 'context',
				oldLine: oldNum,
				newLine: newNum,
				text: line.startsWith(' ') ? line.slice(1) : line
			});
			oldNum += 1;
			newNum += 1;
		}
	}
	return rows;
}

export function buildSplitDiffRows(unified: UnifiedDiffRow[]): SplitDiffRow[] {
	const split: SplitDiffRow[] = [];
	let pendingDels: UnifiedDiffRow[] = [];
	let pendingAdds: UnifiedDiffRow[] = [];

	const flushPending = () => {
		const count = Math.max(pendingDels.length, pendingAdds.length);
		for (let i = 0; i < count; i += 1) {
			const d = pendingDels[i];
			const a = pendingAdds[i];
			split.push({
				kind: 'pair',
				leftLine: d?.oldLine ?? null,
				leftText: d?.text ?? '',
				leftKind: d ? 'del' : 'empty',
				rightLine: a?.newLine ?? null,
				rightText: a?.text ?? '',
				rightKind: a ? 'add' : 'empty'
			});
		}
		pendingDels = [];
		pendingAdds = [];
	};

	for (const row of unified) {
		if (row.kind === 'del') {
			pendingDels.push(row);
		} else if (row.kind === 'add') {
			pendingAdds.push(row);
		} else {
			flushPending();
			if (row.kind === 'meta' || row.kind === 'hunk') {
				split.push({
					kind: row.kind,
					metaText: row.text,
					leftLine: null,
					leftText: '',
					leftKind: 'empty',
					rightLine: null,
					rightText: '',
					rightKind: 'empty'
				});
			} else {
				split.push({
					kind: 'pair',
					leftLine: row.oldLine,
					leftText: row.text,
					leftKind: 'context',
					rightLine: row.newLine,
					rightText: row.text,
					rightKind: 'context'
				});
			}
		}
	}
	flushPending();
	return split;
}
