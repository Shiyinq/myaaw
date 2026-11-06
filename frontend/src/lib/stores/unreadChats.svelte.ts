import { browser } from '$app/environment';

/**
 * Conversations that gained a background turn the reader has not looked at yet.
 * The marker is per agent, because an event can belong to an agent other than
 * the one currently shown, and it must still be there after switching.
 */
type UnreadMap = Record<string, Record<string, boolean>>;

const STORAGE_KEY = 'myaaw_unread_chats';

function load(): UnreadMap {
	if (!browser) return {};
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return {};
		const parsed = JSON.parse(raw) as UnreadMap;
		return parsed && typeof parsed === 'object' ? parsed : {};
	} catch {
		return {};
	}
}

let state = $state<UnreadMap>(load());

function persist() {
	if (!browser) return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
	} catch {
		// A full quota must not break the list; the marker simply does not survive.
	}
}

export const unreadChats = {
	has(agent: string, id: string): boolean {
		return !!(agent && id && state[agent]?.[id]);
	},
	mark(agent: string, id: string) {
		if (!agent || !id) return;
		if (state[agent]?.[id]) return;
		state = { ...state, [agent]: { ...(state[agent] ?? {}), [id]: true } };
		persist();
	},
	clear(agent: string, id: string) {
		if (!agent || !id || !state[agent]?.[id]) return;
		const next = { ...state[agent] };
		delete next[id];
		state = { ...state, [agent]: next };
		persist();
	},
	/** Clears every marker of one agent, e.g. when its list is fully read. */
	clearAgent(agent: string) {
		if (!agent || !state[agent]) return;
		const next = { ...state };
		delete next[agent];
		state = next;
		persist();
	}
};
