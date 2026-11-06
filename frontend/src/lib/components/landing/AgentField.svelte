<script lang="ts">
	import { onMount } from 'svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { cn } from '$lib/utils';

	/**
	 * The hero's constellation: slow-drifting nodes that draw a line whenever two
	 * of them come close, plus a softer web that reaches toward the pointer. It is
	 * the visual shorthand for agents talking to tools and to each other.
	 *
	 * Colour comes from the theme tokens at draw time, so the field re-inks itself
	 * when the dashboard theme changes. It pauses off-screen and on a hidden tab,
	 * and reduced motion gets one still frame instead of an animation.
	 */
	let { class: className = '' }: { class?: string } = $props();

	let host: HTMLDivElement | null = $state(null);
	let canvas: HTMLCanvasElement | null = $state(null);
	let syncMotion: ((_reduce: boolean) => void) | null = null;

	$effect(() => {
		// Reading the preference keeps the effect subscribed; flipping it in
		// Settings while the page is open restarts or freezes the field.
		syncMotion?.(preferences.value.reduceMotion);
	});

	onMount(() => {
		if (!host || !canvas) return;

		// Explicit non-null aliases: the helpers below are function declarations,
		// so TypeScript does not carry the narrowing into their bodies.
		const surface: HTMLDivElement = host;
		const element: HTMLCanvasElement = canvas;
		const found = element.getContext('2d');
		if (!found) return;
		const context: CanvasRenderingContext2D = found;

		interface Dot {
			x: number;
			y: number;
			vx: number;
			vy: number;
			radius: number;
			phase: number;
			pulse: boolean;
		}

		const LINK = 144;
		const POINTER_LINK = 200;
		const POINTER_PULL = 210;

		let dots: Dot[] = [];
		let width = 0;
		let height = 0;
		let frame = 0;
		let running = false;
		let visible = true;
		let ink = 'currentColor';
		let soft = 'currentColor';
		let pointer: { x: number; y: number } | null = null;

		const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		const reduced = () => preferences.value.reduceMotion || motionQuery.matches;

		function readInk() {
			const styles = getComputedStyle(document.documentElement);
			ink = styles.getPropertyValue('--foreground').trim() || 'currentColor';
			soft = styles.getPropertyValue('--muted-foreground').trim() || ink;
		}

		function seed() {
			const count = Math.max(24, Math.min(64, Math.round((width * height) / 24000)));
			dots = Array.from({ length: count }, () => ({
				x: Math.random() * width,
				y: Math.random() * height,
				vx: (Math.random() - 0.5) * 0.24,
				vy: (Math.random() - 0.5) * 0.24,
				radius: 0.8 + Math.random() * 1.5,
				phase: Math.random() * Math.PI * 2,
				pulse: Math.random() < 0.16
			}));
		}

		function paint(time: number) {
			context.clearRect(0, 0, width, height);
			if (dots.length === 0) return;

			// Neighbour links, batched into one path: one stroke for the whole web.
			context.strokeStyle = soft;
			context.lineWidth = 1;
			context.globalAlpha = 0.2;
			context.beginPath();
			for (let i = 0; i < dots.length; i++) {
				const a = dots[i];
				for (let j = i + 1; j < dots.length; j++) {
					const b = dots[j];
					const dx = a.x - b.x;
					const dy = a.y - b.y;
					const distanceSquared = dx * dx + dy * dy;
					if (distanceSquared > LINK * LINK) continue;
					if (Math.sqrt(distanceSquared) > LINK * 0.8) continue;
					context.moveTo(a.x, a.y);
					context.lineTo(b.x, b.y);
				}
			}
			context.stroke();

			// The pointer is just another node the field is aware of.
			if (pointer) {
				context.strokeStyle = ink;
				context.globalAlpha = 0.26;
				context.beginPath();
				for (const dot of dots) {
					const dx = pointer.x - dot.x;
					const dy = pointer.y - dot.y;
					if (dx * dx + dy * dy > POINTER_LINK * POINTER_LINK) continue;
					context.moveTo(dot.x, dot.y);
					context.lineTo(pointer.x, pointer.y);
				}
				context.stroke();
			}

			context.fillStyle = ink;
			for (const dot of dots) {
				const scale = dot.pulse ? 1 + Math.sin(time / 760 + dot.phase) * 0.85 : 1;
				context.globalAlpha = dot.pulse ? 0.8 : 0.48;
				context.beginPath();
				context.arc(dot.x, dot.y, dot.radius * scale, 0, Math.PI * 2);
				context.fill();
			}

			context.globalAlpha = 1;
		}

		function advance() {
			for (const dot of dots) {
				dot.x += dot.vx;
				dot.y += dot.vy;

				if (dot.x < -24) dot.x = width + 24;
				else if (dot.x > width + 24) dot.x = -24;
				if (dot.y < -24) dot.y = height + 24;
				else if (dot.y > height + 24) dot.y = -24;

				if (!pointer) continue;
				const dx = pointer.x - dot.x;
				const dy = pointer.y - dot.y;
				const distanceSquared = dx * dx + dy * dy;
				if (distanceSquared > POINTER_PULL * POINTER_PULL || distanceSquared < 1) continue;
				const distance = Math.sqrt(distanceSquared);
				dot.x -= (dx / distance) * 0.22;
				dot.y -= (dy / distance) * 0.22;
			}
		}

		function loop(time: number) {
			if (!running) return;
			advance();
			paint(time);
			frame = requestAnimationFrame(loop);
		}

		function stop() {
			running = false;
			cancelAnimationFrame(frame);
			frame = 0;
		}

		function start() {
			if (running || !visible || reduced() || width === 0) return;
			running = true;
			frame = requestAnimationFrame(loop);
		}

		function sync() {
			stop();
			if (reduced()) paint(0);
			else if (visible) start();
		}

		function resize() {
			const rect = surface.getBoundingClientRect();
			const nextWidth = Math.max(1, Math.round(rect.width));
			const nextHeight = Math.max(1, Math.round(rect.height));
			if (nextWidth === width && nextHeight === height) return;

			const ratio = Math.min(window.devicePixelRatio || 1, 1.6);
			width = nextWidth;
			height = nextHeight;
			element.width = Math.round(width * ratio);
			element.height = Math.round(height * ratio);
			context.setTransform(ratio, 0, 0, ratio, 0, 0);

			if (dots.length === 0) {
				seed();
			} else {
				for (const dot of dots) {
					dot.x = Math.min(width, Math.max(0, dot.x));
					dot.y = Math.min(height, Math.max(0, dot.y));
				}
			}

			paint(performance.now());
		}

		function onPointerMove(event: PointerEvent) {
			const rect = surface.getBoundingClientRect();
			const x = event.clientX - rect.left;
			const y = event.clientY - rect.top;
			pointer = x < 0 || y < 0 || x > rect.width || y > rect.height ? null : { x, y };
		}

		function clearPointer() {
			pointer = null;
		}

		function onVisibility() {
			if (document.hidden) stop();
			else if (visible) start();
		}

		const resizeObserver = new ResizeObserver(resize);
		resizeObserver.observe(surface);

		const themeObserver = new MutationObserver(() => {
			readInk();
			if (!running) paint(performance.now());
		});
		themeObserver.observe(document.documentElement, {
			attributes: true,
			attributeFilter: ['class', 'data-theme', 'style']
		});

		const visibilityObserver = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					visible = entry.isIntersecting;
					if (visible) {
						if (reduced()) paint(performance.now());
						else start();
					} else {
						stop();
					}
				}
			},
			{ threshold: 0.01 }
		);
		visibilityObserver.observe(surface);

		window.addEventListener('pointermove', onPointerMove, { passive: true });
		window.addEventListener('blur', clearPointer);
		document.addEventListener('visibilitychange', onVisibility);
		motionQuery.addEventListener('change', sync);
		syncMotion = () => sync();

		readInk();
		resize();
		sync();

		return () => {
			syncMotion = null;
			stop();
			resizeObserver.disconnect();
			themeObserver.disconnect();
			visibilityObserver.disconnect();
			window.removeEventListener('pointermove', onPointerMove);
			window.removeEventListener('blur', clearPointer);
			document.removeEventListener('visibilitychange', onVisibility);
			motionQuery.removeEventListener('change', sync);
		};
	});
</script>

<div
	bind:this={host}
	class={cn('pointer-events-none absolute inset-0 overflow-hidden', className)}
	aria-hidden="true"
>
	<canvas bind:this={canvas} class="h-full w-full"></canvas>
</div>
