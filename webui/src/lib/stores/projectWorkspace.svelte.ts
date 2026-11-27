import type { ProjectDiffTarget, ProjectMultiDiffTarget, ProjectWorkspaceTab } from '$lib/types';

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
 * Keeping this outside `+page.svelte` ensures the chat page stays focused on
 * messaging while workspace features can be evolved or swapped independently.
 */
export function createProjectWorkspace() {
	let workingDir = $state('');
	let workingDirExists = $state(true);
	let hasGit = $state(false);
	let panelOpen = $state(false);
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

	function setDirectory(dir: string, exists = true, git = false) {
		workingDir = dir;
		workingDirExists = exists;
		hasGit = git;
	}

	function clearTabs() {
		tabs = [];
		activeTabId = terminalOpen ? 'terminal' : 'chat';
	}

	function reset(closePanel = true) {
		workingDir = '';
		workingDirExists = true;
		hasGit = false;
		tabs = [];
		terminalOpen = false;
		activeTabId = 'chat';
		if (closePanel) panelOpen = false;
	}

	function resetTransient(closePanel = false) {
		tabs = [];
		terminalOpen = false;
		activeTabId = 'chat';
		if (closePanel) panelOpen = false;
	}

	function selectChat() {
		activeTabId = 'chat';
	}

	function selectTerminal() {
		if (!workingDir) {
			dialogOpen = true;
			return;
		}
		terminalOpen = true;
		activeTabId = 'terminal';
	}

	function closeTerminal() {
		terminalOpen = false;
		if (activeTabId === 'terminal') {
			activeTabId = tabs[0]?.id ?? 'chat';
		}
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
	}

	function selectTab(id: string) {
		if (id === 'chat') {
			activeTabId = 'chat';
			return;
		}
		if (id === 'terminal' && terminalOpen) {
			activeTabId = 'terminal';
			return;
		}
		if (tabs.some((tab) => tab.id === id)) {
			activeTabId = id;
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
	}

	function togglePanel() {
		if (!workingDir) {
			dialogOpen = true;
			return;
		}
		panelOpen = !panelOpen;
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
