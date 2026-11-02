<script lang="ts">
	import {
		Check,
		ChevronDown,
		ChevronRight,
		FolderInput,
		FolderPlus,
		Loader2,
		Pencil,
		Pin,
		PinOff,
		Plus,
		Search,
		Trash2,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { browser } from '$app/environment';
	import { untrack } from 'svelte';
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
		/** Starts a conversation already inside the folder. */
		onNewInFolder?: (folderId: string) => void;
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
		onNewInFolder,
		onClose,
		onDeleted,
		onRenamed,
		searchInput = $bindable(null)
	}: Props = $props();

	const { t } = useTranslation();
	const COLLAPSED_KEY = 'myaaw_chat_folders_collapsed';
	const SECTIONS_KEY = 'myaaw_chat_sections_collapsed';

	let editingId = $state('');
	let editingTitle = $state('');
	let editingFolderId = $state('');
	let editingFolderTitle = $state('');
	let creatingFolder = $state(false);
	let newFolderName = $state('');
	/** Whether the search box owns the header row (simple mode). */
	let searchFocused = $state(false);
	/** The inline rename fields, so each one takes focus the moment it appears. */
	let renameInput = $state<HTMLInputElement | null>(null);
	let folderRenameInput = $state<HTMLInputElement | null>(null);
	let newFolderInput = $state<HTMLInputElement | null>(null);
	let draggingId = $state('');
	/** Folder id, or `recents`, currently under a dragged chat. */
	let dragOver = $state('');
	let collapsedFolders = $state<Record<string, boolean>>(loadCollapsed());
	/** Pinned, the folder list and Recents keep the same header, with a toggle. */
	let collapsedSections = $state<Record<string, boolean>>(loadSectionsCollapsed());

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

	const searching = $derived(conversationHistory.query !== '');

	// A folder that was left expanded loads its first page on mount; collapsed
	// folders cost only their header.
	$effect(() => {
		for (const folder of conversationHistory.folders) {
			if (!isCollapsed(folder.id)) untrack(() => conversationHistory.openFolder(folder.id));
		}
	});

	// An inline editor is useless until it has the caret: focus it and select the
	// old name so typing replaces it.
	$effect(() => {
		if (editingId) focusAndSelect(renameInput);
	});
	$effect(() => {
		if (editingFolderId) focusAndSelect(folderRenameInput);
	});
	$effect(() => {
		if (creatingFolder) focusAndSelect(newFolderInput);
	});

	function focusAndSelect(node: HTMLInputElement | null) {
		if (!node) return;
		node.focus();
		node.select();
	}

	// In simple mode the new-chat button shares the header row with the search
	// box. Focusing the search (or its clear button) widens it to the whole row
	// and the button steps aside until the search loses focus again.
	function onSearchFocusIn() {
		searchFocused = true;
	}

	function onSearchFocusOut(event: FocusEvent) {
		const wrapper = event.currentTarget as HTMLElement;
		const next = event.relatedTarget as Node | null;
		if (!next || !wrapper.contains(next)) searchFocused = false;
	}

	function loadCollapsed(): Record<string, boolean> {
		if (!browser) return {};
		try {
			const raw = localStorage.getItem(COLLAPSED_KEY);
			return raw ? (JSON.parse(raw) as Record<string, boolean>) : {};
		} catch {
			return {};
		}
	}

	function saveCollapsed(next: Record<string, boolean>) {
		collapsedFolders = next;
		if (browser) localStorage.setItem(COLLAPSED_KEY, JSON.stringify(next));
	}

	function isCollapsed(id: string) {
		// Folders start folded: a collapsed folder costs only its header.
		return collapsedFolders[id] !== false;
	}

	function toggleFolder(id: string) {
		const next = { ...collapsedFolders, [id]: !isCollapsed(id) };
		saveCollapsed(next);
		if (!next[id]) conversationHistory.openFolder(id);
	}

	function loadSectionsCollapsed(): Record<string, boolean> {
		if (!browser) return {};
		try {
			const raw = localStorage.getItem(SECTIONS_KEY);
			return raw ? (JSON.parse(raw) as Record<string, boolean>) : {};
		} catch {
			return {};
		}
	}

	function isSectionCollapsed(id: 'pinned' | 'folders' | 'recents') {
		return collapsedSections[id] === true;
	}

	function toggleSection(id: 'pinned' | 'folders' | 'recents') {
		const next = { ...collapsedSections, [id]: !isSectionCollapsed(id) };
		collapsedSections = next;
		// A half-typed folder is not worth keeping when its section disappears.
		if (id === 'folders' && next[id]) creatingFolder = false;
		if (browser) localStorage.setItem(SECTIONS_KEY, JSON.stringify(next));
	}

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

	function startFolderCreate() {
		creatingFolder = true;
		newFolderName = '';
	}

	async function saveNewFolder() {
		const name = newFolderName.trim();
		if (!name) return;
		const folder = await conversationHistory.createFolder(name);
		if (!folder) return;
		creatingFolder = false;
		newFolderName = '';
		// A new folder is opened straight away so the reader can drop chats in.
		saveCollapsed({ ...collapsedFolders, [folder.id]: false });
		conversationHistory.openFolder(folder.id);
	}

	function startFolderRename(id: string, name: string) {
		editingFolderId = id;
		editingFolderTitle = name;
	}

	async function saveFolderRename() {
		const name = editingFolderTitle.trim();
		if (!editingFolderId || !name) return;
		if (await conversationHistory.renameFolder(editingFolderId, name)) editingFolderId = '';
	}

	async function deleteFolder(id: string, name: string, count: number) {
		const confirmed = await askConfirm({
			message: t('myaaw.chat.delete_folder_confirm', { name, count }),
			variant: 'destructive'
		});
		if (!confirmed) return;
		const result = await conversationHistory.deleteFolder(id);
		if (!result) return;
		if (activeId && result.deleted_ids?.includes(activeId)) onDeleted?.(activeId);
	}

	function findConversation(id: string): ConversationSummary | undefined {
		const sections = [
			conversationHistory.pinned,
			conversationHistory.recents,
			conversationHistory.results,
			...Object.values(conversationHistory.folders).map((folder) =>
				conversationHistory.sectionFor(folder.id)
			)
		];
		for (const section of sections) {
			const found = section.items.find((item) => item.id === id);
			if (found) return found;
		}
		return undefined;
	}

	function startDrag(event: DragEvent, id: string) {
		// Buttons own their own interactions; starting a drag on one would break
		// pin/rename/delete on touchpads with a nervous finger.
		const target = event.target as HTMLElement | null;
		if (target?.closest('[data-row-action], input, [role="menuitem"]')) {
			event.preventDefault();
			return;
		}
		draggingId = id;
		dragOver = '';
		event.dataTransfer?.setData('text/plain', id);
		if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
	}

	function endDrag() {
		draggingId = '';
		dragOver = '';
	}

	function dragOverTarget(event: DragEvent, target: string) {
		if (!draggingId) return;
		event.preventDefault();
		if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
		dragOver = target;
	}

	function dragLeave(target: string) {
		if (dragOver === target) dragOver = '';
	}

	function dropOn(event: DragEvent, folderId: string | null) {
		event.preventDefault();
		const id = event.dataTransfer?.getData('text/plain') || draggingId;
		draggingId = '';
		dragOver = '';
		if (!id) return;
		const current = findConversation(id);
		if ((current?.folder_id ?? null) === folderId) return;
		void conversationHistory.move(id, folderId);
	}

	/**
	 * True while the Recents sentinel is on screen. Recents is the only section
	 * that keeps plain infinite scroll, so a single flag is enough.
	 */
	let recentsSentinelVisible = $state(false);

	// Load the next page while the end of Recents is visible. The sentinel itself
	// stays mounted and only its spinner changes, so the list never jumps while a
	// page arrives.
	$effect(() => {
		if (!recentsSentinelVisible || isSectionCollapsed('recents')) return;
		const section = conversationHistory.recents;
		if (!section.hasMore || section.loading || section.refreshing) return;
		untrack(() => conversationHistory.loadMoreRecents());
	});

	/** Loads the next page when a section's sentinel scrolls into view. */
	function sentinel(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				recentsSentinelVisible = entries.some((entry) => entry.isIntersecting);
			},
			{ rootMargin: '160px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
				// A collapsed or unmounted list must not keep paging in the background.
				recentsSentinelVisible = false;
			}
		};
	}

	/**
	 * A native tooltip only when the label is actually clipped, so short folder
	 * names do not repeat themselves on hover.
	 */
	function truncationTitle(node: HTMLElement, text: string) {
		let value = text;
		const apply = () => {
			node.title = node.scrollWidth > node.clientWidth ? value : '';
		};
		apply();
		const observer = new ResizeObserver(apply);
		observer.observe(node);
		return {
			update(next: string) {
				value = next;
				apply();
			},
			destroy() {
				observer.disconnect();
			}
		};
	}

	function showMore(kind: 'pinned' | 'recents' | 'results' | string) {
		if (kind === 'pinned') conversationHistory.loadMorePinned();
		else if (kind === 'recents') conversationHistory.loadMoreRecents();
		else if (kind === 'results') conversationHistory.loadMoreResults();
		else conversationHistory.loadMoreFolder(kind);
	}

	function sectionMore(kind: 'pinned' | 'recents' | 'results' | string) {
		if (kind === 'pinned') return conversationHistory.pinned.hasMore;
		if (kind === 'recents') return conversationHistory.recents.hasMore;
		if (kind === 'results') return conversationHistory.results.hasMore;
		return conversationHistory.sectionFor(kind).hasMore;
	}

	function sectionLoading(kind: 'pinned' | 'recents' | 'results' | string) {
		if (kind === 'pinned') return conversationHistory.pinned.loading;
		if (kind === 'recents') return conversationHistory.recents.loading;
		if (kind === 'results') return conversationHistory.results.loading;
		return conversationHistory.sectionFor(kind).loading;
	}
