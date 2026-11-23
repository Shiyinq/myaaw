<script lang="ts">
	import '../app.css';
	// KaTeX ships its own layout for the math markup that Markdown renders.
	import 'katex/dist/katex.min.css';
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		isAuthenticated,
		authStore,
		isInitialDataLoaded,
		accessToken,
		settingsStore
	} from '$lib/stores';
	import { applyPreferences, preferences } from '$lib/stores/preferences.svelte';
	import { locale, type Locale } from '$lib/i18n';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Wifi, WifiOff } from 'lucide-svelte';
	import { ModeWatcher, setMode, theme, userPrefersMode } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import LoadingBar from '$lib/components/LoadingBar.svelte';
	import SplashScreen from '$lib/components/SplashScreen.svelte';
	import SEO from '$lib/components/common/SEO.svelte';
	import { logger } from '$lib/utils/logger';

	import ReloadPrompt from '$lib/components/ReloadPrompt.svelte';
	import { modeForTheme } from '$lib/theme';
	import { documentDescription, documentTitle } from '$lib/utils/pageTitle';

	interface Props {
		data: { locale: string };
		children: import('svelte').Snippet;
	}

	let { data, children }: Props = $props();
	const { t } = useTranslation();

	$effect(() => {
		if (data?.locale) {
			locale.value = data.locale as Locale;
		}
	});

	// A named theme is designed for one mode, and storage can disagree with it (a
	// theme picked before it changed, or a hand-edited value). Reconcile so a light
	// theme never runs with the `dark` class, or the other way round.
	$effect(() => {
		const required = modeForTheme(theme.current);
		if (required && required !== 'system' && userPrefersMode.current !== required) {
			setMode(required);
		}
	});

	// Density, type scale, motion and focus outlines are plain `<html>` switches.
	$effect(() => {
		applyPreferences(preferences.value);
	});

	let mounted = $state(false);
	let connectionStatus = $state<'offline' | 'restored' | null>(null);

	let badgeTimer: number | undefined;
	let previousBadgePreference = untrack(() => preferences.value.showConnectionBadge);
	let previousBadgePreview = 0;

	function showConnectionBadge(status: 'offline' | 'restored', temporary = false) {
		window.clearTimeout(badgeTimer);
		connectionStatus = status;
		if (temporary) badgeTimer = window.setTimeout(() => (connectionStatus = null), 3000);
	}

	$effect(() => {
		if (!mounted) return;
		const enabled = preferences.value.showConnectionBadge;
		const preview = preferences.connectionBadgePreview;
		if (!enabled) {
			window.clearTimeout(badgeTimer);
			connectionStatus = null;
		} else if (preview !== previousBadgePreview) {
			showConnectionBadge(navigator.onLine ? 'restored' : 'offline', true);
		} else if (!previousBadgePreference && !navigator.onLine) {
			showConnectionBadge('offline');
		}
		previousBadgePreview = preview;
		previousBadgePreference = enabled;
	});

	onMount(() => {
		if (accessToken.isExpired) {
			accessToken.clear();
			isAuthenticated.set(false);
		}
		mounted = true;

		const markOffline = () => {
			if (preferences.value.showConnectionBadge) showConnectionBadge('offline');
		};
		const markOnline = () => {
			if (preferences.value.showConnectionBadge) showConnectionBadge('restored', true);
		};
		if (!navigator.onLine) markOffline();
		window.addEventListener('offline', markOffline);
		window.addEventListener('online', markOnline);
		return () => {
			window.clearTimeout(badgeTimer);
			window.removeEventListener('offline', markOffline);
			window.removeEventListener('online', markOnline);
		};
	});

	let isPublicPage = $derived(
		page.url.pathname === '/' ||
			page.url.pathname === '/login' ||
			page.url.pathname.startsWith('/offline') ||
			page.url.pathname.startsWith('/setup')
	);

	let isGuestRoute = $derived(page.url.pathname === '/' || page.url.pathname === '/login');

	// The tab follows the route, so it names the page instead of keeping the
	// title of whichever page was rendered last (e.g. "Myaaw | Login").
	let documentMeta = $derived({
		title: documentTitle(page.url.pathname),
		description: documentDescription(page.url.pathname)
	});

	async function fetchInitialData() {
		if (isInitialDataLoaded.value) return;
		try {
			await Promise.all([authStore.loadProfile(), settingsStore.loadOnce()]);
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
				void goto(authStore.mustChangePassword ? '/app/profile?force=1' : '/app', {
					replaceState: true
				});
			}
		} else if (!isPublicPage) {
			void goto('/login', { replaceState: true });
		}
	});

	// A fresh install still uses the seeded password. Keep the reader on the
	// profile page until it is changed; the login redirect is only a hint, this
	// is the actual guard for direct navigation and reloads.
	$effect(() => {
		if (!mounted || !isAuthenticated.value) return;
		if (!authStore.mustChangePassword) return;
		if (page.url.pathname !== '/app/profile') {
			void goto('/app/profile?force=1');
		}
	});
</script>

<SEO title={documentMeta.title} description={documentMeta.description} />
<ModeWatcher />
<ReloadPrompt />

<div class="min-h-screen flex flex-col relative overflow-x-hidden">
	{#if !mounted || (isAuthenticated.value && isGuestRoute)}
		<SplashScreen />
	{:else}
		<LoadingBar />
		<main class="flex-1 w-full relative">
			{@render children()}
		</main>
		{#if connectionStatus && page.url.pathname !== '/offline'}
			<div
				role="status"
				class="pointer-events-none fixed top-2 left-1/2 z-40 flex -translate-x-1/2 items-center gap-1.5 rounded-full border bg-background px-2.5 py-1 text-xs shadow-sm"
				class:text-muted-foreground={connectionStatus === 'offline'}
				class:text-emerald-700={connectionStatus === 'restored'}
				class:dark:text-emerald-400={connectionStatus === 'restored'}
			>
				{#if connectionStatus === 'restored'}
					<Wifi class="size-3.5 shrink-0" aria-hidden="true" />
					{t('offline.restored')}
				{:else}
					<WifiOff class="size-3.5 shrink-0" aria-hidden="true" />
					{t('offline.indicator')}
				{/if}
			</div>
		{/if}
		<Toaster position="bottom-right" richColors />
	{/if}
</div>
