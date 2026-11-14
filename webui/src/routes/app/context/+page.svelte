<script lang="ts">
	import { context as contextApi } from '$lib/apis/context';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { revealPane } from '$lib/utils/scroll';
	import { formatChatTimestamp } from '$lib/utils/time';
	import { matchesShortcut } from '$lib/utils/keys';
	import { createSearchNav } from '$lib/utils/searchNav.svelte';
	import { formatShortDate, formatSize } from '$lib/utils/format';
	import { fileIcon } from '$lib/utils/fileIcon';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import SidebarHeader from '$lib/components/app/SidebarHeader.svelte';
	import SidebarSearchField from '$lib/components/app/SidebarSearchField.svelte';
	import SidebarSection from '$lib/components/app/SidebarSection.svelte';
	import SidebarSkeletons from '$lib/components/app/SidebarSkeletons.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Eye, Folder, Pencil, Save } from 'lucide-svelte';
	import type { ContextFile } from '$lib/types';

	const { t, locale } = useTranslation();

	// Below lg the list and the editor share one scrolling page, so the editor is
	// brought into view whenever a file further up is picked.
	let detail = $state<HTMLElement | null>(null);

	let files = $state<ContextFile[]>([]);
	let path = $state('');
	let content = $state('');
	// The buffer as last written (or opened), so the header can flag unsaved edits.
	let savedContent = $state('');
	let query = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let editing = $state(false);
	let loadKey = '';

	let agent = $derived(agentSelection.selected);
	let selectedFile = $derived(files.find((file) => file.path === path) ?? null);
	let dirty = $derived(!!path && content !== savedContent);
	let wordCount = $derived((content.trim().match(/\S+/g) ?? []).length);

	let filteredFiles = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return files;
		return files.filter(
			(file) => file.name.toLowerCase().includes(needle) || file.path.toLowerCase().includes(needle)
		);
	});

	let grouped = $derived.by(() => {
		const map = new Map<string, ContextFile[]>();
		for (const file of filteredFiles) {
			const list = map.get(file.folder) ?? [];
			list.push(file);
			map.set(file.folder, list);
		}
		return [...map.entries()];
	});

	let navItems = $derived(
		filteredFiles.map((file) => ({
			id: file.path,
			activate: () => {
				if (!agent) return;
				revealPane(detail);
				void open(agent, file.path);
			}
		}))
	);

	const searchNav = createSearchNav({
		items: () => navItems,
		query: () => query,
		clear: () => (query = '')
	});

	const FOLDER_KEYS: Record<string, string> = {
		root: 'myaaw.context.folder_root',
		memory: 'myaaw.context.folder_memory'
	};

	function folderLabel(folder: string): string {
		const key = FOLDER_KEYS[folder];
		return key ? t(key) : folder;
	}

	function modifiedAt(value: string | undefined): string {
		if (!value) return '';
		return formatChatTimestamp(value, locale.value, t('myaaw.chat.yesterday'));
	}

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const result = await contextApi.list(currentAgent);
			files = result.files ?? [];
			const next = files.find((file) => file.path === path) ?? files[0];
			if (next) {
				await open(currentAgent, next.path);
			} else {
				path = '';
				content = '';
				savedContent = '';
				editing = false;
			}
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	async function open(currentAgent: string, nextPath: string) {
		const file = await contextApi.read(currentAgent, nextPath);
		path = file.path;
		content = file.content;
		savedContent = file.content;
		editing = false;
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	async function save() {
		if (!agent || !path) return;
		saving = true;
		try {
			const saved = await contextApi.write(agent, path, content);
			savedContent = saved.content;
			files = files.map((file) =>
				file.path === path ? { ...file, size: saved.size, mod_time: saved.mod_time } : file
			);
			toastStore.success(t('myaaw.context.saved'));
			editing = false;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	// Cmd/Ctrl+S saves the open file; the sidebar filter owns the shared
	// search/arrow/Enter behaviour.
	function onKeydown(event: KeyboardEvent) {
		if (matchesShortcut(event, 'mod+s')) {
			if (!agent || !path) return;
			event.preventDefault();
			if (dirty) void save();
			return;
		}
		searchNav.onKeydown(event);
	}
</script>

<svelte:window onkeydown={onKeydown} />

<TopBarActions id="context-save">
	{#snippet children()}
		<TopBarButton
			label={t('common.save')}
			icon={Save}
			onclick={save}
			loading={saving}
			disabled={!path}
			variant="default"
		/>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col overflow-y-auto lg:flex-row lg:overflow-hidden">
	{#if !agent}
		<div class="w-full p-4 lg:p-6">
			<Card>
				<CardContent class="py-10 text-center text-sm text-muted-foreground">
					{t('myaaw.agent.none')}
				</CardContent>
			</Card>
		</div>
	{:else}
		<!-- Context files: a second sidebar nested inside the main one. -->
		<aside
			class="flex shrink-0 flex-col border-b bg-card frost-aside lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 space-y-2 border-b px-3 py-2.5">
				<SidebarHeader label={t('myaaw.context.files')} count={files.length} />
				<SidebarSearchField
					bind:inputRef={searchNav.input}
					bind:value={query}
					placeholder={t('common.search')}
				/>
			</div>
			<div bind:this={searchNav.list} class="space-y-3 p-2 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
				{#if loading}
					<SidebarSkeletons class="h-8" />
				{:else if files.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.context.empty')}</p>
				{:else if filteredFiles.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.context.no_matches')}</p>
				{/if}
				{#each grouped as [folder, list] (folder)}
					<SidebarSection icon={Folder} label={folderLabel(folder)} count={list.length}>
						{#each list as file (file.path)}
							{@const Icon = fileIcon(file.path)}
							<button
								class={[
									'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors list-row',
									file.path === path
										? 'bg-accent font-medium text-accent-foreground'
										: 'hover:bg-muted',
									searchNav.cursor === file.path ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={file.path}
								aria-current={file.path === path ? 'true' : undefined}
								title={file.path}
								onclick={() => {
									if (!agent) return;
									revealPane(detail);
									void open(agent, file.path);
								}}
							>
								<Icon
									class={[
										'h-3.5 w-3.5 shrink-0',
										file.path === path ? 'text-accent-foreground' : 'text-muted-foreground'
									]}
								/>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm">{file.name}</span>
									<span class="block truncate text-[10px] text-muted-foreground">
										{formatSize(file.size)}{file.mod_time
											? ` · ${formatShortDate(file.mod_time)}`
											: ''}
									</span>
								</span>
							</button>
						{/each}
					</SidebarSection>
				{/each}
			</div>
		</aside>

		<section bind:this={detail} class="flex min-w-0 flex-col p-4 lg:min-h-0 lg:flex-1 lg:p-6">
			{#if error}
				<div
					class="mb-3 shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			{#if !loading && files.length === 0 && !path}
				<EmptyState
					title={t('myaaw.context.empty')}
					description={t('myaaw.context.empty_desc')}
					bloub="curieux"
				/>
			{:else}
				<Card class="mx-auto flex w-full max-w-5xl flex-col max-lg:h-[60vh] lg:min-h-0 lg:flex-1">
					<CardHeader class="flex shrink-0 flex-row items-start justify-between gap-3">
						<div class="min-w-0">
							<div class="flex min-w-0 items-center gap-1.5">
								{#if path}
									{@const ActiveIcon = fileIcon(path)}
									<ActiveIcon class="h-4 w-4 shrink-0 text-muted-foreground" />
								{/if}
								<CardTitle class="truncate text-sm font-medium">
									{path || t('myaaw.context.editor')}
								</CardTitle>
								{#if dirty}
									<span
										class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary"
										role="status"
										aria-label={t('myaaw.context.unsaved')}
										title={t('myaaw.context.unsaved')}
									></span>
								{/if}
							</div>
							{#if selectedFile}
								<p class="mt-0.5 truncate text-[11px] text-muted-foreground">
									{formatSize(selectedFile.size)}
									{#if selectedFile.mod_time}
										· {modifiedAt(selectedFile.mod_time)}
									{/if}
								</p>
							{/if}
						</div>
						<div class="flex shrink-0 items-center gap-1.5">
							{#if path}
								<span class="hidden text-[11px] tabular-nums text-muted-foreground sm:inline">
									{t('myaaw.context.words', { count: wordCount })}
								</span>
								<Button
									variant="ghost"
									size="icon-xs"
									class="shrink-0 text-muted-foreground"
									onclick={() => (editing = !editing)}
									title={editing ? t('myaaw.context.preview') : t('myaaw.context.edit')}
									aria-label={editing ? t('myaaw.context.preview') : t('myaaw.context.edit')}
								>
									{#if editing}
										<Eye class="h-3.5 w-3.5" />
									{:else}
										<Pencil class="h-3.5 w-3.5" />
									{/if}
								</Button>
							{/if}
							<Badge variant="outline">
								{editing ? t('myaaw.context.editing') : t('myaaw.context.markdown')}
							</Badge>
						</div>
					</CardHeader>
					<CardContent class="min-h-0 flex-1">
						{#if loading && !path}
							<div class="space-y-2">
								<Skeleton class="h-4 w-2/3" />
								<Skeleton class="h-4 w-full" />
								<Skeleton class="h-4 w-5/6" />
								<Skeleton class="h-4 w-3/4" />
							</div>
						{:else if path}
							{#if editing}
								<textarea
									bind:value={content}
									class="h-full w-full resize-none rounded-md border border-input bg-transparent p-3 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								></textarea>
							{:else}
								<div
									class="h-full overflow-y-auto rounded-md border border-input bg-muted/20 px-4 py-3"
								>
									<Markdown {content} />
								</div>
							{/if}
						{:else}
							<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
								{t('myaaw.context.select_file')}
							</p>
						{/if}
					</CardContent>
				</Card>
			{/if}
		</section>
	{/if}
</div>
