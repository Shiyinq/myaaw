import { browser } from '$app/environment';

/**
 * Conversations that gained a turn the reader has not looked at yet. The marker
 * is per agent, because an event can belong to another agent than the one shown,
 * and it must survive a closed tab: the server is the only record of what landed
 * while the dashboard was gone, so a per-conversation `updated_at` watermark
 * decides what is still unread on the next visit.
 */
type UnreadMap = Record<string, Record<string, true>>;

/** The newest server timestamp the reader has acknowledged, per conversation. */
interface SeenState {
	/** Newest timestamp observed when this agent was last synced. */
	baseline: Record<string, string>;
	seen: Record<string, Record<string, string>>;
}

interface SummaryLike {
	id: string;
	updated_at?: string;
}

const UNREAD_KEY = 'myaaw_unread_chats';
const SEEN_KEY = 'myaaw_chat_seen';

function readJson<T>(key: string, fallback: T): T {
	if (!browser) return fallback;
	try {
		const raw = localStorage.getItem(key);
		if (!raw) return fallback;
		const parsed = JSON.parse(raw) as T;
		return parsed && typeof parsed === 'object' ? parsed : fallback;
	} catch {
		return fallback;
	}
}

function writeJson(key: string, value: unknown) {
	if (!browser) return;
	try {
		localStorage.setItem(key, JSON.stringify(value));
	} catch {
		// A full quota must not break the list; the marker simply does not survive.
	}
}

/** Milliseconds from a server timestamp, or 0 when it cannot be read. */
function time(value?: string): number {
	if (!value) return 0;
	const parsed = Date.parse(value);
	return Number.isNaN(parsed) ? 0 : parsed;
}

let state = $state<UnreadMap>(readJson<UnreadMap>(UNREAD_KEY, {}));
let seenState = $state<SeenState>(readJson<SeenState>(SEEN_KEY, { baseline: {}, seen: {} }));

/** The conversation open in the chat route; not persisted, it is only "now". */
let openAgent = '';
let openId = '';

function persistUnread() {
	writeJson(UNREAD_KEY, state);
}

function persistSeen() {
	writeJson(SEEN_KEY, seenState);
}

function mark(agent: string, id: string) {
	if (!agent || !id) return;
	if (state[agent]?.[id]) return;
	state = { ...state, [agent]: { ...(state[agent] ?? {}), [id]: true } };
	persistUnread();
}

function clear(agent: string, id: string): boolean {
	if (!agent || !id || !state[agent]?.[id]) return false;
	const next = { ...state[agent] };
	delete next[id];
	state = { ...state, [agent]: next };
	persistUnread();
	return true;
}

/**
 * Records that the reader has seen a conversation up to `updatedAt` (the server
 * timestamp of the newest turn) and drops its unread marker.
 */
function ack(agent: string, id: string, updatedAt?: string) {
	if (!agent || !id) return;
	const at = time(updatedAt);
	const current = time(seenState.seen[agent]?.[id]);
	if (at > current) {
		seenState = {
			...seenState,
			seen: { ...seenState.seen, [agent]: { ...(seenState.seen[agent] ?? {}), [id]: updatedAt! } }
		};
		persistSeen();
	}
	clear(agent, id);
}

/**
 * Compares a page of conversation summaries with what the reader has seen. The
 * first sync of an agent only records where things stand; after that a newer
 * `updated_at` marks a conversation unread, and a conversation the reader has
 * never seen is unread only when it is newer than that first baseline (so paging
 * through old chats never floods the list).
 */
function observe(agent: string, items: SummaryLike[]) {
	if (!browser || !agent || items.length === 0) return;

	const firstSync = !seenState.baseline[agent];
	const seenFor = { ...(seenState.seen[agent] ?? {}) };
	let baseline: string | undefined = seenState.baseline[agent];
	let baselineAt = time(baseline);
	let seenChanged = false;
	let unreadChanged = false;
	let nextUnread = state;

	for (const item of items) {
		if (!item?.id) continue;
		const at = time(item.updated_at);
		if (at > baselineAt) {
			baseline = item.updated_at;
			baselineAt = at;
			seenChanged = true;
		}
		if (!at) continue;

		if (agent === openAgent && item.id === openId) {
			// The reader is on this conversation right now; it is never unread.
			if (at > time(seenFor[item.id])) {
				seenFor[item.id] = item.updated_at!;
				seenChanged = true;
			}
			if (nextUnread[agent]?.[item.id]) {
				const kept = { ...nextUnread[agent] };
				delete kept[item.id];
				nextUnread = { ...nextUnread, [agent]: kept };
				unreadChanged = true;
			}
			continue;
		}

		if (firstSync) {
			if (!seenFor[item.id]) {
				seenFor[item.id] = item.updated_at!;
				seenChanged = true;
			}
			continue;
		}

		const seenAt = time(seenFor[item.id]);
		const advanced = seenAt ? at > seenAt : at > time(seenState.baseline[agent]);
		if (advanced && !nextUnread[agent]?.[item.id]) {
			nextUnread = {
				...nextUnread,
				[agent]: { ...(nextUnread[agent] ?? {}), [item.id]: true }
			};
			unreadChanged = true;
		}
	}

	if (seenChanged) {
		seenState = {
			...seenState,
			baseline: baseline ? { ...seenState.baseline, [agent]: baseline } : seenState.baseline,
			seen: { ...seenState.seen, [agent]: seenFor }
		};
		persistSeen();
	}
	if (unreadChanged) {
		state = nextUnread;
		persistUnread();
	}
}

export const unreadChats = {
	has(agent: string, id: string): boolean {
		return !!(agent && id && state[agent]?.[id]);
	},
	mark,
	clear,
	ack,
	observe,
	/** True while that conversation is the one open in the chat route. */
	isOpen(agent: string, id: string): boolean {
		return !!agent && agent === openAgent && id === openId;
	},
	/** The chat page tells the store which conversation is on screen. */
	setOpen(agent: string, id: string) {
		openAgent = agent;
		openId = id;
	},
	/** Clears every marker of one agent, e.g. when its list is fully read. */
	clearAgent(agent: string) {
		if (!agent || !state[agent]) return;
		const next = { ...state };
		delete next[agent];
		state = next;
		persistUnread();
	}
};
