<script lang="ts">
	import { files as filesApi } from '$lib/apis/files';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate } from '$lib/utils/validation';
	import { fileNameSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import { detectPreview } from '$lib/utils/filePreview';
	import { parseCsv } from '$lib/utils/csv';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import {
		ChevronLeft,
		Check,
		Download,
		ExternalLink,
		Eye,
		File,
		FilePlus,
		Folder,
		Pencil,
		RefreshCw,
		Save,
		Search,
		Trash2,
		X
	} from 'lucide-svelte';
	import type { FileContent, FileEntry, SearchMatch } from '$lib/types';

	const { t } = useTranslation();

	const PREVIEW_LIMIT = 512 * 1024;

	let path = $state('/home');
	let entries = $state<FileEntry[]>([]);
	let selected = $state<FileContent | null>(null);
	let content = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let editing = $state(false);
	let folderName = $state('');
	let searchQuery = $state('');
	let matches = $state<SearchMatch[]>([]);
	let searchLoading = $state(false);
	let loadKey = '';
	let renaming = $state('');
	let renameValue = $state('');
	let nameError = $state('');

	let agent = $derived(agentSelection.selected);
	let preview = $derived(selected ? detectPreview(selected.path) : null);

	// <img>, <video>, <audio> and the PDF viewer stream the raw bytes; everything
	// text-based keeps going through the JSON read endpoint.
	const MEDIA_KINDS = new Set(['image', 'video', 'audio', 'pdf']);
	let mediaKind = $derived(preview && MEDIA_KINDS.has(preview.kind) ? preview.kind : null);
	let textPreview = $derived(!!preview && !mediaKind && preview.kind !== 'binary');
	let tooLargeToPreview = $derived(!!selected && textPreview && selected.size > PREVIEW_LIMIT);
	let mediaUrl = $derived(selected && agent ? filesApi.rawUrl(agent, selected.path) : '');
	let csvTable = $derived(preview?.kind === 'csv' && selected ? parseCsv(content) : null);
	let mediaFailed = $state(false);

	function downloadFile(target: string) {
		if (!agent) return;
		filesApi.download(agent, target);
	}

	async function load(currentAgent: string, target: string) {
		loading = true;
		error = '';
		try {
			const result = await filesApi.list(currentAgent, target);
			entries = result.entries ?? [];
			path = result.path ?? target;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent, path);
	});

	async function refresh() {
		if (agent) await load(agent, path);
	}

	async function open(entry: FileEntry) {
		if (!agent) return;
		if (entry.is_dir) {
			selected = null;
			await load(agent, entry.path);
			return;
		}

		mediaFailed = false;
		const kind = detectPreview(entry.path).kind;
		if (MEDIA_KINDS.has(kind)) {
			// Binary media never goes through the text reader.
			selected = { path: entry.path, content: '', size: entry.size, mod_time: entry.mod_time };
			content = '';
			editing = false;
			return;
		}

		try {
			const file = await filesApi.read(agent, entry.path);
			selected = file;
			content = file.content;
			editing = false;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function openMatch(match: SearchMatch) {
		if (!agent) return;
		const file = await filesApi.read(agent, match.path);
		selected = file;
		content = file.content;
		editing = false;
		mediaFailed = false;
	}

	async function save() {
		if (!agent || !selected) return;
		saving = true;
		try {
			await filesApi.write(agent, selected.path, content);
			toastStore.success(t('myaaw.files.saved'));
			editing = false;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function remove(entry: FileEntry) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.files.delete_confirm', { name: entry.name }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await filesApi.remove(agent, entry.path, entry.is_dir);
			if (selected?.path === entry.path) {
				selected = null;
				content = '';
				editing = false;
			}
			await load(agent, path);
			toastStore.success(t('myaaw.files.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function createFolder() {
		if (!agent) return;
		const name = newEntryName();
		if (!name) return;
		const target = path === '' ? `/${name}` : `${path}/${name}`;
		try {
			await filesApi.mkdir(agent, target);
			folderName = '';
			await load(agent, path);
			toastStore.success(t('myaaw.files.folder_created'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function createFile() {
		if (!agent) return;
		const name = newEntryName();
		if (!name) return;
		const target = path === '' ? `/${name}` : `${path}/${name}`;
		try {
			await filesApi.write(agent, target, '');
			folderName = '';
			await load(agent, path);
			const file = await filesApi.read(agent, target);
			selected = file;
			content = file.content;
			editing = true;
			toastStore.success(t('myaaw.files.file_created'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	/** Validates the "new file or folder" input; returns the cleaned name. */
	function newEntryName() {
		const result = validate(fileNameSchema, folderName);
		if (!result.ok) {
			nameError = result.errors._ ?? '';
			return '';
		}
		nameError = '';
		return result.data;
	}

	function startRename(entry: FileEntry) {
		renaming = entry.path;
		renameValue = entry.name;
		nameError = '';
	}

	function cancelRename() {
		renaming = '';
		renameValue = '';
		nameError = '';
	}

	async function submitRename(entry: FileEntry) {
		if (!agent) return;
		const result = validate(fileNameSchema, renameValue);
		if (!result.ok) {
			nameError = result.errors._ ?? '';
			return;
		}
		const parent = entry.path.split('/').slice(0, -1).join('/');
		const target = parent ? `${parent}/${result.data}` : `/${result.data}`;
		try {
			await filesApi.rename(agent, entry.path, target);
			if (selected?.path === entry.path) {
				selected = null;
				content = '';
				editing = false;
			}
			cancelRename();
			await load(agent, path);
			toastStore.success(t('myaaw.files.renamed'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function search() {
		if (!agent || !searchQuery.trim()) return;
		searchLoading = true;
		try {
			const result = await filesApi.search(agent, searchQuery.trim(), path || '/home');
			matches = result.matches ?? [];
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			searchLoading = false;
		}
	}

	function goUp() {
		if (path === '' || path === '/' || !path) {
			void refresh();
			return;
		}
		const parent = path.split('/').filter(Boolean).slice(0, -1).join('/');
		if (agent) void load(agent, parent ? `/${parent}` : '');
	}

	function formatSize(size: number) {
		if (size < 1024) return `${size} B`;
		if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
		return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
	}

	function breadcrumb() {
		return path.split('/').filter(Boolean);
	}
</script>

<TopBarActions id="files-refresh">
	{#snippet children()}
		<TopBarButton label={t('common.refresh')} icon={RefreshCw} onclick={refresh} {loading} />
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col lg:flex-row">
	{#snippet previewFallback()}
		<div class="flex h-full flex-col items-center justify-center gap-3 text-center">
			<p class="text-sm text-muted-foreground">
				{preview?.kind === 'binary'
					? t('myaaw.files.preview_unavailable')
					: t('myaaw.files.preview_failed')}
			</p>
			{#if selected}
				<Button variant="outline" size="sm" onclick={() => downloadFile(selected!.path)}>
					<Download class="mr-2 h-4 w-4" />
					{t('myaaw.files.download')}
				</Button>
			{/if}
		</div>
	{/snippet}
	{#if !agent}
		<div class="w-full p-4 lg:p-6">
			<Card>
				<CardContent class="py-10 text-center text-sm text-muted-foreground">
					{t('myaaw.agent.none')}
				</CardContent>
			</Card>
		</div>
	{:else}
		<!-- File list: a second sidebar nested inside the main one. -->
		<aside
			class="flex h-[45vh] shrink-0 flex-col border-b bg-card lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 space-y-2 border-b p-3">
				<div class="flex items-center gap-1">
					<Button variant="ghost" size="icon-xs" onclick={goUp}>
						<ChevronLeft class="h-4 w-4" />
					</Button>
					<div class="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
						<button class="hover:underline" onclick={() => agent && load(agent, '')}>roots</button>
						{#each breadcrumb() as part, index (index)}
							<span>/</span>
							<span class="truncate font-medium text-foreground">{part}</span>
						{/each}
					</div>
				</div>

				<div class="space-y-1">
					<div class="flex gap-2">
						<Input
							bind:value={folderName}
							placeholder={t('myaaw.files.new_name')}
							class="h-8"
							onkeydown={(event) => {
								if (event.key === 'Enter') void createFolder();
							}}
						/>
						<Button
							size="icon-sm"
							variant="outline"
							onclick={createFolder}
							title={t('myaaw.files.new_folder')}
							aria-label={t('myaaw.files.new_folder')}
						>
							<Folder class="h-4 w-4" />
						</Button>
						<Button
							size="icon-sm"
							variant="outline"
							onclick={createFile}
							title={t('myaaw.files.new_file')}
							aria-label={t('myaaw.files.new_file')}
						>
							<FilePlus class="h-4 w-4" />
						</Button>
					</div>
					<FieldError code={nameError} />
				</div>
			</div>

			<div class="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-2">
				{#if loading}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('common.loading')}</p>
				{:else if entries.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.files.empty')}</p>
				{/if}

				{#each entries as entry (entry.path)}
					{#if renaming === entry.path}
						<div class="flex items-center gap-1 rounded-md px-1 py-1">
							<Input
								bind:value={renameValue}
								class="h-7 text-xs"
								onkeydown={(event) => {
									if (event.key === 'Enter') void submitRename(entry);
									if (event.key === 'Escape') cancelRename();
								}}
							/>
							<Button
								size="icon-xs"
								variant="secondary"
								onclick={() => submitRename(entry)}
								aria-label={t('common.save')}
							>
								<Check class="h-3 w-3" />
							</Button>
							<Button
								size="icon-xs"
								variant="ghost"
								onclick={cancelRename}
								aria-label={t('common.cancel')}
							>
								<X class="h-3 w-3" />
							</Button>
						</div>
					{:else}
						<div class="group relative flex items-center gap-2 rounded-md px-1 py-1 hover:bg-muted">
							<button
								class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm"
								onclick={() => open(entry)}
								title={entry.name}
							>
								{#if entry.is_dir}
									<Folder class="h-4 w-4 shrink-0 text-muted-foreground" />
								{:else}
									<File class="h-4 w-4 shrink-0 text-muted-foreground" />
								{/if}
								<span class="truncate">{entry.name}</span>
							</button>
							{#if !entry.is_dir}
								<span class="shrink-0 text-[10px] text-muted-foreground">
									{formatSize(entry.size)}
								</span>
							{/if}
							<!-- Row actions float over the name on hover so the file name can use
							     the full width of the sidebar instead of reserving space for them. -->
							<div
								class="pointer-events-none absolute inset-y-0 right-0 flex items-center gap-0.5 bg-gradient-to-l from-muted via-muted to-transparent pr-1 pl-6 opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100 max-lg:pointer-events-auto max-lg:opacity-100"
							>
								<Button
									variant="ghost"
									size="icon-xs"
									onclick={() => startRename(entry)}
									aria-label={t('myaaw.files.rename')}
								>
									<Pencil class="h-3.5 w-3.5" />
								</Button>
								{#if !entry.is_dir}
									<Button
										variant="ghost"
										size="icon-xs"
										onclick={() => downloadFile(entry.path)}
										aria-label={t('myaaw.files.download')}
										title={t('myaaw.files.download')}
									>
										<Download class="h-3.5 w-3.5" />
									</Button>
								{/if}
								<Button
									variant="ghost"
									size="icon-xs"
									onclick={() => remove(entry)}
									aria-label={t('common.delete')}
								>
									<Trash2 class="h-3.5 w-3.5 text-destructive" />
								</Button>
							</div>
						</div>
					{/if}
				{/each}
			</div>

			<div class="shrink-0 space-y-1 border-t p-2">
				<div class="flex gap-2">
					<Input
						bind:value={searchQuery}
						placeholder={t('myaaw.files.search')}
						class="h-8"
						onkeydown={(event) => {
							if (event.key === 'Enter') void search();
						}}
					/>
					<Button size="sm" variant="outline" onclick={search} loading={searchLoading}>
						<Search class="h-4 w-4" />
					</Button>
				</div>
				{#each matches as match (match.path + match.line)}
					<button
						class="block w-full truncate rounded px-1 py-0.5 text-left text-xs hover:bg-muted"
						onclick={() => openMatch(match)}
					>
						<span class="font-mono">{match.path}:{match.line}</span>
						<span class="text-muted-foreground"> {match.text}</span>
					</button>
				{/each}
			</div>
		</aside>

		<section class="flex min-h-0 flex-1 flex-col p-4 lg:p-6">
			{#if error}
				<div
					class="mb-3 shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			<Card class="mx-auto flex min-h-0 w-full max-w-5xl flex-1 flex-col">
				<CardHeader class="flex shrink-0 flex-row items-center justify-between">
					<div class="flex min-w-0 items-center gap-1">
						<CardTitle class="truncate text-sm font-medium">
							{selected ? selected.path : t('myaaw.files.editor')}
						</CardTitle>
						{#if selected && preview && preview.editable}
							<Button
								variant="ghost"
								size="icon-xs"
								class="shrink-0 text-muted-foreground"
								onclick={() => (editing = !editing)}
								title={editing ? t('myaaw.files.preview') : t('myaaw.files.edit')}
								aria-label={editing ? t('myaaw.files.preview') : t('myaaw.files.edit')}
							>
								{#if editing}
									<Eye class="h-3.5 w-3.5" />
								{:else}
									<Pencil class="h-3.5 w-3.5" />
								{/if}
							</Button>
						{/if}
						{#if selected}
							<Button
								variant="ghost"
								size="icon-xs"
								class="shrink-0 text-muted-foreground"
								onclick={() => selected && downloadFile(selected.path)}
								title={t('myaaw.files.download')}
								aria-label={t('myaaw.files.download')}
							>
								<Download class="h-3.5 w-3.5" />
							</Button>
						{/if}
						{#if selected && mediaKind}
							<Button
								variant="ghost"
								size="icon-xs"
								class="shrink-0 text-muted-foreground"
								onclick={() => window.open(mediaUrl, '_blank', 'noopener,noreferrer')}
								title={t('myaaw.files.open_new_tab')}
								aria-label={t('myaaw.files.open_new_tab')}
							>
								<ExternalLink class="h-3.5 w-3.5" />
							</Button>
						{/if}
					</div>
					{#if selected}
						<div class="flex items-center gap-2">
							<Badge variant="outline">
								{editing ? t('myaaw.files.editing') : preview?.label}
							</Badge>
							<Badge variant="outline">{formatSize(selected.size)}</Badge>
							{#if editing}
								<Button size="sm" onclick={save} loading={saving}>
									<Save class="mr-2 h-4 w-4" />
									{t('common.save')}
								</Button>
							{/if}
						</div>
					{/if}
				</CardHeader>
				<CardContent class="min-h-0 flex-1">
					{#if selected}
						{#if editing}
							<textarea
								bind:value={content}
								class="h-full w-full resize-none rounded-md border border-input bg-transparent p-3 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						{:else if mediaKind === 'image'}
							{#if mediaFailed}
								{@render previewFallback()}
							{:else}
								<div class="flex h-full items-center justify-center overflow-auto">
									<img
										src={mediaUrl}
										alt={selected.path}
										class="max-h-full max-w-full object-contain"
										onerror={() => (mediaFailed = true)}
									/>
								</div>
							{/if}
						{:else if mediaKind === 'video'}
							{#if mediaFailed}
								{@render previewFallback()}
							{:else}
								<div class="flex h-full items-center justify-center">
									<!-- Agent media has no caption track to attach; sound may be needed. -->
									<!-- svelte-ignore a11y_media_has_caption -->
									<video
										src={mediaUrl}
										controls
										preload="metadata"
										class="max-h-full max-w-full rounded-md"
										onerror={() => (mediaFailed = true)}
									></video>
								</div>
							{/if}
						{:else if mediaKind === 'audio'}
							{#if mediaFailed}
								{@render previewFallback()}
							{:else}
								<div class="flex h-full items-center justify-center">
									<audio
										src={mediaUrl}
										controls
										preload="metadata"
										class="w-full max-w-lg"
										onerror={() => (mediaFailed = true)}
									></audio>
								</div>
							{/if}
						{:else if mediaKind === 'pdf'}
							<iframe
								src={mediaUrl}
								title={selected.path}
								class="h-full w-full rounded-md border bg-background"
							></iframe>
						{:else if preview?.kind === 'binary'}
							{@render previewFallback()}
						{:else if tooLargeToPreview}
							<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
								{t('myaaw.files.too_large')}
							</p>
						{:else if preview?.kind === 'markdown'}
							<div
								class="h-full overflow-y-auto rounded-md border border-input bg-muted/20 px-4 py-3"
							>
								<Markdown {content} />
							</div>
						{:else if preview?.kind === 'html'}
							<iframe
								class="h-full w-full rounded-md border bg-white"
								sandbox=""
								srcdoc={content}
								title={selected.path}
							></iframe>
						{:else if preview?.kind === 'csv' && csvTable}
							<div class="flex h-full min-h-0 flex-col gap-2">
								<div class="min-h-0 flex-1 overflow-auto rounded-md border">
									<table class="w-full border-collapse text-xs">
										<thead class="sticky top-0 bg-muted">
											<tr>
												{#each csvTable.headers as header, index (index)}
													<th class="border-b px-2 py-1.5 text-left font-medium">
														{header}
													</th>
												{/each}
											</tr>
										</thead>
										<tbody>
											{#each csvTable.rows as row, rowIndex (rowIndex)}
												<tr class="border-t">
													{#each row as cell, cellIndex (cellIndex)}
														<td class="px-2 py-1 align-top">{cell}</td>
													{/each}
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
								{#if csvTable.truncated}
									<p class="shrink-0 text-[11px] text-muted-foreground">
										{t('myaaw.files.csv_rows', {
											shown: csvTable.rows.length,
											total: csvTable.totalRows
										})}
									</p>
								{/if}
							</div>
						{:else}
							<CodePreview code={content} language={preview?.language ?? 'plaintext'} />
						{/if}
					{:else}
						<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
							{t('myaaw.files.select_file')}
						</p>
					{/if}
				</CardContent>
			</Card>
		</section>
	{/if}
</div>
