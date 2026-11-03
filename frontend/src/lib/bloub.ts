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

/** Static emotion used when a cycle has to freeze (reduced motion). */
export const CYCLE_FALLBACK: Record<BloubCycle, BloubMood> = {
	idle: 'neutre',
	idle2: 'neutre',
	loading: 'attentif',
	notification: 'surpris',
	sleep: 'somnolent',
	wink: 'heureux'
};

export function moodSrc(mood: BloubMood): string {
	return `/myaaw-bloub/bloub-cat-${mood}-encre-anime.svg`;
}

export function cycleSrc(cycle: BloubCycle): string {
	return `/myaaw-bloub/bloub-cat-${cycle}-encre-cycle.gif`;
}
