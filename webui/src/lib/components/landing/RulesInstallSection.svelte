<script lang="ts">
	import { onMount } from 'svelte';
	import { ArrowRight, ArrowUpRight, Terminal } from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import {
		isReducedMotion,
		loadGsap,
		motionButton,
		motionCard,
		motionPop,
		motionPulse
	} from '$lib/landing/motion';
	import {
		INSTALL_METHODS,
		RELEASES_URL,
		REPO_URL,
		RULE_ITEMS,
		SETUP_STEPS,
		type InstallMethodId
	} from '$lib/landing/sections';
	import { isAuthenticated } from '$lib/stores';

	const { t } = useTranslation();

	let rootEl: HTMLDivElement | null = $state(null);
	let bloubWrapEl: HTMLButtonElement | null = $state(null);
	let copyWrapEl: HTMLDivElement | null = $state(null);
	let activeMethod = $state<InstallMethodId>('cli');

	const dark = $derived(mode.current === 'dark');
	const signedIn = $derived(isAuthenticated.value);
	const signInHref = $derived(signedIn ? '/app' : '/login');
	const currentMethod = $derived(
		INSTALL_METHODS.find((m) => m.id === activeMethod) ?? INSTALL_METHODS[0]
	);

	function pulseBloub() {
		motionPulse(bloubWrapEl, { scale: 1.1, duration: 0.3 });
	}

	function pulseCopy() {
		motionPulse(copyWrapEl, { scale: 1.12, duration: 0.25 });
	}

	onMount(() => {
		if (!rootEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !rootEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				gsap.from('[data-rules-hairline]', {
					scaleX: 0,
					transformOrigin: 'left center',
					duration: 0.8,
					ease: 'power3.out',
					scrollTrigger: {
						trigger: '#rules',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-rule-row]', {
					y: 16,
					opacity: 0,
					duration: 0.55,
					stagger: 0.06,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-rules-list]',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-closing-hairline]', {
					scaleX: 0,
					transformOrigin: 'left center',
					duration: 0.8,
					ease: 'power3.out',
					scrollTrigger: {
						trigger: '#closing',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-closing-intro]', {
					y: 20,
					opacity: 0,
					duration: 0.65,
					stagger: 0.08,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '#closing',
						start: 'top 82%',
						once: true
					}
				});

				gsap.from('[data-step-card]', {
					y: 18,
					opacity: 0,
					duration: 0.55,
					stagger: 0.07,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-steps-grid]',
						start: 'top 85%',
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

<div bind:this={rootEl}>
	<!-- Section 05: Principles & Boundaries -->
	<section
		id="rules"
		data-landing-section
		class="relative mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28"
	>
		<div class="mb-12">
			<div class="mb-5 flex items-center gap-3">
				<span class="landing-label text-primary">05</span>
				<span class="landing-label text-muted-foreground">{t('landing.rules.eyebrow')}</span>
			</div>
			<div data-rules-hairline class="mb-8 h-px w-full bg-border/60"></div>

			<div class="grid gap-6 lg:grid-cols-12 lg:items-end">
				<h2
					class="min-w-0 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl lg:col-span-7"
				>
					{t('landing.rules.title')}
				</h2>
				<p class="min-w-0 text-sm leading-relaxed text-muted-foreground sm:text-base lg:col-span-5">
					{t('landing.rules.lead')}
				</p>
			</div>
		</div>

		<ol data-rules-list class="divide-y divide-border/60 border-y border-border/60">
			{#each RULE_ITEMS as item, index (item)}
				<li
					data-rule-row
					class="rule-row grid gap-3 py-6 sm:grid-cols-12 sm:items-baseline sm:gap-6"
				>
					<span class="landing-mono text-xs text-primary sm:col-span-1">
						{String(index + 1).padStart(2, '0')}
					</span>
					<h3 class="min-w-0 text-base font-semibold text-foreground sm:col-span-5">
						{t(`landing.rules.${item}_title`)}
					</h3>
					<p class="min-w-0 text-sm leading-relaxed text-muted-foreground sm:col-span-6">
						{t(`landing.rules.${item}_body`)}
					</p>
				</li>
			{/each}
		</ol>
	</section>

	<!-- Section 06: Closing & Install -->
	<section
		id="closing"
		data-landing-section
		class="relative mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28"
	>
		<div class="mb-12">
			<div class="mb-5 flex items-center gap-3">
				<span class="landing-label text-primary">06</span>
				<span class="landing-label text-muted-foreground">{t('landing.closing.eyebrow')}</span>
			</div>
			<div data-closing-hairline class="mb-10 h-px w-full bg-border/60"></div>

			<div class="grid gap-8 lg:grid-cols-12 lg:items-end">
				<div class="min-w-0 lg:col-span-7">
					<button
						type="button"
						bind:this={bloubWrapEl}
						onclick={pulseBloub}
						onmouseenter={pulseBloub}
						aria-label="Myaaw"
						class="mb-4 inline-flex cursor-pointer items-center justify-center rounded-2xl border border-border/60 bg-muted/40 p-2 outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
					>
						<Bloub cycle="wink" color={dark ? 'orange' : 'encre'} size={68} />
					</button>

					<h2
						data-closing-intro
						class="text-3xl font-semibold tracking-tight text-foreground sm:text-5xl"
					>
						<span class="block">{t('landing.closing.title_1')}</span>
						<span class="block text-muted-foreground">{t('landing.closing.title_2')}</span>
					</h2>
					<p
						data-closing-intro
						class="mt-4 max-w-2xl text-sm leading-relaxed text-muted-foreground sm:text-base"
					>
						{t('landing.closing.lead')}
					</p>
				</div>

				<div
					data-closing-intro
					class="flex flex-wrap items-center gap-3 lg:col-span-5 lg:justify-end"
				>
					<a
						href={signInHref}
						use:motionButton
						class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-95"
					>
						<span>{t('landing.closing.primary')}</span>
						<ArrowRight class="size-4" />
					</a>
					<a
						href={REPO_URL}
						target="_blank"
						rel="noreferrer"
						use:motionButton
						class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 bg-card px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-muted/40"
					>
						<span>{t('landing.closing.secondary')}</span>
						<ArrowUpRight class="size-4" />
					</a>
				</div>
			</div>
		</div>

		<!-- Interactive 3-Tab Modern Install Bar -->
		<div
			data-slot="card"
			class="frost-card rounded-xl bg-card text-card-foreground ring-1 ring-(--surface-ring)"
		>
			<div
				class="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-3"
			>
				<div
					class="flex flex-wrap items-center gap-1.5"
					role="tablist"
					aria-label={t('landing.closing.eyebrow')}
				>
					{#each INSTALL_METHODS as method (method.id)}
						{@const active = activeMethod === method.id}
						<button
							type="button"
							role="tab"
							aria-selected={active}
							use:motionButton
							onclick={() => (activeMethod = method.id)}
							class="rounded-md border px-2.5 py-1 text-xs font-medium transition-colors {active
								? 'border-primary bg-primary text-primary-foreground'
								: 'border-border/60 bg-muted/40 text-muted-foreground hover:text-foreground'}"
						>
							{t(method.labelKey)}
						</button>
					{/each}
				</div>

				<span
					class="landing-mono inline-flex items-center gap-1.5 text-[11px] text-muted-foreground"
				>
					<Terminal class="size-3.5" />
					{t('landing.closing.install_label')}
				</span>
			</div>

			{#key activeMethod}
				<div use:motionPop={{ y: 6, duration: 0.24 }}>
					<div class="flex items-center gap-3 px-4 py-4">
						<pre class="min-w-0 flex-1 overflow-x-auto"><code
								class="landing-mono text-xs text-foreground sm:text-sm"
								><span class="text-muted-foreground select-none"
									>$ </span>{currentMethod.command}</code
							></pre>
						<div
							bind:this={copyWrapEl}
							role="presentation"
							onclick={pulseCopy}
							class="shrink-0 rounded-md border border-border/60 bg-muted/40 px-2 py-1"
						>
							<CopyButton text={currentMethod.command} label />
						</div>
					</div>

					<div
						class="flex flex-col gap-2 border-t border-border/60 px-4 py-3 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
					>
						<p class="min-w-0 leading-relaxed">{t(currentMethod.descKey)}</p>
						<p class="shrink-0 leading-relaxed">
							{t('landing.closing.install_hint')}{' '}
							<a
								href={`${RELEASES_URL}/latest`}
								target="_blank"
								rel="noreferrer"
								class="font-medium text-foreground underline underline-offset-4 hover:opacity-80"
							>
								{t('landing.closing.releases')}
							</a>
						</p>
					</div>
				</div>
			{/key}
		</div>

		<!-- Updated 4 Quickstart Steps -->
		<ol data-steps-grid class="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each SETUP_STEPS as step, index (step)}
				<li
					data-step-card
					data-slot="card"
					use:motionCard
					class="frost-card min-w-0 rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
				>
					<p class="landing-label text-primary">{String(index + 1).padStart(2, '0')}</p>
					<h3 class="mt-2.5 text-sm font-semibold text-foreground">
						{t(`landing.closing.steps.${step}_title`)}
					</h3>
					<p class="mt-2 text-xs leading-relaxed text-muted-foreground">
						{t(`landing.closing.steps.${step}_body`)}
					</p>
				</li>
			{/each}
		</ol>

		<div class="mt-8 flex flex-wrap items-center justify-between gap-4">
			<a
				class="inline-flex items-center gap-1.5 text-sm font-medium text-foreground underline underline-offset-4 hover:opacity-80"
				href={`${REPO_URL}/blob/main/webui/README.md`}
				target="_blank"
				rel="noreferrer"
			>
				{t('landing.closing.guide')}
				<ArrowUpRight class="size-3.5" />
			</a>
			<p class="landing-mono text-xs text-muted-foreground">{t('landing.closing.signature')}</p>
		</div>
	</section>
</div>

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

	.rule-row {
		transition: transform 0.24s cubic-bezier(0.22, 1, 0.36, 1);
	}

	.rule-row:hover {
		transform: translateX(4px);
	}

	@media (prefers-reduced-motion: reduce) {
		.rule-row {
			transition: none;
		}
		.rule-row:hover {
			transform: none;
		}
	}

	:global(html[data-reduce-motion='true']) .rule-row {
		transition: none;
	}

	:global(html[data-reduce-motion='true']) .rule-row:hover {
		transform: none;
	}
</style>
