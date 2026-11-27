import { browser } from '$app/environment';
import Lenis from 'lenis';
import { preferences } from '$lib/stores/preferences.svelte';
import { isReducedMotion, loadGsap, type GsapBundle } from './motion';

export interface LandingScrollToOptions {
	/** Pixel offset applied to the destination (e.g. `-72` for a fixed header). */
	offset?: number;
	/** Jump immediately without smooth interpolation. */
	immediate?: boolean;
	/** Optional smooth scroll duration in seconds (Lenis only). */
	duration?: number;
}

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

let progress = $state(0);
let velocity = $state(0);
let scrollY = $state(0);
let activeSection = $state('');
let direction = $state<'down' | 'up'>('down');
let reduced = $state(false);
let mounted = $state(false);

let activeLenis: Lenis | null = null;
let activeGsap: GsapBundle | null = null;
let sectionIds: string[] = [];
let sectionOffsets: { id: string; top: number }[] = [];
let teardownCurrent: (() => void) | null = null;

function clamp01(value: number): number {
	return Math.min(1, Math.max(0, value));
}

function readScrollableHeight(): number {
	if (!browser) return 1;
	return Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
}

function measureSections() {
	if (!browser) return;
	sectionOffsets = sectionIds
		.map((id) => {
			const el = document.getElementById(id);
			if (!el) return null;
			return { id, top: el.getBoundingClientRect().top + window.scrollY };
		})
		.filter((entry): entry is { id: string; top: number } => entry !== null);
	syncActiveSection(scrollY);
}

function syncActiveSection(currentY: number) {
	if (!browser) return;
	const line = currentY + window.innerHeight * 0.35;
	let current = '';
	for (const entry of sectionOffsets) {
		if (line >= entry.top) current = entry.id;
	}
	if (current !== activeSection) {
		activeSection = current;
	}
}

function updateFromScroll(nextY: number, nextProgress?: number, nextVelocity?: number) {
	if (nextY > scrollY + 0.5) direction = 'down';
	else if (nextY < scrollY - 0.5) direction = 'up';

	scrollY = nextY;
	progress = clamp01(nextProgress ?? nextY / readScrollableHeight());

	if (nextVelocity !== undefined) {
		// Smooth signed velocity to keep marquee / trail modulation fluid.
		velocity = Math.abs(nextVelocity) < 0.01 ? 0 : velocity * 0.65 + nextVelocity * 0.35;
	} else {
		velocity = 0;
	}

	syncActiveSection(nextY);
}

function stripLenisClasses() {
	if (!browser) return;
	const root = document.documentElement;
	for (const token of Array.from(root.classList)) {
		if (token === 'lenis' || token.startsWith('lenis-')) {
			root.classList.remove(token);
		}
	}
}

function resolveElement(target: string | HTMLElement): HTMLElement | null {
	if (typeof target !== 'string') return target;
	if (!browser) return null;
	if (target.startsWith('#')) {
		return document.getElementById(target.slice(1));
	}
	return document.getElementById(target) ?? document.querySelector<HTMLElement>(target);
}

/**
 * Shared Lenis + GSAP ScrollTrigger smooth-scroll controller for the landing
 * page (`/`). Works identically under reduced motion by falling back to native
 * window scrolling while keeping `progress`, `scrollY`, and `activeSection`
 * live for `ScrollTrail`, header indicators, and section choreography.
 */
