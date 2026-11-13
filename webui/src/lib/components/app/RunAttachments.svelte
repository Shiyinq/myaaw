<script lang="ts">
	import {
		ArrowDownToLine,
		ChevronLeft,
		ChevronRight,
		Download,
		File as FileIcon
	} from 'lucide-svelte';
	import { files as filesApi } from '$lib/apis/files';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { Attachment, LightboxImage } from '$lib/types';
	import ImageLightbox from './ImageLightbox.svelte';
	import ProjectCard from './ProjectCard.svelte';

	interface Props {
		agent: string;
		attachments: Attachment[];
	}

	let { agent, attachments }: Props = $props();

	const { t } = useTranslation();

	/** Falls back to the MIME type for attachments produced before `kind` existed. */
	function kindOf(attachment: Attachment): string {
		if (attachment.kind) return attachment.kind;
		const mime = attachment.mime_type ?? '';
		if (mime.startsWith('image/')) return 'image';
		if (mime.startsWith('video/')) return 'video';
		if (mime.startsWith('audio/')) return 'audio';
		if (mime === 'application/pdf') return 'pdf';
		return 'file';
	}

	/** Path-based files stream through `/fs/raw`. */
	function srcOf(attachment: Attachment, download = false): string {
		return attachment.path ? filesApi.rawUrl(agent, attachment.path, { download }) : '';
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

	// Photos and videos share one horizontal carousel when there is more than one;
	// projects get their own card; everything else (audio, PDF, opaque files)
	// keeps its own row below.
	let projectItems = $derived(attachments.filter((a) => kindOf(a) === 'project'));
	let media = $derived(attachments.filter((a) => ['image', 'video'].includes(kindOf(a))));
	let others = $derived(
		attachments.filter((a) => !['image', 'video', 'project'].includes(kindOf(a)))
	);
	let images = $derived(media.filter((a) => kindOf(a) === 'image'));

	// Sign the media paths once per answer so images load without the session token.
	$effect(() => {
		const paths = attachments
			.filter((attachment) => kindOf(attachment) !== 'project')
			.map((attachment) => attachment.path)
			.filter((path): path is string => !!path);
		void ensureSigned(agent, paths);
	});

	// Clicking a photo opens the shared lightbox with the whole image set, so the
	// reader can zoom and step through the answer's pictures without going back.
	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let lightboxItems = $derived<LightboxImage[]>(
		images.map((attachment) => ({
			src: srcOf(attachment),
			name: attachment.file_name,
			downloadSrc: srcOf(attachment, true)
		}))
	);

	/** Suppresses the click that ends a horizontal swipe on the carousel. */
	let pressX = 0;
	let pressY = 0;

	function notePress(event: PointerEvent) {
		pressX = event.clientX;
		pressY = event.clientY;
	}

	function openImage(event: MouseEvent, attachment: Attachment) {
		if (Math.hypot(event.clientX - pressX, event.clientY - pressY) > 8) return;
		const found = images.findIndex((item) => item === attachment);
		lightboxIndex = found < 0 ? 0 : found;
		lightboxOpen = true;
	}

	/** The thumbnail's own button saves the photo without opening the lightbox. */
	function downloadImage(attachment: Attachment) {
		if (attachment.path) {
			filesApi.download(agent, attachment.path);
			return;
		}
		const src = srcOf(attachment);
		if (!src) return;
		const link = document.createElement('a');
		link.href = src;
		link.download = attachment.file_name;
		link.rel = 'noopener';
		document.body.appendChild(link);
		link.click();
		link.remove();
	}

	let track = $state<HTMLDivElement | null>(null);
	let active = $state(0);
	let canPrev = $state(false);
	let canNext = $state(false);

	/** Clamps the active slide when the attachment list shrinks. */
	let current = $derived(Math.min(active, Math.max(0, media.length - 1)));

	/** Refreshes whether the carousel can still move left or right. */
	function syncNav() {
		const node = track;
		if (!node) return;
		canPrev = node.scrollLeft > 4;
		canNext = node.scrollLeft + node.clientWidth < node.scrollWidth - 4;
	}

	/** Marks the slide closest to the left edge as active, for the dots and arrows. */
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

	// Slides size themselves from their media, so the arrow state must be refreshed
	// when the track or a slide changes width (for example a video's metadata).
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

{#snippet mediaItem(attachment: Attachment, carousel: boolean)}
	{#if kindOf(attachment) === 'image'}
		<div class="relative inline-block max-w-full">
			<button
				type="button"
				class="block cursor-zoom-in rounded-xl focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label={attachment.file_name}
				onpointerdown={notePress}
				onclick={(event) => openImage(event, attachment)}
			>
				<img
					src={srcOf(attachment)}
					alt={attachment.file_name}
					loading="lazy"
					draggable="false"
					class={carousel
						? 'h-72 w-auto max-w-none rounded-xl border'
						: 'max-h-72 max-w-full rounded-xl border'}
				/>
			</button>
			<button
				type="button"
				class="absolute right-1.5 bottom-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
				title={t('myaaw.chat.download_file')}
				aria-label={t('myaaw.chat.download_file')}
				onclick={() => downloadImage(attachment)}
			>
				<Download class="h-3.5 w-3.5" />
			</button>
		</div>
	{:else}
		<!-- Agent-produced clips have no caption track to attach. -->
		<!-- svelte-ignore a11y_media_has_caption -->
		<video
			controls
			preload="metadata"
			src={srcOf(attachment)}
			class={carousel
				? 'block h-72 w-auto max-w-none rounded-xl border bg-black object-contain'
				: 'block max-h-[70vh] max-w-full rounded-xl border bg-black object-contain'}
		></video>
	{/if}
{/snippet}

{#if projectItems.length > 0}
	<div class="space-y-2">
		{#each projectItems as attachment, index (`project-${index}-${attachment.path}`)}
			<ProjectCard
				{agent}
				path={attachment.path ?? ''}
				name={attachment.file_name}
				projectType={attachment.project_type}
				size={attachment.size}
			/>
		{/each}
	</div>
{/if}

{#if media.length === 1}
	<div class="space-y-2">
		{@render mediaItem(media[0], false)}
	</div>
{:else if media.length > 1}
	<div class="space-y-1.5">
		<div class="relative">
			<div
				bind:this={track}
				onscroll={onTrackScroll}
				class="flex snap-x snap-mandatory gap-2 overflow-x-auto overscroll-x-contain pb-1 [scrollbar-width:none] [touch-action:pan-x] [-webkit-overflow-scrolling:touch] [&::-webkit-scrollbar]:hidden"
			>
				{#each media as attachment, index (`media-${index}-${attachment.file_name}`)}
					<div class="shrink-0 snap-start">
						{@render mediaItem(attachment, true)}
					</div>
				{/each}
			</div>
			<!-- Touch-first navigation; desktop uses the trackpad, dots and track. -->
			<button
				type="button"
				class="absolute top-1/2 left-1 grid size-8 -translate-y-1/2 place-items-center rounded-full border bg-background/80 text-foreground shadow-sm backdrop-blur disabled:opacity-30 md:hidden"
				aria-label="Previous media"
				disabled={!canPrev}
				onclick={() => goTo(Math.max(0, current - 1))}
			>
				<ChevronLeft class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="absolute top-1/2 right-1 grid size-8 -translate-y-1/2 place-items-center rounded-full border bg-background/80 text-foreground shadow-sm backdrop-blur disabled:opacity-30 md:hidden"
				aria-label="Next media"
				disabled={!canNext}
				onclick={() => goTo(Math.min(media.length - 1, current + 1))}
			>
				<ChevronRight class="h-4 w-4" />
			</button>
		</div>
		<div class="flex justify-center gap-1.5">
			{#each media as _, index (index)}
				<button
					type="button"
					class="h-1.5 rounded-full transition-all {current === index
						? 'w-4 bg-foreground'
						: 'w-1.5 bg-foreground/30 hover:bg-foreground/50'}"
					aria-label={`Show item ${index + 1} of ${media.length}`}
					aria-current={current === index}
					onclick={() => goTo(index)}
				></button>
			{/each}
		</div>
	</div>
{/if}

{#if others.length > 0}
	<div class="space-y-2">
		{#each others as attachment, index (`other-${index}-${attachment.file_name}`)}
			{#if kindOf(attachment) === 'audio'}
				<audio controls preload="metadata" src={srcOf(attachment)} class="w-full max-w-md"></audio>
			{:else if kindOf(attachment) === 'pdf'}
				<iframe
					src={srcOf(attachment)}
					title={attachment.file_name}
					class="h-96 w-full max-w-2xl rounded-xl border"
				></iframe>
			{:else}
				<a
					href={srcOf(attachment, true)}
					download={attachment.file_name}
					class="inline-flex max-w-full items-center gap-2 rounded-xl border px-3 py-2 text-xs hover:bg-muted"
				>
					<FileIcon class="h-4 w-4 shrink-0" />
					<span class="truncate">{attachment.file_name}</span>
					{#if sizeLabel(attachment.size)}
						<span class="shrink-0 text-muted-foreground">{sizeLabel(attachment.size)}</span>
					{/if}
					<ArrowDownToLine class="h-3.5 w-3.5 shrink-0" />
				</a>
			{/if}
		{/each}
	</div>
{/if}

<ImageLightbox bind:open={lightboxOpen} bind:index={lightboxIndex} items={lightboxItems} />
