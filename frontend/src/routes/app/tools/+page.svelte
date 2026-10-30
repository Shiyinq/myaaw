<script lang="ts">
	import { tools as toolsApi } from '$lib/apis/tools';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Plus, RefreshCw, Trash2, Wrench } from 'lucide-svelte';
	import type { ToolsView } from '$lib/types';

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

	async function createCustom() {
		if (!agent || !customName.trim() || !customCommand.trim()) return;
		saving = true;
		try {
			let schema: unknown = undefined;
			if (customSchema.trim()) schema = JSON.parse(customSchema);
			await toolsApi.createCustom(agent, {
				name: customName.trim(),
				command: customCommand.trim(),
				args: splitArgs(customArgs),
				schema,
				enabled: true
			});
			customName = '';
			customCommand = '';
			customArgs = '';
			customSchema = '';
			toastStore.success(t('myaaw.tools.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function createMCP() {
		if (!agent || !mcpName.trim() || !mcpCommand.trim()) return;
		saving = true;
		try {
			await toolsApi.createMCP(agent, {
				name: mcpName.trim(),
				command: mcpCommand.trim(),
				args: splitArgs(mcpArgs),
				env: parseEnv(mcpEnv),
				enabled: true
			});
			mcpName = '';
			mcpCommand = '';
			mcpArgs = '';
			mcpEnv = '';
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
		if (!window.confirm(t('myaaw.tools.delete_confirm', { name }))) return;
		try {
			if (kind === 'custom') await toolsApi.removeCustom(agent, name);
			else await toolsApi.removeMCP(agent, name);
			await load(agent);
			toastStore.success(t('myaaw.tools.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<div class="space-y-4">
	<PageHeader title={t('myaaw.tools.title')} description={t('myaaw.tools.subtitle')}>
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

		<div class="grid gap-4 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<CardTitle class="flex items-center gap-2">
						<Wrench class="h-4 w-4" />
						{t('myaaw.tools.builtin')}
					</CardTitle>
					<CardDescription>{t('myaaw.tools.builtin_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-2">
					{#each view?.builtin ?? [] as tool (tool.name)}
						<label class="flex items-center justify-between gap-2 rounded-md px-1 py-1 text-sm">
							<span class="flex items-center gap-2">
								<input
									type="checkbox"
									checked={tool.enabled}
									onchange={(event) => toggleBuiltin(tool.name, event.currentTarget.checked)}
									class="h-4 w-4 accent-primary"
								/>
								<span class="font-mono">{tool.name}</span>
							</span>
							<span class="truncate text-[11px] text-muted-foreground">{tool.description}</span>
						</label>
					{/each}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>{t('myaaw.tools.custom')}</CardTitle>
					<CardDescription>{t('myaaw.tools.custom_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-3">
					{#each view?.custom ?? [] as tool (tool.name)}
						<div class="flex items-center justify-between gap-2 rounded-md border p-2 text-sm">
							<div class="min-w-0">
								<div class="flex items-center gap-2">
									<span class="font-mono">{tool.name}</span>
									<Badge variant={tool.enabled ? 'secondary' : 'outline'}>
										{tool.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
									</Badge>
								</div>
								<p class="truncate text-[11px] text-muted-foreground">
									{tool.command} {tool.args?.join(' ') ?? ''}
								</p>
							</div>
							<Button size="icon-sm" variant="ghost" onclick={() => remove('custom', tool.name)}>
								<Trash2 class="h-3.5 w-3.5 text-destructive" />
							</Button>
						</div>
					{:else}
						<p class="text-sm text-muted-foreground">{t('myaaw.tools.empty')}</p>
					{/each}

					<Separator />
					<form
						class="space-y-2"
						onsubmit={(event) => {
							event.preventDefault();
							void createCustom();
						}}
					>
						<div class="grid gap-1">
							<Label for="custom-name">{t('myaaw.tools.name')}</Label>
							<Input id="custom-name" bind:value={customName} placeholder="hello_helper" />
						</div>
						<div class="grid gap-1">
							<Label for="custom-command">{t('myaaw.tools.command')}</Label>
							<Input id="custom-command" bind:value={customCommand} placeholder="node" />
						</div>
						<div class="grid gap-1">
							<Label for="custom-args">{t('myaaw.tools.args')}</Label>
							<Input id="custom-args" bind:value={customArgs} placeholder="/path/script.js, --flag" />
						</div>
						<div class="grid gap-1">
							<Label for="custom-schema">{t('myaaw.tools.schema')}</Label>
							<textarea
								id="custom-schema"
								bind:value={customSchema}
								rows="3"
								placeholder={'{"type":"function","function":{"name":"hello_helper","parameters":{}}}'}
								class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							></textarea>
						</div>
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.tools.create')}
						</Button>
					</form>
				</CardContent>
			</Card>

			<Card class="lg:col-span-2">
				<CardHeader>
					<CardTitle>{t('myaaw.tools.mcp')}</CardTitle>
					<CardDescription>{t('myaaw.tools.mcp_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						{#each view?.mcp_servers ?? [] as server (server.name)}
							<div class="flex items-center justify-between gap-2 rounded-md border p-2 text-sm">
								<div class="min-w-0">
									<div class="flex items-center gap-2">
										<span class="font-mono">{server.name}</span>
										<Badge variant={server.enabled ? 'secondary' : 'outline'}>
											{server.enabled ? t('myaaw.tools.enabled') : t('myaaw.tools.disabled')}
										</Badge>
									</div>
									<p class="truncate text-[11px] text-muted-foreground">
										{server.command} {server.args?.join(' ') ?? ''}
									</p>
								</div>
								<Button size="icon-sm" variant="ghost" onclick={() => remove('mcp', server.name)}>
									<Trash2 class="h-3.5 w-3.5 text-destructive" />
								</Button>
							</div>
						{:else}
							<p class="text-sm text-muted-foreground">{t('myaaw.tools.empty')}</p>
						{/each}
					</div>

					<form
						class="space-y-2"
						onsubmit={(event) => {
							event.preventDefault();
							void createMCP();
						}}
					>
						<div class="grid gap-1">
							<Label for="mcp-name">{t('myaaw.tools.name')}</Label>
							<Input id="mcp-name" bind:value={mcpName} placeholder="playwright" />
						</div>
						<div class="grid gap-1">
							<Label for="mcp-command">{t('myaaw.tools.command')}</Label>
							<Input id="mcp-command" bind:value={mcpCommand} placeholder="npx" />
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
						</div>
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.tools.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