export const landingScroll = {
	get progress() {
		return progress;
	},
	get velocity() {
		return velocity;
	},
	get scrollY() {
		return scrollY;
	},
	get activeSection() {
		return activeSection;
	},
	get direction() {
		return direction;
	},
	get reduced() {
		return reduced;
	},
	get mounted() {
		return mounted;
	},

	/**
	 * Activates landing-page scroll tracking and (unless reduced motion is on)
	 * Lenis smooth wheel scrolling synchronized to `gsap.ticker` and
	 * `ScrollTrigger.update()`. Returns a deterministic teardown callback.
	 */
	mount(ids: string[]): () => void {
		if (!browser) return () => {};

		// If remounted before prior teardown ran, clean up deterministically first.
		teardownCurrent?.();

		sectionIds = [...ids];
		let disposed = false;
		let tickerFn: ((time: number) => void) | null = null;
		let fallbackRaf = 0;
		let lenisScrollUnsub: (() => void) | null = null;
		let nativeVelocityTimer: number | undefined;
		let lastNativeY = window.scrollY;
		let lastNativeTime = performance.now();

		document.documentElement.dataset.landing = 'true';
		mounted = true;

		function destroyLenis() {
			if (fallbackRaf) {
				cancelAnimationFrame(fallbackRaf);
				fallbackRaf = 0;
			}
			if (tickerFn && activeGsap) {
				activeGsap.gsap.ticker.remove(tickerFn);
				tickerFn = null;
			}
			lenisScrollUnsub?.();
			lenisScrollUnsub = null;
			if (activeLenis) {
				activeLenis.destroy();
				activeLenis = null;
			}
			stripLenisClasses();
			velocity = 0;
		}

		function startLenis() {
			if (disposed || activeLenis || reduced) return;

			const instance = new Lenis({
				smoothWheel: true,
				syncTouch: false,
				lerp: 0.1,
				wheelMultiplier: 0.95,
				autoRaf: false,
				respectReducedMotion: true
			});
			activeLenis = instance;

			lenisScrollUnsub = instance.on('scroll', (lenis) => {
				if (disposed) return;
				updateFromScroll(lenis.scroll, lenis.progress, lenis.velocity);
				activeGsap?.ScrollTrigger.update();
			});

			const bindToGsap = (bundle: GsapBundle) => {
				if (disposed || activeLenis !== instance) return;
				if (fallbackRaf) {
					cancelAnimationFrame(fallbackRaf);
					fallbackRaf = 0;
				}
				activeGsap = bundle;
				tickerFn = (time: number) => {
					instance.raf(time * 1000);
					// Settle residual velocity cleanly to 0 when Lenis stops scrolling.
					if (!instance.isScrolling && velocity !== 0) {
						velocity = Math.abs(velocity) < 0.02 ? 0 : velocity * 0.7;
					}
				};
				bundle.gsap.ticker.add(tickerFn);
				bundle.gsap.ticker.lagSmoothing(0);
				bundle.ScrollTrigger.refresh();
			};

			if (activeGsap) {
				bindToGsap(activeGsap);
			} else {
				// Drive Lenis via rAF until the async GSAP bundle resolves, then hand off.
				const pump = (now: number) => {
					if (disposed || activeLenis !== instance || tickerFn) {
						fallbackRaf = 0;
						return;
					}
					instance.raf(now);
					fallbackRaf = requestAnimationFrame(pump);
				};
				fallbackRaf = requestAnimationFrame(pump);
				void loadGsap().then((bundle) => {
					if (bundle) bindToGsap(bundle);
				});
			}
		}

		function onNativeScroll() {
			if (disposed || activeLenis) return;
			const now = performance.now();
			const y = window.scrollY;
			const dt = Math.max(16, now - lastNativeTime);
			const rawVelocity = ((y - lastNativeY) / dt) * 16;
			lastNativeY = y;
			lastNativeTime = now;

			updateFromScroll(y, undefined, reduced ? 0 : rawVelocity);

			window.clearTimeout(nativeVelocityTimer);
			nativeVelocityTimer = window.setTimeout(() => {
				if (!disposed && !activeLenis) velocity = 0;
			}, 120);
		}

		function onResize() {
			if (disposed) return;
			measureSections();
			activeLenis?.resize();
			activeGsap?.ScrollTrigger.refresh();
		}

		const mediaQuery = window.matchMedia(REDUCED_MOTION_QUERY);
		let osReduced = $state(mediaQuery.matches);

		const onMediaChange = () => {
			osReduced = mediaQuery.matches;
		};
		mediaQuery.addEventListener('change', onMediaChange);

		const stopEffects = $effect.root(() => {
			$effect(() => {
				const nextReduced = preferences.value.reduceMotion || osReduced;
				reduced = nextReduced;
				if (nextReduced) {
					destroyLenis();
					updateFromScroll(window.scrollY, undefined, 0);
				} else {
					startLenis();
				}
			});
		});

		const resizeObserver = new ResizeObserver(() => {
			if (disposed) return;
			measureSections();
			activeLenis?.resize();
		});
		resizeObserver.observe(document.body);

		window.addEventListener('scroll', onNativeScroll, { passive: true });
		window.addEventListener('resize', onResize, { passive: true });

		// Initial sync on mount.
		reduced = isReducedMotion();
		measureSections();
		updateFromScroll(window.scrollY, undefined, 0);

		const teardown = () => {
			if (disposed) return;
			disposed = true;
			teardownCurrent = null;
			stopEffects();
			destroyLenis();
			window.clearTimeout(nativeVelocityTimer);
			mediaQuery.removeEventListener('change', onMediaChange);
			window.removeEventListener('scroll', onNativeScroll);
			window.removeEventListener('resize', onResize);
			resizeObserver.disconnect();
			delete document.documentElement.dataset.landing;
			stripLenisClasses();
			sectionIds = [];
			sectionOffsets = [];
			velocity = 0;
			mounted = false;
		};

		teardownCurrent = teardown;
		return teardown;
	},

	/**
	 * Scrolls to a numeric Y offset, section ID/selector, or HTMLElement.
	 * Uses Lenis when active and falls back to native `window.scrollTo`.
	 */
	scrollTo(target: string | number | HTMLElement, opts: LandingScrollToOptions = {}) {
		if (!browser) return;
		const offset = opts.offset ?? 0;
		const immediate = opts.immediate ?? reduced;

		if (activeLenis && !reduced) {
			if (typeof target === 'number') {
				activeLenis.scrollTo(Math.max(0, target + offset), {
					immediate,
					duration: opts.duration
				});
				return;
			}
			const element = resolveElement(target);
			if (!element) return;
			activeLenis.scrollTo(element, {
				offset,
				immediate,
				duration: opts.duration
			});
			return;
		}

		// Native fallback (reduced motion or Lenis inactive).
		let destinationY: number;
		if (typeof target === 'number') {
			destinationY = target + offset;
		} else {
			const element = resolveElement(target);
			if (!element) return;
			destinationY = element.getBoundingClientRect().top + window.scrollY + offset;
		}
		window.scrollTo({
			top: Math.max(0, destinationY),
			behavior: immediate ? 'auto' : 'smooth'
		});
	},

	/**
	 * Recalculates section positions, Lenis dimensions, and GSAP ScrollTriggers.
	 */
	refresh() {
		if (!browser) return;
		measureSections();
		activeLenis?.resize();
		activeGsap?.ScrollTrigger.refresh();
	}
};
