import { groupAccent } from './accent';

export interface MentionTarget {
	name: string;
	color?: string;
}

export interface MentionSegment {
	text: string;
	mention?: {
		handle: string;
		isAll: boolean;
		color?: string;
	};
}

// Same boundary rule as the backend orchestrator (`internal/services/controlplane/service/groups.go`):
// matches `@handle` at start or after non-word/non-`.`/non-`@`/non-`/` characters,
// so emails (`a@b.com`) and URLs (`https://x.com/@name`) are never matched.
const MENTION_PATTERN = /(^|[^\w.@/])@([A-Za-z0-9][A-Za-z0-9_-]*)/g;

const SKIP_SELECTOR =
	'code, pre, a, kbd, script, style, [data-mention-pill], [data-code-block], [data-mermaid]';

export function mentionPillClass(color?: string, isAll = false): string {
	const base =
		'inline-block rounded-md px-1 py-0.5 text-[0.92em] leading-tight font-semibold ring-1 ring-inset';
	if (isAll) {
		return `${base} bg-primary/15 text-primary ring-primary/25`;
	}
	const accent = groupAccent(color);
	return `${base} ${accent.softBg} ${accent.text} ${accent.ring}`;
}

function buildLookup(mentions: MentionTarget[]): Map<string, MentionTarget> {
	const map = new Map<string, MentionTarget>();
	for (const entry of mentions) {
		if (entry.name) map.set(entry.name.toLowerCase(), entry);
	}
	return map;
}

/**
 * Splits plain text into regular text runs and valid `@member` / `@all` pill
 * segments, preserving original text verbatim.
 */
export function splitMentionSegments(text: string, mentions: MentionTarget[]): MentionSegment[] {
	if (!text) return [];
	const lookup = buildLookup(mentions);
	const segments: MentionSegment[] = [];
	const regex = new RegExp(MENTION_PATTERN.source, 'g');
	let lastIndex = 0;

	for (const match of text.matchAll(regex)) {
		const fullStart = match.index ?? 0;
		const prefix = match[1] ?? '';
		const handle = match[2] ?? '';
		const lower = handle.toLowerCase();
		const isAll = lower === 'all';
		const target = isAll ? undefined : lookup.get(lower);

		if (!isAll && !target) continue;

		const tokenStart = fullStart + prefix.length;
		const tokenEnd = fullStart + match[0].length;
		if (tokenStart > lastIndex) {
			segments.push({ text: text.slice(lastIndex, tokenStart) });
		}
		segments.push({
			text: text.slice(tokenStart, tokenEnd),
			mention: {
				handle,
				isAll,
				color: target?.color
			}
		});
		lastIndex = tokenEnd;
	}

	if (lastIndex < text.length) {
		segments.push({ text: text.slice(lastIndex) });
	}
	return segments;
}

export interface HighlightMentionsOptions {
	mentions: MentionTarget[];
	/** Reactive trigger key (e.g. message content) so updates re-apply highlights. */
	key: string;
}

const OBSERVE_OPTIONS: MutationObserverInit = {
	childList: true,
	subtree: true,
	characterData: true
};

/**
 * Svelte action that wraps valid `@member` and `@all` tokens inside rendered
 * Markdown text nodes with crisp highlight pills, strictly skipping code,
 * links, keyboard tags, and Mermaid diagrams. Preserves `textContent` verbatim
 * so text-selection quoting (`AskQuote`) stays offset-accurate.
 */
export function highlightMentions(node: HTMLElement, initial: HighlightMentionsOptions) {
	let current = initial;
	let destroyed = false;

	const observer = new MutationObserver(() => {
		if (!destroyed) apply();
	});

	function apply() {
		if (destroyed) return;
		// Disconnect and drain queued records BEFORE mutating the DOM so our own
		// unwrap/wrap changes can never queue an asynchronous microtask loop.
		observer.disconnect();
		observer.takeRecords();
		try {
			// Unwrap any stale pills first so member additions/removals re-evaluate cleanly.
			const existingPills = node.querySelectorAll<HTMLElement>('[data-mention-pill]');
			if (existingPills.length > 0) {
				for (const pill of existingPills) {
					pill.replaceWith(document.createTextNode(pill.textContent ?? ''));
				}
				node.normalize();
			}

			const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
			const textNodes: Text[] = [];
			let cursor: Node | null;
			while ((cursor = walker.nextNode())) {
				const textNode = cursor as Text;
				if (!textNode.nodeValue || !textNode.nodeValue.includes('@')) continue;
				if (textNode.parentElement?.closest(SKIP_SELECTOR)) continue;
				textNodes.push(textNode);
			}

			for (const textNode of textNodes) {
				const raw = textNode.nodeValue ?? '';
				const segments = splitMentionSegments(raw, current.mentions);
				if (!segments.some((seg) => seg.mention)) continue;

				const fragment = document.createDocumentFragment();
				for (const seg of segments) {
					if (seg.mention) {
						const span = document.createElement('span');
						span.dataset.mentionPill = seg.mention.isAll ? 'all' : seg.mention.handle;
						span.className = mentionPillClass(seg.mention.color, seg.mention.isAll);
						span.textContent = seg.text;
						fragment.appendChild(span);
					} else {
						fragment.appendChild(document.createTextNode(seg.text));
					}
				}
				textNode.parentNode?.replaceChild(fragment, textNode);
			}
		} finally {
			if (!destroyed) {
				observer.observe(node, OBSERVE_OPTIONS);
			}
		}
	}

	apply();

	return {
		update(next: HighlightMentionsOptions) {
			current = next;
			apply();
		},
		destroy() {
			destroyed = true;
			observer.disconnect();
		}
	};
}
