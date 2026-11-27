import { browser } from '$app/environment';
import type { ProjectDiffTarget, ProjectMultiDiffTarget, ProjectWorkspaceTab } from '$lib/types';

export type ProjectSidebarTab = 'files' | 'git';
export type TerminalLayoutMode = 'auto' | 'single' | 'split-v' | 'split-h' | 'grid';

export interface PersistedTerminalGroup {
	tabs: Array<{ id: string; num: number }>;
	activeId: string;
	layoutMode: TerminalLayoutMode;
	colSizes: number[];
	rowSizes: number[];
}

const TERMINAL_GROUP_PREFIX = 'myaaw.terminalGroup.';
const EXPANDED_TREE_PREFIX = 'myaaw.workspace.tree.';
const GIT_TREE_PREFIX = 'myaaw.workspace.gitUi.';
const terminalGroupCache = new Map<string, PersistedTerminalGroup>();
const expandedTreeCache = new Map<string, Record<string, boolean>>();

export interface PersistedGitUiState {
	changesSectionOpen: boolean;
	stagedSubOpen: boolean;
	workingSubOpen: boolean;
	stashesSectionOpen: boolean;
	commitsSectionOpen: boolean;
	collapsedStagedDirs: Record<string, boolean>;
	collapsedWorkingDirs: Record<string, boolean>;
}

const gitUiCache = new Map<string, PersistedGitUiState>();

export function readTerminalGroupState(key: string): PersistedTerminalGroup | null {
	if (!key) return null;
	const cached = terminalGroupCache.get(key);
	if (cached) {
		return {
			tabs: cached.tabs.map((t) => ({ ...t })),
			activeId: cached.activeId,
			layoutMode: cached.layoutMode,
			colSizes: [...cached.colSizes],
			rowSizes: [...cached.rowSizes]
		};
	}
	if (!browser) return null;
	try {
		const raw = sessionStorage.getItem(TERMINAL_GROUP_PREFIX + key);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<PersistedTerminalGroup>;
		if (!parsed || !Array.isArray(parsed.tabs) || parsed.tabs.length === 0) return null;
		const cleanTabs = parsed.tabs.filter((item): item is { id: string; num: number } =>
			Boolean(item && typeof item.id === 'string' && typeof item.num === 'number')
		);
		if (cleanTabs.length === 0) return null;
		const validModes: TerminalLayoutMode[] = ['auto', 'single', 'split-v', 'split-h', 'grid'];
		const layoutMode = validModes.includes(parsed.layoutMode as TerminalLayoutMode)
			? (parsed.layoutMode as TerminalLayoutMode)
			: 'single';
		const activeId =
			typeof parsed.activeId === 'string' && cleanTabs.some((t) => t.id === parsed.activeId)
				? parsed.activeId
				: cleanTabs[0].id;
		const state: PersistedTerminalGroup = {
			tabs: cleanTabs,
			activeId,
			layoutMode,
			colSizes: Array.isArray(parsed.colSizes) ? parsed.colSizes.map(Number) : [100],
			rowSizes: Array.isArray(parsed.rowSizes) ? parsed.rowSizes.map(Number) : [100]
		};
		terminalGroupCache.set(key, state);
		return {
			tabs: state.tabs.map((t) => ({ ...t })),
			activeId: state.activeId,
			layoutMode: state.layoutMode,
			colSizes: [...state.colSizes],
			rowSizes: [...state.rowSizes]
		};
	} catch {
		return null;
	}
}

export function writeTerminalGroupState(key: string, state: PersistedTerminalGroup) {
	if (!key) return;
	const copy: PersistedTerminalGroup = {
		tabs: state.tabs.map((t) => ({ ...t })),
		activeId: state.activeId,
		layoutMode: state.layoutMode,
		colSizes: [...state.colSizes],
		rowSizes: [...state.rowSizes]
	};
	terminalGroupCache.set(key, copy);
	if (!browser) return;
	try {
		sessionStorage.setItem(TERMINAL_GROUP_PREFIX + key, JSON.stringify(copy));
	} catch {
		// Ignore quota errors.
	}
}

