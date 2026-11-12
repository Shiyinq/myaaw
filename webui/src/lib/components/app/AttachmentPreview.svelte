<script lang="ts">
	import {
		ArrowDownToLine,
		Download,
		File as FileIcon,
		FileText,
		Image as ImageIcon,
		MessageSquare,
		Music,
		Video,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import ImageLightbox from '$lib/components/app/ImageLightbox.svelte';
	import { files as filesApi } from '$lib/apis/files';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ConversationAttachment, LightboxImage } from '$lib/types';

	interface Props {
		agent: string;
		item: ConversationAttachment;
		onClose: () => void;
		/** Closes the pane and scrolls the transcript to the carrying turn. */
		onShowInChat: (item: ConversationAttachment) => void;
	}

	let { agent, item, onClose, onShowInChat }: Props = $props();

	const { t } = useTranslation();

	let root = $state<HTMLElement | null>(null);
	/** Space kept free at the right so the actions never slide under the top bar. */
	let gutter = $state(8);
	let lightboxOpen = $state(false);
	let imageFailed = $state(false);

	const src = $derived(filesApi.rawUrl(agent, item.path));
	const downloadSrc = $derived(filesApi.rawUrl(agent, item.path, { download: true }));
	const lightboxItems = $derived<LightboxImage[]>([{ src, name: item.file_name, downloadSrc }]);

	const kindLabel = $derived.by(() => {
		switch (item.kind) {
			case 'image':
				return t('myaaw.chat.attachments_kind_image');
			case 'video':
				return t('myaaw.chat.attachments_kind_video');
			case 'audio':
				return t('myaaw.chat.attachments_kind_audio');
			case 'pdf':
				return t('myaaw.chat.attachments_kind_pdf');
			default:
				return t('myaaw.chat.attachments_kind_file');
		}
	});

	const icon = $derived(
		item.kind === 'image'
			? ImageIcon
			: item.kind === 'video'
				? Video
				: item.kind === 'audio'
					? Music
					: item.kind === 'pdf'
						? FileText
						: FileIcon
	);

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

	/** One signature per file; media elements cannot send the session token. */
	$effect(() => {
		const path = item.path;
		if (!path) return;
		imageFailed = false;
		void ensureSigned(agent, [path]);
	});

	/**
	 * The chat shell floats its actions over the top-right of the column. Keep the
	 * header controls clear of them at every width, like the context editor does.
	 */
	function measureGutter() {
		if (!root) return;
		const bar = document.querySelector('header');
		const columnRight = root.getBoundingClientRect().right;
		if (!bar) {
			gutter = 8;
			return;
		}
		const edges = [...bar.querySelectorAll('button')]
			.map((button) => button.getBoundingClientRect())
			.filter((rect) => rect.width > 0 && rect.left > window.innerWidth / 2)
			.map((rect) => rect.left);
		const actionLeft = edges.length > 0 ? Math.min(...edges) : window.innerWidth;
		gutter = Math.max(8, Math.round(columnRight - actionLeft + 8));
	}

	$effect(() => {
		const element = root;
		if (!element) return;
		measureGutter();
		const observer = new ResizeObserver(() => measureGutter());
		observer.observe(element);
		window.addEventListener('resize', measureGutter);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measureGutter);
		};
	});

	function retryImage() {
		if (imageFailed) {
			toastStore.error(t('myaaw.chat.attachments_missing'));
			return;
		}
		imageFailed = true;
		void ensureSigned(agent, [item.path]);
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || event.defaultPrevented) return;
		if (lightboxOpen) return;
		event.preventDefault();
		onClose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div
	class="flex h-full min-h-0 flex-col"
	role="dialog"
	aria-label={item.file_name}
	bind:this={root}
