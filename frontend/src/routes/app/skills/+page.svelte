<script lang="ts">
	import { skills as skillsApi } from '$lib/apis/skills';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { detectPreview, stripFrontmatter } from '$lib/utils/filePreview';
	import { RefreshCw, Sparkles, Trash2 } from 'lucide-svelte';
	import type { SkillDetail, SkillFileContent } from '$lib/types';

	const { t } = useTranslation();

	let skills = $state<SkillDetail[]>([]);
	let selected = $state<SkillDetail | null>(null);
	let fileContent = $state<SkillFileContent | null>(null);
	let filePath = $state('');
	let loading = $state(false);
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

	// The skill body is its SKILL.md, which the detail endpoint already returns,
	// so start on that file instead of firing another request.
	let previewPath = $derived(fileContent?.path || filePath || 'SKILL.md');
	let preview = $derived(detectPreview(previewPath));
	let previewBinary = $derived(!!fileContent?.binary || preview.kind === 'binary');
	let previewText = $derived.by(() => {
		const raw = fileContent ? fileContent.content : (selected?.content ?? '');
		return preview.kind === 'markdown' ? stripFrontmatter(raw) : raw;
	});

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
		filePath = selected.files.find((file) => file.path.toLowerCase().endsWith('.md'))?.path ?? '';
	}

	async function openFile(name: string) {
		if (!agent || !selected) return;
		filePath = name;
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
		if (
			!(await askConfirm({
				message: t('myaaw.skills.delete_confirm', { name }),
				variant: 'destructive'
			}))
		)
			return;
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

<TopBarActions id="skills-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col lg:flex-row">
	{#if !agent}
		<div class="w-full p-4 lg:p-6">
			<Card>
				<CardContent class="py-10 text-center text-sm text-muted-foreground">
					{t('myaaw.agent.none')}
				</CardContent>
			</Card>
		</div>
	{:else}
		<!-- Skill list: a second sidebar nested inside the main one. -->
		<aside
			class="flex h-[45vh] shrink-0 flex-col border-b bg-card lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 border-b px-3 py-2.5">
				<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
					{t('myaaw.skills.list')}
				</p>
			</div>
			<div class="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-2">
				{#if loading}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('common.loading')}</p>
				{:else if skills.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.skills.empty')}</p>
				{/if}

				{#each skills as skill (skill.name)}
					<div
						class={[
							'flex items-center gap-2 rounded-md px-1 py-1',
							selected?.name === skill.name ? 'bg-accent' : 'hover:bg-muted'
						]}
					>
						<input
							type="checkbox"
							checked={skill.enabled}
							onchange={(event) => toggle(skill.name, event.currentTarget.checked)}
							class="h-4 w-4 shrink-0 accent-primary"
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
						<Badge variant="outline" class="shrink-0 text-[10px]">{skill.source}</Badge>
					</div>
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

			{#if !loading && skills.length === 0}
				<EmptyState
					title={t('myaaw.skills.empty')}
					description={t('myaaw.skills.empty_desc')}
					icon={Sparkles}
				/>
			{:else}
				<Card class="mx-auto flex min-h-0 w-full max-w-5xl flex-1 flex-col">
					{#if selected}
						<CardHeader class="flex shrink-0 flex-row items-start justify-between gap-2">
							<div class="min-w-0">
								<CardTitle class="truncate text-base">
									{selected.display_name || selected.name}
								</CardTitle>
								<p class="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
									{selected.description}
								</p>
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

						<CardContent class="shrink-0 space-y-1.5">
							<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
								{t('myaaw.skills.files')}
							</p>
							<div class="flex flex-wrap gap-1">
								{#each selected.files as file (file.path)}
									<button
										class={[
											'rounded-md border px-2 py-1 font-mono text-[11px] transition-colors',
											previewPath === file.path
												? 'bg-accent text-accent-foreground'
												: 'hover:bg-muted'
										]}
										onclick={() => openFile(file.path)}
									>
										{file.path}
									</button>
								{/each}
							</div>
						</CardContent>

						<CardContent class="min-h-0 flex-1">
							{#if previewBinary}
								<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
									{t('myaaw.skills.binary')}
								</p>
							{:else if preview.kind === 'markdown'}
								<div
									class="h-full overflow-y-auto rounded-md border border-input bg-muted/20 px-4 py-3"
								>
									<Markdown content={previewText} />
								</div>
							{:else}
								<CodePreview code={previewText} language={preview.language} />
							{/if}
						</CardContent>
					{:else}
						<CardContent class="py-10 text-center text-sm text-muted-foreground">
							{t('myaaw.skills.select')}
						</CardContent>
					{/if}
				</Card>
			{/if}
		</section>
	{/if}
</div>
