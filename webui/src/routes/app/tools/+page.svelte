<script lang="ts">
	import { tools as toolsApi } from '$lib/apis/tools';
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
	import CustomToolEditor from '$lib/components/tools/CustomToolEditor.svelte';
	import McpServerEditor from '$lib/components/tools/McpServerEditor.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { FileCode2, Pencil, Plug, RefreshCw, Trash2, Wrench } from 'lucide-svelte';
	import type { CustomTool, MCPServer, ToolsView } from '$lib/types';

	const { t } = useTranslation();

	// Below lg the list and the panel share one scrolling page, so the panel is
	// brought into view whenever a row further up is picked.
	let detail = $state<HTMLElement | null>(null);

	let view = $state<ToolsView | null>(null);
	let loading = $state(false);
	let error = $state('');
	let query = $state('');
	let loadKey = '';

	// Which entry the editor below is changing; null means a new one.
	let editingCustom = $state<CustomTool | null>(null);
	let editingMCP = $state<MCPServer | null>(null);

	let agent = $derived(agentSelection.selected);
	let builtinTools = $derived(view?.builtin ?? []);
	let customTools = $derived(view?.custom ?? []);
	let mcpServers = $derived(view?.mcp_servers ?? []);
	let totalTools = $derived(builtinTools.length + customTools.length + mcpServers.length);
	let filteredBuiltin = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return builtinTools;
		return builtinTools.filter(
			(tool) =>
				tool.name.toLowerCase().includes(needle) ||
				(tool.description ?? '').toLowerCase().includes(needle)
		);
	});
	let filteredCustom = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return customTools;
		return customTools.filter(
			(tool) =>
				tool.name.toLowerCase().includes(needle) ||
				tool.command.toLowerCase().includes(needle) ||
				(tool.args?.join(' ') ?? '').toLowerCase().includes(needle)
		);
	});
	let filteredMCP = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return mcpServers;
		return mcpServers.filter(
			(server) =>
				server.name.toLowerCase().includes(needle) ||
				server.command.toLowerCase().includes(needle) ||
				(server.args?.join(' ') ?? '').toLowerCase().includes(needle)
		);
	});
	let hasMatches = $derived(
		filteredBuiltin.length + filteredCustom.length + filteredMCP.length > 0
	);

	let navItems = $derived([
		...filteredBuiltin.map((tool) => ({
			id: `builtin:${tool.name}`,
			activate: () => void toggleBuiltin(tool.name, !tool.enabled)
		})),
		...filteredCustom.map((tool) => ({
			id: `custom:${tool.name}`,
			activate: () => startEditCustom(tool)
		})),
		...filteredMCP.map((server) => ({
			id: `mcp:${server.name}`,
			activate: () => startEditMCP(server)
		}))
	]);

	const searchNav = createSearchNav({
		items: () => navItems,
		query: () => query,
		clear: () => (query = '')
	});

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			view = await toolsApi.list(currentAgent);
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

	async function toggleBuiltin(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.toggleBuiltin(agent, name, enabled);
			view = {
				...view,
				builtin: view.builtin.map((tool) => (tool.name === name ? { ...tool, enabled } : tool))
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			// Put the checkbox back in step with what the server kept.
			await load(agent);
		}
	}

	async function toggleCustom(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.patchCustom(agent, name, { enabled });
			view = {
				...view,
				custom: view.custom.map((tool) => (tool.name === name ? { ...tool, enabled } : tool))
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	async function toggleMCP(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsApi.patchMCP(agent, name, { enabled });
			view = {
				...view,
				mcp_servers: view.mcp_servers.map((server) =>
					server.name === name ? { ...server, enabled } : server
				)
			};
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			await load(agent);
		}
	}

	function toastToggle(name: string, enabled: boolean) {
		toastStore.success(
			enabled ? t('myaaw.tools.enabled_ok', { name }) : t('myaaw.tools.disabled_ok', { name })
		);
	}

	function startEditCustom(tool: CustomTool) {
		editingCustom = tool;
		revealPane(detail);
	}

	function startEditMCP(server: MCPServer) {
		editingMCP = server;
		revealPane(detail);
	}

	async function remove(kind: 'custom' | 'mcp', name: string) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.tools.delete_confirm', { name }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			if (kind === 'custom') await toolsApi.removeCustom(agent, name);
			else await toolsApi.removeMCP(agent, name);
			if (kind === 'custom' && editingCustom?.name === name) editingCustom = null;
			if (kind === 'mcp' && editingMCP?.name === name) editingMCP = null;
			await load(agent);
			toastStore.success(t('myaaw.tools.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<svelte:window onkeydown={searchNav.onKeydown} />

<TopBarActions id="tools-refresh">
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
		<!-- Everything registered for the agent: a second sidebar nested inside the main one. -->
		<aside
			class="flex shrink-0 flex-col border-b bg-card frost-aside lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 space-y-2 border-b px-3 py-2.5">
				<SidebarHeader label={t('myaaw.tools.title')} count={totalTools} />
				<SidebarSearchField
					bind:inputRef={searchNav.input}
					bind:value={query}
					placeholder={t('common.search')}
				/>
			</div>

			<div bind:this={searchNav.list} class="space-y-4 p-2 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
				{#if loading}
					<SidebarSkeletons />
				{:else if query && !hasMatches}
					<p class="px-1 py-2 text-sm text-muted-foreground">
						{t('myaaw.tools.no_matches')}
					</p>
				{/if}

				{#if !loading && (filteredBuiltin.length > 0 || !query)}
					<SidebarSection
						icon={Wrench}
						label={t('myaaw.tools.builtin')}
						count={filteredBuiltin.length}
					>
						{#if filteredBuiltin.length === 0}
							<p class="px-1 py-1 text-xs text-muted-foreground">{t('myaaw.tools.empty')}</p>
						{/if}
						{#each filteredBuiltin as tool (tool.name)}
							<label
								class={[
									'flex items-start gap-2 rounded-md px-1 py-1.5 text-sm hover:bg-muted list-row',
									searchNav.cursor === `builtin:${tool.name}` ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={`builtin:${tool.name}`}
								title={tool.description}
							>
								<input
									type="checkbox"
									checked={tool.enabled}
									onchange={(event) => toggleBuiltin(tool.name, event.currentTarget.checked)}
									class="mt-0.5 h-4 w-4 shrink-0 accent-primary"
								/>
								<span class="min-w-0 flex-1">
									<span class="block truncate font-mono text-xs">{tool.name}</span>
									{#if tool.description}
										<span class="block truncate text-[10px] text-muted-foreground">
											{tool.description}
										</span>
									{/if}
								</span>
							</label>
						{/each}
					</SidebarSection>
				{/if}

				{#if !loading && (filteredCustom.length > 0 || !query)}
					<SidebarSection
						icon={FileCode2}
						label={t('myaaw.tools.custom')}
						count={filteredCustom.length}
					>
						{#if filteredCustom.length === 0}
							<p class="px-1 py-1 text-xs text-muted-foreground">{t('myaaw.tools.empty')}</p>
						{/if}
						{#each filteredCustom as tool (tool.name)}
							<div
								class={[
									'flex items-start gap-2 rounded-md px-1 py-1.5 hover:bg-muted list-row',
									editingCustom?.name === tool.name ? 'bg-accent' : '',
									searchNav.cursor === `custom:${tool.name}` ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={`custom:${tool.name}`}
								aria-current={editingCustom?.name === tool.name ? 'true' : undefined}
							>
								<input
									type="checkbox"
									checked={tool.enabled}
									onchange={(event) => toggleCustom(tool.name, event.currentTarget.checked)}
									aria-label={tool.name}
									class="mt-0.5 h-4 w-4 shrink-0 accent-primary"
								/>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-1.5">
										<span class="min-w-0 truncate font-mono text-xs">{tool.name}</span>
										{#if !tool.enabled}
											<Badge variant="outline" class="shrink-0 text-[10px]">
												{t('myaaw.tools.disabled')}
											</Badge>
										{/if}
									</div>
									<p
										class="truncate text-[10px] text-muted-foreground"
										title={`${tool.command} ${tool.args?.join(' ') ?? ''}`}
									>
										{tool.command}
										{tool.args?.join(' ') ?? ''}
									</p>
								</div>
								<Button
									size="icon-xs"
									variant="ghost"
									class="mt-0.5 shrink-0"
									onclick={() => startEditCustom(tool)}
									aria-label={t('common.edit')}
								>
									<Pencil class="h-3 w-3" />
								</Button>
								<Button
									size="icon-xs"
									variant="ghost"
									class="mt-0.5 shrink-0"
									onclick={() => remove('custom', tool.name)}
									aria-label={t('common.delete')}
								>
									<Trash2 class="h-3 w-3 text-destructive" />
								</Button>
							</div>
						{/each}
					</SidebarSection>
				{/if}

				{#if !loading && (filteredMCP.length > 0 || !query)}
					<SidebarSection icon={Plug} label={t('myaaw.tools.mcp')} count={filteredMCP.length}>
						{#if filteredMCP.length === 0}
							<p class="px-1 py-1 text-xs text-muted-foreground">{t('myaaw.tools.empty')}</p>
						{/if}
						{#each filteredMCP as server (server.name)}
							<div
								class={[
									'flex items-start gap-2 rounded-md px-1 py-1.5 hover:bg-muted list-row',
									editingMCP?.name === server.name ? 'bg-accent' : '',
									searchNav.cursor === `mcp:${server.name}` ? 'ring-1 ring-ring/50' : ''
								]}
								data-nav-id={`mcp:${server.name}`}
								aria-current={editingMCP?.name === server.name ? 'true' : undefined}
							>
								<input
									type="checkbox"
									checked={server.enabled}
									onchange={(event) => toggleMCP(server.name, event.currentTarget.checked)}
									aria-label={server.name}
									class="mt-0.5 h-4 w-4 shrink-0 accent-primary"
								/>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-1.5">
										<span class="min-w-0 truncate font-mono text-xs">{server.name}</span>
										{#if !server.enabled}
											<Badge variant="outline" class="shrink-0 text-[10px]">
												{t('myaaw.tools.disabled')}
											</Badge>
										{/if}
									</div>
									<p
										class="truncate text-[10px] text-muted-foreground"
										title={`${server.command} ${server.args?.join(' ') ?? ''}`}
									>
										{server.command}
										{server.args?.join(' ') ?? ''}
									</p>
								</div>
								<Button
									size="icon-xs"
									variant="ghost"
									class="mt-0.5 shrink-0"
									onclick={() => startEditMCP(server)}
									aria-label={t('common.edit')}
								>
									<Pencil class="h-3 w-3" />
								</Button>
								<Button
									size="icon-xs"
									variant="ghost"
									class="mt-0.5 shrink-0"
									onclick={() => remove('mcp', server.name)}
									aria-label={t('common.delete')}
								>
									<Trash2 class="h-3 w-3 text-destructive" />
								</Button>
							</div>
						{/each}
					</SidebarSection>
				{/if}
			</div>
		</aside>

		<section
			bind:this={detail}
			class="flex flex-col gap-4 p-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:p-6"
		>
			{#if error}
				<div
					class="shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			<Card class="mx-auto w-full max-w-3xl shrink-0">
				<CardHeader>
					<CardTitle class="flex items-center gap-1.5">
						<FileCode2 class="h-4 w-4 shrink-0 text-muted-foreground" />
						{editingCustom ? editingCustom.name : t('myaaw.tools.custom')}
					</CardTitle>
					<CardDescription>{t('myaaw.tools.custom_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					{#key editingCustom?.name ?? 'new-custom'}
						<CustomToolEditor
							{agent}
							editing={editingCustom}
							onSaved={() => {
								editingCustom = null;
								void load(agent);
							}}
							onCancel={() => (editingCustom = null)}
						/>
					{/key}
				</CardContent>
			</Card>

			<Card class="mx-auto w-full max-w-3xl shrink-0">
				<CardHeader>
					<CardTitle class="flex items-center gap-1.5">
						<Plug class="h-4 w-4 shrink-0 text-muted-foreground" />
						{editingMCP ? editingMCP.name : t('myaaw.tools.mcp')}
					</CardTitle>
					<CardDescription>{t('myaaw.tools.mcp_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					{#key editingMCP?.name ?? 'new-mcp'}
						<McpServerEditor
							{agent}
							editing={editingMCP}
							onSaved={() => {
								editingMCP = null;
								void load(agent);
							}}
							onCancel={() => (editingMCP = null)}
						/>
					{/key}
				</CardContent>
			</Card>
		</section>
	{/if}
</div>
