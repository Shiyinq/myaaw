<script lang="ts">
	import { onMount } from 'svelte';
	import { Brain, Cable, Cpu, KeyRound, ShieldCheck, Terminal } from 'lucide-svelte';
	import RunPreview from '$lib/components/landing/RunPreview.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import {
		isReducedMotion,
		loadGsap,
		motionButton,
		motionCard,
		motionPop
	} from '$lib/landing/motion';
	import {
		RUNTIME_PRESETS,
		type RuntimeCategory,
		type RuntimePresetId
	} from '$lib/landing/sections';

	const { t } = useTranslation();

	const CATEGORIES: readonly RuntimeCategory[] = ['native', 'vendor', 'oss', 'custom'];
	const LOOP_NOTES = ['note_1', 'note_2', 'note_3'] as const;

	let rootEl: HTMLElement | null = $state(null);
	let activeCategory = $state<RuntimeCategory | null>(null);
	let selectedId = $state<RuntimePresetId>('claude_code');

	const visiblePresets = $derived(
		activeCategory
			? RUNTIME_PRESETS.filter((preset) => preset.category === activeCategory)
			: RUNTIME_PRESETS
	);

	const selectedRuntime = $derived(
		RUNTIME_PRESETS.find((preset) => preset.id === selectedId) ?? RUNTIME_PRESETS[0]
	);

	function toggleCategory(category: RuntimeCategory) {
		activeCategory = activeCategory === category ? null : category;
		const matching = activeCategory
			? RUNTIME_PRESETS.filter((preset) => preset.category === activeCategory)
			: RUNTIME_PRESETS;
		if (matching.length > 0 && !matching.some((preset) => preset.id === selectedId)) {
			selectedId = matching[0].id;
		}
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

		<!-- 9 Runtime Presets Grid -->
		<div data-rt-grid class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each visiblePresets as preset (preset.id)}
				{@const isSelected = selectedRuntime.id === preset.id}
				<button
					type="button"
					data-rt-card
					data-slot="card"
					aria-pressed={isSelected}
					onclick={() => (selectedId = preset.id)}
					use:motionCard={{ lift: 3, scale: 1.008 }}
					class="frost-card group flex min-w-0 flex-col rounded-xl bg-card p-4 text-left text-card-foreground ring-1 transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {isSelected
						? 'ring-primary bg-card'
						: 'ring-(--surface-ring) hover:ring-foreground/25'}"
				>
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="flex items-center gap-2">
								<span
									class="size-2 shrink-0 rounded-full {isSelected
										? 'bg-primary'
										: 'bg-muted-foreground/40 group-hover:bg-primary/60'}"
								></span>
								<span class="truncate text-sm font-semibold text-foreground">{preset.name}</span>
							</div>
							<span
								class="rt-mono mt-1 block text-[10px] tracking-wider text-muted-foreground uppercase"
							>
								{t(`landing.runtimes.categories.${preset.category}`)}
							</span>
						</div>

						<span
							class="rt-mono shrink-0 rounded border border-border/70 bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground"
						>
							{preset.protocol}
						</span>
					</div>

					<div
						class="mt-3 flex items-center justify-between gap-2 border-t border-border/50 pt-2.5"
					>
						<code class="rt-mono truncate text-[11px] text-foreground/80">
							{preset.command}
						</code>
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
				</button>
			{/each}
		</div>

		<!-- Live Inspection Bar -->
		<div class="mt-4">
			{#key selectedRuntime.id}
				<div
					use:motionPop={{ y: 8, duration: 0.28 }}
					data-slot="card"
					class="frost-card rounded-xl bg-card/90 p-4 text-card-foreground ring-1 ring-(--surface-ring) sm:p-5"
				>
					<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
						<div class="min-w-0 space-y-1.5">
							<div class="flex flex-wrap items-center gap-2">
								<Cpu class="size-4 shrink-0 text-primary" />
								<span class="text-sm font-semibold text-foreground">{selectedRuntime.name}</span>
								<span
									class="rt-mono rounded border border-border/70 bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground"
								>
									{selectedRuntime.protocol}
								</span>
								<span
									class="rt-mono rounded px-1.5 py-0.5 text-[10px] {selectedRuntime.bridgeSupport
										? 'bg-primary/10 text-primary'
										: 'bg-muted/60 text-muted-foreground'}"
								>
									{selectedRuntime.bridgeSupport
										? t('landing.runtimes.bridge_badge')
										: t('landing.runtimes.native_tag')}
								</span>
							</div>
							<p class="text-sm leading-relaxed text-muted-foreground">
								{t(selectedRuntime.summaryKey)}
							</p>
						</div>

						<div
							class="rt-mono flex shrink-0 items-center gap-2 rounded-lg border border-border/70 bg-muted/40 px-3 py-2 text-xs text-foreground"
						>
							<Terminal class="size-3.5 shrink-0 text-muted-foreground" />
							<code class="truncate">{selectedRuntime.command}</code>
						</div>
					</div>
				</div>
			{/key}
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
