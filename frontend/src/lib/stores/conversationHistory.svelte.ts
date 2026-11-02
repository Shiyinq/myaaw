import { conversations as conversationsApi } from '$lib/apis/conversations';
import { t } from '$lib/i18n';
import { agentSelection } from '$lib/stores/selectedAgent.svelte';
import { toastStore } from '$lib/stores/toast.svelte';
import { getErrorMessage } from '$lib/utils/api';
import type { ConversationSummary } from '$lib/types';

/** Conversations per page; the list loads another page when scrolled to the end. */
export const HISTORY_LIMIT = 20;

let items = $state<ConversationSummary[]>([]);
let offset = $state(0);
let hasMore = $state(false);
let loading = $state(false);
/** The text the reader is typing; the debounced copy lives in `query`. */
let search = $state('');
let query = $state('');
let key = '';

async function load(resetPage = false) {
	const agent = agentSelection.selected;
	if (!agent || loading) return;
	loading = true;
	try {
		const pageOffset = resetPage ? 0 : offset;
		const result = await conversationsApi.list(agent, {
			search: query || undefined,
			limit: HISTORY_LIMIT,
			offset: pageOffset
		});
		const pageItems = result.conversations ?? [];
		if (resetPage) {
			items = pageItems;
		} else {
			const known = new Set(items.map((item) => item.id));
			items = [...items, ...pageItems.filter((item) => !known.has(item.id))];
		}
		offset = pageOffset + pageItems.length;
		hasMore = pageItems.length > 0 && result.has_more;
	} catch (err) {
		toastStore.error(getErrorMessage(err));
	} finally {
		loading = false;
	}
}

function loadMore() {
	if (!hasMore || loading) return;
	void load(false);
}

/**
 * One list for the whole app: the chat page's history panel and the simple-mode
 * sidebar both read it, so neither needs its own fetch. `init()` wires the
 * reactive parts once, from the app shell, because effects must start inside a
 * component.
 */
export const conversationHistory = {
	get items() {
		return items;
	},
	get loading() {
		return loading;
	},
	get hasMore() {
		return hasMore;
	},
	get search() {
		return search;
	},
	get query() {
		return query;
	},
	init() {
		// A different agent or search starts the list over; the key keeps the
		// effect from refetching on unrelated state changes.
		$effect(() => {
			const agent = agentSelection.selected;
			if (!agent) return;
			const next = `${agent}:${query}`;
			if (next === key) return;
			key = next;
			items = [];
			offset = 0;
			hasMore = false;
			void load(true);
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
	loadMore,
	/** Reloads the first page, e.g. after a run finished and the title changed. */
	refresh() {
		void load(true);
	},
	/** Keeps the list in step with a title the server renamed in the background. */
	update(id: string, title: string) {
		items = items.map((item) => (item.id === id ? { ...item, title } : item));
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
			items = items.filter((item) => item.id !== id);
			offset = Math.max(0, offset - 1);
			toastStore.success(t('myaaw.conversations.deleted'));
			return true;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			return false;
		}
	}
};
