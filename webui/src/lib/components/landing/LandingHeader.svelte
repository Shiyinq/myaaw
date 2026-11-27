<script lang="ts">
	import { ArrowUpRight } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { motionButton } from '$lib/landing/motion';
	import { LANDING_SECTIONS } from '$lib/landing/sections';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';
	import { isAuthenticated } from '$lib/stores';

	const { t, locale, availableLocales, changeLocale } = useTranslation();

	const signedIn = $derived(isAuthenticated.value);
	const signInHref = $derived(signedIn ? '/app' : '/login');
	const signInLabel = $derived(signedIn ? t('landing.hero.primary') : t('landing.nav.signin'));

	function handleHome(event: MouseEvent) {
		if (window.location.pathname !== '/') return;
		event.preventDefault();
		landingScroll.scrollTo(0);
		history.replaceState(null, '', '/');
	}

	function handleNav(event: MouseEvent, id: string) {
		event.preventDefault();
		landingScroll.scrollTo(`#${id}`, { offset: -64 });
		history.replaceState(null, '', `#${id}`);
	}
</script>

<a
	href="#main"
	class="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-primary focus:px-3 focus:py-2 focus:text-xs focus:font-medium focus:text-primary-foreground focus:ring-2 focus:ring-ring focus:outline-none"
>
	{t('landing.nav.skip')}
</a>

<header class="fixed inset-x-0 top-0 z-40 border-b border-border/70">
	<div class="frost-chrome relative bg-background/80 backdrop-blur-md">
		<div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:px-6">
			<a
				href="/"
				onclick={handleHome}
				use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}
				class="flex shrink-0 items-center gap-2 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
			>
				<img src="/logo.png" alt="" width="24" height="24" class="size-6 rounded-md object-cover" />
				<span class="text-sm font-bold tracking-tight text-foreground">Myaaw</span>
			</a>

			<span
				class="header-mono hidden shrink-0 rounded border border-border/70 bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground xl:inline"
			>
				{t('landing.footer.tagline')}
			</span>

			<nav
				aria-label={t('landing.trail.sections')}
				class="ml-auto hidden min-w-0 items-center gap-3 md:flex lg:gap-5"
			>
				{#each LANDING_SECTIONS as section (section.id)}
					{@const active = landingScroll.activeSection === section.id}
					<a
						href={`#${section.id}`}
						onclick={(event) => handleNav(event, section.id)}
						aria-current={active ? 'location' : undefined}
						use:motionButton={{ scale: 0.97, hoverScale: 1.03 }}
						class="header-label relative inline-flex shrink-0 items-center gap-1 rounded px-1 py-1 whitespace-nowrap transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {active
							? 'text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
					>
						<span class={active ? 'text-primary' : 'text-foreground/45'}>{section.num}</span>
						<span>{t(section.navKey)}</span>
						{#if active}
							<span
								aria-hidden="true"
								class="absolute inset-x-1 -bottom-0.5 h-px rounded-full bg-primary"
							></span>
						{/if}
					</a>
				{/each}
			</nav>

			<div class="ml-auto flex shrink-0 items-center gap-2 md:ml-3 lg:ml-4">
				<div
					class="flex items-center rounded-md border border-border/70 bg-muted/30 p-0.5"
					role="group"
					aria-label={t('landing.nav.language')}
				>
					{#each availableLocales as option (option.code)}
						<button
							type="button"
							aria-pressed={locale.value === option.code}
							onclick={() => changeLocale(option.code)}
							use:motionButton={{ scale: 0.94, hoverScale: 1.03 }}
							class="header-mono rounded px-1.5 py-0.5 text-[10px] tracking-widest uppercase transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {locale.value ===
							option.code
								? 'bg-accent text-accent-foreground'
								: 'text-muted-foreground hover:text-foreground'}"
						>
							{option.code}
						</button>
					{/each}
				</div>

				<span class="inline-flex shrink-0" use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}>
					<Button href={signInHref} size="sm" class="gap-1.5">
						<span class="hidden sm:inline">{signInLabel}</span>
						<span class="sm:hidden">
							{signedIn ? t('landing.nav.open_short') : t('landing.nav.signin')}
						</span>
						<ArrowUpRight class="size-3.5" />
					</Button>
				</span>
			</div>
		</div>

		<div
			aria-hidden="true"
			class="pointer-events-none absolute inset-x-0 bottom-0 h-[2px] overflow-hidden bg-border/30"
		>
			<div
				class="h-full w-full origin-left bg-primary transition-opacity duration-200"
				style="transform: scaleX({landingScroll.progress}); opacity: {landingScroll.progress > 0.002
					? 1
					: 0};"
			></div>
		</div>
	</div>
</header>

<style>
	.header-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.header-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.35;
		letter-spacing: 0.14em;
		text-transform: uppercase;
	}
</style>
