<script lang="ts">
	import { onMount } from 'svelte';
	import { Brain, Cable, KeyRound, ShieldCheck, Terminal } from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import RunPreview from '$lib/components/landing/RunPreview.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isReducedMotion, loadGsap, motionButton, motionCard } from '$lib/landing/motion';
	import { RUNTIME_PRESETS, type RuntimeCategory } from '$lib/landing/sections';

	const { t } = useTranslation();
	const dark = $derived(mode.current === 'dark');

	const CATEGORIES: readonly RuntimeCategory[] = ['native', 'vendor', 'oss', 'custom'];
	const LOOP_NOTES = ['note_1', 'note_2', 'note_3'] as const;

	let rootEl: HTMLElement | null = $state(null);
	let activeCategory = $state<RuntimeCategory | null>(null);

	const visiblePresets = $derived(
		activeCategory
			? RUNTIME_PRESETS.filter((preset) => preset.category === activeCategory)
			: RUNTIME_PRESETS
	);

	function toggleCategory(category: RuntimeCategory) {
		activeCategory = activeCategory === category ? null : category;
	}

	onMount(() => {
		if (!rootEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !rootEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				gsap.from('[data-rt-hairline]', {
					scaleX: 0,
					transformOrigin: 'left center',
					duration: 0.75,
					ease: 'power3.out',
					clearProps: 'transform',
					scrollTrigger: {
						trigger: rootEl,
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-rt-head]', {
					y: 18,
					opacity: 0,
					duration: 0.6,
					stagger: 0.07,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: rootEl,
						start: 'top 82%',
						once: true
					}
				});

				gsap.from('[data-rt-card]', {
					y: 18,
					opacity: 0,
					duration: 0.52,
					stagger: 0.04,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-rt-grid]',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-rt-pillar]', {
					y: 18,
					opacity: 0,
					duration: 0.55,
					stagger: 0.08,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-rt-pillars]',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-rt-loop]', {
					y: 18,
					opacity: 0,
					duration: 0.58,
					stagger: 0.09,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-rt-loop-wrap]',
						start: 'top 84%',
						once: true
					}
				});
			}, rootEl);
		});

		return () => {
			cancelled = true;
			ctx?.revert();
		};
	});
</script>

<section
	id="runtimes"
	data-landing-section
	bind:this={rootEl}
	class="border-b border-border/60 py-16 sm:py-24"
