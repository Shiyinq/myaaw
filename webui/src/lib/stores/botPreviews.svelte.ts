import type { ChatItem } from '$lib/stores/chatRuns.svelte';
import type { Message } from '$lib/types';

export interface BotPreview {
	conversationId: string;
	/** Plain single-line snippet of the agent's latest response. */
	text: string;
	/** Fallback conversation title when no assistant response text exists yet. */
	title?: string;
	/** ISO timestamp displayed on the bot row. */
	updatedAt?: string;
	/**
	 * Monotonic high-water ISO timestamp of actual conversation activity used for
	 * sorting the bot list, so opening New Chat or browsing older history never
	 * drops a bot to the bottom of the sidebar.
	 */
	activityAt?: string;
	/** True when driven directly by the chat view currently open for this bot. */
	fromOpenChat?: boolean;
}

function parseIso(value?: string): number {
	if (!value) return 0;
	const ms = Date.parse(value);
	return Number.isFinite(ms) ? ms : 0;
}

function newerIso(a?: string, b?: string): string | undefined {
	const msA = parseIso(a);
	const msB = parseIso(b);
	if (msA === 0 && msB === 0) return a || b;
	return msB >= msA ? b : a;
}

const MAX_PREVIEW_CHARS = 140;

/** Flattens markdown formatting into a clean single-line inbox snippet. */
export function previewSnippet(raw: string | undefined | null): string {
	if (!raw) return '';
	const cleaned = raw
		.replace(/```[a-z0-9_-]*\n?([\s\S]*?)```/gi, ' $1 ')
		.replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
		.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
		.replace(/`([^`]+)`/g, '$1')
		.replace(/(\*\*|__|~~)(.*?)\1/g, '$2')
		.replace(/(^|\s)\*([^*\n]+)\*(?=\s|$|[.,!?;:])/g, '$1$2')
		.replace(/^\s{0,3}#{1,6}\s+/gm, '')
		.replace(/^\s{0,3}>\s?/gm, '')
		.replace(/^\s*[-*+]\s+/gm, '')
		.replace(/\s+/g, ' ')
		.trim();
	if (cleaned.length <= MAX_PREVIEW_CHARS) return cleaned;
	return `${cleaned.slice(0, MAX_PREVIEW_CHARS - 1).trimEnd()}…`;
}

/** Extracts plain text from a stored assistant message's content payload. */
function messageText(message: Message): string {
	if (typeof message.content === 'string') return message.content;
	if (!Array.isArray(message.content)) return '';
	return message.content
		.filter((part) => part.type === 'text' && typeof part.text === 'string')
		.map((part) => part.text ?? '')
		.join(' ');
}

/** Finds the latest non-empty assistant response inside stored transcript messages. */
export function lastAssistantFromMessages(messages: Message[]): {
	text: string;
	updatedAt?: string;
} {
	for (let i = messages.length - 1; i >= 0; i -= 1) {
		const message = messages[i];
		if (!message || message.role !== 'assistant') continue;
		const text = previewSnippet(messageText(message));
		if (text) return { text, updatedAt: message.created_at };
	}
	return { text: '' };
}

/** Finds the latest non-empty assistant response inside the live chat timeline. */
export function lastAssistantFromItems(items: ChatItem[]): {
	text: string;
	updatedAt?: string;
} {
	for (let i = items.length - 1; i >= 0; i -= 1) {
		const item = items[i];
		if (!item || item.kind !== 'text') continue;
		const text = previewSnippet(item.text);
		if (text) return { text, updatedAt: item.createdAt };
	}
	return { text: '' };
}

/**
 * Finds the assistant text being written for the currently streaming turn
 * (after the last user message), or '' while the agent is still thinking/running tools.
 */
export function streamingReplyFromItems(items: ChatItem[]): string {
	let lastUser = -1;
	for (let i = items.length - 1; i >= 0; i -= 1) {
		if (items[i]?.kind === 'user') {
			lastUser = i;
			break;
		}
	}
	for (let i = items.length - 1; i > lastUser; i -= 1) {
		const item = items[i];
		if (!item || item.kind !== 'text') continue;
		const text = previewSnippet(item.text);
		if (text) return text;
	}
	return '';
}

let previews = $state<Record<string, BotPreview>>({});

/**
 * Reactive per-bot preview state for Bots mode's left sidebar.
 *
 * The open chat page pushes live updates via `setOpenChat` (covering history
 * switches, new chats, and live responses), while `BotsSidebar` seeds other
 * bots from their latest conversation tail via `seed`.
 */
export const botPreviews = {
	get(agent: string): BotPreview | undefined {
		return previews[agent];
	},
	setOpenChat(
		agent: string,
		next: Omit<BotPreview, 'fromOpenChat'>,
		options: { bumpActivity?: boolean } = {}
	) {
		if (!agent) return;
		const current = previews[agent];
		const activityAt = options.bumpActivity
			? newerIso(current?.activityAt, next.activityAt ?? next.updatedAt)
			: (current?.activityAt ?? next.activityAt ?? next.updatedAt);
		if (
			current &&
			current.fromOpenChat &&
			current.conversationId === next.conversationId &&
			current.text === next.text &&
			current.title === next.title &&
			current.updatedAt === next.updatedAt &&
			current.activityAt === activityAt
		) {
			return;
		}
		previews = { ...previews, [agent]: { ...next, activityAt, fromOpenChat: true } };
	},
	seed(agent: string, next: Omit<BotPreview, 'fromOpenChat'>, force = false) {
		if (!agent) return;
		const current = previews[agent];
		const activityAt = newerIso(current?.activityAt, next.activityAt ?? next.updatedAt);
		if (!force && current?.fromOpenChat) {
			if (activityAt && activityAt !== current.activityAt) {
				previews = { ...previews, [agent]: { ...current, activityAt } };
			}
			return;
		}
		previews = { ...previews, [agent]: { ...next, activityAt, fromOpenChat: false } };
	}
};
