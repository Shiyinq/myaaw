<script lang="ts">
	import { Search, Loader2, Pencil, Trash2, X, Check, Plus } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { agentSelection } from '$lib/stores';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { conversationHistory } from '$lib/stores/conversationHistory.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ConversationSummary } from '$lib/types';

	interface Props {
		/** The conversation open in the page, highlighted in the list. */
		activeId?: string;
		onOpen: (id: string) => void;
		onNew?: () => void;
		/** Renders a close button in the header (small screens of the chat panel). */
		onClose?: () => void;
		/** The list itself is not allowed to navigate, so the owner handles the URL. */
		onDeleted?: (id: string) => void;
		onRenamed?: (id: string, title: string) => void;
		searchInput?: HTMLInputElement | null;
	}

	let {
		activeId = '',
		onOpen,
		onNew,
		onClose,
		onDeleted,
		onRenamed,
		searchInput = $bindable(null)
	}: Props = $props();

	const { t } = useTranslation();

	let editingId = $state('');
	let editingTitle = $state('');

	// Conversations whose run is still streaming in the background. The open one
	// is skipped: its own timeline already shows the progress.
	let streaming = $derived(
		new Set(
			chatRuns
				.all()
				.filter(
					(run) =>
						run.streaming && run.agent === agentSelection.selected && run.conversationId !== ''
				)
				.map((run) => run.conversationId)
		)
	);

	function formatWhen(value?: string) {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function startRename(item: ConversationSummary) {
		editingId = item.id;
		editingTitle = item.title;
	}

	async function saveRename() {
		if (!editingId) return;
		const title = editingTitle.trim();
		const id = editingId;
		if (!title) return;
		if (await conversationHistory.rename(id, title)) {
			editingId = '';
			onRenamed?.(id, title);
		}
	}

	async function remove(item: ConversationSummary) {
		if (
			!(await askConfirm({
				message: t('myaaw.conversations.delete_confirm'),
				variant: 'destructive'
			}))
		)
			return;
		if (await conversationHistory.remove(item.id)) onDeleted?.(item.id);
	}

	/** Fetches the next page when the end of the list comes into view. */
	function infinite(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) conversationHistory.loadMore();
			},
			{ rootMargin: '160px' }
		);
		observer.observe(node);
		return { destroy: () => observer.disconnect() };
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="flex shrink-0 items-center gap-2 border-b p-2">
		<div class="relative min-w-0 flex-1">
			<Search
				class="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:ref={searchInput}
				value={conversationHistory.search}
				oninput={(event) => conversationHistory.setSearch(event.currentTarget.value)}
				placeholder={t('myaaw.conversations.search')}
				class="h-8 pr-7 pl-7 text-xs"
				aria-label={t('myaaw.conversations.search')}
				aria-keyshortcuts="Control+Shift+S Meta+Shift+S"
			/>
			{#if conversationHistory.search}
				<button
					type="button"
					class="absolute top-1/2 right-1 flex h-5 w-5 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
					onclick={() => {
						conversationHistory.clearSearch();
						searchInput?.focus();
					}}
					title={t('myaaw.conversations.clear_search')}
					aria-label={t('myaaw.conversations.clear_search')}
				>
					<X class="h-3 w-3" />
				</button>
			{/if}
		</div>
		{#if onClose}
			<Button
				variant="ghost"
				size="icon-sm"
				class="shrink-0 lg:hidden"
				onclick={onClose}
				aria-label={t('myaaw.chat.history_hide')}
				title={t('myaaw.chat.history_hide')}
			>
				<X class="h-4 w-4" />
			</Button>
		{/if}
	</div>

	{#if onNew}
		<button
			type="button"
			class="mx-2 mt-2 flex shrink-0 items-center gap-2 rounded-md border border-dashed px-2 py-1.5 text-xs text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
			onclick={onNew}
		>
			<Plus class="h-3.5 w-3.5" />
			{t('myaaw.chat.new')}
		</button>
	{/if}

	<div class="min-h-0 flex-1 overflow-y-auto p-2">
		{#if conversationHistory.loading && conversationHistory.items.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
		{:else if conversationHistory.items.length === 0}
			<p class="px-2 py-3 text-xs text-muted-foreground">
				{conversationHistory.query
					? t('myaaw.chat.history_no_results')
					: t('myaaw.chat.history_empty')}
			</p>
		{/if}

		{#each conversationHistory.items as entry (entry.id)}
			<div
				class={[
					'group/entry rounded-md transition-colors list-row',
					entry.id === activeId ? 'bg-accent' : 'hover:bg-muted'
				]}
				aria-current={entry.id === activeId ? 'true' : undefined}
			>
				{#if editingId === entry.id}
					<div class="flex items-center gap-1 p-1">
						<Input
							bind:value={editingTitle}
							class="h-7 text-xs"
							onkeydown={(event) => {
								if (event.key === 'Enter') void saveRename();
								if (event.key === 'Escape') editingId = '';
							}}
						/>
						<Button
							size="icon-xs"
							variant="secondary"
							onclick={saveRename}
							aria-label={t('common.save')}
						>
							<Check class="h-3 w-3" />
						</Button>
						<Button
							size="icon-xs"
							variant="ghost"
							onclick={() => (editingId = '')}
							aria-label={t('common.cancel')}
						>
							<X class="h-3 w-3" />
						</Button>
					</div>
				{:else}
					<div class="flex items-start gap-1 p-1">
						<button
							class="min-w-0 flex-1 rounded px-1 py-0.5 text-left"
							onclick={() => onOpen(entry.id)}
						>
							<span class="flex items-center gap-1.5">
								{#if streaming.has(entry.id) && entry.id !== activeId}
									<!-- A run left in the background keeps going; flag it until it is
									     opened, where the live stream itself shows the progress. -->
									<Loader2
										class="h-3 w-3 shrink-0 animate-spin text-muted-foreground"
										aria-label={t('myaaw.chat.streaming')}
										data-streaming="true"
									/>
								{/if}
								<span class="min-w-0 truncate text-xs font-medium">
									{entry.title || entry.id.slice(0, 12)}
								</span>
							</span>
							<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
								{formatWhen(entry.updated_at)} · {t('myaaw.conversations.messages', {
									count: entry.message_count
								})}
							</span>
						</button>
						<div
							class="pointer-events-none flex shrink-0 opacity-0 transition-opacity group-hover/entry:pointer-events-auto group-hover/entry:opacity-100 group-focus-within/entry:pointer-events-auto group-focus-within/entry:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
						>
							<Button
								variant="ghost"
								size="icon-xs"
								onclick={() => startRename(entry)}
								aria-label={t('myaaw.chat.rename')}
							>
								<Pencil class="h-3 w-3" />
							</Button>
							<Button
								variant="ghost"
								size="icon-xs"
								onclick={() => remove(entry)}
								aria-label={t('common.delete')}
							>
								<Trash2 class="h-3 w-3 text-destructive" />
							</Button>
						</div>
					</div>
				{/if}
			</div>
		{/each}

		{#if conversationHistory.hasMore}
			<div use:infinite class="flex justify-center py-3">
				{#if conversationHistory.loading}
					<Loader2 class="h-4 w-4 animate-spin text-muted-foreground" />
				{/if}
			</div>
		{/if}
	</div>
</div>
