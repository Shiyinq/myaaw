import { browser } from '$app/environment';
import { conversations as conversationsApi } from '$lib/apis/conversations';

/**
 * Conversations with a turn the reader has not acknowledged yet.
 *
 * The marker is in memory only: `conversations.read_at` on the server is the
 * single source of truth, and every list response recomputes the dots from it.
 * That is what makes a reload, a closed tab, and another device agree without
 * any local storage. The in-memory copy exists so a live event can paint a dot
 * immediately, before the next list fetch.
 */
type UnreadMap = Record<string, Record<string, true>>;

interface SummaryLike {
	id: string;
	updated_at?: string;
	/** Newest turn this user acknowledged; null means never acknowledged. */
	read_at?: string | null;
}

// Keys of the earlier local-watermark implementation. The server owns this now,
// so anything left in a browser from that version is dropped.
if (browser) {
	localStorage.removeItem('myaaw_unread_chats');
	localStorage.removeItem('myaaw_chat_seen');
}

let state = $state<UnreadMap>({});
/**
 * The conversation open in the chat route; not persisted, it is only "now". Runes
 * so an approval alert re-evaluates when the reader switches chats.
 */
let openAgent = $state('');
let openId = $state('');
/**
 * Bumped whenever a live event marks a conversation unread. A list response that
 * was already in flight when that happened must not clear the newer marker, so
 * its revision is captured before the request.
 */
let revisions = 0;

/** Milliseconds from a server timestamp, or 0 when it cannot be read. */
function time(value?: string | null): number {
	if (!value) return 0;
	const parsed = Date.parse(value);
	return Number.isNaN(parsed) ? 0 : parsed;
}

function mark(agent: string, id: string) {
	if (!agent || !id) return;
	revisions += 1;
	if (state[agent]?.[id]) return;
	state = { ...state, [agent]: { ...(state[agent] ?? {}), [id]: true } };
}

function clear(agent: string, id: string): boolean {
	if (!agent || !id || !state[agent]?.[id]) return false;
	const next = { ...state[agent] };
	delete next[id];
	state = { ...state, [agent]: next };
	return true;
}

/**
 * Records that the reader has seen a conversation up to `updatedAt` and pushes
 * the watermark to the server, so the other devices agree. A live event has no
 * precise timestamp to send, so it acknowledges "now" and lets the server clamp
 * it to `updated_at`.
 */
function ack(agent: string, id: string, updatedAt?: string) {
	if (!agent || !id) return;
	clear(agent, id);
	// Best effort: the dot is already gone locally, and a failed request is
	// corrected by the next list response.
	void conversationsApi.markRead(agent, id, updatedAt).catch(() => {});
}

/**
 * Recomputes the dots from one page of conversation summaries. `read_at` is the
 * server's watermark: a conversation is unread when `updated_at` is newer, and a
 * null watermark means it was never acknowledged. A conversation that is on
 * screen is never dotted, and its watermark is pushed to the server when it lags.
 */
function observe(agent: string, items: SummaryLike[], revisionAtRequest = revisions) {
	if (!browser || !agent || items.length === 0) return;

	for (const item of items) {
		if (!item?.id) continue;
		const at = time(item.updated_at);
		if (!at) continue;

		if (agent === openAgent && item.id === openId) {
			// On screen: never a dot. If the server is behind (a live turn, or a
			// first open), acknowledge it so the other devices agree.
			if (!item.read_at || at > time(item.read_at)) ack(agent, item.id);
			else clear(agent, item.id);
			continue;
		}

		const unreadNow = item.read_at ? at > time(item.read_at) : true;
		if (unreadNow) {
			mark(agent, item.id);
		} else if (revisionAtRequest === revisions) {
			// Another device read it, and no live event arrived while this
			// response was in flight.
			clear(agent, item.id);
		}
	}
}

export const unreadChats = {
	has(agent: string, id: string): boolean {
		return !!(agent && id && state[agent]?.[id]);
	},
	mark,
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
	/** Snapshot of the live-event counter, taken before a list request. */
	revision(): number {
		return revisions;
	}
};
