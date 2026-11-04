import {
	chatFolders as chatFoldersApi,
	conversations as conversationsApi
} from '$lib/apis/conversations';
import { t } from '$lib/i18n';
import { isAuthenticated } from '$lib/stores/authStatus.svelte';
import { agentSelection } from '$lib/stores/selectedAgent.svelte';
import { toastStore } from '$lib/stores/toast.svelte';
import { getErrorMessage } from '$lib/utils/api';
import type { ChatFolder, ChatFolderDelete, ConversationSummary } from '$lib/types';

/** Conversations per page; each section loads another page independently. */
export const HISTORY_LIMIT = 20;

/** One independently paginated part of the list. */
export interface Section {
	items: ConversationSummary[];
	offset: number;
	hasMore: boolean;
	loading: boolean;
	loaded: boolean;
	/** A background reconcile is in flight; it must not start another page load. */
	refreshing: boolean;
}

function emptySection(): Section {
	return { items: [], offset: 0, hasMore: false, loading: false, loaded: false, refreshing: false };
}

/** Read-only fallback for a folder that has never been opened. */
const EMPTY_SECTION: Section = emptySection();

let folders = $state<ChatFolder[]>([]);
let pinned = $state<Section>(emptySection());
let recents = $state<Section>(emptySection());
let folderSections = $state<Record<string, Section>>({});
/** Flat result list used while the search box has text. */
let results = $state<Section>(emptySection());
/** The text the reader is typing; the debounced copy lives in `query`. */
let search = $state('');
let query = $state('');
let key = '';
let generation = 0;
/** Sections mid-background-reload, so a silent refresh never double-fetches. */
const refreshing = new WeakSet<Section>();

async function loadFolders() {
	const agent = agentSelection.selected;
	if (!agent) return;
	const current = generation;
	try {
		const result = await chatFoldersApi.list(agent);
		if (current !== generation) return;
		folders = result.folders ?? [];
	} catch (err) {
		if (current === generation) toastStore.error(getErrorMessage(err));
	}
}

async function loadSection(
	section: Section,
	filter: { pinned?: boolean; unfiled?: boolean; folder?: string },
	resetPage: boolean,
	silent = false
) {
	const agent = agentSelection.selected;
	if (!agent) return;
	if (silent) {
		// A background reconcile must not show a spinner or blank the list; the
		// section keeps whatever it already shows until the response replaces it.
		if (refreshing.has(section)) return;
		refreshing.add(section);
		section.refreshing = true;
	} else if (section.loading || section.refreshing) {
		// A page load (or a reconcile) is already in flight; its offsets are the
		// ones that matter, so a second request would skip or duplicate rows.
		return;
	} else {
		section.loading = true;
	}
	const current = generation;
	const pageOffset = resetPage ? 0 : section.offset;
	try {
		const result = await conversationsApi.list(agent, {
			...filter,
			search: query || undefined,
			limit: HISTORY_LIMIT,
			offset: pageOffset
		});
		if (current !== generation) return;
		const pageItems = result.conversations ?? [];
		if (resetPage) {
			section.items = pageItems;
		} else {
			const known = new Set(section.items.map((item) => item.id));
			section.items = [...section.items, ...pageItems.filter((item) => !known.has(item.id))];
		}
		section.offset = pageOffset + pageItems.length;
		section.hasMore = pageItems.length > 0 && result.has_more;
		section.loaded = true;
	} catch (err) {
		if (current === generation) toastStore.error(getErrorMessage(err));
	} finally {
		if (silent) {
			refreshing.delete(section);
			section.refreshing = false;
		} else {
			section.loading = false;
		}
	}
}

function reset() {
	generation += 1;
	folders = [];
	pinned = emptySection();
	recents = emptySection();
	results = emptySection();
	folderSections = {};
}

function folderSection(id: string): Section {
	let section = folderSections[id];
	if (!section) {
		folderSections[id] = emptySection();
		// Read it back: the stored value is a reactive proxy, while the object we
		// just built is not. Writing to the raw object would never re-render.
		section = folderSections[id];
	}
	return section;
}

/** Pure read, safe from a template or a derived value. */
function peekFolderSection(id: string): Section {
	return folderSections[id] ?? EMPTY_SECTION;
}

function patchTitle(section: Section, id: string, title: string) {
	section.items = section.items.map((item) => (item.id === id ? { ...item, title } : item));
}

/** Removes a row from every loaded section and returns it, if it was visible. */
function removeFromAll(id: string): ConversationSummary | undefined {
	let found: ConversationSummary | undefined;
	for (const section of [pinned, recents, results, ...Object.values(folderSections)]) {
		if (found) break;
		const index = section.items.findIndex((item) => item.id === id);
		if (index !== -1) {
			found = section.items[index];
			section.items = section.items.filter((item) => item.id !== id);
		}
	}
	return found;
}

/** Puts a row back at the top of a section, deduplicated. */
function prepend(section: Section, item: ConversationSummary) {
	section.items = [item, ...section.items.filter((existing) => existing.id !== item.id)];
}

/** Reloads every opened section after a membership change, without a loading flash. */
function refresh() {
	generation += 1;
	// Keep the rows currently on screen; each section is replaced by its next
	// response, so pin/move/delete never blanks the list.
	const openFolders = Object.keys(folderSections);

	void loadFolders();
	if (query) {
		void loadSection(results, {}, true, true);
		return;
	}
	void loadSection(pinned, { pinned: true }, true, true);
	void loadSection(recents, { unfiled: true }, true, true);
	for (const id of openFolders) void loadSection(folderSections[id], { folder: id }, true, true);
}

