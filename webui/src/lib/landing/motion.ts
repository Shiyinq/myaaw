import { browser } from '$app/environment';
import { createSubscriber } from 'svelte/reactivity';
import type { Action } from 'svelte/action';
import { animate, hover, press, spring, type AnimationOptions } from 'motion';
import { preferences } from '$lib/stores/preferences.svelte';

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

/**
 * Shared spring transition presets powered by Motion (`motion@14`).
 */
export const MOTION_SPRING_SNAPPY: AnimationOptions = {
	type: spring,
	stiffness: 460,
	damping: 32,
	mass: 0.75
};

export const MOTION_SPRING_GENTLE: AnimationOptions = {
	type: spring,
	stiffness: 300,
	damping: 28,
	mass: 0.9
};

export const MOTION_SPRING_POP: AnimationOptions = {
	type: spring,
	stiffness: 420,
	damping: 24,
	mass: 0.7
};

/**
 * Checks whether reduced motion is active from either the in-app preference
 * (`preferences.value.reduceMotion`) or the OS media query.
 */
export function isReducedMotion(): boolean {
	if (!browser) return false;
	if (preferences.value.reduceMotion) return true;
	return window.matchMedia(REDUCED_MOTION_QUERY).matches;
}

export interface ReducedMotionState {
	/** Reactive getter: tracks both `preferences.value.reduceMotion` and OS media query changes. */
	readonly current: boolean;
	/** Imperative listener for OS media query changes (returns unsubscribe function). */
	subscribe(callback: (reduced: boolean) => void): () => void;
	/** Removes any imperative listeners registered on this helper. */
	destroy(): void;
}

/**
 * Reactive reduced-motion helper that responds to both OS media query changes
 * and `preferences.value.reduceMotion`. Safe to call during SSR.
 */
export function createReducedMotionState(): ReducedMotionState {
	const listeners = new Set<(reduced: boolean) => void>();
	let mediaQuery: MediaQueryList | null = null;
	let mediaHandler: (() => void) | null = null;

	const subscribeReactive = createSubscriber((update) => {
		if (!browser) return;
		const mq = window.matchMedia(REDUCED_MOTION_QUERY);
		const onChange = () => update();
		mq.addEventListener('change', onChange);
		return () => mq.removeEventListener('change', onChange);
	});

	function ensureImperativeMediaListener() {
		if (!browser || mediaQuery) return;
		mediaQuery = window.matchMedia(REDUCED_MOTION_QUERY);
		mediaHandler = () => {
			const reduced = isReducedMotion();
			for (const cb of listeners) {
				cb(reduced);
			}
		};
		mediaQuery.addEventListener('change', mediaHandler);
	}

	function destroy() {
		listeners.clear();
		if (mediaQuery && mediaHandler) {
			mediaQuery.removeEventListener('change', mediaHandler);
		}
		mediaQuery = null;
		mediaHandler = null;
	}

	return {
		get current() {
			subscribeReactive();
			return isReducedMotion();
		},
		subscribe(callback: (reduced: boolean) => void) {
			if (!browser) return () => {};
			ensureImperativeMediaListener();
			listeners.add(callback);
			return () => {
				listeners.delete(callback);
				if (listeners.size === 0) destroy();
			};
		},
		destroy
	};
}

export interface GsapBundle {
	gsap: typeof import('gsap').gsap;
	ScrollTrigger: typeof import('gsap/ScrollTrigger').ScrollTrigger;
}

let gsapPromise: Promise<GsapBundle | null> | null = null;
let gsapRegistered = false;

/**
 * Browser-only async loader for GSAP + ScrollTrigger. Registers ScrollTrigger
 * once and configures `ignoreMobileResize: true` for touch viewports.
 */
export async function loadGsap(): Promise<GsapBundle | null> {
	if (!browser) return null;
	if (!gsapPromise) {
		gsapPromise = Promise.all([import('gsap'), import('gsap/ScrollTrigger')])
			.then(([gsapMod, stMod]) => {
				const gsap = gsapMod.gsap ?? gsapMod.default;
				const ScrollTrigger = stMod.ScrollTrigger ?? stMod.default;
				if (!gsapRegistered) {
					gsap.registerPlugin(ScrollTrigger);
					ScrollTrigger.config({ ignoreMobileResize: true });
					gsapRegistered = true;
				}
				return { gsap, ScrollTrigger };
			})
			.catch(() => {
				gsapPromise = null;
				return null;
			});
	}
	return gsapPromise;
}

export interface MotionCardOptions {
	/** Vertical lift in pixels on pointer hover (default: 4). */
	lift?: number;
	/** Scale multiplier on pointer hover (default: 1.008). */
	scale?: number;
	/** Enable press settle feedback (default: true; set false for large scrollable panels). */
	pressable?: boolean;
}

/**
 * Svelte 5 action: tactile spring hover lift & press settle powered by Motion's
 * `hover()`, `press()`, `animate()`, and `spring`. Instant no-op under reduced motion.
 */
