<script lang="ts">
	import { page } from '$app/state';
	import { fly, fade } from 'svelte/transition';
	import SEO from '$lib/components/common/SEO.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Search, ServerCrash, ShieldAlert, WifiOff, Home, RefreshCw } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';

	const { t } = useTranslation();

	function getErrorInfo(code: number) {
		switch (code) {
			case 404:
				return {
					title: t('errors.404.title'),
					subtitle: t('errors.404.subtitle'),
					description: t('errors.404.description'),
					icon: Search,
					color: 'text-primary'
				};
			case 500:
				return {
					title: t('errors.500.title'),
					subtitle: t('errors.500.subtitle'),
					description: t('errors.500.description'),
					icon: ServerCrash,
					color: 'text-destructive'
				};
			case 403:
				return {
					title: t('errors.403.title'),
					subtitle: t('errors.403.subtitle'),
					description: t('errors.403.description'),
					icon: ShieldAlert,
					color: 'text-destructive'
				};
			case 401:
				return {
					title: t('errors.401.title'),
					subtitle: t('errors.401.subtitle'),
					description: t('errors.401.description'),
					icon: ShieldAlert,
					color: 'text-primary'
				};
			default:
				return {
					title: t('errors.default.title'),
					subtitle: t('errors.default.subtitle'),
					description: t('errors.default.description'),
					icon: WifiOff,
					color: 'text-muted-foreground'
				};
		}
	}

	let status = $derived(page.status);
	let errorInfo = $derived(getErrorInfo(status));
</script>

<SEO title="FASMO | {status} - {errorInfo.title}" />

<div
	class="min-h-screen flex items-center justify-center p-6 bg-background relative overflow-hidden"
	in:fly={{ y: 20, duration: 1000 }}
>
	<!-- Decorative background element -->
	<div
		class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[500px] h-[500px] bg-primary/5 rounded-full blur-3xl -z-10"
	></div>

	<div class="w-full max-w-lg text-center space-y-6">
		<div class="flex flex-col items-center justify-center space-y-4">
			<div
				class="w-24 h-24 rounded-3xl bg-background border border-border shadow-2xl flex items-center justify-center {errorInfo.color}"
				in:fly={{ y: -20, duration: 800 }}
			>
				<errorInfo.icon size={48} strokeWidth={1.5} />
			</div>
			<div
				class="text-8xl font-black leading-none opacity-20 select-none tracking-tighter"
				in:fade={{ duration: 1000 }}
			>
				{status}
			</div>
		</div>

		<div class="space-y-3" in:fly={{ y: 10, delay: 400, duration: 600 }}>
			<h1 class="text-4xl font-black tracking-tight text-foreground sm:text-5xl uppercase italic">
				{errorInfo.title}
			</h1>
			<p class="text-xl font-bold text-foreground/90">
				{errorInfo.subtitle}
			</p>
			<p class="text-muted-foreground max-w-md mx-auto leading-relaxed text-sm sm:text-base">
				{errorInfo.description}
			</p>
		</div>

		{#if page.error?.message && page.error.message !== errorInfo.subtitle}
			<div
				class="max-w-md mx-auto p-4 bg-muted/50 rounded-xl border border-border/50 font-mono text-xs text-destructive overflow-x-auto"
				in:fly={{ y: 10, delay: 600, duration: 600 }}
			>
				<code>{page.error.message}</code>
			</div>
		{/if}

		<div
			class="flex flex-col sm:flex-row gap-4 justify-center items-center pt-4"
			in:fly={{ y: 10, delay: 800, duration: 600 }}
		>
			<Button href="/" size="lg" class="w-full sm:w-auto px-8 rounded-full font-bold">
				<Home class="mr-2 w-5 h-5" />
				{t('errors.goHome')}
			</Button>
			<Button
				variant="outline"
				size="lg"
				class="w-full sm:w-auto px-8 rounded-full font-bold"
				onclick={() => window.location.reload()}
			>
				<RefreshCw class="mr-2 w-5 h-5" />
				{t('errors.tryAgain')}
			</Button>
		</div>
	</div>
</div>
