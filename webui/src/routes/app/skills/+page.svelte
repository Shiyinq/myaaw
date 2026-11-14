<script lang="ts">
	import { skills as skillsApi } from '$lib/apis/skills';
	import { agents as agentsApi } from '$lib/apis/agents';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { revealPane } from '$lib/utils/scroll';
	import { createSearchNav } from '$lib/utils/searchNav.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import SidebarHeader from '$lib/components/app/SidebarHeader.svelte';
	import SidebarSearchField from '$lib/components/app/SidebarSearchField.svelte';
	import SidebarSection from '$lib/components/app/SidebarSection.svelte';
	import SidebarSkeletons from '$lib/components/app/SidebarSkeletons.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { detectPreview, stripFrontmatter } from '$lib/utils/filePreview';
	import { PackageOpen, Puzzle, RefreshCw, Sparkles, Trash2 } from 'lucide-svelte';
	import type { CapabilityEntry, SkillDetail, SkillFileContent } from '$lib/types';

	const { t } = useTranslation();

	// Below lg the list and the panel share one scrolling page, so the panel is
	// brought into view whenever a row further up is picked.
	let detail = $state<HTMLElement | null>(null);

	let skills = $state<SkillDetail[]>([]);
	let catalog = $state<CapabilityEntry[]>([]);
	let installing = $state('');
	let previewMode = $state(false);
	let selected = $state<SkillDetail | null>(null);
	let fileContent = $state<SkillFileContent | null>(null);
	let filePath = $state('');
	let loading = $state(false);
	let error = $state('');
	let query = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);
	// Built-in names drive which section a skill belongs to; the catalog itself
	// lists every embedded template, installed or not.
	let catalogNames = $derived(new Set(catalog.map((entry) => entry.name)));
	let customSkills = $derived(skills.filter((skill) => !catalogNames.has(skill.name)));
	let builtinRows = $derived(
		catalog.map((entry) => ({
			entry,
			installed: skills.find((skill) => skill.name === entry.name) ?? null
		}))
	);
	let filteredCustomSkills = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return customSkills;
		return customSkills.filter(
			(skill) =>
				(skill.display_name || skill.name).toLowerCase().includes(needle) ||
				skill.name.toLowerCase().includes(needle) ||
				(skill.description ?? '').toLowerCase().includes(needle)
		);
	});
	let filteredBuiltinRows = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return builtinRows;
		return builtinRows.filter(
			(row) =>
				row.entry.name.toLowerCase().includes(needle) ||
				(row.entry.description ?? '').toLowerCase().includes(needle)
		);
	});
	let totalSkills = $derived(customSkills.length + builtinRows.length);
	let hasMatches = $derived(filteredCustomSkills.length + filteredBuiltinRows.length > 0);

	let navItems = $derived([
		...filteredCustomSkills.map((skill) => ({
			id: `custom:${skill.name}`,
			activate: () => {
				if (!agent) return;
				revealPane(detail);
				void open(agent, skill.name);
			}
		})),
		...filteredBuiltinRows.map((row) => ({
			id: `builtin:${row.entry.name}`,
			activate: () => {
				if (row.installed) {
					if (!agent) return;
					revealPane(detail);
					void open(agent, row.entry.name);
				} else {
					void openBuiltin(row.entry.name);
				}
			}
		}))
	]);

	const searchNav = createSearchNav({
		items: () => navItems,
		query: () => query,
		clear: () => (query = '')
	});

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
			const [result, caps] = await Promise.all([
				skillsApi.list(currentAgent),
				agentsApi.capabilities().catch(() => null)
			]);
			if (caps) catalog = caps.skills ?? [];
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
		previewMode = false;
		fileContent = null;
		filePath = selected.files.find((file) => file.path.toLowerCase().endsWith('.md'))?.path ?? '';
	}

	/** Show a built-in template that is not installed yet. */
	async function openBuiltin(name: string) {
		if (!agent) return;
		try {
			selected = await skillsApi.builtin(name);
			previewMode = true;
			fileContent = null;
			filePath = selected.files.find((file) => file.path.toLowerCase().endsWith('.md'))?.path ?? '';
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function install(name: string) {
		if (!agent || installing) return;
		installing = name;
		try {
			await skillsApi.install(agent, name);
			toastStore.success(t('myaaw.skills.installed_ok', { name }));
			await load(agent);
			await open(agent, name);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			installing = '';
		}
	}

	/**
	 * One checkbox for every built-in skill: checking an uninstalled skill copies
	 * it in, checking a disabled one only re-enables it, and unchecking disables
	 * it through skills.json without touching the folder.
	 */
	async function setBuiltin(name: string, enabled: boolean) {
		const installed = skills.some((skill) => skill.name === name);
		if (enabled && !installed) {
			await install(name);
			return;
		}
		if (installed) {
			await toggle(name, enabled);
		}
	}

	async function openFile(name: string) {
		if (!agent || !selected) return;
		filePath = name;
		try {
			fileContent = previewMode
				? await skillsApi.builtinFile(selected.name, name)
				: await skillsApi.file(agent, selected.name, name);
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
			toastStore.success(
				enabled ? t('myaaw.skills.enabled_ok', { name }) : t('myaaw.skills.disabled_ok', { name })
			);
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

<svelte:window onkeydown={searchNav.onKeydown} />

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
		<!-- Skill list: a second sidebar nested inside the main one. -->
		<aside
			class="flex shrink-0 flex-col border-b bg-card frost-aside lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 space-y-2 border-b px-3 py-2.5">
				<SidebarHeader label={t('myaaw.skills.list')} count={totalSkills} />
				<SidebarSearchField
					bind:inputRef={searchNav.input}
					bind:value={query}
					placeholder={t('common.search')}
				/>
			</div>
			<div bind:this={searchNav.list} class="space-y-3 p-2 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
				{#if loading}
					<SidebarSkeletons />
				{:else if query && !hasMatches}
					<p class="px-1 py-2 text-sm text-muted-foreground">
						{t('myaaw.skills.no_matches')}
					</p>
				{/if}

				{#if !loading && (filteredCustomSkills.length > 0 || !query)}
					<SidebarSection
						icon={Puzzle}
						label={t('myaaw.skills.custom')}
						count={filteredCustomSkills.length}
					>
						{#if filteredCustomSkills.length === 0}
							<p class="px-1 py-2 text-sm text-muted-foreground">
								{t('myaaw.skills.custom_empty')}
							</p>
						{/if}

						{#each filteredCustomSkills as skill (skill.name)}
							<div
								class={[
									'flex items-start gap-2 rounded-md px-1 py-1.5 list-row',
									selected?.name === skill.name ? 'bg-accent' : 'hover:bg-muted',
									searchNav.cursor === `custom:${skill.name}` ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={`custom:${skill.name}`}
								aria-current={selected?.name === skill.name ? 'true' : undefined}
							>
								<input
									type="checkbox"
									checked={skill.enabled}
									onchange={(event) => toggle(skill.name, event.currentTarget.checked)}
									class="mt-0.5 h-4 w-4 shrink-0 accent-primary"
									aria-label={t('myaaw.skills.enabled')}
								/>
								<button
									class="min-w-0 flex-1 text-left"
									title={skill.description || skill.name}
									onclick={() => {
										if (!agent) return;
										revealPane(detail);
										void open(agent, skill.name);
									}}
								>
									<span class="block truncate text-sm">{skill.display_name || skill.name}</span>
									{#if skill.description}
										<span class="block truncate text-[10px] text-muted-foreground">
											{skill.description}
										</span>
									{/if}
								</button>
								{#if !skill.parse_ok}
									<Badge variant="destructive">{t('myaaw.skills.invalid')}</Badge>
								{/if}
								<Badge variant="outline" class="shrink-0 text-[10px]">{skill.source}</Badge>
							</div>
						{/each}
					</SidebarSection>
				{/if}

				{#if !loading && (filteredBuiltinRows.length > 0 || (!query && builtinRows.length > 0))}
					<SidebarSection
						icon={PackageOpen}
						label={t('myaaw.skills.builtin')}
						count={filteredBuiltinRows.length}
						title={t('myaaw.skills.builtin_desc')}
					>
						{#each filteredBuiltinRows as row (row.entry.name)}
							<div
								class={[
									'flex items-start gap-2 rounded-md px-1 py-1.5 list-row',
									selected?.name === row.entry.name ? 'bg-accent' : 'hover:bg-muted',
									searchNav.cursor === `builtin:${row.entry.name}` ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={`builtin:${row.entry.name}`}
								aria-current={selected?.name === row.entry.name ? 'true' : undefined}
								title={row.entry.description}
							>
								<input
									type="checkbox"
									checked={!!row.installed?.enabled}
									onchange={(event) => setBuiltin(row.entry.name, event.currentTarget.checked)}
									class="mt-0.5 h-4 w-4 shrink-0 accent-primary"
									aria-label={row.entry.name}
									disabled={installing === row.entry.name}
								/>
								<div class="min-w-0 flex-1">
									{#if row.installed}
										<button
											type="button"
											class="block w-full text-left"
											onclick={() => {
												if (!agent) return;
												revealPane(detail);
												void open(agent, row.entry.name);
											}}
										>
											<span class="block truncate text-sm">
												{row.installed.display_name || row.entry.name}
											</span>
											{#if row.entry.description}
												<span class="block truncate text-[10px] text-muted-foreground">
													{row.entry.description}
												</span>
											{/if}
										</button>
									{:else}
										<button
											type="button"
											class="block w-full text-left"
											onclick={() => openBuiltin(row.entry.name)}
										>
											<span class="block truncate text-sm">{row.entry.name}</span>
											{#if row.entry.description}
												<span class="block truncate text-[10px] text-muted-foreground">
													{row.entry.description}
												</span>
											{/if}
										</button>
									{/if}
								</div>
								{#if row.installed}
									<Badge variant="outline" class="shrink-0 text-[10px]">
										{row.installed.source}
									</Badge>
								{/if}
							</div>
						{/each}
					</SidebarSection>
				{/if}
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

			{#if loading && !selected}
				<div class="mx-auto w-full max-w-5xl space-y-3">
					<Skeleton class="h-7 w-1/2" />
					<Skeleton class="h-4 w-3/4" />
					<Skeleton class="h-64 w-full" />
				</div>
			{:else if !selected}
				<EmptyState
					title={t('myaaw.skills.empty')}
					description={t('myaaw.skills.empty_desc')}
					bloub="curieux"
					icon={Sparkles}
				/>
			{:else}
				<Card
					class="mx-auto flex w-full max-w-5xl min-w-0 flex-col lg:min-h-0 lg:flex-1 lg:overflow-hidden"
				>
					{#if selected}
						{@const HeaderIcon = catalogNames.has(selected.name) ? PackageOpen : Puzzle}
						<CardHeader class="flex shrink-0 flex-row items-start justify-between gap-2">
							<div class="min-w-0">
								<div class="flex min-w-0 items-center gap-1.5">
									<HeaderIcon class="h-4 w-4 shrink-0 text-muted-foreground" />
									<CardTitle class="truncate text-base">
										{selected.display_name || selected.name}
									</CardTitle>
								</div>
								<p class="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
									{selected.description}
								</p>
								<div class="mt-1.5 flex flex-wrap items-center gap-1">
									{#if previewMode}
										<Badge variant="secondary">{t('myaaw.skills.builtin')}</Badge>
									{:else}
										<Badge variant={selected.enabled ? 'secondary' : 'outline'}>
											{selected.enabled ? t('myaaw.skills.enabled') : t('myaaw.skills.disabled')}
										</Badge>
									{/if}
									<Badge variant="outline">{selected.source}</Badge>
									<Badge variant="outline">
										{t('myaaw.skills.files_count', { count: selected.files.length })}
									</Badge>
								</div>
							</div>
							<div class="flex shrink-0 gap-1">
								{#if previewMode}
									<Button
										size="sm"
										loading={installing === selected.name}
										onclick={() => install(selected!.name)}
									>
										{t('myaaw.skills.install')}
									</Button>
								{:else}
									<Button size="sm" variant="outline" onclick={() => clearConfig(selected!.name)}>
										{t('myaaw.skills.reset')}
									</Button>
									<Button size="sm" variant="destructive" onclick={() => remove(selected!.name)}>
										<Trash2 class="mr-2 h-4 w-4" />
										{t('common.delete')}
									</Button>
								{/if}
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

						<CardContent class="min-h-0 min-w-0 max-lg:h-[60vh] lg:flex-1">
							{#if previewBinary}
								<p class="flex h-full items-center justify-center text-sm text-muted-foreground">
									{t('myaaw.skills.binary')}
								</p>
							{:else if preview.kind === 'markdown'}
								<div
									class="h-full min-w-0 overflow-x-auto overflow-y-auto rounded-md border border-input bg-muted/20 px-4 py-3"
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
