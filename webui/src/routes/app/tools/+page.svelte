<script lang="ts">
	import { toolsStore } from '$lib/stores/tools.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { isExternalRuntimeType, isToolAvailableForRuntime } from '$lib/utils/runtimeTools';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import ToolDialog from '$lib/components/tools/ToolDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import ToolGridSkeleton from './ToolGridSkeleton.svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import {
		FileCode2,
		Pencil,
		Plug,
		Plus,
		RefreshCw,
		Search,
		Terminal,
		Trash2,
		Wrench
	} from 'lucide-svelte';
	import type { CustomTool, MCPServer } from '$lib/types';

	const { t } = useTranslation();

	let activeTab = $state<'builtin' | 'custom' | 'mcp'>('builtin');
	let query = $state('');

	let dialogOpen = $state(false);
	let dialogKind = $state<'custom' | 'mcp'>('custom');
	let editingCustom = $state<CustomTool | null>(null);
	let editingMCP = $state<MCPServer | null>(null);

	let agent = $derived(agentSelection.selected);
	useQuery(toolsStore.query, () => (agent ? { agent } : null));

	let isExternalAgent = $derived(isExternalRuntimeType(agentSelection.current?.runtime_type));
	let view = $derived(toolsStore.data ?? null);
	let loading = $derived(toolsStore.loading);
	let error = $derived(toolsStore.error);
	let builtinTools = $derived(
		(view?.builtin ?? []).filter((tool) => isToolAvailableForRuntime(tool.name, isExternalAgent))
	);
	let customTools = $derived(view?.custom ?? []);
	let mcpServers = $derived(view?.mcp_servers ?? []);
	let builtinEnabledCount = $derived(builtinTools.filter((tool) => tool.enabled).length);

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

	function openCreate(kind: 'custom' | 'mcp') {
		dialogKind = kind;
		editingCustom = null;
		editingMCP = null;
		dialogOpen = true;
	}

	function openEditCustom(tool: CustomTool) {
		dialogKind = 'custom';
		editingCustom = tool;
		editingMCP = null;
		dialogOpen = true;
	}

	function openEditMCP(server: MCPServer) {
		dialogKind = 'mcp';
		editingCustom = null;
		editingMCP = server;
		dialogOpen = true;
	}

	async function toggleBuiltin(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsStore.toggleBuiltin(agent, name, enabled);
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function toggleCustom(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsStore.patchCustom(agent, name, { enabled });
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function toggleMCP(name: string, enabled: boolean) {
		if (!agent || !view) return;
		try {
			await toolsStore.patchMCP(agent, name, { enabled });
			toastToggle(name, enabled);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function toastToggle(name: string, enabled: boolean) {
		toastStore.success(
			enabled ? t('myaaw.tools.enabled_ok', { name }) : t('myaaw.tools.disabled_ok', { name })
		);
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
			if (kind === 'custom') await toolsStore.removeCustom(agent, name);
			else await toolsStore.removeMCP(agent, name);
			toastStore.success(t('myaaw.tools.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="tools-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => void toolsStore.refresh()}
			loading={loading || toolsStore.refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto w-full max-w-5xl space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="rounded-xl border bg-card shadow-xs">
			<Tabs.Root bind:value={activeTab}>
				<div class="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
					<Tabs.List variant="line" class="flex-wrap justify-start">
						<Tabs.Trigger value="builtin" class="gap-1.5">
							<Wrench class="h-3.5 w-3.5" />
							{t('myaaw.tools.builtin')}
							<span
								class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] leading-none font-medium text-muted-foreground tabular-nums"
							>
								{builtinEnabledCount}/{builtinTools.length}
							</span>
						</Tabs.Trigger>
						<Tabs.Trigger value="custom" class="gap-1.5">
							<FileCode2 class="h-3.5 w-3.5" />
							{t('myaaw.tools.custom')}
							<span
								class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] leading-none font-medium text-muted-foreground tabular-nums"
							>
								{customTools.length}
							</span>
						</Tabs.Trigger>
						<Tabs.Trigger value="mcp" class="gap-1.5">
							<Plug class="h-3.5 w-3.5" />
							{t('myaaw.tools.mcp')}
							<span
								class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] leading-none font-medium text-muted-foreground tabular-nums"
							>
								{mcpServers.length}
							</span>
						</Tabs.Trigger>
					</Tabs.List>

					<div class="flex w-full items-center gap-2 sm:w-auto">
						<div class="relative flex-1 sm:w-52 sm:flex-none">
							<Search
								class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
							/>
							<Input bind:value={query} placeholder={t('common.search')} class="h-8 pl-8 text-xs" />
						</div>
						{#if activeTab === 'custom'}
							<Button
								size="sm"
								onclick={() => openCreate('custom')}
								class="shrink-0 gap-1.5 rounded-lg"
							>
								<Plus class="h-3.5 w-3.5" />
								{t('myaaw.tools.create')}
							</Button>
						{:else if activeTab === 'mcp'}
							<Button
								size="sm"
								onclick={() => openCreate('mcp')}
								class="shrink-0 gap-1.5 rounded-lg"
							>
								<Plus class="h-3.5 w-3.5" />
								{t('myaaw.tools.create')}
							</Button>
						{/if}
					</div>
				</div>

				<div class="p-4 sm:p-5">
					{#if loading}
						<ToolGridSkeleton />
					{:else}
						<!-- Built-in tools -->
						<Tabs.Content value="builtin" class="mt-0 space-y-3">
							{#if isExternalAgent}
								<p
									class="rounded-lg border border-border/70 bg-muted/35 px-3 py-2 text-xs leading-relaxed text-muted-foreground"
								>
									{t('myaaw.plugins.tools_external_notice')}
								</p>
							{/if}
							{#if builtinTools.length === 0}
								<EmptyState
									title={t('myaaw.tools.empty')}
									description={t('myaaw.tools.builtin_desc')}
									bloub="somnolent"
									icon={Wrench}
									class="min-h-60"
								/>
							{:else if filteredBuiltin.length === 0}
								<p class="py-10 text-center text-sm text-muted-foreground">
									{t('myaaw.tools.no_matches')}
								</p>
							{:else}
								<div class="grid gap-2.5 sm:grid-cols-2">
									{#each filteredBuiltin as tool (tool.name)}
										<div
											class="flex items-start justify-between gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20"
										>
											<div class="min-w-0 flex-1 space-y-1">
												<div class="flex items-center gap-2">
													<span
														aria-hidden="true"
														class={[
															'h-2 w-2 shrink-0 rounded-full',
															tool.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/35'
														]}
													></span>
													<span class="truncate font-mono text-xs font-semibold text-foreground">
														{tool.name}
													</span>
												</div>
												{#if tool.description}
													<p class="line-clamp-2 text-xs text-muted-foreground">
														{tool.description}
													</p>
												{/if}
											</div>
											<Switch
												checked={tool.enabled}
												label={tool.name}
												onchange={(next) => toggleBuiltin(tool.name, next)}
											/>
										</div>
									{/each}
								</div>
							{/if}
						</Tabs.Content>

						<!-- Custom tools -->
						<Tabs.Content value="custom" class="mt-0">
							{#if customTools.length === 0}
								<EmptyState
									title={t('myaaw.tools.empty')}
									description={t('myaaw.tools.custom_desc')}
									bloub="curieux"
									icon={FileCode2}
									class="min-h-60"
								/>
							{:else if filteredCustom.length === 0}
								<p class="py-10 text-center text-sm text-muted-foreground">
									{t('myaaw.tools.no_matches')}
								</p>
							{:else}
								<div class="space-y-2.5">
									{#each filteredCustom as tool (tool.name)}
										<div
											class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
										>
											<div class="min-w-0 flex-1 space-y-1">
												<div class="flex flex-wrap items-center gap-2">
													<span
														aria-hidden="true"
														class={[
															'h-2 w-2 shrink-0 rounded-full',
															tool.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/35'
														]}
													></span>
													<span class="font-mono text-sm font-semibold text-foreground">
														{tool.name}
													</span>
													<Badge
														variant={tool.enabled ? 'secondary' : 'outline'}
														class="text-[10px]"
													>
														{tool.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
													</Badge>
												</div>
												<p
													class="flex items-center gap-1.5 truncate font-mono text-[11px] text-muted-foreground"
													title={`${tool.command} ${tool.args?.join(' ') ?? ''}`}
												>
													<Terminal class="h-3 w-3 shrink-0 opacity-70" />
													<span class="truncate">
														{tool.command}
														{tool.args?.join(' ') ?? ''}
													</span>
												</p>
											</div>

											<div class="flex shrink-0 items-center justify-end gap-1.5">
												<Switch
													checked={tool.enabled}
													label={tool.name}
													onchange={(next) => toggleCustom(tool.name, next)}
												/>
												<Button
													size="icon-sm"
													variant="ghost"
													onclick={() => openEditCustom(tool)}
													aria-label={t('common.edit')}
													title={t('common.edit')}
												>
													<Pencil class="h-3.5 w-3.5" />
												</Button>
												<Button
													size="icon-sm"
													variant="ghost"
													onclick={() => remove('custom', tool.name)}
													aria-label={t('common.delete')}
													title={t('common.delete')}
												>
													<Trash2 class="h-3.5 w-3.5 text-destructive" />
												</Button>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</Tabs.Content>

						<!-- MCP servers -->
						<Tabs.Content value="mcp" class="mt-0">
							{#if mcpServers.length === 0}
								<EmptyState
									title={t('myaaw.tools.empty')}
									description={t('myaaw.tools.mcp_desc')}
									bloub="curieux"
									icon={Plug}
									class="min-h-60"
								/>
							{:else if filteredMCP.length === 0}
								<p class="py-10 text-center text-sm text-muted-foreground">
									{t('myaaw.tools.no_matches')}
								</p>
							{:else}
								<div class="space-y-2.5">
									{#each filteredMCP as server (server.name)}
										<div
											class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
										>
											<div class="min-w-0 flex-1 space-y-1">
												<div class="flex flex-wrap items-center gap-2">
													<span
														aria-hidden="true"
														class={[
															'h-2 w-2 shrink-0 rounded-full',
															server.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/35'
														]}
													></span>
													<span class="font-mono text-sm font-semibold text-foreground">
														{server.name}
													</span>
													<Badge
														variant={server.enabled ? 'secondary' : 'outline'}
														class="text-[10px]"
													>
														{server.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
													</Badge>
												</div>
												<p
													class="flex items-center gap-1.5 truncate font-mono text-[11px] text-muted-foreground"
													title={`${server.command} ${server.args?.join(' ') ?? ''}`}
												>
													<Terminal class="h-3 w-3 shrink-0 opacity-70" />
													<span class="truncate">
														{server.command}
														{server.args?.join(' ') ?? ''}
													</span>
												</p>
											</div>

											<div class="flex shrink-0 items-center justify-end gap-1.5">
												<Switch
													checked={server.enabled}
													label={server.name}
													onchange={(next) => toggleMCP(server.name, next)}
												/>
												<Button
													size="icon-sm"
													variant="ghost"
													onclick={() => openEditMCP(server)}
													aria-label={t('common.edit')}
													title={t('common.edit')}
												>
													<Pencil class="h-3.5 w-3.5" />
												</Button>
												<Button
													size="icon-sm"
													variant="ghost"
													onclick={() => remove('mcp', server.name)}
													aria-label={t('common.delete')}
													title={t('common.delete')}
												>
													<Trash2 class="h-3.5 w-3.5 text-destructive" />
												</Button>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</Tabs.Content>
					{/if}
				</div>
			</Tabs.Root>
		</div>
	{/if}

	<ToolDialog
		open={dialogOpen}
		agent={agent ?? ''}
		kind={dialogKind}
		{editingCustom}
		{editingMCP}
		onOpenChange={(next) => (dialogOpen = next)}
	/>
</div>
