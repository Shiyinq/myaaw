<script lang="ts">
	import { onMount } from 'svelte';
	import { ArrowDown, ArrowUpRight, Cpu } from 'lucide-svelte';
	import AgentField from '$lib/components/landing/AgentField.svelte';
	import RunStream from '$lib/components/landing/RunStream.svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isReducedMotion, loadGsap, motionButton } from '$lib/landing/motion';
	import { REPO_URL } from '$lib/landing/sections';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';
	import { isAuthenticated } from '$lib/stores';

	const { t } = useTranslation();

	const signedIn = $derived(isAuthenticated.value);
	const signInHref = $derived(signedIn ? '/app' : '/login');
	const signInLabel = $derived(signedIn ? t('landing.hero.primary') : t('landing.nav.signin'));

	const HERO_STATS = [
		{ valueKey: 'landing.hero.stat_runtimes_value', labelKey: 'landing.hero.stat_runtimes' },
		{ valueKey: 'landing.hero.stat_terminals_value', labelKey: 'landing.hero.stat_terminals' },
		{ valueKey: 'landing.hero.stat_blocks_value', labelKey: 'landing.hero.stat_blocks' },
		{ valueKey: 'landing.hero.stat_modes_value', labelKey: 'landing.hero.stat_modes' }
	] as const;

	let rootEl: HTMLElement | null = $state(null);
	let fieldParallaxEl: HTMLDivElement | null = $state(null);

	function handleExplore() {
		landingScroll.scrollTo('#runtimes', { offset: -64 });
	}

	onMount(() => {
		if (!rootEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !rootEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				if (fieldParallaxEl) {
					gsap.to(fieldParallaxEl, {
						yPercent: 14,
						ease: 'none',
						scrollTrigger: {
							trigger: rootEl,
							start: 'top top',
							end: 'bottom top',
							scrub: true
						}
					});
				}
			}, rootEl);
		});

		return () => {
			cancelled = true;
			ctx?.revert();
		};
	});
</script>

