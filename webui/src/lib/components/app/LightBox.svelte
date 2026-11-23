<script lang="ts">
	import { untrack } from 'svelte';
	import {
		ChevronLeft,
		ChevronRight,
		Download,
		ExternalLink,
		File as FileIcon,
		Loader2,
		Music,
		Trash2,
		X,
		ZoomIn,
		ZoomOut
	} from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { filesStore } from '$lib/stores/files.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { getErrorMessage } from '$lib/utils/api';
	import { stripFrontmatter, type FilePreviewKind } from '$lib/utils/filePreview';
	import { parseCsv } from '$lib/utils/csv';
	import { formatSize } from '$lib/utils/format';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { LightboxImage } from '$lib/types';

	interface Props {
		items?: LightboxImage[];
		open?: boolean;
		index?: number;
		/** Optional delete callback; renders a delete button in the top bar when set. */
		onDelete?: (item: LightboxImage) => void;
	}

	let { items = [], open = $bindable(false), index = $bindable(0), onDelete }: Props = $props();

	const { t } = useTranslation();

	const MIN_SCALE = 0.5;
	const MAX_SCALE = 6;
	const MAX_TEXT_PREVIEW = 512 * 1024;

	let stage = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);

	let scale = $state(1);
	let offsetX = $state(0);
	let offsetY = $state(0);
	let dragging = $state(false);

	let docContent = $state('');
	let docLoading = $state(false);
	let docError = $state('');
	let loadGeneration = 0;

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
	let kind = $derived<FilePreviewKind>(current?.kind ?? 'image');
	let isImage = $derived(kind === 'image');
	let isTextKind = $derived(
		kind === 'markdown' || kind === 'code' || kind === 'text' || kind === 'csv' || kind === 'html'
	);
	let isMediaKind = $derived(
		kind === 'image' || kind === 'video' || kind === 'audio' || kind === 'pdf'
	);
	let canNavigate = $derived(items.length > 1);
	let isZoomed = $derived(scale > 1.001);
	let zoomLabel = $derived(`${Math.round(scale * 100)}%`);
	let mediaReady = $derived(Boolean(current?.src && current.src !== 'data:,'));
	let csvTable = $derived(kind === 'csv' && docContent ? parseCsv(docContent) : null);

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
		if (!isImage) return;
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
		if (item.agent && item.path) {
			void filesStore.download(item.agent, item.path);
			return;
		}
		void filesStore.downloadUrl(item.downloadSrc || item.src, item.name || 'file');
	}

	// A new item always starts unzoomed, and closing drops any pan so reopening
	// the same item is predictable.
	let lastView = '';
	$effect(() => {
		const view = `${open}:${index}`;
		if (view === lastView) return;
		lastView = view;
		resetZoom();
	});

	// Load text content or sign media URLs on demand whenever the active slide changes.
	$effect(() => {
		if (!open || !current) return;
		const item = current;
		const itemKind = item.kind ?? 'image';
		const agent = item.agent;
		const path = item.path;
		const size = item.size ?? 0;

		untrack(() => {
			const token = ++loadGeneration;
			docContent = '';
			docError = '';
			docLoading = false;

			if (!agent || !path) return;

			if (
				itemKind === 'image' ||
				itemKind === 'video' ||
				itemKind === 'audio' ||
				itemKind === 'pdf'
			) {
				void ensureSigned(agent, [path]);
				return;
			}

			if (
				itemKind === 'markdown' ||
				itemKind === 'code' ||
				itemKind === 'text' ||
				itemKind === 'csv' ||
				itemKind === 'html'
			) {
				if (size > MAX_TEXT_PREVIEW) {
					docError = t('myaaw.artifacts.too_large');
					return;
				}
				docLoading = true;
				void filesStore
					.readFile(agent, path)
					.then((file) => {
						if (token !== loadGeneration) return;
						docContent = file.content ?? '';
					})
					.catch((err) => {
						if (token !== loadGeneration) return;
						docError = getErrorMessage(err);
					})
					.finally(() => {
						if (token === loadGeneration) docLoading = false;
					});
			}
		});
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
			const inMediaControl = event.target instanceof HTMLMediaElement;
			switch (event.key) {
				case 'Escape':
					event.preventDefault();
					close();
					break;
				case '+':
				case '=':
					if (!isImage) return;
					event.preventDefault();
					zoomAt(1.25);
					break;
				case '-':
				case '_':
					if (!isImage) return;
					event.preventDefault();
					zoomAt(0.8);
					break;
				case '0':
					if (!isImage) return;
					event.preventDefault();
					resetZoom();
					break;
				case 'ArrowLeft':
					if (inMediaControl) return;
					event.preventDefault();
					step(-1);
					break;
				case 'ArrowRight':
					if (inMediaControl) return;
					event.preventDefault();
					step(1);
					break;
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	// Pointer and gesture listeners live here rather than in the markup: the stage
	// is a presentation container, and touchmove must be non-passive for image pinch-zoom.
	$effect(() => {
		if (!open) return;
		const node = stage;
		if (!node) return;
		const imageActive = isImage;

		const onWheel = (event: WheelEvent) => {
			if (!imageActive) return;
			event.preventDefault();
			zoomAt(event.deltaY < 0 ? 1.15 : 1 / 1.15, event.clientX, event.clientY);
		};

		const onDoubleClick = (event: MouseEvent) => {
			if (!imageActive) return;
			event.preventDefault();
			if (isZoomed) resetZoom();
			else zoomAt(2.5, event.clientX, event.clientY);
		};

		const onPointerDown = (event: PointerEvent) => {
			activePointers += 1;
			pressedBackdrop = event.target === node;
			panMoved = false;
			if (!imageActive) return;
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
			if (!imageActive || activePointers > 1) return;
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
			if (!imageActive) return;
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
			// Only a press that began on the empty backdrop dismisses the lightbox.
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
		'pointer-events-auto grid size-9 place-items-center rounded-full bg-black/50 text-white transition-colors hover:bg-black/75 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white';
</script>

{#if open && current}
	<div
		class={['fixed inset-0 z-100 bg-black/90 text-white', isImage ? 'select-none' : '']}
		role="dialog"
		aria-modal="true"
		aria-label={current.name}
	>
		<div
			bind:this={stage}
			class={[
				'absolute inset-0 flex items-center justify-center overflow-hidden p-4 pt-16 pb-16 sm:px-14',
				isImage ? 'touch-none' : ''
			]}
			role="presentation"
		>
			{#if kind === 'image'}
				{#if mediaReady}
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
				{:else}
					<Loader2 class="h-7 w-7 animate-spin text-white/70" />
				{/if}
			{:else if kind === 'video'}
				{#if mediaReady}
					{#key current.src}
						<!-- svelte-ignore a11y_media_has_caption -->
						<video
							src={current.src}
							controls
							playsinline
							preload="metadata"
							class="max-h-full max-w-full rounded-2xl bg-black shadow-2xl"
						></video>
					{/key}
				{:else}
					<Loader2 class="h-7 w-7 animate-spin text-white/70" />
				{/if}
			{:else if kind === 'audio'}
				<div
					class="flex w-full max-w-md flex-col items-center gap-5 rounded-2xl bg-card p-8 text-card-foreground shadow-2xl ring-1 ring-white/10"
				>
					<span
						class="grid h-20 w-20 place-items-center rounded-3xl bg-linear-to-br from-fuchsia-500/20 via-fuchsia-500/5 to-transparent text-fuchsia-500 ring-1 ring-border"
					>
						<Music class="h-9 w-9" />
					</span>
					<div class="w-full min-w-0 text-center">
						<p class="truncate text-sm font-semibold">{current.name}</p>
						{#if current.label || current.size}
							<p class="mt-1 text-xs text-muted-foreground">
								{[current.label, current.size ? formatSize(current.size) : '']
									.filter(Boolean)
									.join(' · ')}
							</p>
						{/if}
					</div>
					{#if mediaReady}
						{#key current.src}
							<audio src={current.src} controls preload="metadata" class="w-full"></audio>
						{/key}
					{:else}
						<Loader2 class="h-5 w-5 animate-spin text-muted-foreground" />
					{/if}
				</div>
			{:else if kind === 'pdf'}
				<div
					class="flex h-full w-full max-w-5xl overflow-hidden rounded-2xl bg-card shadow-2xl ring-1 ring-white/10"
				>
					{#if mediaReady}
						<iframe src={current.src} title={current.name} class="h-full w-full bg-background"
						></iframe>
					{:else}
						<div class="flex h-full w-full items-center justify-center">
							<Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
						</div>
					{/if}
				</div>
			{:else if isTextKind}
				<div
					class="flex h-full w-full max-w-5xl flex-col overflow-hidden rounded-2xl bg-card text-card-foreground shadow-2xl ring-1 ring-white/10 select-text"
				>
					{#if docLoading}
						<div class="flex h-full w-full items-center justify-center">
							<Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
						</div>
					{:else if docError}
						<div class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
							<p class="text-sm text-muted-foreground">{docError}</p>
							<Button variant="outline" size="sm" onclick={download}>
								<Download class="mr-2 h-4 w-4" />
								{t('myaaw.files.download')}
							</Button>
						</div>
					{:else if kind === 'markdown'}
						<div class="min-h-0 flex-1 overflow-y-auto p-6 sm:p-8">
							<div class="mx-auto max-w-3xl">
								<Markdown content={stripFrontmatter(docContent)} />
							</div>
						</div>
					{:else if kind === 'html'}
						<iframe
							srcdoc={docContent}
							sandbox="allow-scripts"
							title={current.name}
							class="h-full w-full bg-white"
						></iframe>
					{:else if kind === 'csv' && csvTable}
						<div class="min-h-0 flex-1 overflow-auto p-4 sm:p-6">
							<div class="overflow-x-auto rounded-xl border">
								<table class="w-full border-collapse text-left text-xs">
									<thead class="bg-muted/60">
										<tr>
											{#each csvTable.headers as header, idx (idx)}
												<th class="border-b px-3 py-2 font-medium">{header}</th>
											{/each}
										</tr>
									</thead>
									<tbody class="divide-y">
										{#each csvTable.rows as row, rIdx (rIdx)}
											<tr class="hover:bg-muted/30">
												{#each row as cell, cIdx (cIdx)}
													<td class="px-3 py-1.5 whitespace-nowrap">{cell}</td>
												{/each}
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
							{#if csvTable.truncated}
								<p class="mt-2 text-xs text-muted-foreground">
									{t('myaaw.artifacts.csv_truncated', { count: csvTable.totalRows })}
								</p>
							{/if}
						</div>
					{:else}
						<div class="min-h-0 flex-1 overflow-hidden p-3 sm:p-4">
							<CodePreview code={docContent} language={current.language ?? 'plaintext'} />
						</div>
					{/if}
				</div>
			{:else}
				<div
					class="flex w-full max-w-md flex-col items-center gap-4 rounded-2xl bg-card p-8 text-center text-card-foreground shadow-2xl ring-1 ring-white/10"
				>
					<span
						class="grid h-16 w-16 place-items-center rounded-2xl bg-muted text-muted-foreground"
					>
						<FileIcon class="h-8 w-8" />
					</span>
					<div class="w-full min-w-0">
						<p class="truncate text-sm font-semibold">{current.name}</p>
						<p class="mt-1 text-xs text-muted-foreground">{t('myaaw.artifacts.no_preview')}</p>
					</div>
					<Button size="sm" onclick={download}>
						<Download class="mr-2 h-4 w-4" />
						{t('myaaw.files.download')}
					</Button>
				</div>
			{/if}
		</div>

		<div
			class="pointer-events-none absolute inset-x-0 top-0 flex items-center justify-between gap-2 p-3"
		>
			<div class="flex min-w-0 items-center gap-2">
				{#if canNavigate}
					<span
						class="shrink-0 rounded-full bg-black/50 px-2.5 py-1 text-xs font-medium text-white tabular-nums"
					>
						{position + 1} / {items.length}
					</span>
				{/if}
				<div class="hidden min-w-0 items-center gap-1.5 rounded-full bg-black/50 px-3 py-1 sm:flex">
					<span class="max-w-xs truncate text-xs font-medium text-white/95 md:max-w-md">
						{current.name}
					</span>
					{#if current.meta || current.label || current.size}
						<span class="shrink-0 text-[11px] text-white/65">
							· {[current.meta, current.label, current.size ? formatSize(current.size) : '']
								.filter(Boolean)
								.join(' · ')}
						</span>
					{/if}
				</div>
			</div>

			<div class="flex shrink-0 items-center gap-1">
				{#if isImage}
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
						class="pointer-events-auto min-w-14 rounded-full bg-black/50 px-2 py-1.5 text-xs font-medium text-white tabular-nums transition-colors hover:bg-black/75"
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
				{/if}
				{#if isMediaKind && !isImage && mediaReady}
					<button
						type="button"
						class={buttonClass}
						title={t('myaaw.files.open_new_tab')}
						aria-label={t('myaaw.files.open_new_tab')}
						onclick={() => current && window.open(current.src, '_blank', 'noopener,noreferrer')}
					>
						<ExternalLink class="h-4 w-4" />
					</button>
				{/if}
				<button
					type="button"
					class={buttonClass}
					title={t('myaaw.chat.download_file')}
					aria-label={t('myaaw.chat.download_file')}
					onclick={download}
				>
					<Download class="h-4 w-4" />
				</button>
				{#if onDelete}
					<button
						type="button"
						class="{buttonClass} hover:bg-destructive"
						title={t('common.delete')}
						aria-label={t('common.delete')}
						onclick={() => current && onDelete(current)}
					>
						<Trash2 class="h-4 w-4" />
					</button>
				{/if}
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
