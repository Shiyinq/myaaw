<script lang="ts">
	import { context as contextApi } from '$lib/apis/context';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Save } from 'lucide-svelte';
	import type { ContextFile } from '$lib/types';

	const { t } = useTranslation();

	let files = $state<ContextFile[]>([]);
	let path = $state('');
	let content = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

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
			await contextApi.write(agent, path, content);
			toastStore.success(t('myaaw.context.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	function groups() {
		const map = new Map<string, ContextFile[]>();
		for (const file of files) {
			const list = map.get(file.folder) ?? [];
			list.push(file);
			map.set(file.folder, list);
		}
		return [...map.entries()];
	}
</script>

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
					<CardTitle class="text-sm">{t('myaaw.context.files')}</CardTitle>
				</CardHeader>
				<CardContent class="space-y-3">
					{#if loading}
						<p class="text-sm text-muted-foreground">{t('common.loading')}</p>
					{:else if files.length === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.context.empty')}</p>
					{/if}
					{#each groups() as [folder, list] (folder)}
						<div class="space-y-1">
							<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
								{folder}
							</p>
							{#each list as file (file.path)}
								<button
									class={[
										'block w-full truncate rounded-md px-2 py-1 text-left text-sm transition-colors',
										file.path === path
											? 'bg-accent font-medium text-accent-foreground'
											: 'hover:bg-muted'
									]}
									onclick={() => agent && open(agent, file.path)}
								>
									{file.name}
								</button>
							{/each}
						</div>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-3">
				<CardHeader class="flex flex-row items-center justify-between">
					<CardTitle class="truncate text-sm font-medium">
						{path || t('myaaw.context.editor')}
					</CardTitle>
					<Badge variant="outline">{t('myaaw.context.markdown')}</Badge>
				</CardHeader>
				<CardContent>
					{#if path}
						<textarea
							bind:value={content}
							class="min-h-[32rem] w-full resize-y rounded-md border border-input bg-transparent p-3 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
						></textarea>
					{:else}
						<p class="py-10 text-center text-sm text-muted-foreground">
							{t('myaaw.context.select_file')}
						</p>
					{/if}
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
