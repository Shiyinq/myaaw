<script lang="ts">
	import {
		ArrowDownToLine,
		ChevronLeft,
		ChevronRight,
		Download,
		ExternalLink,
		File as FileIcon,
		FileText
	} from 'lucide-svelte';
	import { filesStore } from '$lib/stores/files.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ExtraMediaBlockData, ExtraMediaItem, LightboxImage } from '$lib/types';
	import LightBox from '$lib/components/app/LightBox.svelte';

	interface Props {
		agent: string;
		data: ExtraMediaBlockData;
	}

	let { agent, data }: Props = $props();

	const { t } = useTranslation();

	function kindOf(item: ExtraMediaItem): string {
		if (item.kind) return item.kind;
		const mime = item.mime_type ?? '';
		if (mime.startsWith('image/')) return 'image';
		if (mime.startsWith('video/')) return 'video';
		if (mime.startsWith('audio/')) return 'audio';
		if (mime === 'application/pdf') return 'pdf';
		return 'file';
	}

	function srcOf(item: ExtraMediaItem, download = false): string {
		if (item.path) {
			return agent ? filesStore.rawUrl(agent, item.path, { download }) : '';
		}
		return item.url ?? '';
	}

	function labelOf(item: ExtraMediaItem): string {
		return item.title || item.file_name || item.url || item.path || 'file';
	}

	function sizeLabel(bytes?: number): string {
		if (!bytes || bytes <= 0) return '';
		const units = ['B', 'KB', 'MB', 'GB'];
		let value = bytes;
		let unit = 0;
		while (value >= 1024 && unit < units.length - 1) {
			value /= 1024;
			unit += 1;
		}
		return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
	}

	let items = $derived(data.items ?? []);
	let visualItems = $derived(items.filter((it) => ['image', 'video'].includes(kindOf(it))));
	let otherItems = $derived(items.filter((it) => !['image', 'video'].includes(kindOf(it))));
	let imageItems = $derived(visualItems.filter((it) => kindOf(it) === 'image'));

	$effect(() => {
		if (!agent) return;
		const paths = items.map((it) => it.path).filter((p): p is string => Boolean(p));
		if (paths.length > 0) {
			void ensureSigned(agent, paths);
		}
	});

	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let lightboxItems = $state<LightboxImage[]>([]);
	let imageLightboxItems = $derived<LightboxImage[]>(
		imageItems.map((item) => ({
			src: srcOf(item),
			name: labelOf(item),
			downloadSrc: srcOf(item, true),
			kind: 'image',
			agent: item.path ? agent : undefined,
			path: item.path,
			size: item.size
		}))
	);

	/** Suppresses the click that ends a horizontal swipe on the media strip. */
	let pressX = 0;
	let pressY = 0;

	function notePress(event: PointerEvent) {
		pressX = event.clientX;
		pressY = event.clientY;
	}

	function openImage(event: MouseEvent, item: ExtraMediaItem) {
		if (Math.hypot(event.clientX - pressX, event.clientY - pressY) > 8) return;
		const idx = imageItems.indexOf(item);
		lightboxItems = imageLightboxItems;
		lightboxIndex = idx < 0 ? 0 : idx;
		lightboxOpen = true;
	}

	function openPdfPreview(item: ExtraMediaItem) {
		lightboxItems = [
			{
				src: srcOf(item),
				name: labelOf(item),
				downloadSrc: srcOf(item, true),
				kind: 'pdf',
				label: 'PDF',
				agent: item.path ? agent : undefined,
				path: item.path,
				size: item.size
			}
		];
		lightboxIndex = 0;
		lightboxOpen = true;
	}

	function downloadItem(item: ExtraMediaItem) {
		if (item.path && agent) {
			filesStore.download(agent, item.path);
			return;
		}
		const src = srcOf(item);
		if (!src) return;
		const link = document.createElement('a');
		link.href = src;
		link.download = item.file_name || 'download';
		link.target = '_blank';
		link.rel = 'noopener noreferrer';
		document.body.appendChild(link);
		link.click();
		link.remove();
	}

	let track = $state<HTMLDivElement | null>(null);
	let active = $state(0);
	let canPrev = $state(false);
	let canNext = $state(false);
	let isScrollable = $state(false);

	let current = $derived(Math.min(active, Math.max(0, visualItems.length - 1)));

	function syncNav() {
		const node = track;
		if (!node) return;
		isScrollable = node.scrollWidth > node.clientWidth + 4;
		canPrev = node.scrollLeft > 4;
		canNext = node.scrollLeft + node.clientWidth < node.scrollWidth - 4;
	}

	function onTrackScroll() {
		const node = track;
		if (!node) return;
		let best = 0;
		let bestDistance = Number.POSITIVE_INFINITY;
		Array.from(node.children).forEach((child, index) => {
			const distance = Math.abs((child as HTMLElement).offsetLeft - node.scrollLeft);
			if (distance < bestDistance) {
				bestDistance = distance;
				best = index;
			}
		});
		active = best;
		syncNav();
	}

	function goTo(index: number) {
		const node = track;
		const child = node?.children[index] as HTMLElement | undefined;
		if (node && child) node.scrollTo({ left: child.offsetLeft, behavior: 'smooth' });
	}

	$effect(() => {
		const node = track;
		if (!node || typeof ResizeObserver === 'undefined') return;
		syncNav();
		const observer = new ResizeObserver(() => syncNav());
		observer.observe(node);
		Array.from(node.children).forEach((child) => observer.observe(child));
		return () => observer.disconnect();
	});