<section bind:this={rootEl} class="relative overflow-hidden border-b border-border/60">
	<div
		bind:this={fieldParallaxEl}
		aria-hidden="true"
		class="landing-gpu pointer-events-none absolute -top-12 inset-x-0 h-[calc(100svh+2.5rem)]"
	>
		<AgentField class="mask-[radial-gradient(125%_115%_at_50%_46%,black_38%,transparent_94%)]" />
	</div>

	<div
		class="relative mx-auto grid max-w-6xl gap-10 px-4 pt-12 pb-16 sm:px-6 lg:grid-cols-[1.06fr_0.94fr] lg:items-center lg:gap-12 lg:pt-20 lg:pb-24"
	>
		<div class="hero-copy min-w-0">
			<p
				data-hero-eyebrow
				class="hero-label flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground"
			>
				<span class="live-dot shrink-0"></span>
				<span>{t('landing.hero.eyebrow')}</span>
				<span class="text-border" aria-hidden="true">/</span>
				<span class="tracking-normal normal-case text-foreground/80">
					{t('landing.hero.backend_badge')}
				</span>
			</p>

			<h1 class="mt-5 sm:mt-6">
				<span
					data-hero-lead
					class="block text-lg font-medium tracking-tight text-muted-foreground sm:text-2xl lg:text-[1.7rem]"
				>
					{t('landing.hero.lead')}
				</span>

				<span class="hero-heading mt-2 block leading-[1.06] font-bold tracking-[-0.045em]">
					<span class="block overflow-hidden py-0.5">
						<span data-hero-line class="hero-line">
							<span class="glyph" aria-hidden="true"><span class="glyph-node"></span></span>
							<span class="hero-text">{t('landing.hero.line_process')}</span>
						</span>
					</span>

					<span class="block overflow-hidden py-0.5">
						<span data-hero-line class="hero-line">
							<span class="glyph" aria-hidden="true">
								<span class="glyph-bars"><i></i><i></i><i></i></span>
							</span>
							<span class="hero-text">{t('landing.hero.line_memory')}</span>
						</span>
					</span>

					<span class="block overflow-hidden py-0.5">
						<span data-hero-line class="hero-line">
							<span class="glyph glyph-runtime" aria-hidden="true">
								<Cpu class="size-[0.34em]" />
							</span>
							<span class="hero-text">{t('landing.hero.line_runtime')}</span>
						</span>
					</span>
				</span>
			</h1>

			<p
				data-hero-body
				class="mt-6 max-w-xl text-[15px] leading-relaxed text-muted-foreground sm:text-base"
			>
				{t('landing.hero.body')}
			</p>

			<div data-hero-cta class="mt-7 flex flex-wrap items-center gap-3">
				<span class="inline-flex" use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}>
					<Button href={signInHref} size="lg" class="gap-2">
						{signInLabel}
						<ArrowUpRight class="size-4" />
					</Button>
				</span>

				<span class="inline-flex" use:motionButton={{ scale: 0.97, hoverScale: 1.02 }}>
					<Button
						href={REPO_URL}
						target="_blank"
						rel="noreferrer"
						variant="outline"
						size="lg"
						class="gap-2"
					>
						<svg viewBox="0 0 24 24" class="size-4 fill-current" aria-hidden="true">
							<path
								d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.55v-2.17c-3.2.7-3.88-1.36-3.88-1.36-.52-1.34-1.28-1.7-1.28-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.12 3.05.74.8 1.18 1.83 1.18 3.09 0 4.42-2.69 5.39-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.66.8.55A11.5 11.5 0 0 0 12 .5Z"
							/>
						</svg>
						{t('landing.hero.secondary')}
					</Button>
				</span>
			</div>

			<p data-hero-cta class="hero-label mt-5 text-muted-foreground">
				{t('landing.hero.meta')}
			</p>

			<div class="mt-7 grid grid-cols-2 gap-2.5 sm:grid-cols-4">
				{#each HERO_STATS as stat (stat.labelKey)}
					<div
						data-hero-stat
						data-slot="card"
						class="frost-card min-w-0 rounded-xl bg-card/85 px-3 py-2.5 text-card-foreground ring-1 ring-(--surface-ring)"
					>
						<div class="hero-mono text-lg leading-tight font-bold text-foreground sm:text-xl">
							{t(stat.valueKey)}
						</div>
						<div class="mt-0.5 text-[11px] leading-snug text-muted-foreground">
							{t(stat.labelKey)}
						</div>
					</div>
				{/each}
			</div>
		</div>

		<div data-hero-stage class="min-w-0">
			<RunStream />

			<div class="mt-4 flex justify-center lg:justify-end">
				<button
					type="button"
					onclick={handleExplore}
					use:motionButton={{ scale: 0.96, hoverScale: 1.02 }}
					class="hero-label inline-flex items-center gap-2 rounded-md px-2 py-1 text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
				>
					<span class="scroll-line" aria-hidden="true"></span>
					<span>{t('landing.hero.scroll')}</span>
					<ArrowDown class="size-3" />
				</button>
			</div>
		</div>
	</div>
</section>

<style>
	@media (prefers-reduced-motion: no-preference) {
		:global(html:not([data-reduce-motion='true'])) [data-hero-eyebrow],
		:global(html:not([data-reduce-motion='true'])) [data-hero-lead],
		:global(html:not([data-reduce-motion='true'])) [data-hero-line],
		:global(html:not([data-reduce-motion='true'])) [data-hero-body],
		:global(html:not([data-reduce-motion='true'])) [data-hero-cta],
		:global(html:not([data-reduce-motion='true'])) [data-hero-stat],
		:global(html:not([data-reduce-motion='true'])) [data-hero-stage] {
			animation: hero-enter 0.7s cubic-bezier(0.22, 1, 0.36, 1) both;
		}

		:global(html:not([data-reduce-motion='true'])) [data-hero-lead] {
			animation-delay: 0.1s;
		}
		:global(html:not([data-reduce-motion='true'])) [data-hero-line] {
			animation-delay: 0.2s;
		}
		:global(html:not([data-reduce-motion='true'])) [data-hero-body] {
			animation-delay: 0.3s;
		}
		:global(html:not([data-reduce-motion='true'])) [data-hero-cta] {
			animation-delay: 0.4s;
		}
		:global(html:not([data-reduce-motion='true'])) [data-hero-stat] {
			animation-delay: 0.5s;
		}
		:global(html:not([data-reduce-motion='true'])) [data-hero-stage] {
			animation-delay: 0.18s;
		}
	}

	@keyframes hero-enter {
		from {
			opacity: 0;
			transform: translateY(14px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	.hero-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.hero-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}

	.hero-copy {
		container-type: inline-size;
	}

	.hero-heading {
		font-size: clamp(2.05rem, 4.6vw, 4.35rem);
	}

	/* Monospace themes need a slightly tighter container-relative scale so lines never overflow. */
	:global(html[data-theme='terminal']) .hero-heading,
	:global(html[data-theme='typewriter']) .hero-heading,
	:global(html[data-theme='phosphor']) .hero-heading,
	:global(html[data-theme='radar']) .hero-heading,
	:global(html[data-theme='dot-matrix']) .hero-heading {
		font-size: clamp(1.65rem, 9.2cqi, 3.6rem);
	}

	.hero-line {
		display: flex;
		align-items: center;
		gap: 0.14em;
	}

	.hero-text {
		min-width: 0;
	}

	.glyph {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		width: 0.46em;
		height: 0.46em;
		color: color-mix(in oklab, var(--foreground) 50%, transparent);
	}

	.glyph-node {
		position: relative;
		display: block;
		width: 0.11em;
		height: 0.11em;
		border-radius: 9999px;
		background: currentColor;
	}

	.glyph-node::after {
		content: '';
		position: absolute;
		inset: -0.08em;
		border-radius: 9999px;
		border: 1px solid currentColor;
		animation: glyph-ring 2.8s ease-out infinite;
	}

	@keyframes glyph-ring {
		0% {
			transform: scale(0.65);
			opacity: 0.65;
		}
		75%,
		100% {
			transform: scale(1.8);
			opacity: 0;
		}
	}

	.glyph-bars {
		display: flex;
		flex-direction: column;
		gap: 0.045em;
	}

	.glyph-bars i {
		display: block;
		height: 0.035em;
		border-radius: 9999px;
		background: currentColor;
		transform-origin: left center;
		animation: glyph-bars 2.6s ease-in-out infinite;
	}

	.glyph-bars i:nth-child(1) {
		width: 0.2em;
	}

	.glyph-bars i:nth-child(2) {
		width: 0.3em;
		animation-delay: 0.2s;
	}

	.glyph-bars i:nth-child(3) {
		width: 0.25em;
		animation-delay: 0.4s;
	}

	@keyframes glyph-bars {
		0%,
		100% {
			opacity: 0.35;
			transform: scaleX(0.8);
		}
		50% {
			opacity: 1;
			transform: scaleX(1);
		}
	}

	.glyph-runtime {
		animation: glyph-pulse 2.8s ease-in-out infinite;
	}

	@keyframes glyph-pulse {
		0%,
		100% {
			opacity: 0.65;
			transform: scale(0.95);
		}
		50% {
			opacity: 1;
			transform: scale(1.06);
		}
	}

	.live-dot {
		position: relative;
		width: 0.4rem;
		height: 0.4rem;
		border-radius: 9999px;
		background: var(--primary);
	}

	.live-dot::after {
		content: '';
		position: absolute;
		inset: -0.18rem;
		border-radius: 9999px;
		border: 1px solid var(--primary);
		opacity: 0.4;
		animation: live 2.4s ease-out infinite;
	}

	@keyframes live {
		0% {
			transform: scale(0.6);
			opacity: 0.5;
		}
		70%,
		100% {
			transform: scale(1.7);
			opacity: 0;
		}
	}

	.scroll-line {
		position: relative;
		display: inline-block;
		width: 1.5rem;
		height: 1px;
		overflow: hidden;
		background: var(--border);
	}

	.scroll-line::after {
		content: '';
		position: absolute;
		inset: 0;
		width: 0.6rem;
		background: var(--foreground);
		animation: scroll-line 2.4s ease-in-out infinite;
	}

	@keyframes scroll-line {
		0% {
			transform: translateX(-100%);
		}
		60%,
		100% {
			transform: translateX(280%);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.glyph-node::after,
		.glyph-bars i,
		.glyph-runtime,
		.live-dot::after,
		.scroll-line::after {
			animation: none;
		}
	}

	:global(html[data-reduce-motion='true']) .glyph-node::after,
	:global(html[data-reduce-motion='true']) .glyph-bars i,
	:global(html[data-reduce-motion='true']) .glyph-runtime,
	:global(html[data-reduce-motion='true']) .live-dot::after,
	:global(html[data-reduce-motion='true']) .scroll-line::after {
		animation: none;
	}
</style>
