<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { providersStore } from '$lib/stores/providers.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { getErrorMessage } from '$lib/utils/api';
	import type { ProviderConnection } from '$lib/types';
	import { Button } from '$lib/components/ui/button';

	interface Props {
		agent: string;
		providerId?: string;
		credentialId?: string;
		connection?: ProviderConnection;
		busy?: boolean;
		onchange?: () => void;
		ondisconnect?: () => void;
	}

	let {
		agent,
		providerId,
		credentialId = $bindable(''),
		connection = $bindable<ProviderConnection | undefined>(),
		busy = $bindable(false),
		onchange,
		ondisconnect
	}: Props = $props();
	const { t } = useTranslation();
	let local = $state(false);
	let pending = $state(false);
	let error = $state('');
	let sessionExpiry = $state('');
	let authorizationUrl = $state('');
	let popupBlocked = $state(false);
	let terminalStatus = $state('');
	let session = '';
	let sessionAgent = '';
	let popup: Window | null = null;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let expiryTimer: ReturnType<typeof setTimeout> | undefined;
	let controller: AbortController | undefined;
	let generation = 0;
	let disposed = false;
	let status = $derived(
		pending ? 'pending' : terminalStatus || connection?.status || 'disconnected'
	);
	let statusKey = $derived(
		[
			'pending',
			'connected',
			'failed',
			'expired',
			'refresh_required',
			'reauthorization_required',
			'unavailable'
		].includes(status)
			? status
			: 'disconnected'
	);

	onMount(() => {
		// A loopback dashboard is necessary, but the backend must also reject a
		// forwarded/remote control plane: only it knows where its callback runs.
		local = ['localhost', '127.0.0.1', '[::1]'].includes(window.location.hostname);
		const leave = () => cancel();
		window.addEventListener('pagehide', leave);
		return () => window.removeEventListener('pagehide', leave);
	});
	onDestroy(() => {
		disposed = true;
		cancel();
	});

	function cancel() {
		generation += 1;
		clearTimeout(timer);
		clearTimeout(expiryTimer);
		controller?.abort();
		popup?.close();
		popup = null;
		authorizationUrl = '';
		popupBlocked = false;
		pending = false;
		busy = false;
		if (session) {
			void providersStore.cancelChatGPT(sessionAgent, session).catch(() => {
				// The backend also expires abandoned sessions if cleanup cannot reach it.
			});
			session = '';
		}
	}

	function finish() {
		clearTimeout(timer);
		clearTimeout(expiryTimer);
		popup?.close();
		popup = null;
		authorizationUrl = '';
		popupBlocked = false;
		session = '';
		pending = false;
		busy = false;
	}

	async function poll(run: number) {
		controller = new AbortController();
		try {
			const result = await providersStore.chatGPTStatus(sessionAgent, session, controller.signal);
			if (disposed || run !== generation) return;
			if (result.status === 'pending') {
				timer = setTimeout(() => void poll(run), 1500);
				return;
			}
			terminalStatus =
				result.status === 'connected' && !result.credential_id ? 'failed' : result.status;
			if (result.status === 'connected' && result.credential_id) {
				credentialId = result.credential_id;
				connection = {
					configured: true,
					status: 'connected',
					account_label: result.email
				};
				onchange?.();
			} else {
				error =
					result.error || t(`myaaw.chatgpt.${result.status === 'expired' ? 'expired' : 'failed'}`);
			}
			finish();
		} catch (err) {
			if (disposed || run !== generation) return;
			cancel();
			error = getErrorMessage(err);
			terminalStatus = 'failed';
		}
	}

	async function connect() {
		if (!local || !agent || busy) return;
		if (credentialId && !(await askConfirm({ message: t('myaaw.chatgpt.reconnect_confirm') })))
			return;
		if (disposed) return;
		cancel();
		const run = generation;
		const target = agent;
		error = '';
		terminalStatus = '';
		pending = true;
		busy = true;
		// Reserve the window in the click handler, before awaiting the API. A
		// blocked popup gets an explicit second user-gesture button instead.
		popup = window.open('about:blank', '_blank');
		if (popup) popup.opener = null;
		try {
			const result = await providersStore.connectChatGPT(target, credentialId || undefined);
			if (disposed || run !== generation) {
				await providersStore.cancelChatGPT(target, result.session_id);
				return;
			}
			session = result.session_id;
			sessionAgent = target;
			const url = new URL(result.authorization_url);
			if (url.origin !== 'https://auth.openai.com' || url.username || url.password) {
				throw new Error(t('myaaw.chatgpt.unsafe_url'));
			}
			const remaining = Date.parse(result.expires_at) - Date.now();
			if (!Number.isFinite(remaining) || remaining <= 0) {
				throw new Error(t('myaaw.chatgpt.expired'));
			}
			sessionExpiry = result.expires_at;
			authorizationUrl = url.href;
			if (popup && !popup.closed) popup.location.replace(authorizationUrl);
			else popupBlocked = true;
			expiryTimer = setTimeout(
				() => {
					cancel();
					terminalStatus = 'expired';
					error = t('myaaw.chatgpt.expired');
				},
				Math.min(remaining, 2147483647)
			);
			void poll(run);
		} catch (err) {
			if (disposed || run !== generation) return;
			cancel();
			terminalStatus = 'failed';
			error = getErrorMessage(err);
		}
	}

	function openPopup() {
		if (!authorizationUrl) return;
		popup = window.open('about:blank', '_blank');
		if (popup) {
			popup.opener = null;
			popup.location.replace(authorizationUrl);
			popupBlocked = false;
		}
	}

	async function disconnect() {
		if (!providerId || busy) return;
		if (
			!(await askConfirm({
				message: t('myaaw.chatgpt.disconnect_confirm'),
				variant: 'destructive'
			}))
		)
			return;
		if (disposed) return;
		busy = true;
		error = '';
		try {
			await providersStore.disconnect(agent, providerId);
			if (disposed) return;
			credentialId = '';
			connection = { configured: false, status: 'disconnected' };
			terminalStatus = '';
			onchange?.();
			ondisconnect?.();
		} catch (err) {
			if (!disposed) error = getErrorMessage(err);
		} finally {
			if (!disposed) busy = false;
		}
	}
