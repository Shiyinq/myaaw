/**
 * Brings a detail pane into view after the reader picks something from a list.
 *
 * Below `lg` the list and the pane share one scrolling page, so the pane can sit
 * under the fold; from `lg` up both panes are on screen and scrolling would only
 * move the page for no reason.
 */
export function revealPane(node: HTMLElement | null | undefined, breakpoint = 1023) {
	if (!node || typeof window === 'undefined') return;
	if (!window.matchMedia(`(max-width: ${breakpoint}px)`).matches) return;
	node.scrollIntoView({
		block: 'start',
		behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
	});
}