/**
 * One list for the whole app: the chat page's history panel and the classic-mode
 * sidebar both read it, so neither needs its own fetch. `init()` wires the
 * reactive parts once, from the app shell, because effects must start inside a
 * component.
 */
export const conversationHistory = {
	get folders() {
		return folders;
	},
	get pinned() {
		return pinned;
	},
	get recents() {
		return recents;
	},
	get results() {
		return results;
	},
	get search() {
		return search;
	},
	get query() {
		return query;
	},
	/** The section for one folder; never creates it, so templates stay pure. */
	sectionFor(id: string) {
		return peekFolderSection(id);
	},
	init() {
		// A different agent or search starts the list over; the key keeps the
		// effect from refetching on unrelated state changes. A previous mount may
		// have run before login (the client redirects to /login on 401), so the
		// key is cleared here to make this mount fetch again.
		key = '';
		$effect(() => {
			const agent = agentSelection.selected;
			// Wait for the session: the shell can mount briefly before the auth
			// guard redirects, and a 401 there would blank the list.
			if (!agent || !isAuthenticated.value) return;
			const next = `${agent}:${query}`;
			if (next === key) return;
			key = next;
			reset();
			if (query) {
				void loadSection(results, {}, true);
				return;
			}
			void loadFolders();
			void loadSection(pinned, { pinned: true }, true);
			void loadSection(recents, { unfiled: true }, true);
		});
		// Typing should not hit the API on every keystroke.
		$effect(() => {
			const value = search.trim();
			if (value === query) return;
			const timer = setTimeout(() => (query = value), 300);
			return () => clearTimeout(timer);
		});
	},
	setSearch(value: string) {
		search = value;
	},
	/** Clears both copies so the list refreshes without waiting for the debounce. */
	clearSearch() {
		search = '';
		query = '';
	},
	/** Loads a folder's first page the first time its section is opened. */
	openFolder(id: string) {
		const section = folderSection(id);
		if (!section.loaded) void loadSection(section, { folder: id }, true);
	},
	loadMorePinned() {
		if (pinned.hasMore) void loadSection(pinned, { pinned: true }, false);
	},
	loadMoreRecents() {
		if (recents.hasMore) void loadSection(recents, { unfiled: true }, false);
	},
	loadMoreResults() {
		if (results.hasMore) void loadSection(results, {}, false);
	},
	loadMoreFolder(id: string) {
		const section = folderSection(id);
		if (section.hasMore) void loadSection(section, { folder: id }, false);
	},
	/** Reloads what is visible, e.g. after a run finished and the title changed. */
	refresh,
	/** Keeps the list in step with a title the server renamed in the background. */
	update(id: string, title: string) {
		patchTitle(pinned, id, title);
		patchTitle(recents, id, title);
		patchTitle(results, id, title);
		for (const section of Object.values(folderSections)) patchTitle(section, id, title);
	},
	async rename(id: string, title: string) {
		const agent = agentSelection.selected;
		const next = title.trim();
		if (!agent || !id || !next) return false;
		try {
			await conversationsApi.rename(agent, id, next);
			conversationHistory.update(id, next);
			toastStore.success(t('myaaw.conversations.renamed'));
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	},
	async remove(id: string) {
		const agent = agentSelection.selected;
		if (!agent || !id) return false;
		try {
			await conversationsApi.remove(agent, id);
			// Drop the row immediately so the confirm does not feel ignored, then
			// let the background refresh reconcile counts and pages.
			removeFromAll(id);
			refresh();
			toastStore.success(t('myaaw.conversations.deleted'));
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	},
	async pin(id: string, value: boolean) {
		const agent = agentSelection.selected;
		if (!agent || !id) return false;
		try {
			await conversationsApi.setPinned(agent, id, value);
			// Move the row optimistically so the pin feels instant; refresh then
			// reconciles the real order and any page boundaries.
			const item = removeFromAll(id);
			if (item) prepend(value ? pinned : recents, { ...item, pinned: value });
			refresh();
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	},
	/** `null` moves the chat out of its folder, into Recents. */
	async move(id: string, folderId: string | null) {
		const agent = agentSelection.selected;
		if (!agent || !id) return false;
		try {
			await conversationsApi.setFolder(agent, id, folderId);
			// Relocate the row now; a folder that is already open updates in
			// place, otherwise its next open fetches the authoritative list.
			const item = removeFromAll(id);
			if (item) {
				const target = folderId ? folderSections[folderId] : recents;
				// An unopened folder is left untouched; its next open fetches the truth.
				if (target) prepend(target, { ...item, folder_id: folderId });
			}
			refresh();
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	},
	async createFolder(name: string) {
		const agent = agentSelection.selected;
		const next = name.trim();
		if (!agent || !next) return null;
		try {
			const folder = await chatFoldersApi.create(agent, next);
			await loadFolders();
			return folder;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return null;
		}
	},
	async renameFolder(id: string, name: string) {
		const agent = agentSelection.selected;
		const next = name.trim();
		if (!agent || !id || !next) return false;
		try {
			const folder = await chatFoldersApi.rename(agent, id, next);
			folders = folders.map((item) => (item.id === id ? folder : item));
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	},
	/** Deletes the folder and its chats; the result lists what disappeared. */
	async deleteFolder(id: string): Promise<ChatFolderDelete | null> {
		const agent = agentSelection.selected;
		if (!agent || !id) return null;
		try {
			const result = await chatFoldersApi.remove(agent, id);
			folders = folders.filter((item) => item.id !== id);
			delete folderSections[id];
			refresh();
			return result;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return null;
		}
	}
};
