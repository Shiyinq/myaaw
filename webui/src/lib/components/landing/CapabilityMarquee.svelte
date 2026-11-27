<script lang="ts">
	import { onMount } from 'svelte';
	import { PawPrint } from 'lucide-svelte';
	import { createReducedMotionState, isReducedMotion, loadGsap } from '$lib/landing/motion';
	import { MARQUEE_PRIMARY, MARQUEE_SECONDARY } from '$lib/landing/sections';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';

	const reducedMotion = createReducedMotionState();

	let rootEl: HTMLDivElement | null = $state(null);
	let trackPrimaryEl: HTMLDivElement | null = $state(null);
	let trackSecondaryEl: HTMLDivElement | null = $state(null);

	let inView = $state(true);
	let hovered = $state(false);

	type GsapTween = {
		timeScale: (value: number) => void;
		pause: () => void;
		play: () => void;
	};

	let primaryTween: GsapTween | null = null;
	let secondaryTween: GsapTween | null = null;

	$effect(() => {
		const reduced = reducedMotion.current;
		const speed = 1 + Math.min(1.4, Math.abs(landingScroll.velocity) * 0.045);
		const shouldRun = !reduced && inView && !hovered;

		for (const tween of [primaryTween, secondaryTween]) {
			if (!tween) continue;
			if (shouldRun) {
				tween.timeScale(speed);
				tween.play();
			} else {
				tween.pause();
			}
		}
	});

	onMount(() => {
		if (!rootEl) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		const observer = new IntersectionObserver(
			(entries) => {
				inView = entries[0]?.isIntersecting ?? true;
			},
			{ rootMargin: '120px 0px' }
		);
		observer.observe(rootEl);

		if (!isReducedMotion()) {
			void loadGsap().then((bundle) => {
				if (cancelled || !bundle || !rootEl || !trackPrimaryEl || !trackSecondaryEl) return;
				if (isReducedMotion()) return;
				const { gsap } = bundle;

				ctx = gsap.context(() => {
					primaryTween = gsap.to(trackPrimaryEl, {
						xPercent: -50,
						duration: 38,
						repeat: -1,
						ease: 'none'
					});

					secondaryTween = gsap.fromTo(
						trackSecondaryEl,
						{ xPercent: -50 },
						{
							xPercent: 0,
							duration: 44,
							repeat: -1,
							ease: 'none'
						}
					);

					if (!inView || hovered || reducedMotion.current) {
						primaryTween.pause();
						secondaryTween.pause();
					}
				}, rootEl);
			});
		}

		return () => {
			cancelled = true;
			observer.disconnect();
			reducedMotion.destroy();
			primaryTween = null;
			secondaryTween = null;
			ctx?.revert();
		};
	});
</script>

<div
	bind:this={rootEl}
	role="presentation"
	aria-hidden="true"
	onpointerenter={() => (hovered = true)}
	onpointerleave={() => (hovered = false)}
	class="border-b border-border/60 bg-muted/20 py-3.5 select-none"
>
	<div class="marquee-mask flex flex-col gap-2.5">
		<div bind:this={trackPrimaryEl} class="landing-gpu flex w-max items-center">
			{#each [0, 1] as copy (copy)}
				<div class="flex shrink-0 items-center gap-6 pr-6">
					{#each MARQUEE_PRIMARY as item (item)}
						<span class="marquee-item inline-flex items-center gap-2.5 text-muted-foreground">
							<PawPrint class="size-3 shrink-0 text-primary/60" />
							<span>{item}</span>
						</span>
					{/each}
				</div>
			{/each}
		</div>

		<div bind:this={trackSecondaryEl} class="landing-gpu flex w-max items-center">
			{#each [0, 1] as copy (copy)}
				<div class="flex shrink-0 items-center gap-6 pr-6">
					{#each MARQUEE_SECONDARY as item (item)}
						<span class="marquee-item inline-flex items-center gap-2.5 text-foreground/60">
							<PawPrint class="size-3 shrink-0 text-muted-foreground/50" />
							<span>{item}</span>
						</span>
					{/each}
				</div>
			{/each}
		</div>
	</div>
</div>

<style>
	.marquee-mask {
		overflow: hidden;
		mask-image: linear-gradient(to right, transparent, black 6%, black 94%, transparent);
		-webkit-mask-image: linear-gradient(to right, transparent, black 6%, black 94%, transparent);
	}

	.marquee-item {
		font-family: var(--font-ui-mono);
		font-size: 0.7rem;
		line-height: 1.3;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		white-space: nowrap;
	}
</style>
