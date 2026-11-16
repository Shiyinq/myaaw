<script lang="ts">
	import { untrack } from 'svelte';
	import {
		File as FileIcon,
		FileText,
		Image as ImageIcon,
		Loader2,
		MessageSquare,
		Music,
		Paperclip,
		Video,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import ChatTime from '$lib/components/common/ChatTime.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { filesStore } from '$lib/stores/files.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { cn } from '$lib/utils';
	import ProjectCard from '$lib/components/app/ProjectCard.svelte';
	import type { ConversationAttachment } from '$lib/types';

	interface Props {
		agent: string;
		/** The conversation whose files are listed. Empty hides the list. */
		conversationId: string;
		/** Bumped by the page when a run finishes, so new files appear. */
		refreshKey?: number;
		/** Opens the shared preview pane for one file. */
		onOpen?: (item: ConversationAttachment) => void;
		/** Closes the panel and scrolls the transcript to the carrying turn. */
		onShowInChat?: (item: ConversationAttachment) => void;
		/** Renders the panel's own header with a close button when set. */
		onClose?: () => void;
	}

	let {
		agent,
		conversationId,
		refreshKey = 0,
		onOpen = () => {},
		onShowInChat = () => {},
		onClose
	}: Props = $props();

	const { t } = useTranslation();

	/** Files per page, matching the conversation list. */
	const PAGE = 50;

	type Filter = 'all' | 'projects' | 'images' | 'videos' | 'audio' | 'documents';
	const FILTERS: { id: Filter; kinds: string }[] = [
		{ id: 'all', kinds: '' },
		{ id: 'projects', kinds: 'project' },
		{ id: 'images', kinds: 'image' },
		{ id: 'videos', kinds: 'video' },
		{ id: 'audio', kinds: 'audio' },
		{ id: 'documents', kinds: 'pdf,file' }
	];

	let filter = $state<Filter>('all');
	let items = $state<ConversationAttachment[]>([]);
	let total = $state(0);
	let hasMore = $state(false);
	let loaded = $state(false);
	let loading = $state(false);
	let loadingMore = $state(false);
	let error = $state('');
	let generation = 0;
	let loadKey = '';

	/** The filter currently requested, so the chips can keep their label. */
	const kinds = $derived(FILTERS.find((entry) => entry.id === filter)?.kinds ?? '');

	let scroller = $state<HTMLElement | null>(null);
	let sentinelVisible = $state(false);

	// Reload from the first page whenever the chat, the chip or the refresh key
	// changes. The generation guard drops a response that lost the race.
	$effect(() => {
		const key = `${agent}:${conversationId}:${filter}:${refreshKey}`;
		if (key === loadKey) return;
		loadKey = key;
		untrack(() => void load(true));
	});

	// The end of the list on screen pulls the next page. The sentinel stays
	// mounted, so the list never jumps while it grows.
	$effect(() => {
		if (!sentinelVisible || !loaded || !hasMore || loading || loadingMore) return;
		untrack(() => void load(false));
	});

	function sentinel(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				sentinelVisible = entries.some((entry) => entry.isIntersecting);
			},
			{ root: scroller, rootMargin: '160px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
				sentinelVisible = false;
			}
		};
	}

	async function load(reset: boolean) {
		if (!agent || !conversationId) {
			items = [];
			total = 0;
			hasMore = false;
			loaded = false;
			loading = false;
			loadingMore = false;
			error = '';
			return;
		}
		if (reset) {
			generation += 1;
			loading = true;
			error = '';
		} else {
			if (loading || loadingMore || !hasMore) return;
			loadingMore = true;
		}

		const current = generation;
		const pageOffset = reset ? 0 : items.length;
		try {
			const page = await conversationsStore.fetchAttachments(agent, conversationId, {
				limit: PAGE,
				offset: pageOffset,
				kind: kinds || undefined
			});
			if (current !== generation) return;
			const fresh = page.items ?? [];
			items = reset
				? fresh
				: [...items, ...fresh.filter((entry) => !items.some((known) => known.path === entry.path))];
			total = page.total ?? items.length;
			hasMore = Boolean(page.has_more) && fresh.length > 0;
			loaded = true;
			error = '';
			void ensureSigned(
				agent,
				items.filter((entry) => entry.kind === 'image').map((entry) => entry.path)
			);
		} catch (err) {
			if (current !== generation) return;
			error = getErrorMessage(err);
		} finally {
			if (current === generation) {
				loading = false;
				loadingMore = false;
			}
		}
	}

	/** A broken thumbnail usually means an expired signature; sign once more. */
	const retried = new Set<string>();
	function retryImage(item: ConversationAttachment) {
		if (retried.has(item.path)) return;
		retried.add(item.path);
		void ensureSigned(agent, [item.path]);
	}

	function thumbSrc(item: ConversationAttachment): string {
		return filesStore.rawUrl(agent, item.path);
	}

	function iconFor(kind: string) {
		switch (kind) {
			case 'image':
				return ImageIcon;
			case 'video':
				return Video;
			case 'audio':
				return Music;
			case 'pdf':
				return FileText;
			default:
				return FileIcon;
		}
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

	function filterLabel(id: Filter): string {
		switch (id) {
			case 'projects':
				return t('myaaw.chat.attachments_filter_projects');
			case 'images':
				return t('myaaw.chat.attachments_filter_images');
			case 'videos':
				return t('myaaw.chat.attachments_filter_videos');
			case 'audio':
				return t('myaaw.chat.attachments_filter_audio');
			case 'documents':
				return t('myaaw.chat.attachments_filter_documents');
			default:
				return t('myaaw.chat.attachments_filter_all');
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	{#if onClose}
		<div class="flex shrink-0 items-center gap-2 border-b p-3">
			<Paperclip class="h-4 w-4 shrink-0 text-muted-foreground" />
			<p class="min-w-0 flex-1 truncate text-sm font-semibold">
				{t('myaaw.chat.attachments_title')}
			</p>
			<Button
				variant="ghost"
				size="icon-sm"
				class="lg:hidden"
				onclick={onClose}
				aria-label={t('myaaw.chat.attachments_close_list')}
				title={t('myaaw.chat.attachments_close_list')}
			>
				<X class="h-4 w-4" />
			</Button>
		</div>
	{/if}

	<div
		role="group"
		aria-label={t('myaaw.chat.attachments_filters')}
		class="flex shrink-0 flex-nowrap items-center gap-0.5 overflow-x-auto overscroll-x-contain px-2 pt-2 pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
	>
		{#each FILTERS as entry (entry.id)}
			<button
				type="button"
				aria-pressed={filter === entry.id}
				class={cn(
					'shrink-0 whitespace-nowrap rounded-full border px-1 py-1 text-[11px] font-medium transition-colors',
					filter === entry.id
						? 'border-transparent bg-primary text-primary-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => (filter = entry.id)}
			>
				{filterLabel(entry.id)}
			</button>
		{/each}
	</div>

	<div bind:this={scroller} class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
		{#if loading && items.length === 0}
			<div class="space-y-1.5" aria-hidden="true">
				{#each { length: 6 } as _, index (index)}
					<div class="flex items-center gap-2.5 rounded-lg px-2 py-1.5">
						<Skeleton class="h-10 w-10 shrink-0 rounded-md" />
						<div class="min-w-0 flex-1 space-y-1.5">
							<Skeleton class="h-2.5 w-2/3 rounded-full" />
							<Skeleton class="h-2 w-1/3 rounded-full" />
						</div>
					</div>
				{/each}
			</div>
		{:else if error && items.length === 0}
			<div class="flex flex-col items-center gap-2 px-2 py-6 text-center">
				<p class="text-xs text-destructive">{error}</p>
				<Button variant="outline" size="sm" onclick={() => void load(true)}>
					{t('myaaw.chat.attachments_retry')}
				</Button>
			</div>
		{:else if items.length === 0}
			<div class="flex flex-col items-center gap-2 px-2 py-6 text-center">
				<Paperclip class="h-5 w-5 text-muted-foreground" />
				<p class="text-xs text-muted-foreground">{t('myaaw.chat.attachments_empty')}</p>
			</div>
		{/if}

		{#if items.length > 0}
			<p class="px-2 pb-1 text-[10px] font-medium tracking-widest text-muted-foreground uppercase">
				{t('myaaw.chat.attachments_count', { count: total })}
			</p>
			<ul class="space-y-0.5">
				{#each items as item (item.path)}
					<li class="flex items-center gap-1 rounded-lg hover:bg-muted/60">
						{#if item.kind === 'project'}
							<ProjectCard
								{agent}
								path={item.path}
								name={item.file_name}
								projectType={item.project_type}
								size={item.size}
								variant="row"
							/>
						{:else}
							<button
								type="button"
								class="flex min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2 py-1.5 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
								title={item.path}
								onclick={() => onOpen(item)}
							>
								{#if item.kind === 'image'}
									<img
										src={thumbSrc(item)}
										alt=""
										loading="lazy"
										draggable="false"
										class="h-10 w-10 shrink-0 rounded-md border object-cover"
										onerror={() => retryImage(item)}
									/>
								{:else}
									{@const Icon = iconFor(item.kind)}
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-md border bg-muted/50 text-muted-foreground"
									>
										<Icon class="h-4 w-4" />
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-xs font-medium">{item.file_name}</span>
									<span class="mt-0.5 flex items-center gap-1.5 text-[10px] text-muted-foreground">
										<span
											class={cn(
												'shrink-0 rounded-full border px-1.5 py-px font-medium',
												item.origin === 'agent'
													? 'border-primary/30 bg-primary/10 text-primary'
													: ''
											)}
										>
											{item.origin === 'agent'
												? t('myaaw.chat.attachments_from_agent')
												: t('myaaw.chat.attachments_from_you')}
										</span>
										{#if sizeLabel(item.size)}
											<span class="shrink-0">{sizeLabel(item.size)}</span>
										{/if}
										<ChatTime at={item.created_at ?? ''} class="truncate text-[10px]" />
									</span>
								</span>
							</button>
						{/if}
						<button
							type="button"
							class="mr-1 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
							title={t('myaaw.chat.attachments_show_in_chat')}
							aria-label={t('myaaw.chat.attachments_show_in_chat')}
							onclick={() => onShowInChat(item)}
						>
							<MessageSquare class="h-3.5 w-3.5" />
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		{#if items.length > 0 && hasMore}
			<div use:sentinel class="flex h-8 items-center justify-center">
				{#if loadingMore}
					<Loader2 class="h-3.5 w-3.5 animate-spin text-muted-foreground" />
				{/if}
			</div>
		{/if}

		{#if error && items.length > 0}
			<div class="flex flex-col items-center gap-1.5 px-2 py-2 text-center">
				<p class="text-[11px] text-destructive">{error}</p>
				<Button variant="ghost" size="xs" onclick={() => void load(false)}>
					{t('myaaw.chat.attachments_retry')}
				</Button>
			</div>
		{/if}
	</div>
</div>
