<script lang="ts">
	import { files as filesApi } from '$lib/apis/files';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import {
		ChevronLeft,
		File,
		FilePlus2,
		Folder,
		RefreshCw,
		Save,
		Search,
		Trash2
	} from 'lucide-svelte';
	import type { FileContent, FileEntry, SearchMatch } from '$lib/types';

	const { t } = useTranslation();

	let path = $state('/home');
	let entries = $state<FileEntry[]>([]);
	let selected = $state<FileContent | null>(null);
	let content = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let folderName = $state('');
	let searchQuery = $state('');
	let matches = $state<SearchMatch[]>([]);
	let searchLoading = $state(false);
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

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
		try {
			const file = await filesApi.read(agent, entry.path);
			selected = file;
			content = file.content;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function save() {
		if (!agent || !selected) return;
		saving = true;
		try {
			await filesApi.write(agent, selected.path, content);
			toastStore.success(t('myaaw.files.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function remove(entry: FileEntry) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.files.delete_confirm', { name: entry.name }))) return;
		try {
			await filesApi.remove(agent, entry.path, entry.is_dir);
			if (selected?.path === entry.path) {
				selected = null;
				content = '';
			}
			await load(agent, path);
			toastStore.success(t('myaaw.files.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function createFolder() {
		if (!agent || !folderName.trim()) return;
		const target = path === '' ? `/${folderName.trim()}` : `${path}/${folderName.trim()}`;
		try {
			await filesApi.mkdir(agent, target);
			folderName = '';
			await load(agent, path);
			toastStore.success(t('myaaw.files.folder_created'));
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
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={refresh}
			loading={loading}
		/>
	{/snippet}
</TopBarActions>

<div class="space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
				{error}
			</div>
		{/if}

		<div class="grid gap-4 lg:grid-cols-5">
			<Card class="lg:col-span-2">
				<CardHeader class="space-y-2">
					<div class="flex items-center gap-2">
						<Button variant="ghost" size="icon-sm" onclick={goUp}>
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

					<div class="flex gap-2">
						<Input bind:value={folderName} placeholder={t('myaaw.files.new_folder')} class="h-8" />
						<Button size="sm" variant="outline" onclick={createFolder}>
							<FilePlus2 class="h-4 w-4" />
						</Button>
					</div>
				</CardHeader>

				<CardContent class="space-y-1">
					{#if loading}
						<p class="text-sm text-muted-foreground">{t('common.loading')}</p>
					{:else if entries.length === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.files.empty')}</p>
					{/if}

					{#each entries as entry (entry.path)}
						<div class="group flex items-center gap-2 rounded-md px-1 py-1 hover:bg-muted">
							<button
								class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm"
								onclick={() => open(entry)}
							>
								{#if entry.is_dir}
									<Folder class="h-4 w-4 shrink-0 text-muted-foreground" />
								{:else}
									<File class="h-4 w-4 shrink-0 text-muted-foreground" />
								{/if}
								<span class="truncate">{entry.name}</span>
							</button>
							{#if !entry.is_dir}
								<span class="text-[10px] text-muted-foreground">{formatSize(entry.size)}</span>
							{/if}
							<Button
								variant="ghost"
								size="icon-xs"
								class="opacity-0 group-hover:opacity-100"
								onclick={() => remove(entry)}
							>
								<Trash2 class="h-3.5 w-3.5 text-destructive" />
							</Button>
						</div>
					{/each}

					<Separator class="my-2" />

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
							onclick={() => agent && filesApi.read(agent, match.path).then((file) => {
								selected = file;
								content = file.content;
							})}
						>
							<span class="font-mono">{match.path}:{match.line}</span>
							<span class="text-muted-foreground"> {match.text}</span>
						</button>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-3">
				<CardHeader class="flex flex-row items-center justify-between">
					<CardTitle class="truncate text-sm font-medium">
						{selected ? selected.path : t('myaaw.files.editor')}
					</CardTitle>
					{#if selected}
						<div class="flex items-center gap-2">
							<Badge variant="outline">{formatSize(selected.size)}</Badge>
							<Button size="sm" onclick={save} loading={saving}>
								<Save class="mr-2 h-4 w-4" />
								{t('common.save')}
							</Button>
						</div>
					{/if}
				</CardHeader>
				<CardContent>
					{#if selected}
						<textarea
							bind:value={content}
							class="min-h-[28rem] w-full resize-y rounded-md border border-input bg-transparent p-3 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
						></textarea>
					{:else}
						<p class="py-10 text-center text-sm text-muted-foreground">
							{t('myaaw.files.select_file')}
						</p>
					{/if}
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
