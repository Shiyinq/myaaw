import type { ExtraMessage, InteractionKind, InteractionState } from '$lib/types';

export interface ParsedInteraction {
	kind: InteractionKind | null;
	payload: Record<string, unknown> | null;
	text: string;
}

const INTERACTION_PREFIX_RE =
	/^\s*<interaction\b[^>]*\bkind="(button|quick_reply|form|checklist)"[^>]*>([\s\S]*?)<\/interaction>\s*/i;

/**
 * Strips the leading `<interaction kind="...">...</interaction>` envelope from
 * a user message so the WebUI can show a dedicated interaction badge (like
 * `Scheduled` / `Sub-agent`) alongside the human-readable summary text.
 */
export function parseInteractionText(raw: string): ParsedInteraction {
	if (!raw) return { kind: null, payload: null, text: '' };
	const match = raw.match(INTERACTION_PREFIX_RE);
	if (!match) {
		return { kind: null, payload: null, text: raw };
	}
	const kind = match[1].toLowerCase() as InteractionKind;
	const jsonBody = (match[2] ?? '').trim();
	let payload: Record<string, unknown> | null = null;
	if (jsonBody) {
		try {
			const parsed: unknown = JSON.parse(jsonBody);
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
				payload = parsed as Record<string, unknown>;
			}
		} catch {
			payload = null;
		}
	}
	const rest = raw.slice(match[0].length).trim();
	return { kind, payload, text: rest };
}

/**
 * Strips any `<interaction>...</interaction>` envelope for plain-text previews
 * (such as sidebar last-message snippets or reply previews).
 */
export function stripInteractionTags(raw: string): string {
	return raw
		.replace(/<interaction\b[^>]*>[\s\S]*?<\/interaction>\s*/gi, '')
		.replace(/<interaction\b[\s\S]*$/i, '')
		.trim();
}

/**
 * Merges incoming extra_messages batches while deduplicating by batch `id`
 * (keeping newer snapshots when an id repeats).
 */
export function mergeExtraMessages(
	existing: ExtraMessage[] | undefined,
	incoming: ExtraMessage[] | undefined
): ExtraMessage[] {
	const base = existing ?? [];
	const next = incoming ?? [];
	if (next.length === 0) return base;
	if (base.length === 0) return next;
	const out = [...base];
	for (const item of next) {
		if (!item?.id) {
			out.push(item);
			continue;
		}
		const idx = out.findIndex((entry) => entry.id === item.id);
		if (idx >= 0) {
			out[idx] = item;
		} else {
			out.push(item);
		}
	}
	return out;
}

/**
 * Immutably updates the `state` of one block inside an `ExtraMessage[]` list.
 */
export function patchExtraBlockState(
	extraMessages: ExtraMessage[] | undefined,
	blockId: string,
	state: InteractionState
): ExtraMessage[] | undefined {
	if (!extraMessages || extraMessages.length === 0 || !blockId) return extraMessages;
	let changed = false;
	const updated = extraMessages.map((msg) => {
		let msgChanged = false;
		const nextBlocks = (msg.blocks ?? []).map((block) => {
			if (block.id !== blockId) return block;
			msgChanged = true;
			changed = true;
			return { ...block, state };
		});
		return msgChanged ? { ...msg, blocks: nextBlocks } : msg;
	});
	return changed ? updated : extraMessages;
}
