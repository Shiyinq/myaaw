import type { AskQuote, AskQuoteSource } from '$lib/types';

/** How many excerpts one message may carry. */
export const MAX_ASK_QUOTES = 8;

/**
 * How much of a single selection travels with a message. A whole tool log would
 * crowd the question out of the context window, so longer excerpts are trimmed.
 */
export const MAX_ASK_QUOTE_CHARS = 4000;

const SELECTIONS_BLOCK = /^\s*<selections\b[^>]*>([\s\S]*?)<\/selections>\s*/i;
const SELECTION_ITEM = /<selection\b([^>]*)>([\s\S]*?)<\/selection>/gi;

function escapeAttribute(value: string): string {
	return value.replace(/["<>&]/g, '').trim();
}

/**
 * A literal closing tag inside an excerpt would end the block early, so the
 * angle bracket is escaped. This sequence is vanishingly rare in chat text and
 * keeps the surrounding format intact.
 */
function escapeClosingTags(text: string): string {
	return text.replace(/<\/(selection|selections)>/gi, '<\\/$1>');
}

function readAttribute(attributes: string, name: string): string {
	const double = attributes.match(new RegExp(`${name}\\s*=\\s*"([^"]*)"`, 'i'));
	if (double) return double[1];
	const single = attributes.match(new RegExp(`${name}\\s*=\\s*'([^']*)'`, 'i'));
	return single ? single[1] : '';
}

/**
 * Builds the prompt for a question that quotes earlier answers. The block is
 * model-facing only: `parseAskPrompt` removes it again for the transcript, and
 * each excerpt carries its source so the model knows where it was taken from.
 */
export function composeAskPrompt(text: string, quotes: AskQuote[] = []): string {
	if (quotes.length === 0) return text;
	const blocks = quotes.map((quote, index) => {
		const from =
			quote.source === 'tool' ? `tool:${escapeAttribute(quote.label ?? '')}` : quote.source;
		return `<selection n="${index + 1}" from="${from}">\n${escapeClosingTags(quote.text)}\n</selection>`;
	});
	return [
		'<selections note="Excerpts the user selected from earlier messages. Treat them as quoted context for the request that follows.">',
		...blocks,
		'</selections>',
		'',
		text
	].join('\n');
}

/** Splits a stored user turn back into its visible question and its excerpts. */
export function parseAskPrompt(raw: string): { text: string; quotes: AskQuote[] } {
	const block = raw.match(SELECTIONS_BLOCK);
	if (!block) return { text: raw, quotes: [] };

	const quotes: AskQuote[] = [];
	for (const item of block[1].matchAll(SELECTION_ITEM)) {
		const text = item[2].trim();
		if (!text) continue;
		const from = readAttribute(item[1], 'from').trim();
		let source: AskQuoteSource = 'answer';
		let label: string | undefined;
		if (from === 'thinking') {
			source = 'thinking';
		} else if (from === 'tool' || from.startsWith('tool:')) {
			source = 'tool';
			label = from.slice('tool:'.length).trim() || undefined;
		}
		quotes.push({ id: `quote-${quotes.length + 1}`, text, source, label });
	}

	// Only an outer block that actually contains excerpts is context. Anything
	// else (a code sample, a stray tag) is ordinary user text and stays visible.
	if (quotes.length === 0) return { text: raw, quotes: [] };

	return { text: raw.slice(block[0].length).trim(), quotes };
}
