<script lang="ts">
	import { tools as toolsApi } from '$lib/apis/tools';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { customToolSchema, mcpServerSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Pencil, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { CustomTool, MCPServer, ToolsView } from '$lib/types';

	const { t } = useTranslation();

	let view = $state<ToolsView | null>(null);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadKey = '';

	let customName = $state('');
	let customCommand = $state('');
	let customArgs = $state('');
	let customSchema = $state('');
	let mcpName = $state('');
	let mcpCommand = $state('');
	let mcpArgs = $state('');
	let mcpEnv = $state('');

	let editingCustom = $state('');
	let editingMCP = $state('');
	let customErrors = $state<FieldErrors>({});
	let mcpErrors = $state<FieldErrors>({});

	let agent = $derived(agentSelection.selected);

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

	function splitArgs(value: string) {
		return value
			.split(',')
			.map((part) => part.trim())
			.filter(Boolean);
	}

	function parseEnv(value: string) {
		const env: Record<string, string> = {};
		for (const line of value.split('\n')) {
			const index = line.indexOf('=');
			if (index > 0) env[line.slice(0, index).trim()] = line.slice(index + 1).trim();
		}
		return env;
	}

	async function toggleBuiltin(name: string, enabled: boolean) {
		if (!agent) return;
		try {
			await toolsApi.toggleBuiltin(agent, name, enabled);
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function startEditCustom(tool: CustomTool) {
		editingCustom = tool.name;
		customName = tool.name;
		customCommand = tool.command;
		customArgs = (tool.args ?? []).join(', ');
		customSchema = tool.schema ? JSON.stringify(tool.schema, null, 2) : '';
		customErrors = {};
	}

	function cancelEditCustom() {
		editingCustom = '';
		customName = '';
		customCommand = '';
		customArgs = '';
		customSchema = '';
		customErrors = {};
	}

	function startEditMCP(server: MCPServer) {
		editingMCP = server.name;
		mcpName = server.name;
		mcpCommand = server.command;
		mcpArgs = (server.args ?? []).join(', ');
		mcpEnv = '';
		mcpErrors = {};
	}

	function cancelEditMCP() {
		editingMCP = '';
		mcpName = '';
		mcpCommand = '';
		mcpArgs = '';
		mcpEnv = '';
		mcpErrors = {};
	}

	async function saveCustom() {
		if (!agent) return;
		const result = validate(customToolSchema, {
			name: customName,
			command: customCommand,
			args: customArgs,
			schema: customSchema
		});
		if (!result.ok) {
			customErrors = result.errors;
			return;
		}
		customErrors = {};
		saving = true;
		try {
			const schema = result.data.schema.trim() ? JSON.parse(result.data.schema) : undefined;
			if (editingCustom) {
				await toolsApi.patchCustom(agent, editingCustom, {
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					schema
				});
			} else {
				await toolsApi.createCustom(agent, {
					name: result.data.name,
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					schema,
					enabled: true
				});
			}
			cancelEditCustom();
			toastStore.success(t('myaaw.tools.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function saveMCP() {
		if (!agent) return;
		const result = validate(mcpServerSchema, {
			name: mcpName,
			command: mcpCommand,
			args: mcpArgs,
			env: mcpEnv
		});
		if (!result.ok) {
			mcpErrors = result.errors;
			return;
		}
		mcpErrors = {};
		saving = true;
		try {
			const env = parseEnv(result.data.env);
			if (editingMCP) {
				await toolsApi.patchMCP(agent, editingMCP, {
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					...(Object.keys(env).length > 0 ? { env } : {})
				});
			} else {
				await toolsApi.createMCP(agent, {
					name: result.data.name,
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					env,
					enabled: true
				});
			}
			cancelEditMCP();
			toastStore.success(t('myaaw.tools.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
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
			if (kind === 'custom' && editingCustom === name) cancelEditCustom();
			if (kind === 'mcp' && editingMCP === name) cancelEditMCP();
			await load(agent);
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
		<!-- Everything registered for the agent: a second sidebar nested inside the main one. -->
		<aside
			class="flex h-[45vh] shrink-0 flex-col border-b bg-card lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 border-b px-3 py-2.5">
				<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
					{t('myaaw.tools.title')}
				</p>
			</div>

			<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-2">
				<section class="space-y-1">
					<p class="px-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t('myaaw.tools.builtin')}
					</p>
					{#each view?.builtin ?? [] as tool (tool.name)}
						<label
							class="flex items-center gap-2 rounded-md px-1 py-1 text-sm hover:bg-muted"
							title={tool.description}
						>
							<input
								type="checkbox"
								checked={tool.enabled}
								onchange={(event) => toggleBuiltin(tool.name, event.currentTarget.checked)}
								class="h-4 w-4 shrink-0 accent-primary"
							/>
							<span class="min-w-0 flex-1 truncate font-mono text-xs">{tool.name}</span>
						</label>
					{/each}
				</section>

				<section class="space-y-1">
					<p class="px-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t('myaaw.tools.custom')}
					</p>
					{#each view?.custom ?? [] as tool (tool.name)}
						<div class="flex items-start gap-1 rounded-md px-1 py-1 hover:bg-muted">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<span class="min-w-0 truncate font-mono text-xs">{tool.name}</span>
									<Badge
										variant={tool.enabled ? 'secondary' : 'outline'}
										class="shrink-0 text-[10px]"
									>
										{tool.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
									</Badge>
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
					{:else}
						<p class="px-1 py-1 text-xs text-muted-foreground">{t('myaaw.tools.empty')}</p>
					{/each}
				</section>

				<section class="space-y-1">
					<p class="px-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t('myaaw.tools.mcp')}
					</p>
					{#each view?.mcp_servers ?? [] as server (server.name)}
						<div class="flex items-start gap-1 rounded-md px-1 py-1 hover:bg-muted">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<span class="min-w-0 truncate font-mono text-xs">{server.name}</span>
									<Badge
										variant={server.enabled ? 'secondary' : 'outline'}
										class="shrink-0 text-[10px]"
									>
										{server.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
									</Badge>
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
					{:else}
						<p class="px-1 py-1 text-xs text-muted-foreground">{t('myaaw.tools.empty')}</p>
					{/each}
				</section>
			</div>
		</aside>

		<section class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 lg:p-6">
			{#if error}
				<div
					class="shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			<Card class="mx-auto w-full max-w-3xl shrink-0">
				<CardHeader>
					<CardTitle>
						{editingCustom ? editingCustom : t('myaaw.tools.custom')}
					</CardTitle>
					<CardDescription>{t('myaaw.tools.custom_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void saveCustom();
						}}
					>
						<div class="grid gap-1">
							<Label for="custom-name">{t('myaaw.tools.name')}</Label>
							<Input
								id="custom-name"
								bind:value={customName}
								placeholder="hello_helper"
								disabled={editingCustom !== ''}
							/>
							<FieldError code={customErrors.name} />
						</div>
						<div class="grid gap-1">
							<Label for="custom-command">{t('myaaw.tools.command')}</Label>
							<Input id="custom-command" bind:value={customCommand} placeholder="node" />
							<FieldError code={customErrors.command} />
						</div>
						<div class="grid gap-1">
							<Label for="custom-args">{t('myaaw.tools.args')}</Label>
							<Input
								id="custom-args"
								bind:value={customArgs}
								placeholder="/path/script.js, --flag"
							/>
						</div>
						<div class="grid gap-1">
							<Label for="custom-schema">{t('myaaw.tools.schema')}</Label>
							<textarea
								id="custom-schema"
								bind:value={customSchema}
								rows="4"
								placeholder={'{"type":"function","function":{"name":"hello_helper","parameters":{}}}'}
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
							<FieldError code={customErrors.schema} />
						</div>
						<div class="flex gap-2">
							<Button type="submit" size="sm" loading={saving}>
								{#if editingCustom}
									{t('common.save')}
								{:else}
									<Plus class="mr-2 h-4 w-4" />
									{t('myaaw.tools.create')}
								{/if}
							</Button>
							{#if editingCustom}
								<Button type="button" size="sm" variant="ghost" onclick={cancelEditCustom}>
									{t('common.cancel')}
								</Button>
							{/if}
						</div>
					</form>
				</CardContent>
			</Card>

			<Card class="mx-auto w-full max-w-3xl shrink-0">
				<CardHeader>
					<CardTitle>{editingMCP ? editingMCP : t('myaaw.tools.mcp')}</CardTitle>
					<CardDescription>{t('myaaw.tools.mcp_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void saveMCP();
						}}
					>
						<div class="grid gap-1">
							<Label for="mcp-name">{t('myaaw.tools.name')}</Label>
							<Input
								id="mcp-name"
								bind:value={mcpName}
								placeholder="playwright"
								disabled={editingMCP !== ''}
							/>
							<FieldError code={mcpErrors.name} />
						</div>
						<div class="grid gap-1">
							<Label for="mcp-command">{t('myaaw.tools.command')}</Label>
							<Input id="mcp-command" bind:value={mcpCommand} placeholder="npx" />
							<FieldError code={mcpErrors.command} />
						</div>
						<div class="grid gap-1">
							<Label for="mcp-args">{t('myaaw.tools.args')}</Label>
							<Input id="mcp-args" bind:value={mcpArgs} placeholder="-y, @playwright/mcp" />
						</div>
						<div class="grid gap-1">
							<Label for="mcp-env">{t('myaaw.tools.env')}</Label>
							<textarea
								id="mcp-env"
								bind:value={mcpEnv}
								rows="3"
								placeholder="TOKEN=secret"
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
							<FieldError code={mcpErrors.env} />
						</div>
						<div class="flex gap-2">
							<Button type="submit" size="sm" loading={saving}>
								{#if editingMCP}
									{t('common.save')}
								{:else}
									<Plus class="mr-2 h-4 w-4" />
									{t('myaaw.tools.create')}
								{/if}
							</Button>
							{#if editingMCP}
								<Button type="button" size="sm" variant="ghost" onclick={cancelEditMCP}>
									{t('common.cancel')}
								</Button>
							{/if}
						</div>
					</form>
				</CardContent>
			</Card>
		</section>
	{/if}
</div>
