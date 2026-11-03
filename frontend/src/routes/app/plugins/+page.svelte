<script lang="ts">
	import { skills as skillsApi } from '$lib/apis/skills';
	import { tools as toolsApi } from '$lib/apis/tools';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import CustomToolEditor from '$lib/components/tools/CustomToolEditor.svelte';
	import McpServerEditor from '$lib/components/tools/McpServerEditor.svelte';
	import PluginDetailsDialog from '$lib/components/plugins/PluginDetailsDialog.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		ChevronRight,
		Loader2,
		Plug,
		Plus,
		Puzzle,
		RefreshCw,
		Search,
		Sparkles,
		SquareTerminal,
		Wrench
	} from 'lucide-svelte';
	import type { LucideIcon } from '$lib/types/icons';
	import type { CustomTool, MCPServer, Skill, SkillDetail, ToolsView } from '$lib/types';
	import type { PluginKind, PluginTarget } from '$lib/types/plugins';
	import type { Snippet } from 'svelte';

	const { t } = useTranslation();

	let agent = $derived(agentSelection.selected);
	let skills = $state<Skill[]>([]);
	let view = $state<ToolsView | null>(null);
	let loading = $state(false);
	let error = $state('');
	let query = $state('');
	let loadKey = '';

	// The entry whose details are open, resolved against the current lists so a
	// toggle is reflected in the dialog without fetching it again.
	let selected = $state<{ kind: PluginKind; name: string } | null>(null);
	let skillDetail = $state<SkillDetail | null>(null);
	let detailLoading = $state(false);

	let customDialogOpen = $state(false);
	let editingCustom = $state<CustomTool | null>(null);
	let mcpDialogOpen = $state(false);
	let editingMCP = $state<MCPServer | null>(null);

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

	const nothingMatches = $derived(
		search !== '' &&
			shownSkills.length === 0 &&
			shownBuiltin.length === 0 &&
			shownCustom.length === 0 &&
			shownServers.length === 0
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

	const target = $derived.by<PluginTarget | null>(() => {
		const pick = selected;
		if (!pick) return null;
		if (pick.kind === 'skill') {
			const skill = skills.find((item) => item.name === pick.name);
			return skill ? { kind: 'skill', skill } : null;
		}
		if (pick.kind === 'builtin') {
			const tool = builtin.find((item) => item.name === pick.name);
			return tool ? { kind: 'builtin', tool } : null;
		}
		if (pick.kind === 'custom') {
			const tool = custom.find((item) => item.name === pick.name);
			return tool ? { kind: 'custom', tool } : null;
		}
		const server = servers.find((item) => item.name === pick.name);
		return server ? { kind: 'mcp', server } : null;
	});

	// Only the entries the agent owns can be changed or removed.
	const editAction = $derived.by<(() => void) | undefined>(() => {
		const pick = target;
		if (pick?.kind === 'custom') return () => editCustom(pick.tool);
		if (pick?.kind === 'mcp') return () => editMcp(pick.server);
		return undefined;
	});

	const deleteAction = $derived.by<(() => void) | undefined>(() => {
		const pick = target;
		if (pick?.kind === 'custom') return () => void removeCustom(pick.tool);
		if (pick?.kind === 'mcp') return () => void removeMcp(pick.server);
		return undefined;
	});

	// Skills keep their instructions in SKILL.md, which the list does not carry.
	$effect(() => {
		const name = selected?.kind === 'skill' ? selected.name : '';
		const currentAgent = agent;
		if (!name || !currentAgent) {
			skillDetail = null;
			return;
		}
		let cancelled = false;
		detailLoading = true;
		skillDetail = null;
		skillsApi
			.get(currentAgent, name)
			.then((detail) => {
				if (!cancelled) skillDetail = detail;
			})
			.catch((err) => {
				if (!cancelled) toastStore.error(getErrorMessage(err));
			})
			.finally(() => {
				if (!cancelled) detailLoading = false;
			});
		return () => {
			cancelled = true;
		};
	});

	function toastToggle(name: string, enabled: boolean) {
		toastStore.success(
			enabled ? t('myaaw.tools.enabled_ok', { name }) : t('myaaw.tools.disabled_ok', { name })
		);
	}

	async function toggleSkill(name: string, enabled: boolean) {
		if (!agent) return;
		try {
			await skillsApi.toggle(agent, name, enabled);
			skills = skills.map((item) => (item.name === name ? { ...item, enabled } : item));
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	async function toggleBuiltin(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.toggleBuiltin(agent, name, enabled);
			view = {
				...view,
				builtin: view.builtin.map((item) => (item.name === name ? { ...item, enabled } : item))
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	async function toggleCustom(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.patchCustom(agent, name, { enabled });
			view = {
				...view,
				custom: view.custom.map((item) => (item.name === name ? { ...item, enabled } : item))
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	async function toggleMcp(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.patchMCP(agent, name, { enabled });
			view = {
				...view,
				mcp_servers: view.mcp_servers.map((item) =>
					item.name === name ? { ...item, enabled } : item
				)
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	/** Every switch in the page and in the dialog goes through here. */
	function setEnabled(current: PluginTarget, enabled: boolean) {
		if (current.kind === 'skill') return void toggleSkill(current.skill.name, enabled);
		if (current.kind === 'builtin') return void toggleBuiltin(current.tool.name, enabled);
		if (current.kind === 'custom') return void toggleCustom(current.tool.name, enabled);
		return void toggleMcp(current.server.name, enabled);
	}

	function openSkill(name: string) {
		selected = { kind: 'skill', name };
	}

	function openBuiltin(name: string) {
		selected = { kind: 'builtin', name };
	}

	function openCustom(name: string) {
		selected = { kind: 'custom', name };
	}

	function openMcp(name: string) {
		selected = { kind: 'mcp', name };
	}

	function editCustom(tool: CustomTool) {
		selected = null;
		editingCustom = tool;
		customDialogOpen = true;
	}

	function editMcp(server: MCPServer) {
		selected = null;
		editingMCP = server;
		mcpDialogOpen = true;
	}

	async function removeCustom(tool: CustomTool) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.tools.delete_confirm', { name: tool.name }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await toolsApi.removeCustom(agent, tool.name);
			selected = null;
			toastStore.success(t('myaaw.tools.deleted'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeMcp(server: MCPServer) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.tools.delete_confirm', { name: server.name }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await toolsApi.removeMCP(agent, server.name);
			selected = null;
			toastStore.success(t('myaaw.tools.deleted'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
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

{#snippet card(
	icon: LucideIcon,
	title: string,
	detail: string,
	enabled: boolean,
	tag: string,
	onopen: () => void,
	ontoggle: (enabled: boolean) => void
)}
	{@const Icon = icon}
	<div
		role="button"
		tabindex="0"
		aria-label={t('myaaw.plugins.open_details', { name: title })}
		onclick={onopen}
		onkeydown={(event) => {
			if (event.key === 'Enter' || event.key === ' ') {
				event.preventDefault();
				onopen();
			}
		}}
		class="flex cursor-pointer items-start gap-3 rounded-xl border bg-card p-3 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
	>
		<span
			class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
		>
			<Icon class="h-4 w-4" />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<p class="truncate text-sm font-medium">{title}</p>
				{#if tag}
					<Badge variant="secondary" class="shrink-0 px-1.5 py-0 text-[10px]">{tag}</Badge>
				{/if}
			</div>
			{#if detail}
				<p class="line-clamp-2 text-xs text-muted-foreground">{detail}</p>
			{/if}
		</div>
		<div class="flex shrink-0 items-center gap-1">
			<Switch
				checked={enabled}
				label={t('myaaw.plugins.toggle', { name: title })}
				onchange={ontoggle}
			/>
			<ChevronRight class="h-4 w-4 text-muted-foreground" />
		</div>
	</div>
{/snippet}

{#snippet groupHeader(icon: LucideIcon, title: string, description: string, action?: Snippet)}
	{@const Icon = icon}
	<div class="flex items-start justify-between gap-2">
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
		{#if action}
			{@render action()}
		{/if}
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
								item.enabled,
								'',
								() => openSkill(item.name),
								(next) => setEnabled({ kind: 'skill', skill: item }, next)
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
							{@render card(
								Wrench,
								item.name,
								item.description ?? '',
								item.enabled,
								'',
								() => openBuiltin(item.name),
								(next) => setEnabled({ kind: 'builtin', tool: item }, next)
							)}
						{/each}
					</div>
				{/if}
			</section>

			<section class="space-y-2">
				{@render groupHeader(
					SquareTerminal,
					t('myaaw.plugins.custom'),
					t('myaaw.plugins.custom_desc'),
					addCustomButton
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
								t('myaaw.plugins.tag_custom'),
								() => openCustom(item.name),
								(next) => setEnabled({ kind: 'custom', tool: item }, next)
							)}
						{/each}
					</div>
				{/if}
			</section>

			<section class="space-y-2">
				{@render groupHeader(
					Plug,
					t('myaaw.plugins.mcp'),
					t('myaaw.plugins.mcp_desc'),
					addMcpButton
				)}
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
								t('myaaw.plugins.tag_mcp'),
								() => openMcp(item.name),
								(next) => setEnabled({ kind: 'mcp', server: item }, next)
							)}
						{/each}
					</div>
				{/if}
			</section>
		{/if}
	{/if}
</div>

{#snippet addCustomButton()}
	<Button
		size="sm"
		variant="outline"
		class="shrink-0"
		onclick={() => {
			editingCustom = null;
			customDialogOpen = true;
		}}
	>
		<Plus class="mr-1 h-3.5 w-3.5" />
		{t('myaaw.plugins.add_custom')}
	</Button>
{/snippet}

{#snippet addMcpButton()}
	<Button
		size="sm"
		variant="outline"
		class="shrink-0"
		onclick={() => {
			editingMCP = null;
			mcpDialogOpen = true;
		}}
	>
		<Plus class="mr-1 h-3.5 w-3.5" />
		{t('myaaw.plugins.add_mcp')}
	</Button>
{/snippet}

{#if target}
	<PluginDetailsDialog
		{agent}
		{target}
		detail={skillDetail}
		loading={detailLoading}
		onToggle={(enabled) => setEnabled(target, enabled)}
		onEdit={editAction}
		onDelete={deleteAction}
		onClose={() => (selected = null)}
	/>
{/if}

<Dialog.Root
	open={customDialogOpen}
	onOpenChange={(open) => {
		customDialogOpen = open;
		if (!open) editingCustom = null;
	}}
>
	<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>
				{editingCustom ? t('common.edit') : t('myaaw.plugins.add_custom')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.plugins.custom_desc')}</Dialog.Description>
		</Dialog.Header>
		{#key `${customDialogOpen}:${editingCustom?.name ?? 'new'}`}
			<CustomToolEditor
				{agent}
				editing={editingCustom}
				onSaved={() => {
					customDialogOpen = false;
					editingCustom = null;
					void load(agent);
				}}
				onCancel={() => {
					customDialogOpen = false;
					editingCustom = null;
				}}
			/>
		{/key}
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	open={mcpDialogOpen}
	onOpenChange={(open) => {
		mcpDialogOpen = open;
		if (!open) editingMCP = null;
	}}
>
	<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>{editingMCP ? t('common.edit') : t('myaaw.plugins.add_mcp')}</Dialog.Title>
			<Dialog.Description>{t('myaaw.plugins.mcp_desc')}</Dialog.Description>
		</Dialog.Header>
		{#key `${mcpDialogOpen}:${editingMCP?.name ?? 'new'}`}
			<McpServerEditor
				{agent}
				editing={editingMCP}
				onSaved={() => {
					mcpDialogOpen = false;
					editingMCP = null;
					void load(agent);
				}}
				onCancel={() => {
					mcpDialogOpen = false;
					editingMCP = null;
				}}
			/>
		{/key}
	</Dialog.Content>
</Dialog.Root>
