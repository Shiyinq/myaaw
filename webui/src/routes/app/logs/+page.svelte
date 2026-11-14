<script lang="ts">
	import { logs as logsApi } from '$lib/apis/logs';
	import { agentSelection } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { revealPane } from '$lib/utils/scroll';
	import { createSearchNav } from '$lib/utils/searchNav.svelte';
	import { formatShortDate, formatSize } from '$lib/utils/format';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import SidebarHeader from '$lib/components/app/SidebarHeader.svelte';
	import SidebarSearchField from '$lib/components/app/SidebarSearchField.svelte';
	import SidebarSkeletons from '$lib/components/app/SidebarSkeletons.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Play, RefreshCw, ScrollText, Square, Terminal } from 'lucide-svelte';
	import type { LogFile } from '$lib/types';

	const { t } = useTranslation();

	// Below lg the list and the panel share one scrolling page, so the panel is
	// brought into view whenever a row further up is picked.
	let detail = $state<HTMLElement | null>(null);

	let files = $state<LogFile[]>([]);
	let file = $state('');
	let lines = $state<string[]>([]);
	let query = $state('');
	let loading = $state(false);
	let streaming = $state(false);
	let reconnecting = $state(false);
	let error = $state('');
	let loadKey = '';
	let controller: AbortController | null = null;
	let output = $state<HTMLDivElement | null>(null);

	let agent = $derived(agentSelection.selected);
	let filteredFiles = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return files;
		return files.filter((entry) => entry.name.toLowerCase().includes(needle));
	});
	let currentFile = $derived(files.find((entry) => entry.name === file) ?? null);
	let navItems = $derived(
		filteredFiles.map((entry) => ({
			id: entry.name,
			activate: () => pick(entry.name)
		}))
	);

	const searchNav = createSearchNav({
		items: () => navItems,
		query: () => query,
		clear: () => (query = '')
	});

	function scrollDown() {
		if (output) output.scrollTop = output.scrollHeight;
	}

	$effect(() => {
		void lines.length;
		scrollDown();
	});

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const result = await logsApi.list(currentAgent);
			files = result.files ?? [];
			if (!file && files.length > 0) file = files[0].name;
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
		void load(currentAgent);
	});

	async function start() {
		if (!agent || !file || streaming) return;
		lines = [];
		streaming = true;
		reconnecting = false;
		error = '';
		controller = new AbortController();
		try {
			await logsApi.stream(
				agent,
				file,
				(event) => {
					reconnecting = false;
					if (event.event === 'line') {
						const data = event.data as { line?: string };
						lines = [...lines, data?.line ?? ''].slice(-1000);
					} else if (event.event === 'ready') {
						const data = event.data as { file?: string };
						if (data?.file) file = data.file;
					}
				},
				{
					signal: controller.signal,
					// The server replays the file from the start, so clear the buffer.
					onReconnect: () => {
						lines = [];
						reconnecting = true;
					}
				}
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				error = getErrorMessage(err);
			}
		} finally {
			streaming = false;
			reconnecting = false;
			controller = null;
		}
	}

	function stop() {
		controller?.abort();
		controller = null;
		streaming = false;
		reconnecting = false;
	}

	function pick(name: string) {
		stop();
		file = name;
		lines = [];
		revealPane(detail);
	}
</script>

<svelte:window onkeydown={searchNav.onKeydown} />

<TopBarActions id="logs-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
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
		<!-- Log files: a second sidebar nested inside the main one. -->
		<aside
			class="flex shrink-0 flex-col border-b bg-card frost-aside lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 space-y-2 border-b px-3 py-2.5">
				<SidebarHeader label={t('myaaw.logs.files')} count={files.length} />
				<SidebarSearchField
					bind:inputRef={searchNav.input}
					bind:value={query}
					placeholder={t('myaaw.logs.filter')}
				/>
			</div>
			<div
				bind:this={searchNav.list}
				class="space-y-0.5 p-2 lg:min-h-0 lg:flex-1 lg:overflow-y-auto"
			>
				{#if loading}
					<SidebarSkeletons />
				{:else if files.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.logs.empty')}</p>
				{:else if filteredFiles.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.logs.no_matches')}</p>
				{:else}
					{#each filteredFiles as entry (entry.name)}
						<button
							class={[
								'flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors list-row',
								entry.name === file ? 'bg-accent font-medium' : 'hover:bg-muted',
								searchNav.cursor === entry.name ? 'ring-1 ring-ring/50' : ''
							]}
							data-nav-id={entry.name}
							aria-current={entry.name === file ? 'true' : undefined}
							onclick={() => pick(entry.name)}
						>
							<ScrollText class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
							<span class="min-w-0 flex-1">
								<span class="block truncate font-mono text-xs">{entry.name}</span>
								<span class="block truncate text-[10px] text-muted-foreground">
									{formatSize(entry.size)}{entry.mod_time
										? ` · ${formatShortDate(entry.mod_time)}`
										: ''}
								</span>
							</span>
						</button>
					{/each}
				{/if}
			</div>
		</aside>

		<section bind:this={detail} class="flex flex-col p-4 lg:min-h-0 lg:flex-1 lg:p-6">
			{#if error}
				<div
					class="mb-3 shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			<Card class="mx-auto flex w-full max-w-5xl flex-col max-lg:h-[60vh] lg:min-h-0 lg:flex-1">
				<CardHeader class="flex shrink-0 flex-row items-center justify-between">
					<div class="min-w-0">
						<CardTitle class="flex items-center gap-2 text-sm">
							<Terminal class="h-4 w-4 shrink-0" />
							<span class="truncate">{file || t('myaaw.logs.select')}</span>
						</CardTitle>
						<CardDescription class="flex items-center gap-1.5">
							<span
								class={[
									'h-1.5 w-1.5 shrink-0 rounded-full',
									reconnecting
										? 'animate-pulse bg-amber-500'
										: streaming
											? 'animate-pulse bg-emerald-500'
											: 'bg-muted-foreground/40'
								]}
								aria-hidden="true"
							></span>
							{reconnecting
								? t('myaaw.logs.reconnecting')
								: streaming
									? t('myaaw.logs.streaming')
									: t('myaaw.logs.idle')}
						</CardDescription>
					</div>
					<div class="flex shrink-0 items-center gap-2">
						{#if currentFile}
							<div class="hidden items-center gap-1 sm:flex">
								<Badge variant="outline">{formatSize(currentFile.size)}</Badge>
								{#if currentFile.mod_time}
									<Badge variant="outline">{formatShortDate(currentFile.mod_time)}</Badge>
								{/if}
							</div>
						{/if}
						{#if streaming}
							<Button size="sm" variant="destructive" onclick={stop}>
								<Square class="mr-2 h-4 w-4" />
								{t('myaaw.logs.stop')}
							</Button>
						{:else}
							<Button size="sm" onclick={start} disabled={!file}>
								<Play class="mr-2 h-4 w-4" />
								{t('myaaw.logs.start')}
							</Button>
						{/if}
					</div>
				</CardHeader>
				<CardContent class="min-h-0 flex-1">
					<div
						bind:this={output}
						class="h-full overflow-auto rounded-md border border-input bg-muted/20 p-3 font-mono text-[11px] leading-relaxed"
					>
						{#if lines.length === 0}
							<p class="text-muted-foreground">{t('myaaw.logs.no_output')}</p>
						{/if}
						{#each lines as line, index (index)}
							<div class="whitespace-pre-wrap break-all">{line}</div>
						{/each}
					</div>
				</CardContent>
			</Card>
		</section>
	{/if}
</div>
