<script lang="ts">
	import { onMount } from 'svelte';
	import { HeartPulse, Monitor, RadioTower, Send, Terminal } from 'lucide-svelte';
	import DiscordIcon from '$lib/components/common/DiscordIcon.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isReducedMotion, loadGsap, motionCard } from '$lib/landing/motion';
	import { DOOR_ITEMS } from '$lib/landing/sections';

	const { t } = useTranslation();

	let sectionEl: HTMLElement | null = $state(null);

	onMount(() => {
		if (!sectionEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !sectionEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				gsap.from('[data-doors-hairline]', {
					scaleX: 0,
					transformOrigin: 'left center',
					duration: 0.8,
					ease: 'power3.out',
					scrollTrigger: {
						trigger: sectionEl,
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-doors-intro]', {
					y: 18,
					opacity: 0,
					duration: 0.65,
					stagger: 0.08,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: sectionEl,
						start: 'top 82%',
						once: true
					}
				});

				gsap.from('[data-door-card]', {
					y: 22,
					opacity: 0,
					duration: 0.6,
					stagger: 0.07,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-doors-grid]',
						start: 'top 84%',
						once: true
					}
				});
			}, sectionEl);
		});

		return () => {
			cancelled = true;
			ctx?.revert();
		};
	});
</script>

<section
	id="doors"
	data-landing-section
	bind:this={sectionEl}
	class="relative mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28"
>
	<div class="mb-12">
		<div class="mb-5 flex items-center gap-3">
			<span class="landing-label text-primary">04</span>
			<span class="landing-label text-muted-foreground">{t('landing.doors.eyebrow')}</span>
		</div>
		<div data-doors-hairline class="mb-8 h-px w-full bg-border/60"></div>

		<div class="grid gap-6 lg:grid-cols-12 lg:items-end">
			<h2
				data-doors-intro
				class="min-w-0 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl lg:col-span-7"
			>
				{t('landing.doors.title')}
			</h2>
			<p
				data-doors-intro
				class="min-w-0 text-sm leading-relaxed text-muted-foreground sm:text-base lg:col-span-5"
			>
				{t('landing.doors.lead')}
			</p>
		</div>
	</div>

	<div data-doors-grid class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each DOOR_ITEMS as door (door.id)}
			<article
				data-door-card
				data-slot="card"
				use:motionCard
				class="frost-card group flex min-w-0 flex-col justify-between rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
			>
				<div>
					<div class="flex items-center justify-between gap-3">
						<span class="door-icon text-foreground">
							{#if door.id === 'telegram'}
								<Send class="size-4" />
							{:else if door.id === 'discord'}
								<DiscordIcon class="size-4" />
							{:else if door.id === 'web'}
								<Monitor class="size-4" />
							{:else if door.id === 'cli'}
								<Terminal class="size-4" />
							{:else if door.id === 'api'}
								<RadioTower class="size-4" />
							{:else}
								<HeartPulse class="size-4" />
							{/if}
						</span>
						<span
							class="landing-mono rounded-full border border-border/60 bg-muted/40 px-2.5 py-0.5 text-[10px] text-muted-foreground"
						>
							{t(door.tagKey)}
						</span>
					</div>

					<h3 class="mt-4 text-base font-semibold text-foreground">{t(door.titleKey)}</h3>
					<p class="mt-2 text-xs leading-relaxed text-muted-foreground sm:text-sm">
						{t(door.bodyKey)}
					</p>
				</div>

				<div class="mt-5 border-t border-border/60 pt-3">
					<p class="landing-mono truncate text-[11px] text-muted-foreground">
						{t(door.metaKey)}
					</p>
				</div>
			</article>
		{/each}
	</div>
</section>

<style>
	.landing-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.landing-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}

	.door-icon {
		display: grid;
		flex: none;
		place-items: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid color-mix(in oklab, var(--border) 75%, transparent);
		border-radius: calc(var(--radius) - 1px);
		background: color-mix(in oklab, var(--muted) 40%, transparent);
		transition: transform 0.3s cubic-bezier(0.22, 1, 0.36, 1);
	}

	:global(.group:hover) .door-icon {
		transform: translateY(-1px) rotate(-4deg);
	}

	@media (prefers-reduced-motion: reduce) {
		.door-icon {
			transition: none;
		}
	}

	:global(html[data-reduce-motion='true']) .door-icon {
		transition: none;
	}
</style>
