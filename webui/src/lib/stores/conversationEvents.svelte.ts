import { browser } from '$app/environment';
import {
	conversationEvents as conversationEventsApi,
	type ConversationUpdatedEvent
} from '$lib/apis/events';
import { showChatNotification } from '$lib/notifications/browser.svelte';
import { approvalQueue, type ApprovalEvent } from '$lib/stores/approvalQueue.svelte';
import { playNotification } from '$lib/sound/notify.svelte';
import { agentSelection } from '$lib/stores/selectedAgent.svelte';
import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
import { conversationsStore } from '$lib/stores/conversations.svelte';
import { groupsStore } from '$lib/stores/groups.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import { unreadChats } from '$lib/stores/unreadChats.svelte';
import type { GroupMessage, GroupRunState } from '$lib/types';

/**
 * Single multiplexed SSE connection for all running agents, kept open while the
 * dashboard is signed in. Using one stream instead of one per agent prevents
 * HTTP/1.1 browser connection pool exhaustion (6 sockets/origin) when multiple
 * bots are running alongside a group stream or active chat run.
 */
type Listener = (event: ConversationUpdatedEvent) => void;

const listeners = new Set<Listener>();
let activeController: AbortController | null = null;
let activeKey = '';
let unloadHooked = false;
/** Last event handled per conversation, so a reconnect cannot double-notify. */
const lastSeen = new Map<string, string>();

function notify(event: ConversationUpdatedEvent) {
	for (const listener of [...listeners]) listener(event);
}

function handle(raw: ConversationUpdatedEvent, connectionAgent: string) {
	const agent = raw.agent || connectionAgent;
	const id = raw.conversation_id;
	if (!agent || !id) return;

	const key = `${agent}:${id}`;
	if (raw.at && lastSeen.get(key) === raw.at) return;
	if (raw.at) lastSeen.set(key, raw.at);

	const event: ConversationUpdatedEvent = { ...raw, agent, conversation_id: id };

	// A background turn changed the stored transcript, so drop cached detail freshness.
	conversationsStore.detail.invalidate();

	// A dot is only for a conversation the reader is not looking at. When it is
	// open, the event is acknowledged instead, which also records the watermark so
	// a later list sync does not call the same turn unread.
	const onScreen = unreadChats.isOpen(agent, id);
	if (onScreen) {
		// No precise server timestamp here: the event's `at` is the publish time, and
		// acknowledging with it can leave the watermark a fraction of a second behind
		// the turn's updated_at, which would show up as unread again.
		unreadChats.ack(agent, id);
	} else {
		unreadChats.mark(agent, id);
	}
	notify(event);

	// The tone is independent of what is on screen, like a finished normal chat.
	// The browser notification is not: it follows the same rule as a chat the
	// reader left, so it only fires when that conversation is not being read
	// (another menu or chat, or a hidden tab).
	if (preferences.value.notifyEnabled) {
		playNotification(preferences.value.notifySound, preferences.value.notifyVolume);
	}
	const hidden = document.visibilityState === 'hidden';
	if (preferences.value.notifyBrowser && (!onScreen || hidden)) {
		showChatNotification({ agent, conversationId: id, trigger: raw.trigger });
	}

	// The list only needs a refetch when the event belongs to the visible agent.
	if (agentSelection.selected === agent) conversationHistory.refresh();
}

function ensureUnloadCleanup() {
	if (!browser || unloadHooked) return;
	unloadHooked = true;
	const stop = () => chatUpdates.stop();
	window.addEventListener('beforeunload', stop);
	window.addEventListener('pagehide', stop);
}

function connectAll(agents: string[], key: string) {
	ensureUnloadCleanup();
	activeController?.abort();
	const controller = new AbortController();
	activeController = controller;
	activeKey = key;

	void conversationEventsApi
		.streamAll(
			agents,
			(sse) => {
				if (sse.event === 'group.updated') {
					const payload = sse.data as {
						group_id?: string;
						run_state?: GroupRunState;
						message?: GroupMessage;
					} | null;
					if (payload?.group_id) {
						if (payload.run_state) {
							groupsStore.syncSummaryFromRunState(payload.group_id, payload.run_state);
							approvalQueue.syncGroup(payload.group_id, payload.run_state.pending_approvals ?? []);
						}
						if (payload.message) {
							groupsStore.syncSummaryFromMessage(payload.group_id, payload.message);
							if (
								payload.message.sender_type === 'agent' &&
								groupsStore.claimMessageAnnouncement(payload.message.id)
							) {
								if (preferences.value.notifyEnabled) {
									playNotification(preferences.value.notifySound, preferences.value.notifyVolume);
								}
								const onScreen = groupsStore.isOpen(payload.group_id);
								if (!onScreen) {
									groupsStore.markUnread(payload.group_id);
								}
								const hidden = document.visibilityState === 'hidden';
								if (preferences.value.notifyBrowser && (!onScreen || hidden)) {
									const groupName =
										groupsStore.nameOf(payload.group_id) || payload.message.sender_name || 'Group';
									showChatNotification({
										agent: groupName,
										conversationId: `group:${payload.group_id}`,
										trigger: 'chat'
									});
								}
							}
						}
					}
					return;
				}
				if (sse.event === 'approval.required') {
					const payload = sse.data as (ApprovalEvent & { agent?: string }) | null;
					const agent = payload?.agent || '';
					if (payload && agent) approvalQueue.required(agent, payload);
					return;
				}
				if (sse.event === 'approval.resolved') {
					const payload = sse.data as ApprovalEvent | null;
					if (payload) approvalQueue.resolved(payload);
					return;
				}
				if (sse.event !== 'conversation.updated') return;
				const data = sse.data as ConversationUpdatedEvent | null;
				if (!data?.conversation_id) return;
				if (data.scope?.startsWith('group:')) return;
				handle(data, data.agent || '');
			},
			controller.signal
		)
		.catch(() => {
			if (activeController === controller) {
				activeController = null;
				activeKey = '';
			}
		});
}

export const chatUpdates = {
	/** Subscribes a consumer (the open chat) to every background turn. */
	subscribe(listener: Listener) {
		listeners.add(listener);
		return () => listeners.delete(listener);
	},
	/**
	 * Reconciles the multiplexed event stream with the set of currently running
	 * agents. Stopped agents are excluded so the control plane does not spin
	 * trying to dial them.
	 */
	sync(agents: string[]) {
		if (!browser) return;
		const wanted = [...new Set(agents.filter(Boolean))].sort();
		if (wanted.length === 0) {
			chatUpdates.stop();
			return;
		}
		const key = wanted.join(',');
		if (activeController && activeKey === key) return;
		connectAll(wanted, key);
	},
	stop() {
		activeController?.abort();
		activeController = null;
		activeKey = '';
	}
};