>
	<div class="flex shrink-0 items-center gap-2 border-b p-2" style:padding-right="{gutter}px">
		{#if item.kind === 'image'}
			<img
				{src}
				alt=""
				draggable="false"
				class="h-8 w-8 shrink-0 rounded-md border object-cover"
				onerror={() => void retryImage()}
			/>
		{:else}
			{@const Icon = icon}
			<span
				class="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground"
			>
				<Icon class="h-4 w-4" />
			</span>
		{/if}
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium">{item.file_name}</p>
			<p class="flex items-center gap-1.5 truncate text-[11px] text-muted-foreground">
				<span>{kindLabel}</span>
				{#if sizeLabel(item.size)}
					<span>·</span>
					<span>{sizeLabel(item.size)}</span>
				{/if}
				<span>·</span>
				<span>
					{item.origin === 'agent'
						? t('myaaw.chat.attachments_from_agent')
						: t('myaaw.chat.attachments_from_you')}
				</span>
			</p>
		</div>

		<Button
			variant="ghost"
			size="icon-sm"
			onclick={() => onShowInChat(item)}
			title={t('myaaw.chat.attachments_show_in_chat')}
			aria-label={t('myaaw.chat.attachments_show_in_chat')}
		>
			<MessageSquare class="h-4 w-4" />
		</Button>
		<Button
			variant="outline"
			size="icon-sm"
			onclick={() => void filesApi.download(agent, item.path)}
			title={t('myaaw.chat.download_file')}
			aria-label={t('myaaw.chat.download_file')}
		>
			<Download class="h-4 w-4" />
		</Button>
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={onClose}
			aria-label={t('myaaw.chat.attachments_close')}
			title={t('myaaw.chat.attachments_close')}
		>
			<X class="h-4 w-4" />
		</Button>
	</div>

	<div class="min-h-0 flex-1 overflow-auto bg-muted/20">
		{#if item.kind === 'image'}
			<div class="flex h-full min-h-48 items-center justify-center p-4">
				<button
					type="button"
					class="block max-h-full cursor-zoom-in focus-visible:outline-2 focus-visible:outline-offset-2"
					aria-label={t('myaaw.chat.lightbox_open')}
					onclick={() => (lightboxOpen = true)}
				>
					<img
						{src}
						alt={item.file_name}
						draggable="false"
						class="max-h-[calc(100vh-9rem)] max-w-full rounded-xl border object-contain shadow-sm"
						onerror={() => void retryImage()}
					/>
				</button>
			</div>
		{:else if item.kind === 'video'}
			<div class="flex h-full min-h-48 items-center justify-center p-4">
				<!-- Agent-produced clips have no caption track to attach. -->
				<!-- svelte-ignore a11y_media_has_caption -->
				<video
					controls
					preload="metadata"
					{src}
					class="max-h-[calc(100vh-9rem)] max-w-full rounded-xl border bg-black"
				></video>
			</div>
		{:else if item.kind === 'audio'}
			<div class="flex h-full min-h-48 items-center justify-center p-6">
				<audio controls preload="metadata" {src} class="w-full max-w-md"></audio>
			</div>
		{:else if item.kind === 'pdf'}
			<iframe {src} title={item.file_name} class="h-full min-h-96 w-full"></iframe>
		{:else}
			{@const Icon = icon}
			<div class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
				<span class="grid h-16 w-16 place-items-center rounded-2xl border bg-card">
					<Icon class="h-7 w-7 text-muted-foreground" />
				</span>
				<div>
					<p class="max-w-full truncate text-sm font-medium">{item.file_name}</p>
					<p class="mt-0.5 text-xs text-muted-foreground">
						{kindLabel}{#if sizeLabel(item.size)}
							· {sizeLabel(item.size)}{/if}
					</p>
				</div>
				<Button
					variant="outline"
					size="sm"
					onclick={() => void filesApi.download(agent, item.path)}
				>
					<ArrowDownToLine class="mr-1.5 h-4 w-4" />
					{t('myaaw.chat.download_file')}
				</Button>
			</div>
		{/if}
	</div>
</div>

<ImageLightbox bind:open={lightboxOpen} items={lightboxItems} index={0} />
