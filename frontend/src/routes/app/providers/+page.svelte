<script lang="ts">
	import { providers as providersApi } from '$lib/apis/providers';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
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
	import { Separator } from '$lib/components/ui/separator';
	import { Cpu, Plus, RefreshCw, Star, Trash2, Wand2 } from 'lucide-svelte';
	import type { ModelListView, ProviderView } from '$lib/types';

	const { t } = useTranslation();

	let providers = $state<ProviderView[]>([]);
	let defaultProvider = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let discovering = $state(false);
	let error = $state('');
	let loadKey = '';

	let name = $state('');
	let type = $state('openai');
	let baseUrl = $state('');
	let apiKey = $state('');
	let defaultModel = $state('');
	let discovered = $state<ModelListView | null>(null);

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const view = await providersApi.list(currentAgent);
			providers = view.providers ?? [];
			defaultProvider = view.default_provider;
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

	async function discover() {
		discovering = true;
		discovered = null;
		try {
			discovered = await providersApi.modelsForCredentials({
				type,
				base_url: baseUrl || undefined,
				api_key: apiKey || undefined
			});
			if (!defaultModel && discovered.models.length > 0) {
				defaultModel = discovered.models[0];
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			discovering = false;
		}
	}

	async function create() {
		if (!agent || !name.trim() || !type) return;
		saving = true;
		try {
			await providersApi.create(agent, {
				name: name.trim(),
				type,
				base_url: baseUrl.trim() || undefined,
				api_key: apiKey || undefined,
				default_model: defaultModel || undefined
			});
			toastStore.success(t('myaaw.providers.saved'));
			name = '';
			baseUrl = '';
			apiKey = '';
			defaultModel = '';
			discovered = null;
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function setDefault(provider: string) {
		if (!agent) return;
		try {
			await providersApi.setDefault(agent, provider);
			defaultProvider = provider;
			toastStore.success(t('myaaw.providers.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function remove(provider: string) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.providers.delete_confirm', { name: provider }))) return;
		try {
			await providersApi.remove(agent, provider);
			await load(agent);
			toastStore.success(t('myaaw.providers.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="providers-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
	{/snippet}
</TopBarActions>

<div class="space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="grid gap-4 lg:grid-cols-5">
			<Card class="lg:col-span-3">
				<CardHeader>
					<CardTitle class="text-sm">{t('myaaw.providers.list')}</CardTitle>
					<CardDescription>
						{t('myaaw.providers.default')}:
						<span class="font-mono">{defaultProvider || '—'}</span>
					</CardDescription>
				</CardHeader>
				<CardContent class="space-y-2">
					{#if !loading && providers.length === 0}
						<EmptyState
							title={t('myaaw.providers.empty')}
							description={t('myaaw.providers.empty_desc')}
							icon={Cpu}
						/>
					{/if}
					{#each providers as provider (provider.name)}
						<div
							class="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="min-w-0 space-y-1">
								<div class="flex items-center gap-2">
									<span class="font-medium">{provider.name}</span>
									<Badge variant="outline">{provider.type}</Badge>
									{#if provider.name === defaultProvider}
										<Badge variant="secondary">{t('myaaw.providers.default')}</Badge>
									{/if}
								</div>
								<div class="flex flex-wrap gap-2 text-[11px] text-muted-foreground">
									{#if provider.default_model}
										<span class="font-mono">{provider.default_model}</span>
									{/if}
									<span
										>{provider.api_key.configured
											? provider.api_key.masked
											: t('myaaw.providers.no_key')}</span
									>
									{#if provider.base_url}
										<span class="truncate">{provider.base_url}</span>
									{/if}
								</div>
							</div>
							<div class="flex shrink-0 items-center gap-1">
								<Button
									size="sm"
									variant="outline"
									disabled={provider.name === defaultProvider}
									onclick={() => setDefault(provider.name)}
								>
									<Star class="mr-2 h-4 w-4" />
									{t('myaaw.providers.set_default')}
								</Button>
								<Button size="icon-sm" variant="ghost" onclick={() => remove(provider.name)}>
									<Trash2 class="h-3.5 w-3.5 text-destructive" />
								</Button>
							</div>
						</div>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-2">
				<CardHeader>
					<CardTitle class="text-sm">{t('myaaw.providers.create')}</CardTitle>
					<CardDescription>{t('myaaw.providers.create_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void create();
						}}
					>
						<div class="grid gap-1">
							<Label for="provider-name">{t('myaaw.providers.name')}</Label>
							<Input id="provider-name" bind:value={name} placeholder="deepseek" required />
						</div>
						<div class="grid gap-1">
							<Label for="provider-type">{t('myaaw.providers.type')}</Label>
							<select
								id="provider-type"
								bind:value={type}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="openai">openai</option>
								<option value="gemini">gemini</option>
							</select>
						</div>
						<div class="grid gap-1">
							<Label for="provider-base-url">{t('myaaw.providers.base_url')}</Label>
							<Input
								id="provider-base-url"
								bind:value={baseUrl}
								placeholder="https://api.deepseek.com"
							/>
						</div>
						<div class="grid gap-1">
							<Label for="provider-api-key">{t('myaaw.providers.api_key')}</Label>
							<Input
								id="provider-api-key"
								type="password"
								bind:value={apiKey}
								placeholder="sk-..."
							/>
						</div>

						<div class="flex gap-2">
							<Button
								type="button"
								variant="outline"
								size="sm"
								onclick={discover}
								loading={discovering}
							>
								<Wand2 class="mr-2 h-4 w-4" />
								{t('myaaw.providers.discover')}
							</Button>
						</div>

						{#if discovered}
							{#if discovered.warning}
								<p class="text-[11px] text-amber-600 dark:text-amber-400">{discovered.warning}</p>
							{/if}
							<div class="grid gap-1">
								<Label for="provider-model">{t('myaaw.providers.model')}</Label>
								<select
									id="provider-model"
									bind:value={defaultModel}
									class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								>
									{#each discovered.models as model (model)}
										<option value={model}>{model}</option>
									{/each}
								</select>
							</div>
						{:else}
							<div class="grid gap-1">
								<Label for="provider-model-manual">{t('myaaw.providers.model')}</Label>
								<Input
									id="provider-model-manual"
									bind:value={defaultModel}
									placeholder="deepseek-chat"
								/>
							</div>
						{/if}

						<Separator />
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.providers.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
