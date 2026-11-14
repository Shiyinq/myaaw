import { browser } from '$app/environment';
import type { Mermaid, MermaidConfig } from 'mermaid';

/*
 * Mermaid needs a real DOM and roughly half a megabyte of JavaScript, so it is
 * loaded on demand and only in the browser. The renderer in Markdown.svelte
 * leaves a container behind; this module fills it.
 */

export type MermaidMode = 'light' | 'dark';

let loading: Promise<Mermaid> | null = null;
let configured: MermaidMode | null = null;

/*
 * Re-rendering is expensive and the Markdown HTML is rebuilt on every streamed
 * token, so finished SVGs are cached per mode. Switching themes misses the
 * cache on purpose: mermaid bakes the palette into the SVG.
 */
const cache = new Map<string, string>();
const CACHE_LIMIT = 80;
let sequence = 0;

function config(mode: MermaidMode): MermaidConfig {
	return {
		startOnLoad: false,
		// Models can emit anything; strict keeps mermaid's own DOMPurify pass and
		// disables click handlers and `javascript:` links.
		securityLevel: 'strict',
		// A parse failure must not paint an error diagram into the page.
		suppressErrorRendering: true,
		theme: mode === 'dark' ? 'dark' : 'default',
		fontFamily: 'inherit'
	};
}

async function load(mode: MermaidMode): Promise<Mermaid> {
	loading ??= import('mermaid').then((module) => module.default);
	const mermaid = await loading;
	if (configured !== mode) {
		mermaid.initialize(config(mode));
		configured = mode;
	}
	return mermaid;
}

/*
 * Mermaid fits every diagram to its container, which turns a wide flowchart into
 * an unreadable strip on a phone. Rendering at the diagram's own size and letting
 * the container scroll keeps the labels legible.
 */
function atNaturalSize(svg: string): string {
	const viewBox = /viewBox="([^"]+)"/.exec(svg);
	if (!viewBox) return svg;
	const width = Number(viewBox[1].trim().split(/\s+/)[2]);
	if (!Number.isFinite(width) || width <= 0) return svg;
	return svg
		.replace(/\swidth="100%"/, ` width="${Math.ceil(width)}"`)
		.replace(/\sstyle="max-width:\s*[\d.]+px;?"/, '');
}

/**
 * Renders one diagram to SVG markup. Throws when the source is not valid
 * mermaid, which the caller turns into the source view plus an error note.
 */
export async function renderMermaid(source: string, mode: MermaidMode): Promise<string> {
	if (!browser) throw new Error('mermaid is browser-only');

	const key = `${mode}\u0000${source}`;
	const cached = cache.get(key);
	if (cached) return cached;

	const mermaid = await load(mode);
	const id = `myaaw-mermaid-${(sequence += 1)}`;
	try {
		const { svg } = await mermaid.render(id, source);
		const markup = atNaturalSize(svg);
		if (cache.size >= CACHE_LIMIT) cache.clear();
		cache.set(key, markup);
		return markup;
	} finally {
		// A failed parse leaves the temporary measuring node behind.
		document.getElementById(id)?.remove();
		document.getElementById(`d${id}`)?.remove();
	}
}
