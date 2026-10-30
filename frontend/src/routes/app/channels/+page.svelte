<script lang="ts">
	import { channels as channelsApi } from '$lib/apis/channels';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { RefreshCw, Save, Trash2 } from 'lucide-svelte';
	import type { SecretState } from '$lib/types';

	const { t } = useTranslation();

	let telegram = $state({
		active: false,
		token: '',
		mode: 'polling',
		ngrok_active: false,
		ngrok_auth_token: ''
	});
	let discord = $state({ active: false, token: '' });
	let telegramToken = $state<SecretState | null>(null);
	let discordToken = $state<SecretState | null>(null);
	let loading = $state(false);
	let saving = $state<string>('');
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const view = await channelsApi.list(currentAgent);
			const channels = view.channels ?? [];
			const tg = channels.find((channel) => channel.name === 'telegram');
			const dc = channels.find((channel) => channel.name === 'discord');
			if (tg) {
				telegram.active = tg.active;
				telegram.mode = tg.mode ?? 'polling';
				telegram.ngrok_active = Boolean(tg.ngrok_active);
				telegram.token = '';
				telegram.ngrok_auth_token = '';
				telegramToken = tg.token;
			} else {
				telegramToken = null;
			}
			if (dc) {
				discord.active = dc.active;
				discord.token = '';
				discordToken = dc.token;
			} else {
				discordToken = null;
			}
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

	async function save(channel: 'telegram' | 'discord') {
		if (!agent) return;
		saving = channel;
		try {
			await channelsApi.upsert(
				agent,
				channel,
				channel === 'telegram' ? { ...telegram } : { ...discord }
			);
			toastStore.success(t('myaaw.channels.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = '';
		}
	}

	async function disable(channel: 'telegram' | 'discord') {
		if (!agent) return;
		if (!window.confirm(t('myaaw.channels.disable_confirm'))) return;
		try {
			await channelsApi.disable(agent, channel);
			toastStore.success(t('myaaw.channels.disabled'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<div class="space-y-4">
	<PageHeader title={t('myaaw.channels.title')} description={t('myaaw.channels.subtitle')}>
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

		<div class="grid gap-4 md:grid-cols-2">
			<Card>
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle>{t('myaaw.channels.telegram')}</CardTitle>
						<CardDescription>{t('myaaw.channels.telegram_desc')}</CardDescription>
					</div>
					{#if telegram.active}
						<Badge variant="secondary">{t('myaaw.channels.active')}</Badge>
					{:else}
						<Badge variant="outline">{t('myaaw.channels.inactive')}</Badge>
					{/if}
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void save('telegram');
						}}
					>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={telegram.active} class="h-4 w-4 accent-primary" />
							{t('myaaw.channels.active')}
						</label>

						<div class="grid gap-1">
							<Label for="telegram-token">{t('myaaw.channels.token')}</Label>
							<Input
								id="telegram-token"
								type="password"
								bind:value={telegram.token}
								placeholder={telegramToken?.configured ? telegramToken.masked : '123456:ABC'}
							/>
							<p class="text-[10px] text-muted-foreground">{t('myaaw.channels.token_hint')}</p>
						</div>

						<div class="grid gap-1">
							<Label for="telegram-mode">{t('myaaw.channels.mode')}</Label>
							<select
								id="telegram-mode"
								bind:value={telegram.mode}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
							>
								<option value="polling">polling</option>
								<option value="webhook">webhook</option>
							</select>
						</div>

						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								bind:checked={telegram.ngrok_active}
								class="h-4 w-4 accent-primary"
							/>
							ngrok
						</label>
						{#if telegram.ngrok_active}
							<div class="grid gap-1">
								<Label for="telegram-ngrok">{t('myaaw.channels.ngrok_token')}</Label>
								<Input id="telegram-ngrok" type="password" bind:value={telegram.ngrok_auth_token} />
							</div>
						{/if}

						<div class="flex gap-2">
							<Button type="submit" size="sm" loading={saving === 'telegram'}>
								<Save class="mr-2 h-4 w-4" />
								{t('common.save')}
							</Button>
							<Button
								type="button"
								size="sm"
								variant="destructive"
								onclick={() => disable('telegram')}
							>
								<Trash2 class="mr-2 h-4 w-4" />
								{t('myaaw.channels.disable')}
							</Button>
						</div>
					</form>
				</CardContent>
			</Card>

			<Card>
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle>{t('myaaw.channels.discord')}</CardTitle>
						<CardDescription>{t('myaaw.channels.discord_desc')}</CardDescription>
					</div>
					{#if discord.active}
						<Badge variant="secondary">{t('myaaw.channels.active')}</Badge>
					{:else}
						<Badge variant="outline">{t('myaaw.channels.inactive')}</Badge>
					{/if}
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void save('discord');
						}}
					>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={discord.active} class="h-4 w-4 accent-primary" />
							{t('myaaw.channels.active')}
						</label>

						<div class="grid gap-1">
							<Label for="discord-token">{t('myaaw.channels.token')}</Label>
							<Input
								id="discord-token"
								type="password"
								bind:value={discord.token}
								placeholder={discordToken?.configured ? discordToken.masked : 'BOT_TOKEN'}
							/>
							<p class="text-[10px] text-muted-foreground">{t('myaaw.channels.token_hint')}</p>
						</div>

						<div class="flex gap-2">
							<Button type="submit" size="sm" loading={saving === 'discord'}>
								<Save class="mr-2 h-4 w-4" />
								{t('common.save')}
							</Button>
							<Button
								type="button"
								size="sm"
								variant="destructive"
								onclick={() => disable('discord')}
							>
								<Trash2 class="mr-2 h-4 w-4" />
								{t('myaaw.channels.disable')}
							</Button>
						</div>
					</form>
				</CardContent>
			</Card>
		</div>

		<p class="text-xs text-muted-foreground">{t('myaaw.channels.restart_hint')}</p>
	{/if}
</div>
