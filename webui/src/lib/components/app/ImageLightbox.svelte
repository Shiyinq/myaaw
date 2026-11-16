<script lang="ts">
	import { ChevronLeft, ChevronRight, Download, X, ZoomIn, ZoomOut } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { filesStore } from '$lib/stores/files.svelte';
	import type { LightboxImage } from '$lib/types';

	interface Props {
		items?: LightboxImage[];
		open?: boolean;
		index?: number;
	}

	let { items = [], open = $bindable(false), index = $bindable(0) }: Props = $props();

	const { t } = useTranslation();

	const MIN_SCALE = 0.5;
	const MAX_SCALE = 6;

	let stage = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);

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
	 * The click that dismisses the lightbox must start on the empty backdrop.
	 * Pointer capture retargets the follow-up click to the stage even when the press
	 * began on the photo, and a pan that ends over the photo would otherwise close
	 * the lightbox mid-gesture.
	 */
	let pressedBackdrop = false;
	let panMoved = false;

	let position = $derived(Math.min(Math.max(index, 0), Math.max(0, items.length - 1)));
	let current = $derived(items[position] as LightboxImage | undefined);
	let canNavigate = $derived(items.length > 1);
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

	/**
	 * Zooms around a viewport point so the detail under the cursor or pinch stays
	 * put. Without a point the image grows from its centre.
	 */
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

	function step(delta: number) {
		if (!canNavigate) return;
		index = Math.min(items.length - 1, Math.max(0, position + delta));
	}

	function close() {
		open = false;
	}

	function download() {
		const item = current;
		if (!item) return;
		void filesStore.downloadUrl(item.downloadSrc || item.src, item.name || 'image');
	}

	// A new image always starts unzoomed, and closing drops any pan so reopening
	// the same photo is predictable.
	let lastView = '';
	$effect(() => {
		const view = `${open}:${index}`;
		if (view === lastView) return;
		lastView = view;
		resetZoom();
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
				case 'ArrowLeft':
					event.preventDefault();
					step(-1);
					break;
				case 'ArrowRight':
					event.preventDefault();
					step(1);
					break;
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	// Pointer and gesture listeners live here rather than in the markup: the stage
	// is a presentation container, and touchmove must be non-passive to stop the
	// page from panning or zooming underneath the lightbox.
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
			// Only a press that began on the empty area dismisses the photo, and never
			// after a pan: the drag was meant to move the picture, not to close it.
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

{#if open && current}
	<div
		class="fixed inset-0 z-100 select-none bg-black/90 text-white"
		role="dialog"
		aria-modal="true"
		aria-label={current.name}
	>
		<div
			bind:this={stage}
			class="absolute inset-0 flex touch-none items-center justify-center overflow-hidden p-4 pt-16 pb-16"
			role="presentation"
		>
			<img
				src={current.src}
				alt={current.name}
				draggable="false"
				class="max-h-full max-w-full object-contain will-change-transform"
				class:transition-transform={!dragging}
				class:duration-150={!dragging}
				class:cursor-grab={isZoomed && !dragging}
				class:cursor-grabbing={isZoomed && dragging}
				class:cursor-zoom-in={!isZoomed}
				style:transform={`translate(${offsetX}px, ${offsetY}px) scale(${scale})`}
			/>
		</div>

		<div
			class="pointer-events-none absolute inset-x-0 top-0 flex items-start justify-between gap-2 p-3"
		>
			{#if canNavigate}
				<span
					class="rounded-full bg-black/50 px-2.5 py-1 text-xs font-medium text-white tabular-nums"
				>
					{position + 1} / {items.length}
				</span>
			{:else}
				<span></span>
			{/if}

			<div class="flex items-center gap-1">
				<button
					type="button"
					class={buttonClass}
					title={t('myaaw.chat.lightbox_zoom_out')}
					aria-label={t('myaaw.chat.lightbox_zoom_out')}
					onclick={() => zoomAt(0.8)}
				>
					<ZoomOut class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="pointer-events-auto min-w-14 rounded-full bg-black/50 px-2 py-1.5 text-xs font-medium text-white tabular-nums transition-colors hover:bg-black/70"
					title={t('myaaw.chat.lightbox_reset')}
					aria-label={t('myaaw.chat.lightbox_reset')}
					onclick={resetZoom}
				>
					{zoomLabel}
				</button>
				<button
					type="button"
					class={buttonClass}
					title={t('myaaw.chat.lightbox_zoom_in')}
					aria-label={t('myaaw.chat.lightbox_zoom_in')}
					onclick={() => zoomAt(1.25)}
				>
					<ZoomIn class="h-4 w-4" />
				</button>
				<button
					type="button"
					class={buttonClass}
					title={t('myaaw.chat.download_file')}
					aria-label={t('myaaw.chat.download_file')}
					onclick={download}
				>
					<Download class="h-4 w-4" />
				</button>
				<button
					bind:this={closeButton}
					type="button"
					class={buttonClass}
					title={t('myaaw.chat.lightbox_close')}
					aria-label={t('myaaw.chat.lightbox_close')}
					onclick={close}
				>
					<X class="h-4 w-4" />
				</button>
			</div>
		</div>

		{#if canNavigate}
			<div class="pointer-events-none absolute inset-y-0 left-0 flex items-center p-2">
				<button
					type="button"
					class="{buttonClass} disabled:opacity-30"
					title={t('myaaw.chat.lightbox_previous')}
					aria-label={t('myaaw.chat.lightbox_previous')}
					disabled={position <= 0}
					onclick={() => step(-1)}
				>
					<ChevronLeft class="h-5 w-5" />
				</button>
			</div>
			<div class="pointer-events-none absolute inset-y-0 right-0 flex items-center p-2">
				<button
					type="button"
					class="{buttonClass} disabled:opacity-30"
					title={t('myaaw.chat.lightbox_next')}
					aria-label={t('myaaw.chat.lightbox_next')}
					disabled={position >= items.length - 1}
					onclick={() => step(1)}
				>
					<ChevronRight class="h-5 w-5" />
				</button>
			</div>
		{/if}
	</div>
{/if}
