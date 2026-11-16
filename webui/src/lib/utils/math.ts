import katex from 'katex';
import type { TokenizerAndRendererExtension, Tokens } from 'marked';

/*
 * Math is rendered while the Markdown is parsed, not after mount: KaTeX turns
 * LaTeX into plain HTML with no DOM access, so the result survives SSR and the
 * sanitizer, and a chat answer never flashes raw `$x^2$` before settling.
 *
 * The extension is deliberately hand-rolled instead of using
 * `marked-katex-extension`, whose peer range does not cover marked 17, and
 * because the inline rule below has to reject prices such as `$5 and $10`.
 */

const KATEX_OPTIONS = {
	throwOnError: false,
	// Models emit macros and non-strict syntax all the time; a warning per
	// occurrence would flood the console.
	strict: false,
	trust: false,
	output: 'htmlAndMathml',
	errorColor: 'var(--destructive)'
} as const;

interface MathToken extends Tokens.Generic {
	type: 'inlineMath' | 'blockMath';
	text: string;
}

function render(tex: string, displayMode: boolean): string {
	return katex.renderToString(tex, { ...KATEX_OPTIONS, displayMode });
}

/*
 * `$...$` or `\(...\)`, on one line. The content may not start or end with
 * whitespace and the closing delimiter may not be followed by a digit, which is
 * what keeps `$5 and $10` (and `$5`) literal text.
 */
const inlineMath: TokenizerAndRendererExtension = {
	name: 'inlineMath',
	level: 'inline',
	start(src) {
		const dollar = src.indexOf('$');
		const paren = src.indexOf('\\(');
		if (dollar < 0) return paren;
		if (paren < 0) return dollar;
		return Math.min(dollar, paren);
	},
	tokenizer(src) {
		const dollar = /^\$(?!\$)(\S(?:[^$\n]*[^\s$])?)\$(?!\d)/.exec(src);
		if (dollar) {
			return { type: 'inlineMath', raw: dollar[0], text: dollar[1] };
		}
		const paren = /^\\\(([^\n]+?)\\\)/.exec(src);
		if (paren) {
			return { type: 'inlineMath', raw: paren[0], text: paren[1] };
		}
		return undefined;
	},
	renderer(token) {
		return `<span class="md-math">${render((token as MathToken).text, false)}</span>`;
	}
};

/*
 * `$$...$$` and `\[...\]` become their own block. No `start` hook means the
 * fence is only recognised where a block begins, so a `$$` in the middle of a
 * sentence never splits the paragraph.
 */
const BLOCK_MATH = [
	/^\$\$[ \t]*\n?([\s\S]*?)\n?[ \t]*\$\$[ \t]*(?:\n+|$)/,
	/^\\\[[ \t]*\n?([\s\S]*?)\n?[ \t]*\\\][ \t]*(?:\n+|$)/
];

const blockMath: TokenizerAndRendererExtension = {
	name: 'blockMath',
	level: 'block',
	tokenizer(src) {
		for (const pattern of BLOCK_MATH) {
			const match = pattern.exec(src);
			if (match) return { type: 'blockMath', raw: match[0], text: match[1].trim() };
		}
		return undefined;
	},
	renderer(token) {
		return `<div class="md-math">${render((token as MathToken).text, true)}</div>`;
	}
};

/** Marked extensions that turn `$…$` and `$$…$$` into KaTeX markup. */
export const markdownMath: TokenizerAndRendererExtension[] = [inlineMath, blockMath];
