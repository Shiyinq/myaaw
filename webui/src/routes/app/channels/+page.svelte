<script lang="ts">
	import { channelsStore } from '$lib/stores/channels.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import DiscordIcon from '$lib/components/common/DiscordIcon.svelte';
	import ChannelDialog from '$lib/components/channels/ChannelDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		Info,
		KeyRound,
		PowerOff,
		Radio,
		RefreshCw,
		Send,
		SlidersHorizontal
	} from 'lucide-svelte';
	import type { ChannelView } from '$lib/types';

	const { t } = useTranslation();

	let dialogOpen = $state(false);
	let editingChannel = $state<'telegram' | 'discord' | null>(null);

	let agent = $derived(agentSelection.selected);
	useQuery(channelsStore.query, () => (agent ? { agent } : null));

	let loading = $derived(channelsStore.loading);
	let error = $derived(channelsStore.error);

	let telegram = $derived<ChannelView | null>(
		channelsStore.data?.channels?.find((channel) => channel.name === 'telegram') ?? null
	);
	let discord = $derived<ChannelView | null>(
		channelsStore.data?.channels?.find((channel) => channel.name === 'discord') ?? null
	);
	let activeCount = $derived((telegram?.active ? 1 : 0) + (discord?.active ? 1 : 0));

	let editingCurrent = $derived<ChannelView | null>(
		editingChannel === 'telegram' ? telegram : editingChannel === 'discord' ? discord : null
	);

	function openConfigure(channel: 'telegram' | 'discord') {
		editingChannel = channel;
		dialogOpen = true;
	}

	async function disable(channel: 'telegram' | 'discord') {
		if (!agent) return;
		if (!(await askConfirm({ message: t('myaaw.channels.disable_confirm') }))) return;
		try {
			await channelsStore.disable(agent, channel);
			toastStore.success(t('myaaw.channels.disabled'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="channels-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => void channelsStore.refresh()}
			loading={loading || channelsStore.refreshing}
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
			<div class="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
				<div class="flex items-center gap-2">
					<Radio class="h-4 w-4 text-primary" />
					<h2 class="text-sm font-semibold text-foreground">{t('myaaw.channels.list')}</h2>
					<span
						class="rounded-full bg-muted px-2 py-0.5 text-[11px] leading-none font-medium text-muted-foreground tabular-nums"
					>
						{activeCount}/2
					</span>
				</div>
			</div>

			<div class="p-4 sm:p-5">
				{#if loading}
					<div class="space-y-2.5">
						<Skeleton class="h-24 w-full rounded-xl" />
						<Skeleton class="h-24 w-full rounded-xl" />
					</div>
				{:else}
					<div class="space-y-2.5">
						<!-- Telegram -->
						<div
							class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="flex min-w-0 items-start gap-3">
								<div
									class="mt-0.5 grid h-10 w-10 shrink-0 place-items-center rounded-xl border bg-muted/40 text-foreground"
								>
									<Send class="h-4 w-4" />
								</div>
								<div class="min-w-0 flex-1 space-y-1">
									<div class="flex flex-wrap items-center gap-2">
										<span
											aria-hidden="true"
											class={[
												'h-2 w-2 shrink-0 rounded-full',
												telegram?.active ? 'bg-emerald-500' : 'bg-muted-foreground/35'
											]}
										></span>
										<span class="text-sm font-semibold text-foreground">
											{t('myaaw.channels.telegram')}
										</span>
										<Badge variant={telegram?.active ? 'secondary' : 'outline'} class="text-[10px]">
											{telegram?.active ? t('myaaw.channels.active') : t('myaaw.channels.inactive')}
										</Badge>
									</div>

									<p class="text-xs text-muted-foreground">
										{t('myaaw.channels.telegram_desc')}
									</p>

									<div
										class="flex flex-wrap items-center gap-x-3 gap-y-1 pt-0.5 text-[11px] text-muted-foreground"
									>
										<span class="inline-flex items-center gap-1 font-mono">
											<KeyRound class="h-3 w-3 opacity-70" />
											{telegram?.token?.configured
												? telegram.token.masked
												: t('myaaw.providers.no_key')}
										</span>
										<span
											class="rounded-md border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-foreground/85"
										>
											{telegram?.mode || 'polling'}
										</span>
										{#if telegram?.ngrok_active}
											<Badge variant="outline" class="text-[10px]">ngrok</Badge>
										{/if}
									</div>
								</div>
							</div>

							<div class="flex shrink-0 items-center justify-end gap-1.5">
								<Button
									size="sm"
									variant="outline"
									onclick={() => openConfigure('telegram')}
									class="h-8 gap-1.5 text-xs"
								>
									<SlidersHorizontal class="h-3.5 w-3.5" />
									{t('myaaw.channels.configure')}
								</Button>
								{#if telegram?.active}
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => disable('telegram')}
										aria-label={t('myaaw.channels.disable')}
										title={t('myaaw.channels.disable')}
									>
										<PowerOff class="h-3.5 w-3.5 text-destructive" />
									</Button>
								{/if}
							</div>
						</div>

						<!-- Discord -->
						<div
							class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="flex min-w-0 items-start gap-3">
								<div
									class="mt-0.5 grid h-10 w-10 shrink-0 place-items-center rounded-xl border bg-muted/40 text-foreground"
								>
									<DiscordIcon class="h-4 w-4" />
								</div>
								<div class="min-w-0 flex-1 space-y-1">
									<div class="flex flex-wrap items-center gap-2">
										<span
											aria-hidden="true"
											class={[
												'h-2 w-2 shrink-0 rounded-full',
												discord?.active ? 'bg-emerald-500' : 'bg-muted-foreground/35'
											]}
										></span>
										<span class="text-sm font-semibold text-foreground">
											{t('myaaw.channels.discord')}
										</span>
										<Badge variant={discord?.active ? 'secondary' : 'outline'} class="text-[10px]">
											{discord?.active ? t('myaaw.channels.active') : t('myaaw.channels.inactive')}
										</Badge>
									</div>

									<p class="text-xs text-muted-foreground">
										{t('myaaw.channels.discord_desc')}
									</p>

									<div
										class="flex flex-wrap items-center gap-x-3 gap-y-1 pt-0.5 text-[11px] text-muted-foreground"
									>
										<span class="inline-flex items-center gap-1 font-mono">
											<KeyRound class="h-3 w-3 opacity-70" />
											{discord?.token?.configured
												? discord.token.masked
												: t('myaaw.providers.no_key')}
										</span>
									</div>
								</div>
							</div>

							<div class="flex shrink-0 items-center justify-end gap-1.5">
								<Button
									size="sm"
									variant="outline"
									onclick={() => openConfigure('discord')}
									class="h-8 gap-1.5 text-xs"
								>
									<SlidersHorizontal class="h-3.5 w-3.5" />
									{t('myaaw.channels.configure')}
								</Button>
								{#if discord?.active}
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => disable('discord')}
										aria-label={t('myaaw.channels.disable')}
										title={t('myaaw.channels.disable')}
									>
										<PowerOff class="h-3.5 w-3.5 text-destructive" />
									</Button>
								{/if}
							</div>
						</div>
					</div>
				{/if}
			</div>
		</div>

		<p class="flex items-center gap-1.5 px-1 text-xs text-muted-foreground">
			<Info class="h-3.5 w-3.5 shrink-0 opacity-75" />
			{t('myaaw.channels.restart_hint')}
		</p>
	{/if}

	<ChannelDialog
		open={dialogOpen}
		agent={agent ?? ''}
		channel={editingChannel}
		current={editingCurrent}
		onOpenChange={(next) => (dialogOpen = next)}
	/>
</div>
