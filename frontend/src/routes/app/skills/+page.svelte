<script lang="ts">
	import { skills as skillsApi } from '$lib/apis/skills';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Separator } from '$lib/components/ui/separator';
	import { RefreshCw, Sparkles, Trash2 } from 'lucide-svelte';
	import type { SkillDetail, SkillFileContent } from '$lib/types';

	const { t } = useTranslation();

	let skills = $state<SkillDetail[]>([]);
	let selected = $state<SkillDetail | null>(null);
	let fileContent = $state<SkillFileContent | null>(null);
	let loading = $state(false);
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const result = await skillsApi.list(currentAgent);
			const list = result.skills ?? [];
			skills = list.map((skill) => ({ ...skill, content: '', files: [] }));
			const keep = list.find((skill) => skill.name === selected?.name) ?? list[0];
			if (keep) {
				await open(currentAgent, keep.name);
			} else {
				selected = null;
			}
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	async function open(currentAgent: string, name: string) {
		selected = await skillsApi.get(currentAgent, name);
		fileContent = null;
	}

	async function openFile(name: string) {
		if (!agent || !selected) return;
		try {
			fileContent = await skillsApi.file(agent, selected.name, name);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	async function toggle(name: string, enabled: boolean) {
		if (!agent) return;
		try {
			await skillsApi.toggle(agent, name, enabled);
			skills = skills.map((skill) => (skill.name === name ? { ...skill, enabled } : skill));
			if (selected?.name === name) selected = { ...selected, enabled };
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function remove(name: string) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.skills.delete_confirm', { name }))) return;
		try {
			await skillsApi.remove(agent, name);
			toastStore.success(t('myaaw.skills.deleted'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function clearConfig(name: string) {
		if (!agent) return;
		try {
			await skillsApi.removeConfig(agent, name);
			toastStore.success(t('myaaw.skills.config_cleared'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<div class="space-y-4">
	<PageHeader title={t('myaaw.skills.title')} description={t('myaaw.skills.subtitle')}>
		<Button variant="outline" size="sm" onclick={() => agent && load(agent)} loading={loading}>
			<RefreshCw class="mr-2 h-4 w-4" />
			{t('common.refresh')}
		</Button>
	</PageHeader>

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

		{#if !loading && skills.length === 0}
			<EmptyState
				title={t('myaaw.skills.empty')}
				description={t('myaaw.skills.empty_desc')}
				icon={Sparkles}
			/>
		{:else}
			<div class="grid gap-4 lg:grid-cols-5">
				<Card class="lg:col-span-2">
					<CardHeader>
						<CardTitle class="text-sm">{t('myaaw.skills.list')}</CardTitle>
					</CardHeader>
					<CardContent class="space-y-1">
						{#each skills as skill (skill.name)}
							<div
								class={[
									'flex items-center gap-2 rounded-md px-2 py-1.5',
									selected?.name === skill.name ? 'bg-accent' : 'hover:bg-muted'
								]}
							>
								<input
									type="checkbox"
									checked={skill.enabled}
									onchange={(event) => toggle(skill.name, event.currentTarget.checked)}
									class="h-4 w-4 accent-primary"
									aria-label={t('myaaw.skills.enabled')}
								/>
								<button
									class="min-w-0 flex-1 truncate text-left text-sm"
									onclick={() => agent && open(agent, skill.name)}
								>
									{skill.display_name || skill.name}
								</button>
								{#if !skill.parse_ok}
									<Badge variant="destructive">{t('myaaw.skills.invalid')}</Badge>
								{/if}
								<Badge variant="outline" class="text-[10px]">{skill.source}</Badge>
							</div>
						{/each}
					</CardContent>
				</Card>

				<Card class="lg:col-span-3">
					{#if selected}
						<CardHeader class="flex flex-row items-start justify-between gap-2">
							<div class="min-w-0">
								<CardTitle class="truncate text-base">
									{selected.display_name || selected.name}
								</CardTitle>
								<p class="mt-1 text-xs text-muted-foreground">{selected.description}</p>
							</div>
							<div class="flex shrink-0 gap-1">
								<Button size="sm" variant="outline" onclick={() => clearConfig(selected!.name)}>
									{t('myaaw.skills.reset')}
								</Button>
								<Button size="sm" variant="destructive" onclick={() => remove(selected!.name)}>
									<Trash2 class="mr-2 h-4 w-4" />
									{t('common.delete')}
								</Button>
							</div>
						</CardHeader>
						<CardContent class="space-y-3">
							<div class="max-h-[28rem] overflow-y-auto rounded-md border bg-muted/20 p-3">
								<Markdown content={selected.content} />
							</div>

							<Separator />
							<div>
								<p class="mb-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
									{t('myaaw.skills.files')}
								</p>
								<div class="flex flex-wrap gap-1">
									{#each selected.files as file (file.path)}
										<button
											class="rounded-md border px-2 py-1 font-mono text-[11px] hover:bg-muted"
											onclick={() => openFile(file.path)}
										>
											{file.path}
										</button>
									{/each}
								</div>
							</div>

							{#if fileContent}
								<pre class="max-h-64 overflow-auto rounded-md border bg-muted/20 p-2 text-[11px]">{fileContent.binary ? t('myaaw.skills.binary') : fileContent.content}</pre>
							{/if}
						</CardContent>
					{:else}
						<CardContent class="py-10 text-center text-sm text-muted-foreground">
							{t('myaaw.skills.select')}
						</CardContent>
					{/if}
				</Card>
			</div>
		{/if}
	{/if}
</div>
