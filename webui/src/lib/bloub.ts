/**
 * Bloub is the Myaaw mascot: a cat with one file per emotion (self-animating
 * SVG) plus a handful of looping cycles (GIF). Everything here keeps the asset
 * names in one place so call sites only deal with moods and cycles.
 *
 * The assets live in `static/myaaw-bloub/` and are served from the site root.
 */

export const BLOUB_MOODS = [
	'attentif',
	'blase',
	'colere',
	'confus',
	'curieux',
	'effraye',
	'excite',
	'fier',
	'heureux',
	'hilare',
	'mefiant',
	'neutre',
	'somnolent',
	'surpris',
	'timide',
	'triste'
] as const;

export type BloubMood = (typeof BLOUB_MOODS)[number];

export const BLOUB_CYCLES = ['idle', 'idle2', 'loading', 'notification', 'sleep', 'wink'] as const;

export type BloubCycle = (typeof BLOUB_CYCLES)[number];

/**
 * The palette the artwork ships in, in picker order. These ids double as the
 * keys stored in the agent registry, so they must keep matching the file names
 * in `static/myaaw-bloub/` on both sides.
 */
export const BLOUB_COLORS = [
	'encre',
	'brun',
	'rouge',
	'orange',
	'ambre',
	'vert',
	'turquoise',
	'bleu',
	'violet',
	'rose',
	'gris',
	'creme'
] as const;

export type BloubColor = (typeof BLOUB_COLORS)[number];

/** Ink. Agents registered before colours existed resolve to this. */
export const BLOUB_DEFAULT_COLOR: BloubColor = 'encre';

/** Folds empty and unknown keys onto the default; use it on values read back from the API. */
export function colorOf(value: string | null | undefined): BloubColor {
	return BLOUB_COLORS.includes(value as BloubColor) ? (value as BloubColor) : BLOUB_DEFAULT_COLOR;
}

/** Static emotion used when a cycle has to freeze (reduced motion). */
export const CYCLE_FALLBACK: Record<BloubCycle, BloubMood> = {
	idle: 'neutre',
	idle2: 'neutre',
	loading: 'attentif',
	notification: 'surpris',
	sleep: 'somnolent',
	wink: 'heureux'
};

export function moodSrc(mood: BloubMood, color: BloubColor = BLOUB_DEFAULT_COLOR): string {
	return `/myaaw-bloub/bloub-cat-${mood}-${color}-anime.svg`;
}

export function cycleSrc(cycle: BloubCycle, color: BloubColor = BLOUB_DEFAULT_COLOR): string {
	return `/myaaw-bloub/bloub-cat-${cycle}-${color}-cycle.gif`;
}