</script>

{#snippet showMoreButton(kind: 'pinned' | 'recents' | 'results' | string)}
	{#if sectionMore(kind)}
		<div class="flex justify-center py-1">
			<Button
				variant="ghost"
				size="sm"
				class="h-7 gap-1.5 px-2 text-[11px] font-normal text-muted-foreground"
				disabled={sectionLoading(kind)}
				onclick={() => showMore(kind)}
			>
				{#if sectionLoading(kind)}
					<Loader2 class="h-3 w-3 animate-spin" />
				{/if}
				{t('myaaw.chat.show_more')}
			</Button>
		</div>
	{/if}
{/snippet}

{#snippet infiniteSentinel()}
	{#if conversationHistory.recents.hasMore}
		<div use:sentinel class="flex h-6 items-center justify-center">
			{#if conversationHistory.recents.loading}
				<Loader2 class="h-3.5 w-3.5 animate-spin text-muted-foreground" />
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet conversationRow(entry: ConversationSummary, folderId: string | null)}
	<div
		role="group"
		aria-label={entry.title || entry.id}
		data-conversation-id={entry.id}
		class={[
			'group/entry rounded-md transition-colors list-row',
			entry.id === activeId ? 'bg-accent' : 'hover:bg-muted',
			draggingId === entry.id ? 'opacity-50' : ''
		]}
		aria-current={entry.id === activeId ? 'true' : undefined}
		draggable="true"
		ondragstart={(event) => startDrag(event, entry.id)}
		ondragend={endDrag}
	>
		{#if editingId === entry.id}
			<div class="flex items-center gap-1 p-1">
				<Input
					bind:value={editingTitle}
					bind:ref={renameInput}
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
			<div class="relative flex items-start gap-1 p-1">
				<button
					class="min-w-0 flex-1 rounded px-1 py-0.5 text-left"
					onclick={() => onOpen(entry.id)}
				>
					<span class="flex min-w-0 flex-1 items-center gap-1.5">
						{#if streaming.has(entry.id) && entry.id !== activeId}
							<!-- A run left in the background keeps going; flag it until it is
							     opened, where the live stream itself shows the progress. -->
							<Loader2
								class="h-3 w-3 shrink-0 animate-spin text-muted-foreground"
								aria-label={t('myaaw.chat.streaming')}
								data-streaming="true"
							/>
						{/if}
						{#if entry.pinned}
							<Pin class="h-3 w-3 shrink-0 text-muted-foreground" aria-hidden="true" />
						{/if}
						<span class="min-w-0 flex-1 truncate text-xs font-medium">
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
					data-row-action
					class="absolute top-1 right-1 flex items-center gap-0.5 rounded-md bg-muted pl-0.5 opacity-0 transition-opacity pointer-events-none group-hover/entry:opacity-100 group-hover/entry:pointer-events-auto group-focus-within/entry:opacity-100 group-focus-within/entry:pointer-events-auto [&:has([aria-expanded=true])]:opacity-100 [&:has([aria-expanded=true])]:pointer-events-auto [@media(hover:none)]:static [@media(hover:none)]:translate-y-0 [@media(hover:none)]:gap-0 [@media(hover:none)]:bg-transparent [@media(hover:none)]:pl-0 [@media(hover:none)]:opacity-100 [@media(hover:none)]:pointer-events-auto"
				>
					<Button
						variant="ghost"
						size="icon-xs"
						onclick={() => void conversationHistory.pin(entry.id, !entry.pinned)}
						aria-label={entry.pinned ? t('myaaw.chat.unpin') : t('myaaw.chat.pin')}
						title={entry.pinned ? t('myaaw.chat.unpin') : t('myaaw.chat.pin')}
					>
						{#if entry.pinned}
							<PinOff class="h-3 w-3" />
						{:else}
							<Pin class="h-3 w-3" />
						{/if}
					</Button>
					{#if conversationHistory.folders.length > 0 || folderId}
						<DropdownMenu.Root>
							<DropdownMenu.Trigger>
								{#snippet child({ props })}
									<Button
										{...props}
										variant="ghost"
										size="icon-xs"
										aria-label={t('myaaw.chat.move_to_folder')}
										title={t('myaaw.chat.move_to_folder')}
									>
										<FolderInput class="h-3 w-3" />
									</Button>
								{/snippet}
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end" class="w-44">
								{#each conversationHistory.folders as folder (folder.id)}
									<DropdownMenu.Item
										disabled={folderId === folder.id}
										onclick={() => void conversationHistory.move(entry.id, folder.id)}
									>
										<span class="truncate">{folder.name}</span>
									</DropdownMenu.Item>
								{/each}
								{#if folderId !== null}
									{#if conversationHistory.folders.length > 0}
										<DropdownMenu.Separator />
									{/if}
									<DropdownMenu.Item onclick={() => void conversationHistory.move(entry.id, null)}>
										{t('myaaw.chat.no_folder')}
									</DropdownMenu.Item>
								{/if}
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					{/if}
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
{/snippet}

<div class="flex h-full min-h-0 flex-col">
	<div class="flex shrink-0 items-center gap-2 border-b p-2">
		<div class="relative min-w-0 flex-1" onfocusin={onSearchFocusIn} onfocusout={onSearchFocusOut}>
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
		{#if onNew && !searching && !searchFocused}
			<button
				type="button"
				class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-dashed text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
				onclick={onNew}
				aria-label={t('myaaw.chat.new')}
				title={t('myaaw.chat.new')}
			>
				<Plus class="h-4 w-4" />
			</button>
		{/if}
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

	<div class="min-h-0 flex-1 overflow-y-auto p-2">
		{#if searching}
			{#if conversationHistory.results.loading && conversationHistory.results.items.length === 0}
				<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
			{:else if conversationHistory.results.items.length === 0}
				<p class="px-2 py-3 text-xs text-muted-foreground">
					{t('myaaw.chat.history_no_results')}
				</p>
			{/if}
			{#each conversationHistory.results.items as entry (entry.id)}
				{@render conversationRow(entry, entry.folder_id ?? null)}
			{/each}
			{@render showMoreButton('results')}
		{:else}
			{#if conversationHistory.pinned.items.length > 0}
				<button
					type="button"
					class="mb-1 flex w-full items-center gap-1 px-1 text-left text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
					onclick={() => toggleSection('pinned')}
					aria-expanded={!isSectionCollapsed('pinned')}
				>
					{#if isSectionCollapsed('pinned')}
						<ChevronRight class="h-3 w-3 shrink-0" />
					{:else}
						<ChevronDown class="h-3 w-3 shrink-0" />
					{/if}
					<Pin class="h-3 w-3" />
					{t('myaaw.chat.pinned')}
				</button>
				{#if !isSectionCollapsed('pinned')}
					{#each conversationHistory.pinned.items as entry (entry.id)}
						{@render conversationRow(entry, entry.folder_id ?? null)}
					{/each}
					{@render showMoreButton('pinned')}
				{/if}
			{/if}

			{#if conversationHistory.folders.length > 0}
				<button
					type="button"
					class="mt-2 mb-1 flex w-full items-center gap-1 px-1 text-left text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
					onclick={() => toggleSection('folders')}
					aria-expanded={!isSectionCollapsed('folders')}
				>
					{#if isSectionCollapsed('folders')}
						<ChevronRight class="h-3 w-3 shrink-0" />
					{:else}
						<ChevronDown class="h-3 w-3 shrink-0" />
					{/if}
					{t('myaaw.chat.folders')}
				</button>
			{/if}

			{#if conversationHistory.folders.length > 0 && !isSectionCollapsed('folders')}
				<div class="space-y-0.5">
					{#each conversationHistory.folders as folder (folder.id)}
						<div
							role="group"
							aria-label={folder.name}
							class={[
								'rounded-md transition-colors',
								dragOver === folder.id ? 'bg-accent ring-1 ring-primary/40' : ''
							]}
							ondragover={(event) => dragOverTarget(event, folder.id)}
							ondragleave={() => dragLeave(folder.id)}
							ondrop={(event) => dropOn(event, folder.id)}
						>
							{#if editingFolderId === folder.id}
								<div class="flex items-center gap-1 p-1">
									<Input
										bind:value={editingFolderTitle}
										bind:ref={folderRenameInput}
										placeholder={t('myaaw.chat.folder_name')}
										class="h-7 text-xs"
										onkeydown={(event) => {
											if (event.key === 'Enter') void saveFolderRename();
											if (event.key === 'Escape') editingFolderId = '';
										}}
									/>
									<Button
										size="icon-xs"
										variant="secondary"
										onclick={saveFolderRename}
										aria-label={t('common.save')}
									>
										<Check class="h-3 w-3" />
									</Button>
									<Button
										size="icon-xs"
										variant="ghost"
										onclick={() => (editingFolderId = '')}
										aria-label={t('common.cancel')}
									>
										<X class="h-3 w-3" />
									</Button>
								</div>
							{:else}
								<div class="group/folder relative flex items-center rounded-md hover:bg-muted">
									<button
										type="button"
										class="flex min-w-0 flex-1 items-center gap-1.5 rounded px-2 py-1 text-left text-xs font-medium"
										onclick={() => toggleFolder(folder.id)}
										aria-expanded={!isCollapsed(folder.id)}
									>
										{#if isCollapsed(folder.id)}
											<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										{:else}
											<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										{/if}
										<!-- The name gets the whole row; the count yields to the actions. -->
										<span use:truncationTitle={folder.name} class="min-w-0 flex-1 truncate">
											{folder.name}
										</span>
										<Badge
											variant="outline"
											class="shrink-0 transition-opacity group-hover/folder:opacity-0"
										>
											{folder.count}
										</Badge>
									</button>
									<!-- Overlaid on hover so a long name is not squeezed by buttons; a
									     touch screen has no hover, so there the buttons stay visible.
									     The trigger stays mounted (opacity, not display) so its dropdown
									     never closes because the row lost hover/focus. -->
									<div
										data-row-action
										class="absolute top-1/2 right-1 flex -translate-y-1/2 items-center rounded-md bg-muted pl-1 opacity-0 transition-opacity pointer-events-none group-hover/folder:opacity-100 group-hover/folder:pointer-events-auto group-focus-within/folder:opacity-100 group-focus-within/folder:pointer-events-auto [&:has([aria-expanded=true])]:opacity-100 [&:has([aria-expanded=true])]:pointer-events-auto [@media(hover:none)]:static [@media(hover:none)]:translate-y-0 [@media(hover:none)]:bg-transparent [@media(hover:none)]:pl-0 [@media(hover:none)]:opacity-100 [@media(hover:none)]:pointer-events-auto"
									>
										{#if onNewInFolder}
											<Button
												variant="ghost"
												size="icon-xs"
												onclick={() => onNewInFolder(folder.id)}
												aria-label={t('myaaw.chat.add_chat_to_folder')}
												title={t('myaaw.chat.add_chat_to_folder')}
											>
												<Plus class="h-3 w-3" />
											</Button>
										{/if}
										<DropdownMenu.Root>
											<DropdownMenu.Trigger>
												{#snippet child({ props })}
													<Button
														{...props}
														variant="ghost"
														size="icon-xs"
														aria-label={t('myaaw.chat.folder_actions')}
														title={t('myaaw.chat.folder_actions')}
													>
														<span class="text-sm leading-none">⋯</span>
													</Button>
												{/snippet}
											</DropdownMenu.Trigger>
											<DropdownMenu.Content align="end" class="w-44">
												<DropdownMenu.Item
													onclick={() => startFolderRename(folder.id, folder.name)}
												>
													{t('myaaw.chat.rename_folder')}
												</DropdownMenu.Item>
												<DropdownMenu.Item
													variant="destructive"
													onclick={() => deleteFolder(folder.id, folder.name, folder.count)}
												>
													{t('myaaw.chat.delete_folder')}
												</DropdownMenu.Item>
											</DropdownMenu.Content>
										</DropdownMenu.Root>
									</div>
								</div>
							{/if}

							{#if !isCollapsed(folder.id)}
								<div class="ml-4 border-l pl-1">
									{#if conversationHistory.sectionFor(folder.id).loading && conversationHistory.sectionFor(folder.id).items.length === 0}
										<p class="px-2 py-2 text-[11px] text-muted-foreground">
											{t('common.loading')}
										</p>
									{:else if conversationHistory.sectionFor(folder.id).items.length === 0}
										<p class="px-2 py-2 text-[11px] text-muted-foreground">
											{t('myaaw.chat.folder_empty')}
										</p>
									{/if}
									{#each conversationHistory.sectionFor(folder.id).items as entry (entry.id)}
										{@render conversationRow(entry, folder.id)}
									{/each}
									{@render showMoreButton(folder.id)}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if conversationHistory.folders.length === 0 || !isSectionCollapsed('folders')}
				{#if creatingFolder}
					<div class="mt-1 flex items-center gap-1 p-1">
						<Input
							bind:value={newFolderName}
							bind:ref={newFolderInput}
							placeholder={t('myaaw.chat.folder_name')}
							class="h-7 text-xs"
							onkeydown={(event) => {
								if (event.key === 'Enter') void saveNewFolder();
								if (event.key === 'Escape') creatingFolder = false;
							}}
						/>
						<Button
							size="icon-xs"
							variant="secondary"
							onclick={saveNewFolder}
							aria-label={t('common.save')}
						>
							<Check class="h-3 w-3" />
						</Button>
						<Button
							size="icon-xs"
							variant="ghost"
							onclick={() => (creatingFolder = false)}
							aria-label={t('common.cancel')}
						>
							<X class="h-3 w-3" />
						</Button>
					</div>
				{:else}
					<button
						type="button"
						class="mx-1 mt-2 flex items-center gap-2 rounded px-2 py-1.5 text-left text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
						onclick={startFolderCreate}
					>
						<FolderPlus class="h-3.5 w-3.5" />
						{t('myaaw.chat.new_folder')}
					</button>
				{/if}
			{/if}

			<div
				role="group"
				aria-label={t('myaaw.chat.recents')}
				class={[
					'mt-3 rounded-md transition-colors',
					dragOver === 'recents' ? 'bg-accent ring-1 ring-primary/40' : ''
				]}
				ondragover={(event) => dragOverTarget(event, 'recents')}
				ondragleave={() => dragLeave('recents')}
				ondrop={(event) => dropOn(event, null)}
			>
				<div class="mb-1 flex items-center justify-between gap-1 px-1">
					<button
						type="button"
						class="flex items-center gap-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
						onclick={() => toggleSection('recents')}
						aria-expanded={!isSectionCollapsed('recents')}
					>
						{#if isSectionCollapsed('recents')}
							<ChevronRight class="h-3 w-3 shrink-0" />
						{:else}
							<ChevronDown class="h-3 w-3 shrink-0" />
						{/if}
						{t('myaaw.chat.recents')}
					</button>
					{#if dragOver === 'recents'}
						<span class="text-[10px] text-muted-foreground">{t('myaaw.chat.no_folder')}</span>
					{/if}
				</div>
				{#if !isSectionCollapsed('recents')}
					{#if conversationHistory.recents.loading && conversationHistory.recents.items.length === 0}
						<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
					{:else if conversationHistory.recents.items.length === 0}
						<p class="px-2 py-3 text-xs text-muted-foreground">{t('myaaw.chat.history_empty')}</p>
					{/if}
					{#each conversationHistory.recents.items as entry (entry.id)}
						{@render conversationRow(entry, null)}
					{/each}
					{@render infiniteSentinel()}
				{/if}
			</div>
		{/if}
	</div>
</div>