export function clearTerminalGroupState(key: string) {
	if (!key) return;
	terminalGroupCache.delete(key);
	if (!browser) return;
	try {
		sessionStorage.removeItem(TERMINAL_GROUP_PREFIX + key);
	} catch {
		// Nothing to do.
	}
}

function sanitizeBoolRecord(raw: unknown): Record<string, boolean> {
	if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
	const out: Record<string, boolean> = {};
	for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
		if (v === true) out[k] = true;
	}
	return out;
}

export function readExpandedTree(scopeKey: string): Record<string, boolean> {
	if (!scopeKey) return { '/project': true };
	const cached = expandedTreeCache.get(scopeKey);
	if (cached) return { '/project': true, ...cached };
	if (!browser) return { '/project': true };
	try {
		const raw = sessionStorage.getItem(EXPANDED_TREE_PREFIX + scopeKey);
		if (!raw) return { '/project': true };
		const parsed = { '/project': true, ...sanitizeBoolRecord(JSON.parse(raw)) };
		expandedTreeCache.set(scopeKey, parsed);
		return { ...parsed };
	} catch {
		return { '/project': true };
	}
}

export function writeExpandedTree(scopeKey: string, next: Record<string, boolean>) {
	if (!scopeKey) return;
	const clean: Record<string, boolean> = { '/project': true };
	for (const [k, v] of Object.entries(next)) {
		if (v) clean[k] = true;
	}
	expandedTreeCache.set(scopeKey, clean);
	if (!browser) return;
	try {
		sessionStorage.setItem(EXPANDED_TREE_PREFIX + scopeKey, JSON.stringify(clean));
	} catch {
		// Ignore quota errors.
	}
}

export function readGitUiState(scopeKey: string): PersistedGitUiState | null {
	if (!scopeKey) return null;
	const cached = gitUiCache.get(scopeKey);
	if (cached) {
		return {
			...cached,
			collapsedStagedDirs: { ...cached.collapsedStagedDirs },
			collapsedWorkingDirs: { ...cached.collapsedWorkingDirs }
		};
	}
	if (!browser) return null;
	try {
		const raw = sessionStorage.getItem(GIT_TREE_PREFIX + scopeKey);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<PersistedGitUiState>;
		if (!parsed || typeof parsed !== 'object') return null;
		const normalized: PersistedGitUiState = {
			changesSectionOpen: parsed.changesSectionOpen !== false,
			stagedSubOpen: parsed.stagedSubOpen !== false,
			workingSubOpen: parsed.workingSubOpen !== false,
			stashesSectionOpen: Boolean(parsed.stashesSectionOpen),
			commitsSectionOpen: Boolean(parsed.commitsSectionOpen),
			collapsedStagedDirs: sanitizeBoolRecord(parsed.collapsedStagedDirs),
			collapsedWorkingDirs: sanitizeBoolRecord(parsed.collapsedWorkingDirs)
		};
		gitUiCache.set(scopeKey, normalized);
		return {
			...normalized,
			collapsedStagedDirs: { ...normalized.collapsedStagedDirs },
			collapsedWorkingDirs: { ...normalized.collapsedWorkingDirs }
		};
	} catch {
		return null;
	}
}

export function writeGitUiState(scopeKey: string, state: PersistedGitUiState) {
	if (!scopeKey) return;
	const copy: PersistedGitUiState = {
		changesSectionOpen: state.changesSectionOpen,
		stagedSubOpen: state.stagedSubOpen,
		workingSubOpen: state.workingSubOpen,
		stashesSectionOpen: state.stashesSectionOpen,
		commitsSectionOpen: state.commitsSectionOpen,
		collapsedStagedDirs: sanitizeBoolRecord(state.collapsedStagedDirs),
		collapsedWorkingDirs: sanitizeBoolRecord(state.collapsedWorkingDirs)
	};
	gitUiCache.set(scopeKey, copy);
	if (!browser) return;
	try {
		sessionStorage.setItem(GIT_TREE_PREFIX + scopeKey, JSON.stringify(copy));
	} catch {
		// Ignore quota errors.
	}
}

export function clearGitUiState(scopeKey: string) {
	if (!scopeKey) return;
	gitUiCache.delete(scopeKey);
	if (!browser) return;
	try {
		sessionStorage.removeItem(GIT_TREE_PREFIX + scopeKey);
	} catch {
		// Nothing to do.
	}
}

