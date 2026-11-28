<script lang="ts">
	import { untrack } from 'svelte';
	import {
		Download,
		File as FileIcon,
		FileCode,
		FileSpreadsheet,
		FileText,
		FolderOpen,
		Globe,
		Image as ImageIcon,
		Layers,
		LayoutGrid,
		List,
		Music,
		Play,
		RefreshCw,
		Search,
		Trash2,
		Video,
		X
	} from 'lucide-svelte';
	import { agentSelection, filesStore, toastStore } from '$lib/stores';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { detectPreview, type FilePreview } from '$lib/utils/filePreview';
	import { formatShortDate, formatSize } from '$lib/utils/format';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import LightBox from '$lib/components/app/LightBox.svelte';
	import ProjectDrawer from '$lib/components/app/ProjectDrawer.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { Button } from '$lib/components/ui/button';
	import ArtifactGridSkeleton from './ArtifactGridSkeleton.svelte';
	import type { FileEntry, LightboxImage } from '$lib/types';

	const { t } = useTranslation();

	type Category = 'all' | 'images' | 'videos' | 'audio' | 'documents' | 'projects';
	type SourceBucket = 'uploads' | 'downloads' | 'screenshots' | 'projects' | 'mcp' | 'workspace';
	type SourceFilter = 'all' | 'agent' | 'downloads' | 'screenshots' | 'uploads';

	interface ArtifactItem {
		id: string;
		name: string;
		path: string;
		size: number;
		modTime: string;
		timestamp: number;
		bucket: SourceBucket;
		origin: 'agent' | 'upload';
		category: Exclude<Category, 'all'>;
		isProject: boolean;
		preview: FilePreview | null;
	}

	const IGNORED_DIRS = new Set([
		'node_modules',
		'.git',
		'.next',
		'.svelte-kit',
		'.venv',
		'__pycache__',
		'dist',
		'build',
		'target',
		'.cache'
	]);
	const IGNORED_FILES = new Set(['.ds_store', 'thumbs.db', 'desktop.ini']);
	const MAX_WALK_DEPTH = 3;
	const PAGE_SIZE = 24;

	let agent = $derived(agentSelection.selected);
	let items = $state<ArtifactItem[]>([]);
	let loading = $state(false);
	let refreshing = $state(false);
	let error = $state('');
	let category = $state<Category>('all');
	let sourceFilter = $state<SourceFilter>('all');
	let query = $state('');
	let viewMode = $state<'grid' | 'list'>('grid');
	let visibleLimit = $state(PAGE_SIZE);

	// Preview states: projects open ProjectDrawer; everything else (images, videos,
	// audio, and documents) opens in the unified full-screen lightbox.
	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let activeProject = $state<ArtifactItem | null>(null);
	let failedThumbs = $state<Record<string, boolean>>({});
	let generation = 0;

	function isHiddenOrIgnoredFile(name: string): boolean {
		if (!name || name.startsWith('.')) return true;
		return IGNORED_FILES.has(name.toLowerCase());
	}

	function isIgnoredDir(name: string): boolean {
		if (!name || name.startsWith('.')) return true;
		return IGNORED_DIRS.has(name.toLowerCase());
	}

	async function safeList(targetAgent: string, path: string): Promise<FileEntry[]> {
		try {
			const res = await filesStore.listDir(targetAgent, path);
			return res.entries ?? [];
		} catch (err) {
			const status = (err as { status?: number }).status;
			if (status === 404) return [];
			throw err;
		}
	}

	function toCategory(preview: FilePreview): Exclude<Category, 'all' | 'projects'> {
		if (preview.kind === 'image') return 'images';
		if (preview.kind === 'video') return 'videos';
		if (preview.kind === 'audio') return 'audio';
		return 'documents';
	}

	function makeFileItem(
		entry: FileEntry,
		bucket: SourceBucket,
		origin: 'agent' | 'upload'
	): ArtifactItem {
		const preview = detectPreview(entry.path);
		const parsed = entry.mod_time ? Date.parse(entry.mod_time) : 0;
		return {
			id: entry.path,
			name: entry.name,
			path: entry.path,
			size: entry.size ?? 0,
			modTime: entry.mod_time ?? '',
			timestamp: Number.isNaN(parsed) ? 0 : parsed,
			bucket,
			origin,
			category: toCategory(preview),
			isProject: false,
			preview
		};
	}

	function makeProjectItem(entry: FileEntry): ArtifactItem {
		const parsed = entry.mod_time ? Date.parse(entry.mod_time) : 0;
		return {
			id: entry.path,
			name: entry.name,
			path: entry.path,
			size: entry.size ?? 0,
			modTime: entry.mod_time ?? '',
			timestamp: Number.isNaN(parsed) ? 0 : parsed,
			bucket: 'projects',
			origin: 'agent',
			category: 'projects',
			isProject: true,
			preview: null
		};
	}

	async function walkFiles(
		targetAgent: string,
		dir: string,
		bucket: SourceBucket,
		origin: 'agent' | 'upload',
		depth: number,
		out: ArtifactItem[]
	): Promise<void> {
		const entries = await safeList(targetAgent, dir);
		const subdirs: FileEntry[] = [];
		for (const entry of entries) {
			if (entry.is_dir) {
				if (depth < MAX_WALK_DEPTH && !isIgnoredDir(entry.name)) {
					subdirs.push(entry);
				}
			} else if (!isHiddenOrIgnoredFile(entry.name)) {
				out.push(makeFileItem(entry, bucket, origin));
			}
		}
		if (subdirs.length > 0) {
			await Promise.all(
				subdirs.map((sub) => walkFiles(targetAgent, sub.path, bucket, origin, depth + 1, out))
			);
		}
	}

	const signedPaths = new Set<string>();
	const retriedThumbs = new Set<string>();

	function handleThumbError(item: ArtifactItem, currentUrl: string) {
		if (!agent || !currentUrl || currentUrl === 'data:,') return;
		const key = `${agent}:${item.path}`;
		if (!retriedThumbs.has(key)) {
			retriedThumbs.add(key);
			signedPaths.delete(key);
			void ensureSigned(agent, [item.path]);
			return;
		}
		failedThumbs = { ...failedThumbs, [item.id]: true };
	}

	function signVisibleSlice(targetAgent: string, list: ArtifactItem[]) {
		const pending = list
			.filter(
				(item) =>
					!item.isProject &&
					item.preview &&
					(item.preview.kind === 'image' || item.preview.kind === 'video') &&
					!signedPaths.has(`${targetAgent}:${item.path}`)
			)
			.map((item) => item.path);
		if (pending.length === 0) return;
		for (const p of pending) signedPaths.add(`${targetAgent}:${p}`);
		void ensureSigned(targetAgent, pending);
	}

	function loadMoreSentinel(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					visibleLimit += PAGE_SIZE;
				}
			},
			{ rootMargin: '400px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}

	async function loadArtifacts(targetAgent: string, isRefresh = false) {
		const current = ++generation;
		if (isRefresh) refreshing = true;
		else loading = true;
		error = '';
		try {
			const collected: ArtifactItem[] = [];
			const [uploadRoot, workspaceRoot] = await Promise.all([
				safeList(targetAgent, '/user/uploads'),
				safeList(targetAgent, '/home/workspace')
			]);

			const tasks: Promise<void>[] = [];

			// User uploads: loose files + nested folders under /user/uploads.
			for (const entry of uploadRoot) {
				if (entry.is_dir) {
					if (!isIgnoredDir(entry.name)) {
						tasks.push(walkFiles(targetAgent, entry.path, 'uploads', 'upload', 1, collected));
					}
				} else if (!isHiddenOrIgnoredFile(entry.name)) {
					collected.push(makeFileItem(entry, 'uploads', 'upload'));
				}
			}

			// Agent workspace: loose files directly in /home/workspace plus known
			// and custom subfolders (projects stay grouped as project cards).
			for (const entry of workspaceRoot) {
				if (!entry.is_dir) {
					if (!isHiddenOrIgnoredFile(entry.name)) {
						collected.push(makeFileItem(entry, 'workspace', 'agent'));
					}
					continue;
				}
				if (isIgnoredDir(entry.name)) continue;

				const lower = entry.name.toLowerCase();
				if (lower === 'projects') {
					tasks.push(
						safeList(targetAgent, entry.path).then((projectEntries) => {
							for (const child of projectEntries) {
								if (child.is_dir) {
									if (!isIgnoredDir(child.name)) collected.push(makeProjectItem(child));
								} else if (!isHiddenOrIgnoredFile(child.name)) {
									collected.push(makeFileItem(child, 'projects', 'agent'));
								}
							}
						})
					);
				} else if (lower === 'downloads') {
					tasks.push(walkFiles(targetAgent, entry.path, 'downloads', 'agent', 1, collected));
				} else if (lower === 'screenshots') {
					tasks.push(walkFiles(targetAgent, entry.path, 'screenshots', 'agent', 1, collected));
				} else if (lower === 'mcp') {
					tasks.push(walkFiles(targetAgent, entry.path, 'mcp', 'agent', 1, collected));
				} else {
					tasks.push(walkFiles(targetAgent, entry.path, 'workspace', 'agent', 1, collected));
				}
			}

			await Promise.all(tasks);
			if (current !== generation) return;

			collected.sort((a, b) => {
				if (b.timestamp !== a.timestamp) return b.timestamp - a.timestamp;
				return a.name.localeCompare(b.name);
			});

			items = collected;
			failedThumbs = {};
			visibleLimit = PAGE_SIZE;
		} catch (err) {
			if (current !== generation) return;
			error = getErrorMessage(err);
		} finally {
			if (current === generation) {
				loading = false;
				refreshing = false;
			}
		}
	}

	let loadedAgent = '';
	$effect(() => {
		const current = agent;
		if (!current || current === loadedAgent) return;
		loadedAgent = current;
		category = 'all';
		sourceFilter = 'all';
		query = '';
		lightboxOpen = false;
		activeProject = null;
		untrack(() => void loadArtifacts(current));
	});

	function matchesSource(item: ArtifactItem, filter: SourceFilter): boolean {
		if (filter === 'all') return true;
		if (filter === 'uploads') return item.origin === 'upload';
		if (filter === 'downloads') return item.bucket === 'downloads';
		if (filter === 'screenshots') return item.bucket === 'screenshots';
		return item.origin === 'agent' && item.bucket !== 'downloads';
	}

	const searchTerm = $derived(query.trim().toLowerCase());

	const sourceFiltered = $derived(
		items.filter((item) => {
			if (!matchesSource(item, sourceFilter)) return false;
			if (!searchTerm) return true;
			return (
				item.name.toLowerCase().includes(searchTerm) ||
				(item.preview?.label.toLowerCase().includes(searchTerm) ?? false) ||
				bucketLabel(item.bucket).toLowerCase().includes(searchTerm)
			);
		})
	);

	const visibleItems = $derived(
		category === 'all'
			? sourceFiltered
			: sourceFiltered.filter((item) => item.category === category)
	);

	// Reset pagination when filters or search change so the gallery stays crisp.
	$effect(() => {
		void category;
		void sourceFilter;
		void searchTerm;
		visibleLimit = PAGE_SIZE;
	});

	const renderedItems = $derived(visibleItems.slice(0, visibleLimit));
	const hasMore = $derived(renderedItems.length < visibleItems.length);

	// Sign only the thumbnails currently rendered in the viewport slice.
	$effect(() => {
		const currentAgent = agent;
		const slice = renderedItems;
		if (!currentAgent || slice.length === 0) return;
		untrack(() => signVisibleSlice(currentAgent, slice));
	});

	const categoryCounts = $derived.by(() => {
		const counts: Record<Category, number> = {
			all: sourceFiltered.length,
			images: 0,
			videos: 0,
			audio: 0,
			documents: 0,
			projects: 0
		};
		for (const item of sourceFiltered) {
			counts[item.category] += 1;
		}
		return counts;
	});

	const availableSources = $derived.by<SourceFilter[]>(() => {
		const list: SourceFilter[] = ['all'];
		if (items.some((item) => matchesSource(item, 'agent'))) list.push('agent');
		if (items.some((item) => item.bucket === 'downloads')) list.push('downloads');
		if (items.some((item) => item.bucket === 'screenshots')) list.push('screenshots');
		if (items.some((item) => item.origin === 'upload')) list.push('uploads');
		return list;
	});

	const previewableItems = $derived(visibleItems.filter((item) => !item.isProject));
	const lightboxItems = $derived<LightboxImage[]>(
		lightboxOpen && agent
			? previewableItems.map((item) => {
					const kind = item.preview?.kind ?? 'binary';
					const needsRawUrl =
						kind === 'image' || kind === 'video' || kind === 'audio' || kind === 'pdf';
					return {
						src: needsRawUrl ? filesStore.rawUrl(agent, item.path) : '',
						name: item.name,
						downloadSrc: needsRawUrl
							? filesStore.rawUrl(agent, item.path, { download: true })
							: undefined,
						kind,
						language: item.preview?.language,
						label: item.preview?.label,
						agent,
						path: item.path,
						size: item.size,
						meta: bucketLabel(item.bucket)
					};
				})
			: []
	);

	// Keep the active lightbox slide and its immediate neighbours signed when media.
	$effect(() => {
		if (!lightboxOpen || !agent) return;
		const currentAgent = agent;
		const slice = [
			previewableItems[lightboxIndex - 1],
			previewableItems[lightboxIndex],
			previewableItems[lightboxIndex + 1]
		].filter((entry): entry is ArtifactItem => Boolean(entry));
		if (slice.length > 0) {
			untrack(() => signVisibleSlice(currentAgent, slice));
		}
	});

	function categoryLabel(id: Category): string {
		switch (id) {
			case 'images':
				return t('myaaw.artifacts.cat_images');
			case 'videos':
				return t('myaaw.artifacts.cat_videos');
			case 'audio':
				return t('myaaw.artifacts.cat_audio');
			case 'documents':
				return t('myaaw.artifacts.cat_documents');
			case 'projects':
				return t('myaaw.artifacts.cat_projects');
			default:
				return t('myaaw.artifacts.cat_all');
		}
	}

	function sourceFilterLabel(id: SourceFilter): string {
		switch (id) {
			case 'agent':
				return t('myaaw.artifacts.source_agent');
			case 'downloads':
				return t('myaaw.artifacts.source_downloads');
			case 'screenshots':
				return t('myaaw.artifacts.source_screenshots');
			case 'uploads':
				return t('myaaw.artifacts.source_uploads');
			default:
				return t('myaaw.artifacts.source_all');
		}
	}

	function bucketLabel(bucket: SourceBucket): string {
		switch (bucket) {
			case 'uploads':
				return t('myaaw.artifacts.badge_upload');
			case 'downloads':
				return t('myaaw.artifacts.badge_download');
			case 'screenshots':
				return t('myaaw.artifacts.badge_screenshot');
			case 'projects':
				return t('myaaw.artifacts.badge_project');
			case 'mcp':
				return t('myaaw.artifacts.badge_mcp');
			default:
				return t('myaaw.artifacts.badge_agent');
		}
	}

	function docIconFor(item: ArtifactItem) {
		if (item.isProject) return FolderOpen;
		switch (item.preview?.kind) {
			case 'image':
				return ImageIcon;
			case 'video':
				return Video;
			case 'audio':
				return Music;
			case 'csv':
				return FileSpreadsheet;
			case 'html':
				return Globe;
			case 'code':
				return FileCode;
			case 'markdown':
			case 'pdf':
			case 'text':
				return FileText;
			default:
				return FileIcon;
		}
	}

	function thumbTone(item: ArtifactItem): string {
		if (item.isProject) return 'from-primary/12 via-primary/5 to-transparent text-primary';
		switch (item.preview?.kind) {
			case 'pdf':
				return 'from-rose-500/12 via-rose-500/4 to-transparent text-rose-600 dark:text-rose-400';
			case 'markdown':
				return 'from-sky-500/12 via-sky-500/4 to-transparent text-sky-600 dark:text-sky-400';
			case 'csv':
				return 'from-emerald-500/12 via-emerald-500/4 to-transparent text-emerald-600 dark:text-emerald-400';
			case 'html':
				return 'from-amber-500/12 via-amber-500/4 to-transparent text-amber-600 dark:text-amber-400';
			case 'code':
				return 'from-violet-500/12 via-violet-500/4 to-transparent text-violet-600 dark:text-violet-400';
			case 'audio':
				return 'from-fuchsia-500/12 via-fuchsia-500/4 to-transparent text-fuchsia-600 dark:text-fuchsia-400';
			default:
				return 'from-muted-foreground/10 via-muted/40 to-transparent text-muted-foreground';
		}
	}

	async function openArtifact(item: ArtifactItem) {
		if (!agent) return;
		if (item.isProject) {
			activeProject = item;
			return;
		}
		const kind = item.preview?.kind;
		if (kind === 'image' || kind === 'video' || kind === 'audio' || kind === 'pdf') {
			void ensureSigned(agent, [item.path]);
		}
		const idx = previewableItems.findIndex((entry) => entry.path === item.path);
		lightboxIndex = idx >= 0 ? idx : 0;
		lightboxOpen = true;
	}

	async function removeArtifact(item: ArtifactItem, event?: MouseEvent) {
		event?.stopPropagation();
		if (!agent) return;
		const confirmed = await askConfirm({
			message: t('myaaw.artifacts.delete_confirm', { name: item.name }),
			variant: 'destructive'
		});
		if (!confirmed) return;
		try {
			await filesStore.remove(agent, item.path, undefined, item.isProject);
			items = items.filter((entry) => entry.path !== item.path);
			if (lightboxOpen && previewableItems.length === 0) {
				lightboxOpen = false;
			}
			if (activeProject?.path === item.path) {
				activeProject = null;
			}
			toastStore.success(t('myaaw.artifacts.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function removeFromLightbox(entry: LightboxImage) {
		if (!entry.path) return;
		const target = items.find((item) => item.path === entry.path);
		if (target) void removeArtifact(target);
	}

	function downloadArtifact(item: ArtifactItem, event?: MouseEvent) {
		event?.stopPropagation();
		if (!agent || item.isProject) return;
		void filesStore.download(agent, item.path);
	}

	function refresh() {
		if (!agent) return;
		signedPaths.clear();
		retriedThumbs.clear();
		void loadArtifacts(agent, true);
	}

	const CATEGORIES: Category[] = ['all', 'images', 'videos', 'audio', 'documents', 'projects'];
</script>

<TopBarActions id="artifacts-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={refresh}
			loading={loading || refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto flex w-full max-w-7xl flex-col gap-5">
	{#if !agent}
		<div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
			<Bloub mood="confus" size={56} tight color={agentSelection.color} />
			<p class="text-sm text-muted-foreground">{t('myaaw.agent.none')}</p>
		</div>
	{:else}
		<!-- Controls: category pills, friendly source pills, search, and view toggle -->
		<div class="flex flex-col gap-3">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<div
					role="tablist"
					aria-label={t('myaaw.artifacts.categories')}
					class="flex min-w-0 flex-wrap items-center gap-1.5"
				>
					{#each CATEGORIES as cat (cat)}
						{@const count = categoryCounts[cat]}
						{#if cat === 'all' || count > 0 || category === cat}
							<button
								type="button"
								role="tab"
								aria-selected={category === cat}
								onclick={() => (category = cat)}
								class={[
									'inline-flex items-center gap-1.5 rounded-full px-3.5 py-1.5 text-xs font-medium transition-colors',
									category === cat
										? 'bg-primary text-primary-foreground shadow-xs'
										: 'bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground'
								]}
							>
								<span>{categoryLabel(cat)}</span>
								{#if count > 0}
									<span
										class={[
											'text-[10px] tabular-nums',
											category === cat ? 'opacity-80' : 'opacity-65'
										]}
									>
										{count}
									</span>
								{/if}
							</button>
						{/if}
					{/each}
				</div>

				<div class="flex items-center gap-2 max-sm:w-full">
					<div class="relative min-w-0 flex-1 sm:w-56">
						<Search
							class="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
						/>
						<input
							type="search"
							bind:value={query}
							placeholder={t('myaaw.artifacts.search')}
							aria-label={t('myaaw.artifacts.search')}
							class="h-8.5 w-full rounded-full bg-muted/60 pr-8 pl-8.5 text-xs text-foreground placeholder:text-muted-foreground outline-none transition-colors focus:bg-muted focus:ring-1 focus:ring-ring/40"
						/>
						{#if query}
							<button
								type="button"
								onclick={() => (query = '')}
								aria-label={t('myaaw.conversations.clear_search')}
								class="absolute top-1/2 right-2 grid h-5 w-5 -translate-y-1/2 place-items-center rounded-full text-muted-foreground hover:bg-background/80 hover:text-foreground"
							>
								<X class="h-3 w-3" />
							</button>
						{/if}
					</div>

					<div
						class="inline-flex shrink-0 items-center rounded-full bg-muted/60 p-0.5"
						role="group"
						aria-label={t('myaaw.artifacts.view_mode')}
					>
						<button
							type="button"
							onclick={() => (viewMode = 'grid')}
							aria-pressed={viewMode === 'grid'}
							title={t('myaaw.artifacts.view_grid')}
							aria-label={t('myaaw.artifacts.view_grid')}
							class={[
								'grid h-7.5 w-7.5 place-items-center rounded-full transition-colors',
								viewMode === 'grid'
									? 'bg-background text-foreground shadow-xs'
									: 'text-muted-foreground hover:text-foreground'
							]}
						>
							<LayoutGrid class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							onclick={() => (viewMode = 'list')}
							aria-pressed={viewMode === 'list'}
							title={t('myaaw.artifacts.view_list')}
							aria-label={t('myaaw.artifacts.view_list')}
							class={[
								'grid h-7.5 w-7.5 place-items-center rounded-full transition-colors',
								viewMode === 'list'
									? 'bg-background text-foreground shadow-xs'
									: 'text-muted-foreground hover:text-foreground'
							]}
						>
							<List class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>
			</div>

			{#if availableSources.length > 2}
				<div class="flex flex-wrap items-center gap-1.5">
					{#each availableSources as src (src)}
						<button
							type="button"
							aria-pressed={sourceFilter === src}
							onclick={() => (sourceFilter = src)}
							class={[
								'rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors',
								sourceFilter === src
									? 'bg-accent text-accent-foreground'
									: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
							]}
						>
							{sourceFilterLabel(src)}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		{#if error}
			<div
				class="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		{#if loading && items.length === 0}
			<ArtifactGridSkeleton />
		{:else if items.length === 0}
			<EmptyState
				title={t('myaaw.artifacts.empty')}
				description={t('myaaw.artifacts.empty_desc')}
				bloub="curieux"
				icon={Layers}
				class="min-h-80"
			/>
		{:else if visibleItems.length === 0}
			<EmptyState
				title={t('myaaw.artifacts.no_matches')}
				description={t('myaaw.artifacts.no_matches_desc')}
				bloub="confus"
				icon={Search}
				class="min-h-64"
			/>
		{:else if viewMode === 'grid'}
			<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
				{#each renderedItems as item (item.id)}
					{@const Icon = docIconFor(item)}
					<div
						data-slot="card"
						class="group relative flex flex-col overflow-hidden rounded-2xl bg-card ring-1 ring-(--surface-ring) [contain-intrinsic-size:220px_240px] [content-visibility:auto] transition-shadow duration-150 hover:shadow-md"
					>
						<button
							type="button"
							onclick={() => void openArtifact(item)}
							class="flex min-w-0 flex-1 flex-col text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
						>
							<div class="relative aspect-4/3 w-full overflow-hidden bg-muted/40">
								{#if item.category === 'images' && !failedThumbs[item.id]}
									{@const url = filesStore.rawUrl(agent, item.path)}
									{#if url !== 'data:,'}
										<img
											src={url}
											alt={item.name}
											loading="lazy"
											decoding="async"
											fetchpriority="low"
											draggable="false"
											class="h-full w-full object-cover"
											onerror={() => handleThumbError(item, url)}
										/>
									{:else}
										<div class="h-full w-full bg-muted/50"></div>
									{/if}
								{:else if item.category === 'videos'}
									{@const url = filesStore.rawUrl(agent, item.path)}
									<div class="relative flex h-full w-full items-center justify-center bg-black/85">
										{#if url !== 'data:,'}
											<video
												src={url}
												muted
												playsinline
												preload="metadata"
												class="h-full w-full object-cover opacity-80"
											></video>
										{/if}
										<span
											class="absolute grid h-11 w-11 place-items-center rounded-full bg-black/65 text-white"
										>
											<Play class="ml-0.5 h-5 w-5 fill-current" />
										</span>
									</div>
								{:else}
									<div
										class={[
											'flex h-full w-full flex-col items-center justify-center gap-2.5 bg-linear-to-br p-4',
											thumbTone(item)
										]}
									>
										<span
											class="grid h-12 w-12 place-items-center rounded-2xl bg-background/90 shadow-xs ring-1 ring-border/60"
										>
											<Icon class="h-6 w-6" />
										</span>
										<span
											class="rounded-full bg-background/90 px-2.5 py-0.5 text-[10px] font-semibold tracking-wide text-foreground/80 ring-1 ring-border/50"
										>
											{item.isProject
												? t('myaaw.artifacts.live_preview')
												: (item.preview?.label ?? 'File')}
										</span>
									</div>
								{/if}
							</div>

							<div class="flex min-w-0 flex-col gap-1 p-3">
								<p class="truncate text-sm font-medium text-foreground" title={item.name}>
									{item.name}
								</p>
								<div
									class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-muted-foreground"
								>
									<span class="rounded-full bg-muted/70 px-1.5 py-px text-[10px] font-medium">
										{bucketLabel(item.bucket)}
									</span>
									{#if item.modTime}
										<span>{formatShortDate(item.modTime)}</span>
									{/if}
									{#if !item.isProject && item.size > 0}
										<span>· {formatSize(item.size)}</span>
									{/if}
								</div>
							</div>
						</button>

						<div
							class="pointer-events-none absolute top-2 right-2 flex items-center gap-1 opacity-0 transition-opacity duration-150 group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100"
						>
							{#if !item.isProject}
								<button
									type="button"
									onclick={(event) => downloadArtifact(item, event)}
									title={t('myaaw.files.download')}
									aria-label={t('myaaw.files.download')}
									class="grid h-7 w-7 place-items-center rounded-full bg-background/95 text-foreground shadow-xs ring-1 ring-border/60 transition-colors hover:bg-background"
								>
									<Download class="h-3.5 w-3.5" />
								</button>
							{/if}
							<button
								type="button"
								onclick={(event) => void removeArtifact(item, event)}
								title={t('common.delete')}
								aria-label={t('common.delete')}
								class="grid h-7 w-7 place-items-center rounded-full bg-background/95 text-muted-foreground shadow-xs ring-1 ring-border/60 transition-colors hover:bg-destructive hover:text-destructive-foreground"
							>
								<Trash2 class="h-3.5 w-3.5" />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if hasMore}
				<div use:loadMoreSentinel class="h-8 w-full" aria-hidden="true"></div>
			{/if}
		{:else}
			<div
				data-slot="card"
				class="divide-y divide-border/60 overflow-hidden rounded-2xl bg-card ring-1 ring-(--surface-ring) frost-card"
			>
				{#each renderedItems as item (item.id)}
					{@const Icon = docIconFor(item)}
					<div
						class="flex items-center gap-3 px-3 py-2.5 [contain-intrinsic-size:auto_64px] [content-visibility:auto] transition-colors hover:bg-muted/40"
					>
						<button
							type="button"
							onclick={() => void openArtifact(item)}
							class="flex min-w-0 flex-1 items-center gap-3 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
						>
							{#if item.category === 'images' && !failedThumbs[item.id]}
								{@const url = filesStore.rawUrl(agent, item.path)}
								{#if url !== 'data:,'}
									<img
										src={url}
										alt=""
										loading="lazy"
										decoding="async"
										fetchpriority="low"
										draggable="false"
										class="h-11 w-11 shrink-0 rounded-xl border object-cover"
										onerror={() => handleThumbError(item, url)}
									/>
								{:else}
									<span class="h-11 w-11 shrink-0 rounded-xl bg-muted/50"></span>
								{/if}
							{:else}
								<span
									class={[
										'grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-linear-to-br ring-1 ring-border/50',
										thumbTone(item)
									]}
								>
									<Icon class="h-5 w-5" />
								</span>
							{/if}

							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-foreground" title={item.name}>
									{item.name}
								</p>
								<p class="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
									<span class="rounded-full bg-muted/80 px-2 py-px text-[10px] font-medium">
										{bucketLabel(item.bucket)}
									</span>
									{#if item.isProject}
										<span>{t('myaaw.artifacts.live_preview')}</span>
									{:else if item.preview}
										<span>{item.preview.label}</span>
									{/if}
									{#if item.modTime}
										<span>· {formatShortDate(item.modTime)}</span>
									{/if}
									{#if !item.isProject && item.size > 0}
										<span>· {formatSize(item.size)}</span>
									{/if}
								</p>
							</div>
						</button>

						<div class="flex shrink-0 items-center gap-1">
							{#if !item.isProject}
								<Button
									variant="ghost"
									size="icon-sm"
									class="rounded-full text-muted-foreground hover:text-foreground"
									onclick={(event) => downloadArtifact(item, event)}
									title={t('myaaw.files.download')}
									aria-label={t('myaaw.files.download')}
								>
									<Download class="h-4 w-4" />
								</Button>
							{/if}
							<Button
								variant="ghost"
								size="icon-sm"
								class="rounded-full text-muted-foreground hover:text-destructive"
								onclick={(event) => void removeArtifact(item, event)}
								title={t('common.delete')}
								aria-label={t('common.delete')}
							>
								<Trash2 class="h-4 w-4" />
							</Button>
						</div>
					</div>
				{/each}
			</div>
			{#if hasMore}
				<div use:loadMoreSentinel class="h-8 w-full" aria-hidden="true"></div>
			{/if}
		{/if}
	{/if}
</div>

<LightBox
	bind:open={lightboxOpen}
	bind:index={lightboxIndex}
	items={lightboxItems}
	onDelete={removeFromLightbox}
/>

{#if agent && activeProject}
	<ProjectDrawer
		{agent}
		path={activeProject.path}
		name={activeProject.name}
		onClose={() => (activeProject = null)}
	/>
{/if}
