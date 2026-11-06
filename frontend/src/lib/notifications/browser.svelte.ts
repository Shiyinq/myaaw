import { browser } from '$app/environment';
import { t } from '$lib/i18n';

export type NotificationStatus = NotificationPermission | 'unsupported';

/**
 * The browser's local Notification API, not Web Push: nothing leaves the
 * machine and no service worker or push server is involved. Used only for
 * background chat turns; every other action keeps the in-app toast.
 */
export function notificationStatus(): NotificationStatus {
	if (!browser || typeof Notification === 'undefined') return 'unsupported';
	return Notification.permission;
}

export async function requestNotificationPermission(): Promise<NotificationStatus> {
	if (!browser || typeof Notification === 'undefined') return 'unsupported';
	if (Notification.permission === 'granted') return 'granted';
	try {
		return await Notification.requestPermission();
	} catch {
		return Notification.permission;
	}
}

interface ChatNotification {
	agent: string;
	conversationId: string;
	trigger: string;
}

function triggerLabel(trigger: string): string {
	switch (trigger) {
		case 'cron':
			return t('myaaw.chat.trigger_cron');
		case 'heartbeat':
			return t('myaaw.chat.trigger_heartbeat');
		case 'subagent':
			return t('myaaw.chat.trigger_subagent');
		case 'chat':
			return t('myaaw.chat.trigger_chat');
		default:
			return trigger;
	}
}

/**
 * Fires a one-off notification so the reader can verify the OS/browser path
 * independently of the in-app toggle and of a real cron turn. A fresh tag per
 * click keeps a repeat test visible: reusing one tag makes the platform replace
 * the notification instead of raising a new one.
 */
let previewSeq = 0;

export function previewBrowserNotification(): boolean {
	if (!browser || typeof Notification === 'undefined') return false;
	if (Notification.permission !== 'granted') return false;
	try {
		previewSeq += 1;
		new Notification('Myaaw', {
			body: t('myaaw.settings.notify_browser_test_body'),
			tag: `myaaw-preview:${previewSeq}`
		});
		return true;
	} catch {
		return false;
	}
}

/**
 * Alerts the reader that a tool call is waiting for approval. Like the turn
 * notification, the body is a signal only: the tool name, never its arguments.
 */
export function showApprovalNotification({
	agent,
	conversationId,
	toolName,
	subAgent
}: {
	agent: string;
	conversationId: string;
	toolName: string;
	subAgent?: string;
}) {
	if (!browser || typeof Notification === 'undefined') return;
	if (Notification.permission !== 'granted') return;

	const tool = subAgent ? `${toolName} · ${subAgent}` : toolName;
	try {
		new Notification(t('myaaw.chat.notify_title', { agent: agent || 'Myaaw' }), {
			body: t('myaaw.chat.notify_approval_body', { tool }),
			tag: `myaaw-approval:${agent}:${conversationId}`,
			silent: true
		});
	} catch {
		// The in-chat card and the sidebar group still mark the conversation.
	}
}

/**
 * Alerts the reader that a triggered turn produced a message. The body is a
 * signal only — agent name plus turn type — never the transcript: a system
 * notification is visible to anyone near the screen.
 */
export function showChatNotification({ agent, conversationId, trigger }: ChatNotification) {
	if (!browser || typeof Notification === 'undefined') return;
	if (Notification.permission !== 'granted') return;

	try {
		new Notification(t('myaaw.chat.notify_title', { agent: agent || 'Myaaw' }), {
			body: t('myaaw.chat.notify_body', { trigger: triggerLabel(trigger) }),
			tag: `myaaw-chat:${agent}:${conversationId}`,
			// The app plays its own sound, gated by the separate sound setting.
			silent: true
		});
	} catch {
		// Some platforms reject the constructor without a service worker; the
		// in-app badge still marks the conversation.
	}
}
