<script lang="ts">
	import { onMount } from 'svelte';
	import { backups as backupsApi } from '$lib/apis/backup';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardAction,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { Input } from '$lib/components/ui/input';
	import {
		Archive,
		Download,
		HardDriveDownload,
		RefreshCw,
		RotateCcw,
		Trash2
	} from 'lucide-svelte';
	import type { BackupArchive, BackupList } from '$lib/types';

	interface Props {
		/**
		 * When set, the panel is scoped to this agent: the list only shows its
		 * archives and every new archive contains it alone. Leave empty on the
		 * fleet page, where all archives are managed together.
		 */
		agent?: string;
	}

	let { agent }: Props = $props();

	const { t } = useTranslation();

	let loading = $state(false);
	let creating = $state(false);
	let restoring = $state(false);
	let error = $state('');
	let data = $state<BackupList | null>(null);

	let showCreate = $state(false);
	let allAgents = $state(true);
	let chosenAgents = $state<string[]>([]);
	let includeWorkspace = $state(false);
	let includeLogs = $state(false);
	let includeVenv = $state(false);
	let includeCaches = $state(false);
	let noSecrets = $state(false);

	let restoreName = $state('');
	let restoreForce = $state(true);
	let restoreStart = $state(true);
	let restoreAs = $state('');
	let confirmDelete = $state('');

	let fleet = $derived(agentSelection.agents);
	let scoped = $derived(Boolean(agent));
	// The API returns every archive in the directory; an agent page only shows
	// the ones that belong to its agent. The global archive stays in the fleet.
	let archives = $derived(
		(data?.archives ?? []).filter((archive) => !scoped || archive.agent === agent)
	);

	function toggleCreate() {
		showCreate = !showCreate;
		if (!showCreate) return;
		// Start from the agent in the sidebar on the fleet page; the CLI defaults
		// to every agent, so "all" stays the default here as well.
		if (!scoped && chosenAgents.length === 0 && agent) chosenAgents = [agent];
	}

	function toggleAgent(name: string, checked: boolean) {
		chosenAgents = checked
			? [...chosenAgents.filter((item) => item !== name), name]
			: chosenAgents.filter((item) => item !== name);
	}

	onMount(refresh);

	async function refresh() {
		loading = true;
		error = '';
		try {
			data = await backupsApi.list();
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	async function create() {
		// Names can disappear between opening the form and submitting it.
		const targets = chosenAgents.filter((name) => fleet.some((item) => item.name === name));
		if (scoped && !agent) return;
		if (!scoped && !allAgents && targets.length === 0) {
			toastStore.error(t('myaaw.backups.pick_agent'));
			return;
		}

		creating = true;
		error = '';
		try {
			await backupsApi.create({
				agents: scoped ? [agent as string] : allAgents ? [] : targets,
				include_workspace: includeWorkspace,
				include_logs: includeLogs,
				include_venv: includeVenv,
				include_caches: includeCaches,
				no_secrets: noSecrets
			});
			toastStore.success(t('myaaw.backups.created_ok'));
			showCreate = false;
			await refresh();
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		} finally {
			creating = false;
		}
	}

	async function restore(archive: BackupArchive) {
		restoring = true;
		error = '';
		try {
			const result = await backupsApi.restore(archive.name, {
				agent: restoreAs.trim() || undefined,
				force: restoreForce,
				start: restoreStart
			});
			toastStore.success(t('myaaw.backups.restored_ok', { agent: result.agent }));
			if (result.port_warning) toastStore.error(result.port_warning);
			restoreName = '';
			await refresh();
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		} finally {
			restoring = false;
		}
	}

	async function remove(archive: BackupArchive) {
		error = '';
		try {
			await backupsApi.remove(archive.name);
			toastStore.success(t('myaaw.backups.deleted_ok'));
			confirmDelete = '';
			await refresh();
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		}
	}

	function formatBytes(value: number) {
		if (!value) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
		const size = value / 1024 ** exponent;
		return `${size.toFixed(exponent === 0 ? 0 : 1)} ${units[exponent]}`;
	}

	function formatDate(value: string) {
		const parsed = new Date(value);
		if (Number.isNaN(parsed.getTime())) return value;
		return parsed.toLocaleString();
	}
</script>

{#snippet createForm()}
	<div class="space-y-4">
		{#if scoped}
			<div class="flex flex-wrap items-center gap-2 text-sm">
				<span class="text-muted-foreground">{t('myaaw.backups.scope')}</span>
				<Badge variant="secondary">{agent}</Badge>
			</div>
		{:else}
			<div class="space-y-2">
				<Label>{t('myaaw.backups.scope')}</Label>
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={allAgents} class="h-4 w-4 accent-primary" />
					{t('myaaw.backups.scope_all')}
				</label>
				{#if !allAgents}
					<div class="flex flex-wrap gap-2">
						{#each fleet as item (item.name)}
							<label
								class="flex cursor-pointer items-center gap-2 rounded-md border px-2 py-1 text-sm hover:bg-muted/40"
							>
								<input
									type="checkbox"
									checked={chosenAgents.includes(item.name)}
									onchange={(event) => toggleAgent(item.name, event.currentTarget.checked)}
									class="h-4 w-4 accent-primary"
								/>
								{item.name}
							</label>
						{/each}
					</div>
					<p class="text-xs text-muted-foreground">
						{t('myaaw.backups.scope_agents_hint')}
					</p>
				{/if}
			</div>
		{/if}

		<div class="grid gap-2 sm:grid-cols-2">
			<label class="flex items-start gap-2 text-sm">
				<input
					type="checkbox"
					bind:checked={includeWorkspace}
					class="mt-0.5 h-4 w-4 accent-primary"
				/>
				<span>
					{t('myaaw.backups.include_workspace')}
					<span class="block text-xs text-muted-foreground">
						{t('myaaw.backups.include_workspace_hint')}
					</span>
				</span>
			</label>
			<label class="flex items-start gap-2 text-sm">
				<input type="checkbox" bind:checked={includeLogs} class="mt-0.5 h-4 w-4 accent-primary" />
				<span>
					{t('myaaw.backups.include_logs')}
					<span class="block text-xs text-muted-foreground">
						{t('myaaw.backups.include_logs_hint')}
					</span>
				</span>
			</label>
			<label class="flex items-start gap-2 text-sm">
				<input type="checkbox" bind:checked={includeVenv} class="mt-0.5 h-4 w-4 accent-primary" />
				<span>
					{t('myaaw.backups.include_venv')}
					<span class="block text-xs text-muted-foreground">
						{t('myaaw.backups.include_venv_hint')}
					</span>
				</span>
			</label>
			<label class="flex items-start gap-2 text-sm">
				<input type="checkbox" bind:checked={includeCaches} class="mt-0.5 h-4 w-4 accent-primary" />
				<span>
					{t('myaaw.backups.include_caches')}
					<span class="block text-xs text-muted-foreground">
						{t('myaaw.backups.include_caches_hint')}
					</span>
				</span>
			</label>
			<label class="flex items-start gap-2 text-sm">
				<input type="checkbox" bind:checked={noSecrets} class="mt-0.5 h-4 w-4 accent-primary" />
				<span>
					{t('myaaw.backups.no_secrets')}
					<span class="block text-xs text-muted-foreground">
						{t('myaaw.backups.no_secrets_hint')}
					</span>
				</span>
			</label>
		</div>

		<div class="flex flex-wrap items-center gap-2">
			<Button onclick={create} disabled={creating}>
				{creating ? t('myaaw.backups.creating') : t('myaaw.backups.create')}
			</Button>
			<Button variant="ghost" onclick={() => (showCreate = false)}>
				{t('common.cancel')}
			</Button>
		</div>
	</div>
{/snippet}

{#snippet archiveList()}
	{#if archives.length === 0}
		<EmptyState
			title={t('myaaw.backups.empty')}
			description={scoped
				? t('myaaw.backups.empty_desc_agent', { agent: agent ?? '' })
				: t('myaaw.backups.empty_desc')}
			icon={Archive}
		/>
	{:else}
		<div class="divide-y rounded-xl border">
			{#each archives as archive (archive.name)}
				<div class="p-4">
					<div class="flex flex-wrap items-start justify-between gap-3">
						<div class="min-w-0">
							<p class="truncate font-mono text-sm">{archive.name}</p>
							<p class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
								<Badge variant="secondary">{archive.agent}</Badge>
								<span>{formatBytes(archive.size)}</span>
								<span>·</span>
								<span>{formatDate(archive.created_at)}</span>
							</p>
						</div>
						<div class="flex shrink-0 items-center gap-1">
							<Button
								variant="ghost"
								size="icon-sm"
								onclick={() => backupsApi.download(archive.name)}
								aria-label={t('myaaw.backups.download')}
								title={t('myaaw.backups.download')}
							>
								<Download class="h-4 w-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								onclick={() => {
									restoreName = restoreName === archive.name ? '' : archive.name;
									restoreAs = '';
									confirmDelete = '';
								}}
								aria-label={t('myaaw.backups.restore')}
								title={t('myaaw.backups.restore')}
							>
								<RotateCcw class="h-4 w-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								onclick={() => {
									confirmDelete = confirmDelete === archive.name ? '' : archive.name;
									restoreName = '';
								}}
								aria-label={t('common.delete')}
								title={t('common.delete')}
							>
								<Trash2 class="h-4 w-4" />
							</Button>
						</div>
					</div>

					{#if restoreName === archive.name}
						<div class="mt-3 space-y-3 rounded-lg border bg-muted/30 p-3">
							<p class="text-xs text-muted-foreground">
								{t('myaaw.backups.restore_hint', { agent: archive.agent })}
							</p>
							<div class="grid gap-1 sm:max-w-xs">
								<Label for="restore-as-{archive.name}">
									{t('myaaw.backups.restore_as')}
								</Label>
								<Input
									id="restore-as-{archive.name}"
									bind:value={restoreAs}
									placeholder={archive.agent}
								/>
								<span class="text-xs text-muted-foreground">
									{t('myaaw.backups.restore_as_hint', { agent: archive.agent })}
								</span>
							</div>
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={restoreForce} class="h-4 w-4 accent-primary" />
								{t('myaaw.backups.restore_force')}
							</label>
							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={restoreStart} class="h-4 w-4 accent-primary" />
								{t('myaaw.backups.restore_start')}
							</label>
							<div class="flex flex-wrap gap-2">
								<Button size="sm" onclick={() => restore(archive)} disabled={restoring}>
									{restoring ? t('myaaw.backups.restoring') : t('myaaw.backups.restore')}
								</Button>
								<Button size="sm" variant="ghost" onclick={() => (restoreName = '')}>
									{t('common.cancel')}
								</Button>
							</div>
						</div>
					{/if}

					{#if confirmDelete === archive.name}
						<div
							class="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-3"
						>
							<p class="text-xs text-destructive">{t('myaaw.backups.delete_confirm')}</p>
							<div class="flex gap-2">
								<Button size="sm" variant="destructive" onclick={() => remove(archive)}>
									{t('common.delete')}
								</Button>
								<Button size="sm" variant="ghost" onclick={() => (confirmDelete = '')}>
									{t('common.cancel')}
								</Button>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
{/snippet}

<div class="flex flex-col gap-4">
	{#if error}
		<div
			class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{error}
		</div>
	{/if}

	{#if scoped}
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.backups.dir')}</CardTitle>
				<CardDescription>{t('myaaw.backups.dir_hint')}</CardDescription>
				<CardAction class="flex items-center gap-1">
					<Button
						variant={showCreate ? 'secondary' : 'default'}
						size="sm"
						onclick={toggleCreate}
						disabled={!agent}
					>
						<HardDriveDownload class="mr-1.5 h-4 w-4" />
						{t('myaaw.backups.create')}
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						onclick={refresh}
						{loading}
						aria-label={t('common.refresh')}
						title={t('common.refresh')}
					>
						<RefreshCw class="h-4 w-4" />
					</Button>
				</CardAction>
			</CardHeader>
			<CardContent>
				<code class="block truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs">
					{data?.dir ?? '—'}
				</code>
			</CardContent>
		</Card>

		{#if showCreate}
			<Card>
				<CardHeader>
					<CardTitle>{t('myaaw.backups.create_title')}</CardTitle>
					<CardDescription>
						{t('myaaw.backups.create_desc_agent', { agent: agent ?? '' })}
					</CardDescription>
				</CardHeader>
				<CardContent>
					{@render createForm()}
				</CardContent>
			</Card>
		{/if}

		<Card>
			<CardHeader>
				<CardTitle class="flex items-center gap-2">
					<Archive class="h-4 w-4 shrink-0" />
					{t('myaaw.backups.archives')}
				</CardTitle>
				<CardDescription>
					{t('myaaw.backups.archives_desc_agent', { agent: agent ?? '' })}
				</CardDescription>
			</CardHeader>
			<CardContent>
				{@render archiveList()}
			</CardContent>
		</Card>
	{:else}
		<!-- Fleet view: one card for the whole backup surface, with the directory
		     shown inline and the archives below it. -->
		<Card>
			<CardHeader>
				<CardTitle class="flex items-center gap-2">
					<Archive class="h-4 w-4 shrink-0" />
					{t('myaaw.backups.archives')}
				</CardTitle>
				<CardDescription>{t('myaaw.backups.archives_desc')}</CardDescription>
				<CardAction class="flex items-center gap-1">
					<Button variant={showCreate ? 'secondary' : 'default'} size="sm" onclick={toggleCreate}>
						<HardDriveDownload class="mr-1.5 h-4 w-4" />
						{t('myaaw.backups.create')}
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						onclick={refresh}
						{loading}
						aria-label={t('common.refresh')}
						title={t('common.refresh')}
					>
						<RefreshCw class="h-4 w-4" />
					</Button>
				</CardAction>
			</CardHeader>
			<CardContent class="space-y-4">
				{#if showCreate}
					<div class="rounded-xl border bg-muted/20 p-4">
						<div class="mb-3 space-y-1">
							<p class="font-medium">{t('myaaw.backups.create_title')}</p>
							<p class="text-xs text-muted-foreground">{t('myaaw.backups.create_desc')}</p>
						</div>
						{@render createForm()}
					</div>
				{/if}

				<div class="space-y-2 rounded-lg border bg-muted/20 p-3">
					<p class="text-xs text-muted-foreground">{t('myaaw.backups.dir_hint')}</p>
					<code class="block truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs">
						{data?.dir ?? '—'}
					</code>
				</div>

				{@render archiveList()}
			</CardContent>
		</Card>
	{/if}
</div>
