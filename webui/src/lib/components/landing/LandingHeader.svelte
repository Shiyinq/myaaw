<script lang="ts">
	import { ArrowUpRight } from 'lucide-svelte';
	import { animate, spring } from 'motion';
	import type { Action } from 'svelte/action';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isReducedMotion, motionButton } from '$lib/landing/motion';
	import { LANDING_SECTIONS } from '$lib/landing/sections';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';
	import { isAuthenticated } from '$lib/stores';

	const { t, locale, availableLocales, changeLocale } = useTranslation();

	const signedIn = $derived(isAuthenticated.value);
	const signInHref = $derived(signedIn ? '/app' : '/login');
	const signInLabel = $derived(signedIn ? t('landing.hero.primary') : t('landing.nav.signin'));

	const islandMotion: Action<HTMLElement, { collapsed: boolean; reduced: boolean }> = (
		node,
		initial
	) => {
		const desktop = window.matchMedia('(min-width: 768px)');
		const content = node.firstElementChild as HTMLElement;
		let options = initial;
		let animations: ReturnType<typeof animate>[] = [];

		function update(instant = false, entrance = false) {
			animations.forEach((animation) => animation.stop());
			animations = [];
			const collapsed = desktop.matches && options.collapsed;
			node.inert = collapsed;
			if (!desktop.matches) {
				node.style.clipPath = '';
				node.style.opacity = '';
				content.style.opacity = '';
				return;
			}

			const clipPath = collapsed
				? 'inset(0 calc(50% - 1.75rem) round 1.75rem)'
				: 'inset(0 0% round 2rem)';
			if (instant || options.reduced) {
				node.style.clipPath = clipPath;
				node.style.opacity = collapsed ? '0' : '1';
				content.style.opacity = collapsed ? '0' : '1';
				return;
			}

			animations = [
				animate(
					node,
					{ clipPath },
					{
						type: spring,
						stiffness: entrance ? 120 : collapsed ? 210 : 350,
						damping: entrance ? 24 : collapsed ? 28 : 30,
						mass: 0.9
					}
				),
				animate(
					node,
					{ opacity: collapsed ? 0 : 1 },
					{ duration: entrance ? 0.32 : 0.18, delay: collapsed ? 0.35 : 0 }
				),
				animate(
					content,
					{ opacity: collapsed ? 0 : 1 },
					{ duration: entrance ? 0.25 : 0.16, delay: collapsed ? 0 : entrance ? 0.3 : 0.1 }
				)
			];
		}

		const onResize = () => update(true);
		desktop.addEventListener('change', onResize);
		if (desktop.matches && !options.collapsed && window.scrollY <= 8 && !isReducedMotion()) {
			node.style.clipPath = 'inset(0 calc(50% - 1.75rem) round 1.75rem)';
			node.style.opacity = '0';
			content.style.opacity = '0';
			update(false, true);
		} else {
			update(true);
		}
		return {
			update(next) {
				if (next.collapsed === options.collapsed && next.reduced === options.reduced) return;
				options = next;
				update();
			},
			destroy() {
				desktop.removeEventListener('change', onResize);
				animations.forEach((animation) => animation.stop());
			}
		};
	};

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

<header
	class="landing-header pointer-events-none fixed inset-x-0 top-0 z-40 md:top-3 md:px-6"
	class:scrolled={landingScroll.scrollY > 8}
>
	<div
		use:islandMotion={{ collapsed: landingScroll.scrollY > 8, reduced: landingScroll.reduced }}
		class="landing-island pointer-events-auto relative mx-auto w-full max-w-full border-b border-border/70 bg-background/95 text-foreground backdrop-blur-md md:w-fit md:overflow-hidden md:rounded-[2rem] md:border md:border-border md:shadow-[0_3px_12px_rgba(0,0,0,0.06)]"
	>
		<div class="flex min-h-14 items-center gap-2 px-3 sm:px-4 lg:gap-3">
			<a
				href="/"
				onclick={handleHome}
				use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}
				class="flex shrink-0 items-center gap-2 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring/70 md:rounded-full"
			>
				<img src="/logo.png" alt="" width="24" height="24" class="size-6 rounded-md object-cover" />
				<span class="text-sm font-bold tracking-tight text-foreground">Myaaw</span>
			</a>

			<span
				class="header-mono hidden shrink-0 rounded-full border border-border bg-muted/40 px-2 py-1 text-[10px] text-muted-foreground 2xl:inline"
			>
				{t('landing.footer.tagline')}
			</span>

			<nav
				aria-label={t('landing.trail.sections')}
				class="hidden min-w-0 items-center gap-1 md:flex"
			>
				{#each LANDING_SECTIONS as section (section.id)}
					{@const active = landingScroll.activeSection === section.id}
					<a
						href={`#${section.id}`}
						onclick={(event) => handleNav(event, section.id)}
						aria-current={active ? 'location' : undefined}
						use:motionButton={{ scale: 0.97, hoverScale: 1.03 }}
						class="header-label relative inline-flex shrink-0 items-center gap-1 rounded-full px-1 py-2 whitespace-nowrap lg:px-2 transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/70 {active
							? 'bg-accent text-accent-foreground'
							: 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
					>
						<span class="hidden lg:inline"
							><span class={active ? 'text-primary' : 'text-muted-foreground'}>{section.num}</span
							></span
						>
						<span>{t(section.navKey)}</span>
					</a>
				{/each}
			</nav>

			<div class="ml-auto flex shrink-0 items-center gap-2">
				<div
					class="flex items-center rounded-md border border-border bg-muted/40 p-0.5 md:rounded-full"
					role="group"
					aria-label={t('landing.nav.language')}
				>
					{#each availableLocales as option (option.code)}
						<button
							type="button"
							aria-pressed={locale.value === option.code}
							onclick={() => changeLocale(option.code)}
							use:motionButton={{ scale: 0.94, hoverScale: 1.03 }}
							class="header-mono rounded px-1.5 py-0.5 text-[10px] tracking-widest uppercase transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/70 md:rounded-full md:py-1 {locale.value ===
							option.code
								? 'bg-accent text-accent-foreground'
								: 'text-muted-foreground hover:text-foreground'}"
						>
							{option.code}
						</button>
					{/each}
				</div>

				<span class="inline-flex shrink-0" use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}>
					<Button
						href={signInHref}
						size="sm"
						class="gap-1.5 bg-primary text-primary-foreground hover:bg-primary/90 md:rounded-full"
					>
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
			class="pointer-events-none absolute inset-x-0 bottom-0 h-[2px] overflow-hidden bg-border/30 md:inset-x-8 md:h-px md:bg-transparent"
		>
			<div
				class="h-full w-full origin-left bg-primary/50 transition-opacity duration-200"
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
		font-size: 0.625rem;
		line-height: 1.35;
		letter-spacing: 0.1em;
		text-transform: uppercase;
	}

	@media (min-width: 1024px) {
		.header-label {
			font-size: 0.6875rem;
			letter-spacing: 0.14em;
		}
	}
</style>
