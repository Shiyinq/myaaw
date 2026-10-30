<script lang="ts">
	import { logs as logsApi } from '$lib/apis/logs';
	import { agentSelection } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Play, RefreshCw, Square, Terminal } from 'lucide-svelte';
	import type { LogFile } from '$lib/types';

	const { t } = useTranslation();

	let files = $state<LogFile[]>([]);
	let file = $state('');
	let lines = $state<string[]>([]);
	let loading = $state(false);
	let streaming = $state(false);
	let error = $state('');
	let loadKey = '';
	let controller: AbortController | null = null;
	let output = $state<HTMLDivElement | null>(null);

	let agent = $derived(agentSelection.selected);

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
		error = '';
		controller = new AbortController();
		try {
			await logsApi.stream(
				agent,
				file,
				(event) => {
					if (event.event === 'line') {
						const data = event.data as { line?: string };
						lines = [...lines, data?.line ?? ''].slice(-1000);
					} else if (event.event === 'ready') {
						const data = event.data as { file?: string };
						if (data?.file) file = data.file;
					}
				},
				controller.signal
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				error = getErrorMessage(err);
			}
		} finally {
			streaming = false;
			controller = null;
		}
	}

	function stop() {
		controller?.abort();
		controller = null;
		streaming = false;
	}

	function pick(name: string) {
		stop();
		file = name;
		lines = [];
	}
</script>

<TopBarActions id="logs-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
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

		<div class="grid gap-4 lg:grid-cols-4">
			<Card class="lg:col-span-1">
				<CardHeader>
					<CardTitle class="text-sm">{t('myaaw.logs.files')}</CardTitle>
					<CardDescription>{t('myaaw.logs.files_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-1">
					{#if files.length === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.logs.empty')}</p>
					{/if}
					{#each files as entry (entry.name)}
						<button
							class={[
								'flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors',
								entry.name === file ? 'bg-accent font-medium' : 'hover:bg-muted'
							]}
							onclick={() => pick(entry.name)}
						>
							<span class="truncate font-mono text-xs">{entry.name}</span>
							<span class="text-[10px] text-muted-foreground">{Math.round(entry.size / 1024)}K</span>
						</button>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-3">
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle class="flex items-center gap-2 text-sm">
							<Terminal class="h-4 w-4" />
							{file || t('myaaw.logs.select')}
						</CardTitle>
						<CardDescription>
							{streaming ? t('myaaw.logs.streaming') : t('myaaw.logs.idle')}
						</CardDescription>
					</div>
					<div class="flex gap-2">
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
				<CardContent>
					<div
						bind:this={output}
						class="h-[28rem] overflow-auto rounded-md border bg-muted/20 p-2 font-mono text-[11px] leading-relaxed"
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
		</div>
	{/if}
</div>