interface SavedWorkspaceState {
	tabs: ProjectWorkspaceTab[];
	activeTabId: string;
	terminalOpen: boolean;
	panelOpen: boolean;
	sidebarTab: ProjectSidebarTab;
}

const STORAGE_PREFIX = 'myaaw.workspace.ui.';
const memoryCache = new Map<string, SavedWorkspaceState>();

function scopeKeyFor(agent: string, conversationId: string): string {
	return agent && conversationId ? `${agent}:${conversationId}` : '';
}

function readSavedState(key: string): SavedWorkspaceState | null {
	if (!key) return null;
	const cached = memoryCache.get(key);
	if (cached) {
		return {
			tabs: cached.tabs.map((tab) => ({ ...tab })),
			activeTabId: cached.activeTabId,
			terminalOpen: cached.terminalOpen,
			panelOpen: cached.panelOpen,
			sidebarTab: cached.sidebarTab
		};
	}
	if (!browser) return null;
	try {
		const raw = sessionStorage.getItem(STORAGE_PREFIX + key);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<SavedWorkspaceState>;
		if (!parsed || !Array.isArray(parsed.tabs)) return null;
		const normalized: SavedWorkspaceState = {
			tabs: parsed.tabs.filter((tab): tab is ProjectWorkspaceTab =>
				Boolean(tab && typeof tab.id === 'string' && typeof tab.kind === 'string')
			),
			activeTabId: typeof parsed.activeTabId === 'string' ? parsed.activeTabId : 'chat',
			terminalOpen: Boolean(parsed.terminalOpen),
			panelOpen: Boolean(parsed.panelOpen),
			sidebarTab: parsed.sidebarTab === 'git' ? 'git' : 'files'
		};
		memoryCache.set(key, normalized);
		return {
			tabs: normalized.tabs.map((tab) => ({ ...tab })),
			activeTabId: normalized.activeTabId,
			terminalOpen: normalized.terminalOpen,
			panelOpen: normalized.panelOpen,
			sidebarTab: normalized.sidebarTab
		};
	} catch {
		return null;
	}
}

function writeSavedState(key: string, state: SavedWorkspaceState) {
	if (!key) return;
	const copy: SavedWorkspaceState = {
		tabs: state.tabs.map((tab) => ({ ...tab })),
		activeTabId: state.activeTabId,
		terminalOpen: state.terminalOpen,
		panelOpen: state.panelOpen,
		sidebarTab: state.sidebarTab
	};
	memoryCache.set(key, copy);
	if (!browser) return;
	try {
		sessionStorage.setItem(STORAGE_PREFIX + key, JSON.stringify(copy));
	} catch {
		// Ignore quota errors; in-memory cache still preserves switches within the SPA.
	}
}

function deleteSavedState(key: string) {
	if (!key) return;
	memoryCache.delete(key);
	if (!browser) return;
	try {
		sessionStorage.removeItem(STORAGE_PREFIX + key);
	} catch {
		// Nothing to do.
	}
}

function fileLabel(path: string): string {
	return path.replace(/\/+$/, '').split('/').pop() || path;
}

function fileTabId(virtualPath: string): string {
	return `file:${virtualPath}`;
}

function diffTabId(req: ProjectDiffTarget): string {
	if (req.commit) return `diff:commit:${req.commit}:${req.path}`;
	return `diff:${req.staged ? 'staged' : req.untracked ? 'untracked' : 'work'}:${req.path}`;
}

function multiDiffTabId(req: ProjectMultiDiffTarget): string {
	if (req.commit) return `multi-diff:commit:${req.commit}`;
	return `multi-diff:${req.scope}`;
}

/**
 * Self-contained reactive controller for a chat's optional attached working
 * directory: unified top tab bar (`chat` | `terminal` | open files & diffs),
 * dedicated project sidebar drawer, and folder picker dialog.
 *
 * Per-conversation UI state (open file/diff tabs, active surface, terminal open
 * flag, project sidebar open state & Files/Git sub-tab) is cached in memory and
 * `sessionStorage` keyed by `agent:conversationId`, so switching agents, chats,
 * or dashboard pages and returning restores the exact workspace layout.
 */