</script>

{#snippet mediaItem(item: ExtraMediaItem, strip: boolean)}
	{#if kindOf(item) === 'image'}
		<div class="relative inline-block max-w-full">
			<button
				type="button"
				class="block cursor-zoom-in rounded-xl focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label={labelOf(item)}
				onpointerdown={notePress}
				onclick={(event) => openImage(event, item)}
			>
				<img
					src={srcOf(item)}
					alt={labelOf(item)}
					loading="lazy"
					draggable="false"
					referrerpolicy="no-referrer"
					class={strip
						? 'h-72 w-auto max-w-none rounded-xl border'
						: 'max-h-72 max-w-full rounded-xl border'}
				/>
			</button>
			<button
				type="button"
				onclick={() => downloadItem(item)}
				title={t('myaaw.chat.download_file')}
				aria-label={t('myaaw.chat.download_file')}
				class="absolute right-1.5 bottom-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
			>
				<Download class="h-3.5 w-3.5" />
			</button>
		</div>
	{:else}
		<!-- svelte-ignore a11y_media_has_caption -->
		<video
			src={srcOf(item)}
			controls
			preload="metadata"
			class={strip
				? 'block h-72 w-auto max-w-none rounded-xl border bg-black object-contain'
				: 'block max-h-[70vh] max-w-full rounded-xl border bg-black object-contain'}
		></video>
	{/if}
{/snippet}

<div class="space-y-2">
	{#if visualItems.length === 1 || data.layout === 'list'}
		<div class="space-y-2">
			{#each visualItems as item, idx (`list-${idx}`)}
				<div class="space-y-1">
					{@render mediaItem(item, false)}
					{#if item.caption || item.title}
						<p class="text-[11px] text-muted-foreground">{item.caption || item.title}</p>
					{/if}
				</div>
			{/each}
		</div>
	{:else if visualItems.length > 1}
		<div class="space-y-1.5">
			<div class="relative">
				<div
					bind:this={track}
					onscroll={onTrackScroll}
					class="flex snap-x snap-mandatory gap-2 overflow-x-auto overscroll-x-contain pb-1 [scrollbar-width:none] [touch-action:pan-x] [-webkit-overflow-scrolling:touch] [&::-webkit-scrollbar]:hidden"
				>
					{#each visualItems as item, idx (`strip-${idx}`)}
						<div class="shrink-0 snap-start space-y-1">
							{@render mediaItem(item, true)}
							{#if item.caption || item.title}
								<p class="max-w-[18rem] truncate text-[11px] text-muted-foreground">
									{item.caption || item.title}
								</p>
							{/if}
						</div>
					{/each}
				</div>
				{#if canPrev}
					<button
						type="button"
						onclick={() => goTo(Math.max(0, current - 1))}
						aria-label={t('myaaw.extra.prev')}
						class="absolute top-1/2 left-1.5 grid size-8 -translate-y-1/2 place-items-center rounded-full border bg-background/85 text-foreground shadow-sm backdrop-blur transition-colors hover:bg-background"
					>
						<ChevronLeft class="h-4 w-4" />
					</button>
				{/if}
				{#if canNext}
					<button
						type="button"
						onclick={() => goTo(Math.min(visualItems.length - 1, current + 1))}
						aria-label={t('myaaw.extra.next')}
						class="absolute top-1/2 right-1.5 grid size-8 -translate-y-1/2 place-items-center rounded-full border bg-background/85 text-foreground shadow-sm backdrop-blur transition-colors hover:bg-background"
					>
						<ChevronRight class="h-4 w-4" />
					</button>
				{/if}
			</div>
			{#if isScrollable}
				<div class="flex justify-center gap-1.5">
					{#each visualItems as _, index (index)}
						<button
							type="button"
							class="h-1.5 rounded-full transition-all {current === index
								? 'w-4 bg-foreground'
								: 'w-1.5 bg-muted-foreground/30 hover:bg-muted-foreground/60'}"
							aria-label={`Item ${index + 1}`}
							onclick={() => goTo(index)}
						></button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	{#if otherItems.length > 0}
		<div class="max-w-lg space-y-1.5">
			{#each otherItems as item, idx (`other-${idx}`)}
				{@const k = kindOf(item)}
				{#if k === 'audio'}
					<div class="space-y-1.5 rounded-xl border bg-card p-2.5">
						<div class="flex items-center justify-between gap-2 text-xs">
							<span class="truncate font-medium">{labelOf(item)}</span>
							<button
								type="button"
								onclick={() => downloadItem(item)}
								class="inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground"
							>
								<ArrowDownToLine class="h-3 w-3" />
								<span>{t('myaaw.chat.download_file')}</span>
							</button>
						</div>
						<audio src={srcOf(item)} controls preload="metadata" class="w-full"></audio>
					</div>
				{:else}
					<div class="flex items-center gap-2.5 rounded-xl border bg-card px-3 py-2">
						{#if k === 'pdf'}
							<button
								type="button"
								onclick={() => openPdfPreview(item)}
								class="flex min-w-0 flex-1 items-center gap-2.5 text-left transition-opacity hover:opacity-80"
								title={labelOf(item)}
							>
								<FileText class="h-4 w-4 shrink-0 text-rose-500" />
								<div class="min-w-0 flex-1">
									<p class="truncate text-xs font-medium text-foreground">{labelOf(item)}</p>
									{#if item.caption || item.size || item.mime_type}
										<p class="truncate text-[11px] text-muted-foreground">
											{[item.caption, sizeLabel(item.size), item.mime_type]
												.filter(Boolean)
												.join(' · ')}
										</p>
									{/if}
								</div>
							</button>
						{:else}
							<FileIcon class="h-4 w-4 shrink-0 text-muted-foreground" />
							<div class="min-w-0 flex-1">
								<p class="truncate text-xs font-medium text-foreground">{labelOf(item)}</p>
								{#if item.caption || item.size || item.mime_type}
									<p class="truncate text-[11px] text-muted-foreground">
										{[item.caption, sizeLabel(item.size), item.mime_type]
											.filter(Boolean)
											.join(' · ')}
									</p>
								{/if}
							</div>
						{/if}
						{#if item.url && !item.path}
							<a
								href={item.url}
								target="_blank"
								rel="noopener noreferrer"
								class="grid size-7 shrink-0 place-items-center rounded-lg border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
								title={t('myaaw.extra.open_link')}
								aria-label={t('myaaw.extra.open_link')}
							>
								<ExternalLink class="h-3.5 w-3.5" />
							</a>
						{/if}
						<button
							type="button"
							onclick={() => downloadItem(item)}
							class="grid size-7 shrink-0 place-items-center rounded-lg border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
							title={t('myaaw.chat.download_file')}
							aria-label={t('myaaw.chat.download_file')}
						>
							<ArrowDownToLine class="h-3.5 w-3.5" />
						</button>
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	{#if data.caption}
		<p class="text-xs text-muted-foreground">{data.caption}</p>
	{/if}
</div>

<LightBox bind:open={lightboxOpen} items={lightboxItems} bind:index={lightboxIndex} />