</script>

<section class="min-w-0 space-y-3 rounded-lg border p-3" aria-label={t('myaaw.chatgpt.title')}>
	<h3 class="text-sm font-medium">{t('myaaw.chatgpt.title')}</h3>
	<p class="text-xs text-muted-foreground">{t('myaaw.chatgpt.local_only')}</p>
	{#if !local}
		<p class="text-xs text-amber-600 dark:text-amber-400">{t('myaaw.chatgpt.remote_disabled')}</p>
	{/if}
	<div class="space-y-1 text-xs" aria-live="polite">
		<p>{t(`myaaw.chatgpt.${statusKey}`)}</p>
		{#if connection?.account_label}
			<p class="wrap-anywhere">{connection.account_label}</p>
		{/if}
		{#if pending && sessionExpiry}
			<p>{t('myaaw.chatgpt.expires', { at: new Date(sessionExpiry).toLocaleString() })}</p>
		{:else if connection?.expires_at}
			<p>{t('myaaw.chatgpt.expires', { at: new Date(connection.expires_at).toLocaleString() })}</p>
		{/if}
	</div>
	{#if error}<p role="alert" class="text-xs wrap-anywhere text-destructive">{error}</p>{/if}
	<div class="flex flex-wrap gap-2">
		<Button
			type="button"
			variant="outline"
			size="sm"
			data-oai-siwc-button
			disabled={!local || !agent || busy}
			onclick={connect}
		>
			Continue with ChatGPT
		</Button>
		{#if credentialId}<span class="self-center text-xs text-muted-foreground"
				>{t('myaaw.chatgpt.reconnect')}</span
			>{/if}
		{#if pending}
			<Button type="button" variant="ghost" size="sm" onclick={cancel}>{t('common.cancel')}</Button>
		{/if}
		{#if popupBlocked && pending}
			<Button type="button" variant="outline" size="sm" onclick={openPopup}
				>{t('myaaw.chatgpt.open_browser')}</Button
			>
		{/if}
		{#if providerId && credentialId}
			<Button type="button" variant="ghost" size="sm" disabled={busy} onclick={disconnect}
				>{t('myaaw.chatgpt.disconnect')}</Button
			>
		{/if}
	</div>
	{#if popupBlocked && pending}<p class="text-xs text-muted-foreground">
			{t('myaaw.chatgpt.popup_blocked')}
		</p>{/if}
	{#if !agent}<p class="text-xs text-muted-foreground">{t('myaaw.chatgpt.agent_required')}</p>{/if}
	<p class="text-xs text-muted-foreground">{t('myaaw.chatgpt.binding_hint')}</p>
</section>
