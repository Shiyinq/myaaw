import { browser } from '$app/environment';
import { showChatNotification } from '$lib/notifications/browser.svelte';
import { playNotification } from '$lib/sound/notify.svelte';
import { botPreviews, lastAssistantFromItems } from '$lib/stores/botPreviews.svelte';
import { chatRuns, type ChatRun } from '$lib/stores/chatRuns.svelte';
import { conversationsStore } from '$lib/stores/conversations.svelte';
import { observabilityStore } from '$lib/stores/observability.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import { unreadChats } from '$lib/stores/unreadChats.svelte';

/**
 * A chat the reader typed is a run of its own. It keeps streaming after its page
 * unmounts, so a reader who switched chats, opened another menu, or reloaded
 * would otherwise never learn that the answer arrived. Triggered turns already
 * announce themselves over SSE; this watcher does the same for the runs the
 * reader started, using the store they already live in.
 */

/** Last `streaming` value per run, so only the true → false edge announces. */
const previous = new Map<string, boolean>();

function announce(run: ChatRun) {
	const conversationId = run.conversationId;
	if (!conversationId) return;

	// A finished turn made any cached transcript or stats for this conversation
	// stale; mark them dirty so reopening the chat never serves the old copy.
	conversationsStore.detail.invalidate();
	observabilityStore.sessionStats.invalidate({ agent: run.agent, id: conversationId });

	// Push the finished answer straight into Bots mode's left sidebar preview so
	// switching away to another bot still shows the new reply immediately.
	const last = lastAssistantFromItems(run.items);
	const updatedAt =
		last.updatedAt ||
		(run.finishedAt ? new Date(run.finishedAt).toISOString() : new Date().toISOString());
	const existing = botPreviews.get(run.agent);
	botPreviews.seed(
		run.agent,
		{
			conversationId,
			text: last.text || existing?.text || '',
			title: existing?.title,
			updatedAt
		},
		true
	);

	// The run finished normally, so the tone belongs to the completion itself,
	// whichever page happens to be mounted. Keeping it here means one alert path
	// for a chat the reader typed, the way SSE is one path for a triggered turn.
	if (preferences.value.notifyEnabled) {
		playNotification(preferences.value.notifySound, preferences.value.notifyVolume);
	}

	// A mounted page means the chat is on screen: the reader needs no unread
	// marker for a chat they are already looking at.
	const onScreen = run.view !== null;
	if (!onScreen) {
		unreadChats.mark(run.agent, conversationId);
	}
	// A hidden tab is not being read, even when the page is still mounted.
	const hidden = document.visibilityState === 'hidden';
	if (preferences.value.notifyBrowser && (!onScreen || hidden)) {
		showChatNotification({ agent: run.agent, conversationId, trigger: 'chat' });
	}
}

export const runCompletion = {
	/**
	 * Compares the runs against the previous pass. Call it from an effect that
	 * reads the store: reading `streaming` there is what re-runs it on a finish.
	 */
	watch() {
		if (!browser) return;
		const current = new Set<string>();
		for (const run of chatRuns.all()) {
			current.add(run.requestId);
			const was = previous.get(run.requestId);
			if (was === true && !run.streaming && run.finishReason === 'done') announce(run);
			previous.set(run.requestId, run.streaming);
		}
		for (const key of [...previous.keys()]) {
			if (!current.has(key)) previous.delete(key);
		}
	}
};
