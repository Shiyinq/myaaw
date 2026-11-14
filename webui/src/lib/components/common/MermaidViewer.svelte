<script lang="ts">
	import { X, ZoomIn, ZoomOut } from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { renderMermaid, type MermaidMode } from '$lib/utils/mermaid';

	/*
	 * Fullscreen reader for one mermaid fence. Markdown.svelte opens it from the
	 * diagram toolbar; the SVG is rendered here from the same source, so the
	 * viewer works even when the inline block is showing its source.
	 *
	 * Zoom and pan follow ImageLightbox: the picture is fitted to the viewport at
	 * 100%, and the transform grows it around the pointer or pinch, so a dense
	 * diagram can be read without changing the page layout around it.
	 */

	interface Props {
		/** Mermaid source, exactly as it appeared in the answer. */
		source: string;
		open?: boolean;
	}

	let { source, open = $bindable(false) }: Props = $props();

	const { t } = useTranslation();

	const MIN_SCALE = 0.25;
	const MAX_SCALE = 8;

	let stage = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);

	let svg = $state('');
	let failed = $state(false);
	let scale = $state(1);
	let offsetX = $state(0);
	let offsetY = $state(0);
	let dragging = $state(false);

	/** Pointer bookkeeping for panning, plus the click used to dismiss the backdrop. */
	let panPointer: number | null = null;
	let panStartX = 0;
	let panStartY = 0;
	let panOriginX = 0;
	let panOriginY = 0;
	let pinchDistance = 0;
	/** Fingers currently on the stage, so a second one starts a pinch, not a pan. */
	let activePointers = 0;
	/**
	 * The click that dismisses the viewer must start on the empty backdrop. Pointer
	 * capture retargets the follow-up click to the stage even when the press began
	 * on the diagram, and a pan that ends over the diagram would otherwise close it.
	 */
	let pressedBackdrop = false;
	let panMoved = false;

	let diagramMode: MermaidMode = $derived(mode.current === 'dark' ? 'dark' : 'light');
	let isZoomed = $derived(scale > 1.001);
	let zoomLabel = $derived(`${Math.round(scale * 100)}%`);

	function resetZoom() {
		scale = 1;
		offsetX = 0;
		offsetY = 0;
		dragging = false;
		panPointer = null;
		activePointers = 0;
		pinchDistance = 0;
	}

	function clampScale(value: number) {
		return Math.min(MAX_SCALE, Math.max(MIN_SCALE, value));
	}

	/** Zooms around a viewport point so the detail under the cursor stays put. */
	function zoomAt(factor: number, clientX?: number, clientY?: number) {
		const next = clampScale(scale * factor);
		if (Math.abs(next - scale) < 0.001) return;
		const node = stage;
		if (node && clientX != null && clientY != null) {
			const rect = node.getBoundingClientRect();
			const x = clientX - rect.left - rect.width / 2;
			const y = clientY - rect.top - rect.height / 2;
			const ratio = next / scale;
			offsetX = x - (x - offsetX) * ratio;
			offsetY = y - (y - offsetY) * ratio;
		}
		scale = next;
		if (next <= 1) {
			offsetX = 0;
			offsetY = 0;
			dragging = false;
		}
	}

	function close() {
		open = false;
	}

	$effect(() => {
		const src = source;
		const diagramTheme = diagramMode;
		let cancelled = false;
		svg = '';
		failed = false;
		void renderMermaid(src, diagramTheme)
			.then((markup) => {
				if (!cancelled) svg = markup;
			})
			.catch(() => {
				if (!cancelled) failed = true;
			});
		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		if (!open) return;
		closeButton?.focus({ preventScroll: true });
	});

	$effect(() => {
		if (!open || typeof document === 'undefined') return;
		const previous = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		return () => {
			document.body.style.overflow = previous;
		};
	});

	$effect(() => {
		if (!open) return;
		const onKey = (event: KeyboardEvent) => {
			switch (event.key) {
				case 'Escape':
					event.preventDefault();
					close();
					break;
				case '+':
				case '=':
					event.preventDefault();
					zoomAt(1.25);
					break;
				case '-':
				case '_':
					event.preventDefault();
					zoomAt(0.8);
					break;
				case '0':
					event.preventDefault();
					resetZoom();
					break;
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	// Pointer listeners live here because touchmove must be non-passive to stop the
	// page from panning or zooming underneath the viewer.
	$effect(() => {
		if (!open) return;
		const node = stage;
		if (!node) return;

		const onWheel = (event: WheelEvent) => {
			event.preventDefault();
			zoomAt(event.deltaY < 0 ? 1.15 : 1 / 1.15, event.clientX, event.clientY);
		};

		const onDoubleClick = (event: MouseEvent) => {
			event.preventDefault();
			if (isZoomed) resetZoom();
			else zoomAt(2.5, event.clientX, event.clientY);
		};

		const onPointerDown = (event: PointerEvent) => {
			activePointers += 1;
			pressedBackdrop = event.target === node;
			panMoved = false;
			if (activePointers > 1) {
				dragging = false;
				panPointer = null;
				return;
			}
			if (!isZoomed) return;
			if (event.pointerType === 'mouse' && event.button !== 0) return;
			panPointer = event.pointerId;
			dragging = true;
			panStartX = event.clientX;
			panStartY = event.clientY;
			panOriginX = offsetX;
			panOriginY = offsetY;
			node.setPointerCapture?.(event.pointerId);
		};

		const onPointerMove = (event: PointerEvent) => {
			if (activePointers > 1) return;
			if (panPointer !== event.pointerId || !dragging) return;
			if (!panMoved && Math.hypot(event.clientX - panStartX, event.clientY - panStartY) > 4) {
				panMoved = true;
			}
			offsetX = panOriginX + (event.clientX - panStartX);
			offsetY = panOriginY + (event.clientY - panStartY);
		};

		const onPointerUp = (event: PointerEvent) => {
			activePointers = Math.max(0, activePointers - 1);
			if (panPointer !== event.pointerId) return;
			panPointer = null;
			dragging = false;
			node.releasePointerCapture?.(event.pointerId);
		};

		const onTouchMove = (event: TouchEvent) => {
			if (event.touches.length !== 2) {
				pinchDistance = 0;
				return;
			}
			event.preventDefault();
			const [first, second] = [event.touches[0], event.touches[1]];
			const distance = Math.hypot(first.clientX - second.clientX, first.clientY - second.clientY);
			if (pinchDistance > 0) {
				zoomAt(
					distance / pinchDistance,
					(first.clientX + second.clientX) / 2,
					(first.clientY + second.clientY) / 2
				);
			}
			pinchDistance = distance;
		};

		const onTouchEnd = () => {
			pinchDistance = 0;
		};

		const onClick = (event: MouseEvent) => {
			// Only a press that began on the empty area dismisses the viewer, and never
			// after a pan: the drag was meant to move the diagram, not to close it.
			const dismiss = pressedBackdrop && !panMoved && event.target === node;
			pressedBackdrop = false;
			panMoved = false;
			if (dismiss) close();
		};

		node.addEventListener('wheel', onWheel, { passive: false });
		node.addEventListener('dblclick', onDoubleClick);
		node.addEventListener('pointerdown', onPointerDown);
		node.addEventListener('pointermove', onPointerMove);
		node.addEventListener('pointerup', onPointerUp);
		node.addEventListener('pointercancel', onPointerUp);
		node.addEventListener('touchmove', onTouchMove, { passive: false });
		node.addEventListener('touchend', onTouchEnd);
		node.addEventListener('touchcancel', onTouchEnd);
		node.addEventListener('click', onClick);

		return () => {
			node.removeEventListener('wheel', onWheel);
			node.removeEventListener('dblclick', onDoubleClick);
			node.removeEventListener('pointerdown', onPointerDown);
			node.removeEventListener('pointermove', onPointerMove);
			node.removeEventListener('pointerup', onPointerUp);
			node.removeEventListener('pointercancel', onPointerUp);
			node.removeEventListener('touchmove', onTouchMove);
			node.removeEventListener('touchend', onTouchEnd);
			node.removeEventListener('touchcancel', onTouchEnd);
			node.removeEventListener('click', onClick);
		};
	});

	const buttonClass =
		'pointer-events-auto grid size-9 place-items-center rounded-full bg-black/50 text-white transition-colors hover:bg-black/70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white';
</script>

{#if open}
	<div
		class="fixed inset-0 z-100 select-none bg-black/90 text-white"
		role="dialog"
		aria-modal="true"
		aria-label={t('common.markdown.expand')}
	>
		<div
			bind:this={stage}
			class="mermaid-viewer absolute inset-0 flex touch-none items-center justify-center overflow-hidden p-4 pt-20 pb-16"
			role="presentation"
		>
			{#if svg}
				<div
					class="rounded-lg bg-background p-3 shadow-2xl will-change-transform"
					class:transition-transform={!dragging}
					class:duration-150={!dragging}
					class:cursor-grab={isZoomed && !dragging}
					class:cursor-grabbing={isZoomed && dragging}
					class:cursor-zoom-in={!isZoomed}
					style:transform={`translate(${offsetX}px, ${offsetY}px) scale(${scale})`}
				>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html svg}
				</div>
			{:else if failed}
				<pre
					class="max-h-full max-w-full overflow-auto rounded-lg bg-black/40 p-4 text-xs text-white/80">{source}</pre>
			{:else}
				<span class="text-xs text-white/60">{t('common.loading')}</span>
			{/if}
		</div>

		<div class="pointer-events-none absolute inset-x-0 top-0 flex justify-end gap-1 p-3">
			<button
				type="button"
				class={buttonClass}
				title={t('common.markdown.zoom_out')}
				aria-label={t('common.markdown.zoom_out')}
				onclick={() => zoomAt(0.8)}
			>
				<ZoomOut class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="pointer-events-auto min-w-14 rounded-full bg-black/50 px-2 py-1.5 text-xs font-medium text-white tabular-nums transition-colors hover:bg-black/70"
				title={t('common.markdown.zoom_reset')}
				aria-label={t('common.markdown.zoom_reset')}
				onclick={resetZoom}
			>
				{zoomLabel}
			</button>
			<button
				type="button"
				class={buttonClass}
				title={t('common.markdown.zoom_in')}
				aria-label={t('common.markdown.zoom_in')}
				onclick={() => zoomAt(1.25)}
			>
				<ZoomIn class="h-4 w-4" />
			</button>
			<button
				bind:this={closeButton}
				type="button"
				class={buttonClass}
				title={t('common.markdown.close')}
				aria-label={t('common.markdown.close')}
				onclick={close}
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>
{/if}

<style>
	/*
	 * The diagram is fitted to the viewport at 100% (max-width and max-height keep
	 * the ratio), then the wrapper's transform zooms it around the pointer.
	 */
	.mermaid-viewer :global(svg) {
		display: block;
		max-width: calc(100vw - 4rem);
		max-height: calc(100dvh - 11rem);
		height: auto;
	}
</style>
