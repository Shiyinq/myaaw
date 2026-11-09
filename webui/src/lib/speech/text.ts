/**
 * Text preparation for read-aloud. Answers are Markdown, and a synthesiser
 * happily reads back backticks, table pipes and URLs, so the transcript is
 * flattened to prose first and then cut into chunks the browser can speak.
 *
 * Nothing in this file touches the DOM: the pure functions are easy to reason
 * about (and to test) on their own.
 */

const HTML_ENTITIES: Record<string, string> = {
	'&amp;': '&',
	'&lt;': '<',
	'&gt;': '>',
	'&quot;': '"',
	'&#39;': "'",
	'&nbsp;': ' '
};

/** Turns Markdown from the transcript into plain text that reads well aloud. */
export function speakableText(markdown: string): string {
	let text = markdown.replace(/\r\n?/g, '\n');

	// Fenced code is not prose; drop it whole instead of reading punctuation.
	text = text.replace(/```[\s\S]*?(?:```|$)/g, '\n');
	// Images and links keep their label; a bare URL is noise when spoken.
	text = text.replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1');
	text = text.replace(/\[([^\]]+)\]\([^)]*\)/g, '$1');
	text = text.replace(/<(?:https?|mailto):[^>\s]+>/gi, '');
	text = text.replace(/\b(?:https?:\/\/|www\.)\S+/gi, '');
	text = text.replace(/<[^>]+>/g, ' ');

	// Table separators and pipes read as pauses at best, gibberish at worst.
	text = text.replace(/^\s*\|?[\s:|-]{3,}\|?\s*$/gm, '');
	text = text.replace(/\|/g, ' ');

	// Block markers: headings, quotes, bullets, horizontal rules.
	text = text.replace(/^\s{0,3}#{1,6}\s+/gm, '');
	text = text.replace(/^\s{0,3}>\s?/gm, '');
	text = text.replace(/^\s{0,3}(?:[-*+]|\d+[.)])\s+/gm, '');
	text = text.replace(/^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/gm, '');

	// Inline code keeps its content, emphasis keeps its words.
	text = text.replace(/`+([^`]*)`+/g, '$1');
	text = text.replace(/(\*{1,3})(?=\S)([\s\S]+?)(?<=\S)\1/g, '$2');
	text = text.replace(/~~(?=\S)([\s\S]+?)(?<=\S)~~/g, '$1');
	text = text.replace(/\\([\\`*_{}[\]()#+\-.!>|])/g, '$1');

	text = text.replace(/&(?:amp|lt|gt|quot|#39|nbsp);/g, (entity) => HTML_ENTITIES[entity] ?? ' ');

	// Collapse layout whitespace but keep paragraph breaks as a spoken pause.
	text = text.replace(/[ \t]+/g, ' ');
	text = text.replace(/ ?\n ?/g, '\n');
	text = text.replace(/\n{3,}/g, '\n\n');

	return text.trim();
}

const SENTENCE_BREAK = /(?<=[.!?…])["'”’)\]]*\s+/;

/**
 * Chrome cuts an utterance after roughly fifteen seconds, so long answers are
 * queued in sentence-sized pieces. Boundaries stay on punctuation, which keeps
 * the pauses natural and makes stop/pause react quickly. A period inside a word
 * (`MEMORY.md`) is not a boundary: the split needs whitespace after it.
 */
export function chunkForSpeech(text: string, target = 220, max = 300): string[] {
	const trimmed = text.trim();
	if (!trimmed) return [];

	const sentences: string[] = [];
	for (const paragraph of trimmed.split(/\n+/)) {
		for (const sentence of paragraph.split(SENTENCE_BREAK)) {
			const value = sentence.trim();
			if (value) sentences.push(value);
		}
	}

	const chunks: string[] = [];
	let current = '';

	for (const sentence of sentences) {
		if (sentence.length > max) {
			if (current) {
				chunks.push(current);
				current = '';
			}
			chunks.push(...splitLong(sentence, target));
			continue;
		}

		if (current && current.length + sentence.length + 1 > max) {
			chunks.push(current);
			current = sentence;
		} else {
			current = current ? `${current} ${sentence}` : sentence;
		}
	}

	if (current) chunks.push(current);
	return chunks;
}

/** Breaks a sentence that is longer than one utterance at the nearest pause. */
function splitLong(sentence: string, target: number): string[] {
	const pieces: string[] = [];
	let rest = sentence;

	while (rest.length > target) {
		let cut = 0;
		for (const mark of [',', ';', ':', ' ']) {
			const at = rest.lastIndexOf(mark, target);
			if (at > 20) {
				cut = mark === ' ' ? at : at + 1;
				break;
			}
		}
		if (cut === 0) cut = target;
		const piece = rest.slice(0, cut).trim();
		if (piece) pieces.push(piece);
		rest = rest.slice(cut).trim();
	}

	if (rest) pieces.push(rest);
	return pieces;
}

const INDONESIAN_WORDS = new Set([
	'ada',
	'adalah',
	'akan',
	'anda',
	'bisa',
	'belum',
	'dalam',
	'dan',
	'dari',
	'dengan',
	'di',
	'ini',
	'itu',
	'jadi',
	'juga',
	'kalau',
	'kamu',
	'karena',
	'ke',
	'kita',
	'maka',
	'mereka',
	'saja',
	'saya',
	'sudah',
	'tidak',
	'tapi',
	'untuk',
	'yang'
]);

const ENGLISH_WORDS = new Set([
	'and',
	'are',
	'but',
	'can',
	'for',
	'from',
	'have',
	'in',
	'is',
	'not',
	'of',
	'on',
	'or',
	'should',
	'that',
	'the',
	'these',
	'this',
	'to',
	'was',
	'were',
	'will',
	'with',
	'you'
]);

/**
 * Guesses the language of an answer so the voice matches. It is a heuristic:
 * the browser exposes no automatic language detection, and short answers often
 * carry no signal at all, in which case `null` lets the caller fall back.
 */
export function detectSpeechLang(text: string): 'id' | 'en' | null {
	const words = text.toLowerCase().match(/[a-zà-ÿ]+/g) ?? [];
	let indonesian = 0;
	let english = 0;

	for (const word of words) {
		if (INDONESIAN_WORDS.has(word)) indonesian += 1;
		if (ENGLISH_WORDS.has(word)) english += 1;
	}

	if (indonesian === english) return null;
	return indonesian > english ? 'id' : 'en';
}
