<script lang="ts">
	import { skills as skillsApi } from '$lib/apis/skills';
	import { tools as toolsApi } from '$lib/apis/tools';
	import { agentSelection } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import {
		Loader2,
		Plug,
		Puzzle,
		RefreshCw,
		Search,
		Sparkles,
		SquareTerminal,
		Wrench
	} from 'lucide-svelte';
	import type { Skill, ToolsView } from '$lib/types';

	const { t } = useTranslation();

	let agent = $derived(agentSelection.selected);
	let skills = $state<Skill[]>([]);
	let view = $state<ToolsView | null>(null);
	let loading = $state(false);
	let error = $state('');
	let query = $state('');
	let loadKey = '';

	$effect(() => {
		const name = agent;
		if (!name || name === loadKey) return;
		loadKey = name;
		void load(name);
	});

	async function load(name: string) {
		loading = true;
		error = '';
		try {
			const [skillList, toolsView] = await Promise.all([skillsApi.list(name), toolsApi.list(name)]);
			skills = skillList.skills ?? [];
			view = toolsView;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	const search = $derived(query.trim().toLowerCase());

	function hit(...values: (string | undefined | null)[]) {
		return values.some((value) => (value ?? '').toLowerCase().includes(search));
	}

	const builtin = $derived(view?.builtin ?? []);
	const custom = $derived(view?.custom ?? []);
	const servers = $derived(view?.mcp_servers ?? []);

	const shownSkills = $derived(
		search ? skills.filter((item) => hit(item.display_name, item.name, item.description)) : skills
	);
	const shownBuiltin = $derived(
		search ? builtin.filter((item) => hit(item.name, item.description)) : builtin
	);
	const shownCustom = $derived(
		search ? custom.filter((item) => hit(item.name, item.command)) : custom
	);
	const shownServers = $derived(
		search ? servers.filter((item) => hit(item.name, item.command)) : servers
	);

	const stats = $derived([
		{
			icon: Sparkles,
			label: t('myaaw.plugins.skills'),
			active: skills.filter((item) => item.enabled).length,
			total: skills.length
		},
		{
			icon: Wrench,
			label: t('myaaw.plugins.tools'),
			active: builtin.filter((item) => item.enabled).length,
			total: builtin.length
		},
		{
			icon: SquareTerminal,
			label: t('myaaw.plugins.custom'),
			active: custom.filter((item) => item.enabled).length,
			total: custom.length
		},
		{
			icon: Plug,
			label: t('myaaw.plugins.mcp'),
			active: servers.filter((item) => item.enabled).length,
			total: servers.length
		}
	]);

	const nothingMatches = $derived(
		search !== '' &&
			shownSkills.length === 0 &&
			shownBuiltin.length === 0 &&
			shownCustom.length === 0 &&
			shownServers.length === 0
	);
</script>

<TopBarActions id="plugins-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			{loading}
			onclick={() => agent && load(agent)}
		/>
	{/snippet}
</TopBarActions>

{#snippet status(enabled: boolean)}
	<span
		class={[
			'flex shrink-0 items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium',
			enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground'
		]}
	>
		<span
			aria-hidden="true"
			class={['h-1.5 w-1.5 rounded-full', enabled ? 'bg-emerald-500' : 'bg-muted-foreground/40']}
		></span>
		{enabled ? t('myaaw.plugins.active') : t('myaaw.plugins.off')}
	</span>
{/snippet}

{#snippet card(icon: typeof Sparkles, name: string, detail: string, enabled: boolean, tag = '')}
	{@const Icon = icon}
	<div
		class="flex items-start gap-3 rounded-xl border bg-card p-3 transition-colors hover:bg-muted/50"
	>
		<span
			class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
		>
			<Icon class="h-4 w-4" />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<p class="truncate text-sm font-medium">{name}</p>
				{#if tag}
					<Badge variant="secondary" class="shrink-0 px-1.5 py-0 text-[10px]">{tag}</Badge>
				{/if}
			</div>
			{#if detail}
				<p class="line-clamp-2 text-xs text-muted-foreground">{detail}</p>
			{/if}
		</div>
		{@render status(enabled)}
	</div>
{/snippet}

{#snippet groupHeader(icon: typeof Sparkles, title: string, description: string)}
	{@const Icon = icon}
	<div class="flex items-start gap-2">
		<span
			class="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
		>
			<Icon class="h-3.5 w-3.5" />
		</span>
		<div class="min-w-0">
			<h2 class="text-sm font-semibold">{title}</h2>
			<p class="text-xs text-muted-foreground">{description}</p>
		</div>
	</div>
{/snippet}

<div class="mx-auto w-full max-w-5xl space-y-6">
	<div class="flex items-start gap-3">
		<span
			class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground"
		>
			<Puzzle class="h-5 w-5" />
		</span>
		<div class="min-w-0">
			<p class="text-sm font-semibold">{t('myaaw.plugins.intro')}</p>
			<p class="text-xs text-muted-foreground">
				{t('myaaw.plugins.agent', { name: agent || '—' })}
			</p>
		</div>
	</div>

	{#if !agent}
		<p class="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
			{t('myaaw.agent.none')}
		</p>
	{:else if error}
		<div class="flex flex-col items-start gap-2 rounded-xl border border-destructive/40 p-4">
			<p class="text-sm text-destructive">{error}</p>
			<Button variant="outline" size="sm" onclick={() => load(agent)}>
				{t('common.refresh')}
			</Button>
		</div>
	{:else}
		<div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
			{#each stats as stat (stat.label)}
				{@const StatIcon = stat.icon}
				<div class="rounded-xl border bg-card p-3">
					<div class="flex items-center gap-2 text-muted-foreground">
						<StatIcon class="h-3.5 w-3.5" />
						<span class="truncate text-[11px]">{stat.label}</span>
					</div>
					<p class="mt-1 text-lg font-semibold tabular-nums">{stat.total}</p>
					<p class="text-[10px] text-muted-foreground">
						{t('myaaw.plugins.count_active', { active: stat.active, total: stat.total })}
					</p>
				</div>
			{/each}
		</div>

		<div class="relative">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:value={query}
				placeholder={t('myaaw.plugins.search')}
				class="h-9 pl-8"
				aria-label={t('myaaw.plugins.search')}
			/>
		</div>

		{#if loading && !view && skills.length === 0}
			<div class="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if nothingMatches}
			<p class="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
				{t('myaaw.plugins.no_results')}
			</p>
		{:else}
			<section class="space-y-2">
				{@render groupHeader(Sparkles, t('myaaw.plugins.skills'), t('myaaw.plugins.skills_desc'))}
				{#if shownSkills.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-center text-xs text-muted-foreground">
						{t('myaaw.plugins.empty')}
					</p>
				{:else}
					<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
						{#each shownSkills as item (item.name)}
							{@render card(
								Sparkles,
								item.display_name || item.name,
								item.description,
								item.enabled
							)}
						{/each}
					</div>
				{/if}
			</section>

			<section class="space-y-2">
				{@render groupHeader(Wrench, t('myaaw.plugins.tools'), t('myaaw.plugins.tools_desc'))}
				{#if shownBuiltin.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-center text-xs text-muted-foreground">
						{t('myaaw.plugins.empty')}
					</p>
				{:else}
					<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
						{#each shownBuiltin as item (item.name)}
							{@render card(Wrench, item.name, item.description ?? '', item.enabled)}
						{/each}
					</div>
				{/if}
			</section>

			<section class="space-y-2">
				{@render groupHeader(
					SquareTerminal,
					t('myaaw.plugins.custom'),
					t('myaaw.plugins.custom_desc')
				)}
				{#if shownCustom.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-center text-xs text-muted-foreground">
						{t('myaaw.plugins.empty')}
					</p>
				{:else}
					<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
						{#each shownCustom as item (item.name)}
							{@render card(
								SquareTerminal,
								item.name,
								[item.command, ...(item.args ?? [])].join(' '),
								item.enabled,
								t('myaaw.plugins.tag_custom')
							)}
						{/each}
					</div>
				{/if}
			</section>

			<section class="space-y-2">
				{@render groupHeader(Plug, t('myaaw.plugins.mcp'), t('myaaw.plugins.mcp_desc'))}
				{#if shownServers.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-center text-xs text-muted-foreground">
						{t('myaaw.plugins.empty')}
					</p>
				{:else}
					<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
						{#each shownServers as item (item.name)}
							{@render card(
								Plug,
								item.name,
								[item.command, ...(item.args ?? [])].join(' '),
								item.enabled,
								t('myaaw.plugins.tag_mcp')
							)}
						{/each}
					</div>
				{/if}
			</section>
		{/if}
	{/if}
</div>