export const motionCard: Action<HTMLElement, MotionCardOptions | undefined> = (
	node,
	initialOptions = {}
) => {
	if (!browser) return {};

	let opts: MotionCardOptions = { ...initialOptions };
	let hovered = false;
	let pressed = false;
	let activeAnimation: ReturnType<typeof animate> | null = null;

	function restState() {
		if (isReducedMotion()) {
			activeAnimation?.stop();
			node.style.transform = '';
			return;
		}
		const lift = opts.lift ?? 4;
		const hoverScale = opts.scale ?? 1.008;

		if (pressed) {
			activeAnimation = animate(
				node,
				{ y: Math.max(-1, -lift * 0.25), scale: 0.992 },
				MOTION_SPRING_SNAPPY
			);
		} else if (hovered) {
			activeAnimation = animate(node, { y: -lift, scale: hoverScale }, MOTION_SPRING_GENTLE);
		} else {
			activeAnimation = animate(node, { y: 0, scale: 1 }, MOTION_SPRING_GENTLE);
		}
	}

	const stopHover = hover(node, () => {
		if (isReducedMotion()) return;
		hovered = true;
		restState();
		return () => {
			hovered = false;
			restState();
		};
	});

	let stopPress: VoidFunction | null = null;
	function bindPress() {
		stopPress?.();
		stopPress = null;
		if (opts.pressable === false) return;
		const hadTabIndex = node.hasAttribute('tabindex');
		stopPress = press(node, () => {
			if (isReducedMotion()) return;
			pressed = true;
			restState();
			return () => {
				pressed = false;
				restState();
			};
		});
		// Preserve non-interactive semantics if Motion's press gesture added tabindex.
		if (!hadTabIndex && node.getAttribute('tabindex') === '0') {
			node.removeAttribute('tabindex');
		}
	}

	bindPress();

	return {
		update(nextOptions: MotionCardOptions = {}) {
			const prevPressable = opts.pressable !== false;
			opts = { ...nextOptions };
			const nextPressable = opts.pressable !== false;
			if (prevPressable !== nextPressable) {
				pressed = false;
				bindPress();
			}
		},
		destroy() {
			stopHover();
			stopPress?.();
			activeAnimation?.stop();
		}
	};
};

export interface MotionButtonOptions {
	/** Scale multiplier while pressed (default: 0.965). */
	scale?: number;
	/** Scale multiplier while hovered (default: 1.02). */
	hoverScale?: number;
}

/**
 * Svelte 5 action: crisp spring hover & press feedback for interactive buttons
 * and pills using Motion's `hover()`, `press()`, `animate()`, and `spring`.
 */
export const motionButton: Action<HTMLElement, MotionButtonOptions | undefined> = (
	node,
	initialOptions = {}
) => {
	if (!browser) return {};

	let opts: MotionButtonOptions = { ...initialOptions };
	let hovered = false;
	let pressed = false;
	let activeAnimation: ReturnType<typeof animate> | null = null;

	function isDisabled(): boolean {
		return (
			('disabled' in node && Boolean((node as { disabled?: unknown }).disabled)) ||
			node.hasAttribute('disabled') ||
			node.getAttribute('aria-disabled') === 'true'
		);
	}

	function settle() {
		if (isReducedMotion() || isDisabled()) {
			activeAnimation?.stop();
			node.style.transform = '';
			return;
		}
		const pressScale = opts.scale ?? 0.965;
		const hoverScale = opts.hoverScale ?? 1.02;

		if (pressed) {
			activeAnimation = animate(node, { scale: pressScale }, MOTION_SPRING_SNAPPY);
		} else if (hovered) {
			activeAnimation = animate(node, { scale: hoverScale }, MOTION_SPRING_SNAPPY);
		} else {
			activeAnimation = animate(node, { scale: 1 }, MOTION_SPRING_SNAPPY);
		}
	}

	const stopHover = hover(node, () => {
		if (isReducedMotion() || isDisabled()) return;
		hovered = true;
		settle();
		return () => {
			hovered = false;
			settle();
		};
	});

	const stopPress = press(node, () => {
		if (isReducedMotion() || isDisabled()) return;
		pressed = true;
		settle();
		return () => {
			pressed = false;
			settle();
		};
	});

	return {
		update(nextOptions: MotionButtonOptions = {}) {
			opts = { ...nextOptions };
		},
		destroy() {
			stopHover();
			stopPress();
			activeAnimation?.stop();
		}
	};
};

export interface MotionPopOptions {
	/** Initial vertical offset in pixels (default: 10). */
	y?: number;
	/** Visual duration in seconds (default: 0.34). */
	duration?: number;
	/** Optional delay in seconds (default: 0). */
	delay?: number;
	/** Initial scale before settling to 1 (default: 0.985). */
	scale?: number;
}

/**
 * Svelte 5 action: plays a subtle spring entrance animation when mounted
 * (ideal for tab panel swaps, expanded details, and interactive cards).
 * No-op when reduced motion is active.
 */
export const motionPop: Action<HTMLElement, MotionPopOptions | undefined> = (
	node,
	options = {}
) => {
	if (!browser || isReducedMotion()) return {};

	const y = options.y ?? 10;
	const duration = options.duration ?? 0.34;
	const delay = options.delay ?? 0;
	const scale = options.scale ?? 0.985;

	const anim = animate(
		node,
		{ opacity: [0, 1], y: [y, 0], scale: [scale, 1] },
		{
			type: spring,
			visualDuration: duration,
			bounce: 0.18,
			delay
		}
	);

	return {
		destroy() {
			anim.stop();
		}
	};
};

/**
 * Triggers a one-shot spring scale pop on an element (e.g., copy confirmation,
 * theme swatch selection, or replay control click). No-op under reduced motion.
 */
export function motionPulse(
	node: HTMLElement | null | undefined,
	options?: { scale?: number; duration?: number }
): void {
	if (!browser || !node || isReducedMotion()) return;
	const peak = options?.scale ?? 1.08;
	const duration = options?.duration ?? 0.28;
	animate(
		node,
		{ scale: [1, peak, 1] },
		{
			duration,
			ease: [0.22, 1, 0.36, 1]
		}
	);
}