export function createProjectWorkspace() {
	let currentScopeKey = '';
	let workingDir = $state('');
	let workingDirExists = $state(true);
	let hasGit = $state(false);
	let panelOpen = $state(false);
	let sidebarTab = $state<ProjectSidebarTab>('files');
	let dialogOpen = $state(false);
	let terminalOpen = $state(false);
	let refreshKey = $state(0);

	let tabs = $state<ProjectWorkspaceTab[]>([]);
	/** Active center surface: `'chat'`, `'terminal'`, or one of `tabs[i].id`. */
	let activeTabId = $state('chat');

	const workingDirName = $derived(
		workingDir ? workingDir.replace(/\/+$/, '').split('/').pop() || workingDir : ''
	);
	/** Tab bar appears whenever at least one file/diff tab OR the terminal tab is open. */
	const hasTabBar = $derived(tabs.length > 0 || terminalOpen);
	const activeTab = $derived(tabs.find((t) => t.id === activeTabId) ?? null);
	const showingTerminal = $derived(terminalOpen && activeTabId === 'terminal');
	const showingFileTab = $derived(Boolean(activeTab));
	const showingChat = $derived(!showingTerminal && !showingFileTab);
	const activeFilePath = $derived(activeTab?.kind === 'file' ? activeTab.path : '');

	function persistCurrent() {
		if (!currentScopeKey || !workingDir) return;
		writeSavedState(currentScopeKey, {
			tabs,
			activeTabId,
			terminalOpen,
			panelOpen,
			sidebarTab
		});
	}

	function restoreScope(key: string) {
		const saved = readSavedState(key);
		if (!saved) return;
		tabs = saved.tabs;
		terminalOpen = saved.terminalOpen;
		panelOpen = saved.panelOpen;
		sidebarTab = saved.sidebarTab;
		if (
			saved.activeTabId === 'chat' ||
			(saved.activeTabId === 'terminal' && saved.terminalOpen) ||
			saved.tabs.some((tab) => tab.id === saved.activeTabId)
		) {
			activeTabId = saved.activeTabId;
		} else {
			activeTabId = saved.terminalOpen ? 'terminal' : (saved.tabs[0]?.id ?? 'chat');
		}
	}

	/**
	 * Switches the active agent+conversation scope, saving the previous scope's
	 * workspace state before resetting the visible view for the next conversation.
	 */
	function bindScope(agent: string, conversationId: string) {
		const nextKey = scopeKeyFor(agent, conversationId);
		if (nextKey === currentScopeKey) return;
		persistCurrent();
		currentScopeKey = nextKey;
		workingDir = '';
		workingDirExists = true;
		hasGit = false;
		tabs = [];
		terminalOpen = false;
		activeTabId = 'chat';
		panelOpen = false;
		sidebarTab = 'files';
	}

	function clearSavedScope(agent?: string, conversationId?: string) {
		const key = agent && conversationId ? scopeKeyFor(agent, conversationId) : currentScopeKey;
		if (!key) return;
		deleteSavedState(key);
		clearGitUiState(key);
		// Also clear expanded file-tree snapshots matching this conversation scope.
		const prefix = `${key}:`;
		for (const k of Array.from(expandedTreeCache.keys())) {
			if (k.startsWith(prefix)) expandedTreeCache.delete(k);
		}
		if (browser) {
			try {
				const storagePrefix = EXPANDED_TREE_PREFIX + prefix;
				for (let i = sessionStorage.length - 1; i >= 0; i -= 1) {
					const sk = sessionStorage.key(i);
					if (sk && sk.startsWith(storagePrefix)) sessionStorage.removeItem(sk);
				}
			} catch {
				// Nothing to do.
			}
		}
	}

	function setDirectory(dir: string, exists = true, git = false, agent = '', conversationId = '') {
		const nextKey = agent && conversationId ? scopeKeyFor(agent, conversationId) : currentScopeKey;
		const scopeChanged = Boolean(nextKey && nextKey !== currentScopeKey);
		if (scopeChanged) {
			persistCurrent();
			currentScopeKey = nextKey;
		}

		const prevDir = workingDir;
		workingDir = dir;
		workingDirExists = exists;
		hasGit = git;

		if (!dir) {
			tabs = [];
			terminalOpen = false;
			activeTabId = 'chat';
			panelOpen = false;
			sidebarTab = 'files';
			return;
		}

		// When loading a conversation that already has a saved workspace layout,
		// restore its open tabs, terminal, and sidebar state seamlessly.
		if (currentScopeKey && (!prevDir || scopeChanged)) {
			restoreScope(currentScopeKey);
		}
		persistCurrent();
	}

	function clearTabs() {
		tabs = [];
		activeTabId = terminalOpen ? 'terminal' : 'chat';
		persistCurrent();
	}

	function reset(closePanel = true) {
		persistCurrent();
		currentScopeKey = '';
		workingDir = '';
		workingDirExists = true;
		hasGit = false;
		tabs = [];
		terminalOpen = false;
		activeTabId = 'chat';
		sidebarTab = 'files';
		if (closePanel) panelOpen = false;
	}

	function resetTransient(closePanel = false) {
		persistCurrent();
		tabs = [];
		terminalOpen = false;
		activeTabId = 'chat';
		if (closePanel) panelOpen = false;
	}

	function selectChat() {
		activeTabId = 'chat';
		persistCurrent();
	}

	function selectTerminal() {
		if (!workingDir) {
			dialogOpen = true;
			return;
		}
		terminalOpen = true;
		activeTabId = 'terminal';
		persistCurrent();
	}

	function closeTerminal() {
		terminalOpen = false;
		if (activeTabId === 'terminal') {
			activeTabId = tabs[0]?.id ?? 'chat';
		}
		persistCurrent();
	}

	function openFile(virtualPath: string, collapseOnMobile = false) {
		const id = fileTabId(virtualPath);
		if (!tabs.some((tab) => tab.id === id)) {
			tabs = [
				...tabs,
				{
					id,
					kind: 'file',
					path: virtualPath,
					label: fileLabel(virtualPath)
				}
			];
		}
		activeTabId = id;
		if (collapseOnMobile) panelOpen = false;
		persistCurrent();
	}

	function openDiff(req: ProjectDiffTarget, collapseOnMobile = false) {
		const id = diffTabId(req);
		if (!tabs.some((tab) => tab.id === id)) {
			tabs = [
				...tabs,
				{
					id,
					kind: 'diff',
					path: req.path,
					label: fileLabel(req.path),
					staged: Boolean(req.staged),
					untracked: Boolean(req.untracked),
					commit: req.commit
				}
			];
		}
		activeTabId = id;
		if (collapseOnMobile) panelOpen = false;
		persistCurrent();
	}

	function openMultiDiff(req: ProjectMultiDiffTarget, collapseOnMobile = false) {
		const id = multiDiffTabId(req);
		const label =
			req.label ||
			(req.commit
				? `Commit ${req.commit.slice(0, 7)}`
				: req.scope === 'staged'
					? 'Staged Changes'
					: 'Changes');
		const existingIdx = tabs.findIndex((tab) => tab.id === id);
		if (existingIdx < 0) {
			tabs = [
				...tabs,
				{
					id,
					kind: 'multi-diff',
					path: id,
					label,
					scope: req.scope,
					commit: req.commit
				}
			];
		} else {
			tabs = tabs.map((tab, idx) => (idx === existingIdx ? { ...tab, label } : tab));
		}
		activeTabId = id;
		if (collapseOnMobile) panelOpen = false;
		persistCurrent();
	}

	function selectTab(id: string) {
		if (id === 'chat') {
			activeTabId = 'chat';
			persistCurrent();
			return;
		}
		if (id === 'terminal' && terminalOpen) {
			activeTabId = 'terminal';
			persistCurrent();
			return;
		}
		if (tabs.some((tab) => tab.id === id)) {
			activeTabId = id;
			persistCurrent();
		}
	}

	function closeTab(id: string) {
		const idx = tabs.findIndex((tab) => tab.id === id);
		if (idx < 0) return;
		const nextTabs = tabs.filter((tab) => tab.id !== id);
		tabs = nextTabs;
		if (activeTabId === id) {
			const fallback = nextTabs[Math.min(idx, nextTabs.length - 1)];
			activeTabId = fallback?.id ?? (terminalOpen ? 'terminal' : 'chat');
		}
		persistCurrent();
	}

	function handleFileRenamed(fromPath: string, toPath: string) {
		const prefix = `${fromPath}/`;
		let nextActive = activeTabId;
		tabs = tabs.map((tab) => {
			if (tab.kind !== 'file') return tab;
			if (tab.path === fromPath) {
				const nextId = fileTabId(toPath);
				if (nextActive === tab.id) nextActive = nextId;
				return { ...tab, id: nextId, path: toPath, label: fileLabel(toPath) };
			}
			if (tab.path.startsWith(prefix)) {
				const moved = `${toPath}${tab.path.slice(fromPath.length)}`;
				const nextId = fileTabId(moved);
				if (nextActive === tab.id) nextActive = nextId;
				return { ...tab, id: nextId, path: moved, label: fileLabel(moved) };
			}
			return tab;
		});
		activeTabId = nextActive;
		persistCurrent();
	}

	function handleFileDeleted(deletedPath: string) {
		const prefix = `${deletedPath}/`;
		const nextTabs = tabs.filter((tab) => {
			if (tab.kind !== 'file') return true;
			return tab.path !== deletedPath && !tab.path.startsWith(prefix);
		});
		tabs = nextTabs;
		if (
			activeTabId !== 'chat' &&
			activeTabId !== 'terminal' &&
			!nextTabs.some((tab) => tab.id === activeTabId)
		) {
			activeTabId = nextTabs[nextTabs.length - 1]?.id ?? (terminalOpen ? 'terminal' : 'chat');
		}
		persistCurrent();
	}

	function togglePanel() {
		if (!workingDir) {
			dialogOpen = true;
			return;
		}
		panelOpen = !panelOpen;
		persistCurrent();
	}

	/** Opens/focuses the Terminal tab without closing open files; clicking again while active switches back to Chat. */
	function toggleTerminal() {
		if (!workingDir) {
			dialogOpen = true;
			return;
		}
		if (!terminalOpen) {
			terminalOpen = true;
			activeTabId = 'terminal';
		} else if (activeTabId !== 'terminal') {
			activeTabId = 'terminal';
		} else {
			activeTabId = 'chat';
		}
		persistCurrent();
	}

	function bumpRefresh() {
		if (workingDir) refreshKey += 1;
	}

	return {
		get workingDir() {
			return workingDir;
		},
		get workingDirExists() {
			return workingDirExists;
		},
		get hasGit() {
			return hasGit;
		},
		get workingDirName() {
			return workingDirName;
		},
		get panelOpen() {
			return panelOpen;
		},
		set panelOpen(v: boolean) {
			panelOpen = v;
			persistCurrent();
		},
		get sidebarTab() {
			return sidebarTab;
		},
		set sidebarTab(v: ProjectSidebarTab) {
			sidebarTab = v;
			persistCurrent();
		},
		get dialogOpen() {
			return dialogOpen;
		},
		set dialogOpen(v: boolean) {
			dialogOpen = v;
		},
		get terminalOpen() {
			return terminalOpen;
		},
		set terminalOpen(v: boolean) {
			terminalOpen = v;
			persistCurrent();
		},
		get refreshKey() {
			return refreshKey;
		},
		get tabs() {
			return tabs;
		},
		get activeTabId() {
			return activeTabId;
		},
		get activeTab() {
			return activeTab;
		},
		get hasTabBar() {
			return hasTabBar;
		},
		get showingChat() {
			return showingChat;
		},
		get showingTerminal() {
			return showingTerminal;
		},
		get showingFileTab() {
			return showingFileTab;
		},
		get activeFilePath() {
			return activeFilePath;
		},
		bindScope,
		clearSavedScope,
		setDirectory,
		clearTabs,
		reset,
		resetTransient,
		selectChat,
		selectTerminal,
		closeTerminal,
		openFile,
		openDiff,
		openMultiDiff,
		selectTab,
		closeTab,
		handleFileRenamed,
		handleFileDeleted,
		togglePanel,
		toggleTerminal,
		bumpRefresh
	};
}
