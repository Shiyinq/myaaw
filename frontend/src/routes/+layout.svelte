<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { isAuthenticated, authStore, isInitialDataLoaded, accessToken } from '$lib/stores';
	import { locale, type Locale } from '$lib/i18n';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import LoadingBar from '$lib/components/LoadingBar.svelte';
	import SplashScreen from '$lib/components/SplashScreen.svelte';
	import SEO from '$lib/components/common/SEO.svelte';
	import { logger } from '$lib/utils/logger';
	import OfflinePage from './offline/+page.svelte';
	import ReloadPrompt from '$lib/components/ReloadPrompt.svelte';

	interface Props {
		data: { locale: string };
		children: import('svelte').Snippet;
	}

	let { data, children }: Props = $props();

	$effect(() => {
		if (data?.locale) {
			locale.value = data.locale as Locale;
		}
	});

	let mounted = $state(false);
	let isOffline = $state(false);

	onMount(() => {
		mounted = true;

		if (accessToken.isExpired) {
			accessToken.clear();
			isAuthenticated.set(false);
		}

		isOffline = !navigator.onLine;
		const setOffline = () => (isOffline = true);
		const setOnline = () => (isOffline = false);
		window.addEventListener('offline', setOffline);
		window.addEventListener('online', setOnline);

		return () => {
			window.removeEventListener('offline', setOffline);
			window.removeEventListener('online', setOnline);
		};
	});

	let isPublicPage = $derived(
		page.url.pathname === '/' ||
			page.url.pathname === '/login' ||
			page.url.pathname.startsWith('/offline')
	);

	let isGuestRoute = $derived(page.url.pathname === '/login');

	async function fetchInitialData() {
		if (isInitialDataLoaded.value) return;
		try {
			await authStore.loadProfile();
		} catch (err) {
			logger.error('Failed to load profile', err, { context: 'layout' });
		} finally {
			isInitialDataLoaded.value = true;
		}
	}

	$effect(() => {
		if (!mounted) return;
		if (isAuthenticated.value) {
			void fetchInitialData();
			if (isGuestRoute) {
				void goto('/app');
			}
		} else if (!isPublicPage) {
			void goto('/login');
		}
	});
</script>

<SEO />
<ModeWatcher />
<ReloadPrompt />

<div class="min-h-screen flex flex-col relative overflow-x-hidden">
	{#if !mounted}
		<SplashScreen />
	{:else}
		<LoadingBar />
		<main class="flex-1 w-full relative">
			{#if isOffline}
				<OfflinePage />
			{:else}
				{@render children()}
			{/if}
		</main>
		<Toaster position="bottom-right" richColors />
	{/if}
</div>
