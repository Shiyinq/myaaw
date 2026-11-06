import { browser } from '$app/environment';
import {
	conversationEvents as conversationEventsApi,
	type ConversationUpdatedEvent
} from '$lib/apis/events';
import { showChatNotification } from '$lib/notifications/browser.svelte';
import { playNotification } from '$lib/sound/notify.svelte';
import { agentSelection } from '$lib/stores/selectedAgent.svelte';
import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import { unreadChats } from '$lib/stores/unreadChats.svelte';

/**
 * One SSE connection per agent, kept open while the dashboard is signed in. A
 * background turn (cron, heartbeat, sub-agent) publishes `conversation.updated`
 * on the agent server; this store fans it out to the list, the open chat, the
 * sound, and the browser notification.
 */
type Listener = (event: ConversationUpdatedEvent) => void;

const listeners = new Set<Listener>();
const controllers = new Map<string, AbortController>();
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

	unreadChats.mark(agent, id);
	notify(event);

	// Sound and browser notification are deliberately independent settings, and
	// they fire for every agent, not only the one currently shown.
	if (preferences.value.notifyEnabled) {
		playNotification(preferences.value.notifySound, preferences.value.notifyVolume);
	}
	if (preferences.value.notifyBrowser) {
		showChatNotification({ agent, conversationId: id, trigger: raw.trigger });
	}

	// The list only needs a refetch when the event belongs to the visible agent.
	if (agentSelection.selected === agent) conversationHistory.refresh();
}

function connect(agent: string) {
	if (!agent || controllers.has(agent)) return;
	const controller = new AbortController();
	controllers.set(agent, controller);
	void conversationEventsApi
		.stream(
			agent,
			(sse) => {
				if (sse.event !== 'conversation.updated') return;
				const data = sse.data as ConversationUpdatedEvent | null;
				if (!data?.conversation_id) return;
				handle(data, agent);
			},
			controller.signal
		)
		.catch(() => {
			// Retries are exhausted (agent stopped, network gone). A later `start`
			// connects again instead of leaving a dead controller behind.
			controllers.delete(agent);
		});
}

export const chatUpdates = {
	/** Subscribes a consumer (the open chat) to every background turn. */
	subscribe(listener: Listener) {
		listeners.add(listener);
		return () => listeners.delete(listener);
	},
	/**
	 * Reconciles the open streams with the agents whose server is running: stops
	 * the ones that went away and connects the ones that just started. A stopped
	 * agent must not be polled, or every retry turns into a 502 behind the proxy.
	 */
	sync(agents: string[]) {
		if (!browser) return;
		const wanted = new Set(agents);
		for (const [agent, controller] of controllers) {
			if (wanted.has(agent)) continue;
			controller.abort();
			controllers.delete(agent);
		}
		for (const agent of wanted) connect(agent);
	},
	stop() {
		for (const controller of controllers.values()) controller.abort();
		controllers.clear();
	}
};
