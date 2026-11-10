<script lang="ts">
	import { browser } from '$app/environment';
	import { writable } from 'svelte/store';
	import { useRegisterSW } from 'virtual:pwa-register/svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Button } from '$lib/components/ui/button';

	// `useRegisterSW` reads `navigator` as soon as it is called, so it must only
	// run in the browser. SvelteKit renders this layout on the server during
	// `vite build` and on every request, where `navigator` may not exist (Node 20
	// has no such global). Register from the browser bundle only.
	const { needRefresh, updateServiceWorker } = browser
		? useRegisterSW({
				onRegistered(r: ServiceWorkerRegistration | undefined) {
					console.log('SW Registered:', r);
				},
				onRegisterError(error: Error | unknown) {
					console.error('SW error', error);
				}
			})
		: {
				needRefresh: writable(false),
				updateServiceWorker: (_reloadPage?: boolean) => Promise.resolve()
			};

	const { t } = useTranslation();
</script>

{#if $needRefresh}
	<div
		class="fixed bottom-6 right-6 bg-card frost-card text-card-foreground p-4 rounded-xl shadow-lg z-50 border border-border"
	>
		<p class="font-medium">{t('pwa.newVersion')}</p>
		<div class="flex gap-2 mt-4 justify-end">
			<Button variant="ghost" size="sm" onclick={() => ($needRefresh = false)}>
				{t('pwa.later')}
			</Button>
			<Button size="sm" onclick={() => updateServiceWorker(true)}>
				{t('pwa.reload')}
			</Button>
		</div>
	</div>
{/if}
