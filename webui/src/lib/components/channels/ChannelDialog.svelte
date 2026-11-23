<script lang="ts">
	import { untrack } from 'svelte';
	import { Loader2, Save, Send } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Switch from '$lib/components/common/Switch.svelte';
	import DiscordIcon from '$lib/components/common/DiscordIcon.svelte';
	import { channelsStore } from '$lib/stores/channels.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ChannelView } from '$lib/types';

	interface Props {
		open: boolean;
		agent: string;
		channel: 'telegram' | 'discord' | null;
		current: ChannelView | null;
		onOpenChange: (open: boolean) => void;
	}

	let { open, agent, channel, current, onOpenChange }: Props = $props();
	const { t } = useTranslation();

	let saving = $state(false);
	let formError = $state('');

	let active = $state(false);
	let token = $state('');
	let mode = $state<'polling' | 'webhook'>('polling');
	let ngrokActive = $state(false);
	let ngrokAuthToken = $state('');

	const isTelegram = $derived(channel === 'telegram');

	const modeOptions: { id: 'polling' | 'webhook'; label: string; hint: string }[] = [
		{ id: 'polling', label: 'Polling', hint: 'Long polling' },
		{ id: 'webhook', label: 'Webhook', hint: 'HTTPS callback' }
	];

	// Hydrate fields cleanly whenever the dialog opens.
	$effect(() => {
		if (!open || !channel) return;
		const snapshot = current;
		untrack(() => {
			active = Boolean(snapshot?.active);
			token = '';
			mode = snapshot?.mode === 'webhook' ? 'webhook' : 'polling';
			ngrokActive = Boolean(snapshot?.ngrok_active);
			ngrokAuthToken = '';
			formError = '';
			saving = false;
		});
	});

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!agent || !channel || saving) return;

		saving = true;
		formError = '';
		try {
			await channelsStore.upsert(
				agent,
				channel,
				channel === 'telegram'
					? {
							active,
							token,
							mode,
							ngrok_active: ngrokActive,
							ngrok_auth_token: ngrokAuthToken
						}
					: {
							active,
							token
						}
			);
			toastStore.success(t('myaaw.channels.saved'));
			onOpenChange(false);
		} catch (err) {
			formError = getErrorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				{#if isTelegram}
					<Send class="h-4 w-4 text-primary" />
					{t('myaaw.channels.telegram')}
				{:else}
					<DiscordIcon class="h-4 w-4 text-primary" />
					{t('myaaw.channels.discord')}
				{/if}
			</Dialog.Title>
			<Dialog.Description>
				{isTelegram ? t('myaaw.channels.telegram_desc') : t('myaaw.channels.discord_desc')}
			</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={handleSubmit}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<!-- Active switch -->
				<div class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5">
					<div class="min-w-0">
						<p class="text-xs font-medium text-foreground">{t('myaaw.channels.active')}</p>
						<p class="text-[11px] text-muted-foreground">{t('myaaw.channels.restart_hint')}</p>
					</div>
					<Switch
						checked={active}
						label={t('myaaw.channels.active')}
						onchange={(next) => (active = next)}
					/>
				</div>

				<!-- Bot token -->
				<div class="space-y-1.5">
					<Label for="channel-token">{t('myaaw.channels.token')}</Label>
					<Input
						id="channel-token"
						type="password"
						bind:value={token}
						placeholder={current?.token?.configured
							? current.token.masked
							: isTelegram
								? '123456:ABC'
								: 'BOT_TOKEN'}
					/>
					<p class="text-[11px] text-muted-foreground">{t('myaaw.channels.token_hint')}</p>
				</div>

				{#if isTelegram}
					<!-- Mode picker -->
					<div class="space-y-2">
						<Label>{t('myaaw.channels.mode')}</Label>
						<div class="grid grid-cols-2 gap-1.5">
							{#each modeOptions as opt (opt.id)}
								{@const selected = mode === opt.id}
								<button
									type="button"
									onclick={() => (mode = opt.id)}
									aria-pressed={selected}
									class={[
										'flex flex-col rounded-xl border p-2.5 text-left transition-colors',
										selected
											? 'border-primary bg-primary/8 text-foreground'
											: 'border-border hover:bg-muted/60'
									]}
								>
									<span class="text-xs font-semibold">{opt.label}</span>
									<span class="mt-0.5 text-[10px] text-muted-foreground">{opt.hint}</span>
								</button>
							{/each}
						</div>
					</div>

					<!-- ngrok toggle + token -->
					<div class="space-y-3 rounded-xl border p-3">
						<div class="flex items-center justify-between gap-3">
							<div class="min-w-0">
								<p class="text-xs font-medium text-foreground">ngrok</p>
								<p class="text-[11px] text-muted-foreground">
									{t('myaaw.channels.ngrok_token')}
								</p>
							</div>
							<Switch
								checked={ngrokActive}
								label="ngrok"
								onchange={(next) => (ngrokActive = next)}
							/>
						</div>

						{#if ngrokActive}
							<div class="h-px bg-border/60"></div>
							<div class="space-y-1.5">
								<Label for="channel-ngrok" class="text-xs">
									{t('myaaw.channels.ngrok_token')}
								</Label>
								<Input id="channel-ngrok" type="password" bind:value={ngrokAuthToken} />
							</div>
						{/if}
					</div>
				{/if}

				{#if formError}
					<p
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{formError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => onOpenChange(false)}
					disabled={saving}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={saving}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else}
						<Save class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{t('common.save')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
