import type { Action } from 'svelte/action';

export interface RevealOptions {
	/** Milliseconds to wait after the element enters the viewport. */
	delay?: number;
	/** How far the element travels up, in pixels. */
	distance?: number;
	/** IntersectionObserver root margin; the default leaves 10% at the bottom. */
	margin?: string;
}

/**
 * Fades an element up the first time it scrolls into view. The classes live in
 * the page that uses the action, because different pages may want a different
 * distance and easing. Motion is a presentation detail: reduced-motion users
 * still get the element, just without the travel.
 */
export const reveal: Action<HTMLElement, RevealOptions | undefined> = (node, options = {}) => {
	function apply(next: RevealOptions = {}) {
		node.classList.add('reveal');
		node.style.setProperty('--reveal-delay', `${next.delay ?? 0}ms`);
		if (next.distance !== undefined) {
			node.style.setProperty('--reveal-distance', `${next.distance}px`);
		}
	}

	apply(options);

	if (typeof IntersectionObserver === 'undefined') {
		node.classList.add('is-revealed');
		return { update: apply };
	}

	const observer = new IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				if (!entry.isIntersecting) continue;
				node.classList.add('is-revealed');
				observer.unobserve(node);
			}
		},
		{ rootMargin: options.margin ?? '0px 0px -10% 0px', threshold: 0.12 }
	);
	observer.observe(node);

	return {
		update: apply,
		destroy: () => observer.disconnect()
	};
};