>
	<div class="mx-auto max-w-6xl px-4 sm:px-6">
		<div data-rt-hairline class="mb-6 h-px w-full bg-border/70"></div>

		<div class="grid gap-6 lg:grid-cols-[1.2fr_0.8fr] lg:items-end">
			<div class="min-w-0">
				<p data-rt-head class="rt-label flex items-center gap-2 text-muted-foreground">
					<span class="text-primary">01</span>
					<span class="text-border">/</span>
					<span>{t('landing.runtimes.eyebrow')}</span>
				</p>
				<h2
					data-rt-head
					class="mt-3 text-2xl leading-[1.12] font-bold tracking-tight text-foreground sm:text-4xl"
				>
					{t('landing.runtimes.title')}
				</h2>
			</div>

			<p data-rt-head class="text-sm leading-relaxed text-muted-foreground sm:text-[15px]">
				{t('landing.runtimes.lead')}
			</p>
		</div>

		<!-- Category filter bar -->
		<div class="mt-8 flex flex-wrap items-center gap-2">
			{#each CATEGORIES as category (category)}
				{@const active = activeCategory === category}
				<button
					type="button"
					aria-pressed={active}
					onclick={() => toggleCategory(category)}
					use:motionButton={{ scale: 0.96, hoverScale: 1.02 }}
					class="rt-mono inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-[11px] tracking-wide transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {active
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border/70 bg-muted/40 text-muted-foreground hover:border-border hover:text-foreground'}"
				>
					<span>{t(`landing.runtimes.categories.${category}`)}</span>
					<span class="opacity-70">
						{RUNTIME_PRESETS.filter((preset) => preset.category === category).length}
					</span>
				</button>
			{/each}
		</div>

		<!-- 9 Runtime Presets Grid (matches Settings -> External Runtimes card design without host status/action buttons) -->
		<div data-rt-grid class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each visiblePresets as preset (preset.id)}
				<article
					data-rt-card
					data-slot="card"
					use:motionCard={{ lift: 3, scale: 1.008, pressable: false }}
					class="frost-card group flex min-w-0 flex-col justify-between overflow-hidden rounded-xl border border-border/80 bg-card text-left text-card-foreground transition-colors hover:border-foreground/25"
				>
					<div class="flex flex-1 items-start gap-3 p-4">
						{#if preset.id === 'myaaw'}
							<span
								class="mt-0.5 grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-xl border border-border/70 bg-card shadow-xs"
							>
								<Bloub mood="attentif" size={26} tight color={dark ? 'orange' : 'encre'} />
							</span>
						{:else}
							<RuntimeBadge
								runtimeType={preset.id}
								runtimePreset={preset.id}
								size="lg"
								inline
								class="mt-0.5"
							/>
						{/if}

						<div class="min-w-0 flex-1 space-y-1">
							<div class="flex items-start justify-between gap-2">
								<span class="block min-w-0 truncate text-sm font-semibold text-foreground">
									{preset.name}
								</span>

								<span
									class="rt-mono shrink-0 rounded border border-border/70 bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground"
								>
									{preset.protocol}
								</span>
							</div>

							<p class="text-xs leading-relaxed text-muted-foreground">
								{t(preset.summaryKey)}
							</p>
						</div>
					</div>

					<div
						class="flex items-center justify-between gap-2 border-t border-border/60 bg-muted/25 px-4 py-2"
					>
						<span
							class="inline-flex min-w-0 items-center gap-1.5 font-mono text-[11px] text-muted-foreground"
						>
							<Terminal class="h-3 w-3 shrink-0 opacity-70" />
							<code class="truncate">{preset.command}</code>
						</span>
						<span
							class="rt-mono shrink-0 rounded px-1.5 py-0.5 text-[10px] {preset.bridgeSupport
								? 'bg-primary/10 text-primary'
								: 'bg-muted/60 text-muted-foreground'}"
						>
							{preset.bridgeSupport
								? t('landing.runtimes.bridge_tag')
								: t('landing.runtimes.native_tag')}
						</span>
					</div>
				</article>
			{/each}
		</div>

		<!-- Highlight Strip: mcp-bridge, Native Providers, Thinking Levels -->
		<div data-rt-pillars class="mt-6 grid gap-4 md:grid-cols-3">
			<div
				data-rt-pillar
				data-slot="card"
				use:motionCard={{ lift: 3, pressable: false }}
				class="frost-card min-w-0 rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
			>
				<div class="flex items-center justify-between gap-2">
					<span
						class="inline-flex size-8 items-center justify-center rounded-lg border border-border/70 bg-muted/40 text-primary"
					>
						<Cable class="size-4" />
					</span>
					<span
						class="rt-mono rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px] text-muted-foreground"
					>
						{t('landing.runtimes.bridge_badge')}
					</span>
				</div>
				<h3 class="mt-3.5 text-sm font-semibold text-foreground">
					{t('landing.runtimes.bridge_title')}
				</h3>
				<p class="mt-1.5 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
					{t('landing.runtimes.bridge_body')}
				</p>
			</div>

			<div
				data-rt-pillar
				data-slot="card"
				use:motionCard={{ lift: 3, pressable: false }}
				class="frost-card min-w-0 rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
			>
				<span
					class="inline-flex size-8 items-center justify-center rounded-lg border border-border/70 bg-muted/40 text-primary"
				>
					<KeyRound class="size-4" />
				</span>
				<h3 class="mt-3.5 text-sm font-semibold text-foreground">
					{t('landing.runtimes.providers_title')}
				</h3>
				<p class="mt-1.5 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
					{t('landing.runtimes.providers_body')}
				</p>
			</div>

			<div
				data-rt-pillar
				data-slot="card"
				use:motionCard={{ lift: 3, pressable: false }}
				class="frost-card min-w-0 rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
			>
				<span
					class="inline-flex size-8 items-center justify-center rounded-lg border border-border/70 bg-muted/40 text-primary"
				>
					<Brain class="size-4" />
				</span>
				<h3 class="mt-3.5 text-sm font-semibold text-foreground">
					{t('landing.runtimes.thinking_title')}
				</h3>
				<p class="mt-1.5 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
					{t('landing.runtimes.thinking_body')}
				</p>
			</div>
		</div>

		<!-- Telemetry Off by Default note -->
		<div
			data-rt-pillar
			class="mt-4 flex items-start gap-2.5 rounded-xl border border-border/60 bg-muted/30 px-4 py-3 text-xs leading-relaxed text-muted-foreground"
		>
			<ShieldCheck class="mt-0.5 size-4 shrink-0 text-primary" />
			<p class="min-w-0">{t('landing.runtimes.telemetry_note')}</p>
		</div>

		<!-- The Loop & Observability Trajectory Split -->
		<div
			data-rt-loop-wrap
			class="mt-12 grid gap-8 border-t border-border/60 pt-10 lg:grid-cols-[0.95fr_1.05fr] lg:items-start lg:gap-10"
		>
			<div class="min-w-0">
				<p data-rt-loop class="rt-label text-muted-foreground">{t('landing.loop.index')}</p>
				<h3
					data-rt-loop
					class="mt-2.5 text-xl leading-snug font-bold tracking-tight text-foreground sm:text-2xl"
				>
					{t('landing.loop.title')}
				</h3>
				<p data-rt-loop class="mt-2.5 text-sm leading-relaxed text-muted-foreground">
					{t('landing.loop.body')}
				</p>

				<div class="mt-6 space-y-4">
					{#each LOOP_NOTES as note, idx (note)}
						<div
							data-rt-loop
							class="flex gap-3.5 border-t border-border/50 pt-4 first:border-t-0 first:pt-0"
						>
							<span class="rt-mono mt-0.5 shrink-0 text-xs text-primary">0{idx + 1}</span>
							<div class="min-w-0">
								<h4 class="text-sm font-semibold text-foreground">
									{t(`landing.loop.${note}_title`)}
								</h4>
								<p class="mt-1 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
									{t(`landing.loop.${note}_body`)}
								</p>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<div data-rt-loop class="min-w-0">
				<RunPreview />
			</div>
		</div>
	</div>
</section>

<style>
	.rt-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.rt-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}
</style>
