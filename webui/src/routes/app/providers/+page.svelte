<script lang="ts">
	import { providersStore } from '$lib/stores/providers.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { loadModelCapabilities } from '$lib/stores/modelCatalog';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import ProviderDialog from '$lib/components/providers/ProviderDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		Brain,
		Cpu,
		Globe,
		KeyRound,
		Pencil,
		Plus,
		RefreshCw,
		Star,
		Trash2
	} from 'lucide-svelte';
	import type { ModelCapabilitiesView, ProviderConnection, ProviderView } from '$lib/types';

	const { t } = useTranslation();

	let dialogOpen = $state(false);
	let editing = $state<ProviderView | null>(null);
	let modelCapabilities = $state<ModelCapabilitiesView | null>(null);

	let agent = $derived(agentSelection.selected);
	let selectedAgentInfo = $derived(
		agentSelection.agents.find((entry) => entry.name === agent) ?? null
	);
	let isExternalRuntime = $derived(
		!!selectedAgentInfo?.runtime_type && selectedAgentInfo.runtime_type !== 'myaaw'
	);
	useQuery(providersStore.list, () => (agent && !isExternalRuntime ? { agent } : null));

	let providers = $derived(providersStore.providers);
	let defaultProvider = $derived(providersStore.defaultProvider);
	let loading = $derived(providersStore.loading);
	let error = $derived(providersStore.error);

	$effect(() => {
		if (modelCapabilities) return;
		void loadModelCapabilities()
			.then((view) => {
				modelCapabilities = view;
			})
			.catch(() => {
				// The catalog is optional: the thinking field simply stays hidden.
			});
	});

	function thinkingLabel(value: string) {
		if (value === 'none' || value === 'off') return t('myaaw.chat.thinking_off');
		if (value === 'xhigh') return 'XHigh';
		return value.charAt(0).toUpperCase() + value.slice(1);
	}

	function connectionStatusKey(connection?: ProviderConnection) {
		const status = connection?.status ?? 'disconnected';
		return [
			'connected',
			'refresh_required',
			'reauthorization_required',
			'unavailable',
			'expired',
			'failed'
		].includes(status)
			? status
			: 'disconnected';
	}

	function openCreate() {
		if (isExternalRuntime) return;
		editing = null;
		dialogOpen = true;
	}

	function openEdit(provider: ProviderView) {
		editing = provider;
		dialogOpen = true;
	}

	async function setDefault(provider: string) {
		if (!agent) return;
		try {
			await providersStore.setDefault(agent, provider);
			toastStore.success(t('myaaw.providers.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function remove(provider: string) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.providers.delete_confirm', { name: provider }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await providersStore.remove(agent, provider);
			if (editing?.name === provider) {
				editing = null;
				dialogOpen = false;
			}
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
			onclick={() => void providersStore.refresh()}
			loading={loading || providersStore.refreshing}
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
		{#if error && !isExternalRuntime}
			<div
				class="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="rounded-xl border bg-card shadow-xs">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
				<div class="flex flex-wrap items-center gap-2">
					<Cpu class="h-4 w-4 text-primary" />
					<h2 class="text-sm font-semibold text-foreground">{t('myaaw.providers.list')}</h2>
					{#if !isExternalRuntime}
						<span
							class="rounded-full bg-muted px-2 py-0.5 text-[11px] leading-none font-medium text-muted-foreground tabular-nums"
						>
							{providers.length}
						</span>
						{#if defaultProvider}
							<span
								class="inline-flex items-center gap-1 rounded-full border bg-muted/40 px-2 py-0.5 text-[11px] text-muted-foreground"
							>
								<Star class="h-2.5 w-2.5 text-primary" />
								{t('myaaw.providers.default')}:
								<span class="font-mono font-medium text-foreground">{defaultProvider}</span>
							</span>
						{/if}
					{/if}
				</div>

				{#if !isExternalRuntime}
					<Button size="sm" onclick={openCreate} class="shrink-0 gap-1.5 rounded-lg">
						<Plus class="h-3.5 w-3.5" />
						{t('myaaw.providers.create')}
					</Button>
				{/if}
			</div>

			<div class="p-4 sm:p-5">
				{#if isExternalRuntime}
					<div
						class="flex min-h-60 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border/80 bg-muted/20 px-6 py-10 text-center"
					>
						<RuntimeBadge
							runtimeType={selectedAgentInfo?.runtime_type}
							runtimePreset={selectedAgentInfo?.runtime_preset}
							size="lg"
							inline
						/>
						<div class="max-w-md space-y-1">
							<p class="text-sm font-semibold text-foreground">
								{t('myaaw.providers.external_runtime_title')}
								<span class="font-mono text-primary">
									{selectedAgentInfo?.runtime_preset || selectedAgentInfo?.runtime_type}
								</span>
							</p>
							<p class="text-xs leading-relaxed text-muted-foreground">
								{t('myaaw.providers.external_runtime_desc')}
							</p>
						</div>
						<Button
							variant="outline"
							size="sm"
							href={`/app/new?agent=${encodeURIComponent(agent)}`}
							class="mt-1"
						>
							{t('myaaw.providers.configure_runtime')}
						</Button>
					</div>
				{:else if loading}
					<div class="space-y-2.5">
						<Skeleton class="h-20 w-full rounded-xl" />
						<Skeleton class="h-20 w-full rounded-xl" />
					</div>
				{:else if providers.length === 0}
					<EmptyState
						title={t('myaaw.providers.empty')}
						description={t('myaaw.providers.empty_desc')}
						bloub="curieux"
						icon={Cpu}
						class="min-h-64"
					/>
				{:else}
					<div class="space-y-2.5">
						{#each providers as provider (provider.name)}
							{@const isDefault = provider.name === defaultProvider}
							<div
								class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
							>
								<div class="min-w-0 space-y-1.5">
									<div class="flex flex-wrap items-center gap-2">
										<span
											aria-hidden="true"
											class={[
												'h-2 w-2 shrink-0 rounded-full',
												isDefault ? 'bg-emerald-500' : 'bg-muted-foreground/35'
											]}
										></span>
										<span class="text-sm font-semibold text-foreground wrap-anywhere">
											{provider.name}
										</span>
										<Badge variant="outline" class="text-[10px]">
											{provider.type === 'openai_chatgpt'
												? t('myaaw.chatgpt.title')
												: provider.type}
										</Badge>
										{#if isDefault}
											<Badge variant="secondary" class="text-[10px]">
												{t('myaaw.providers.default')}
											</Badge>
										{/if}
									</div>

									<div
										class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground"
									>
										{#if provider.default_model}
											<span
												class="rounded-md border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-foreground/90"
											>
												{provider.default_model}
											</span>
										{/if}
										{#if provider.thinking_level}
											<span class="inline-flex items-center gap-1">
												<Brain class="h-3 w-3 opacity-70" />
												{thinkingLabel(provider.thinking_level)}
											</span>
										{/if}
										{#if provider.type === 'openai_chatgpt'}
											<span>{t(`myaaw.chatgpt.${connectionStatusKey(provider.connection)}`)}</span>
											{#if provider.connection?.account_label}
												<span class="wrap-anywhere">{provider.connection.account_label}</span>
											{/if}
										{:else}
											<span class="inline-flex items-center gap-1">
												<KeyRound class="h-3 w-3 opacity-70" />
												{provider.api_key.configured
													? provider.api_key.masked
													: t('myaaw.providers.no_key')}
											</span>
										{/if}
										{#if provider.type !== 'openai_chatgpt' && provider.base_url}
											<span class="inline-flex min-w-0 items-center gap-1 truncate font-mono">
												<Globe class="h-3 w-3 shrink-0 opacity-70" />
												<span class="truncate">{provider.base_url}</span>
											</span>
										{/if}
									</div>
								</div>

								<div class="flex shrink-0 items-center justify-end gap-1">
									<Button
										size="sm"
										variant="outline"
										disabled={isDefault}
										onclick={() => setDefault(provider.name)}
										class="h-8 gap-1.5 text-xs"
									>
										<Star class="h-3.5 w-3.5" />
										{t('myaaw.providers.set_default')}
									</Button>
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => openEdit(provider)}
										aria-label={t('myaaw.providers.edit')}
										title={t('myaaw.providers.edit')}
									>
										<Pencil class="h-3.5 w-3.5" />
									</Button>
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => remove(provider.name)}
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
			</div>
		</div>
	{/if}

	{#if !isExternalRuntime}
		<ProviderDialog
			open={dialogOpen}
			agent={agent ?? ''}
			{editing}
			{modelCapabilities}
			onOpenChange={(next) => (dialogOpen = next)}
		/>
	{/if}
</div>
