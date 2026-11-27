<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Archive,
		Clock,
		Cpu,
		Layers,
		LayoutGrid,
		MessageSquare,
		Puzzle,
		Sparkles,
		Users
	} from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import {
		isReducedMotion,
		loadGsap,
		motionButton,
		motionCard,
		motionPop
	} from '$lib/landing/motion';
	import {
		CAPABILITY_ITEMS,
		FLEET_AGENTS,
		SHELL_MODES,
		type CapabilityId,
		type ShellModeId
	} from '$lib/landing/sections';

	const { t } = useTranslation();

	type GroupMode = 'autonomous' | 'all' | 'mention';
	const GROUP_MODES: readonly GroupMode[] = ['autonomous', 'all', 'mention'];

	let sectionEl: HTMLElement | null = $state(null);
	let hoveredAgent = $state('');
	let groupMode = $state<GroupMode>('autonomous');
	let activeMode = $state<ShellModeId>('bots');

	const dark = $derived(mode.current === 'dark');
	const currentShellMode = $derived(
		SHELL_MODES.find((item) => item.id === activeMode) ?? SHELL_MODES[0]
	);

	function capIcon(id: CapabilityId) {
		switch (id) {
			case 'providers':
				return Cpu;
			case 'extra_messages':
				return Sparkles;
			case 'extensions':
				return Puzzle;
			case 'delegation':
				return Users;
			case 'schedule':
				return Clock;
			case 'backup':
				return Archive;
		}
	}

	onMount(() => {
		if (!sectionEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !sectionEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				gsap.from('[data-fleet-hairline]', {
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

				gsap.from('[data-fleet-intro]', {
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

				gsap.from('[data-fleet-card]', {
					y: 22,
					opacity: 0,
					duration: 0.6,
					stagger: 0.09,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-fleet-grid]',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-fleet-block]', {
					y: 22,
					opacity: 0,
					duration: 0.65,
					stagger: 0.12,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-fleet-block]',
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-cap-card]', {
					y: 18,
					opacity: 0,
					duration: 0.55,
					stagger: 0.06,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-cap-grid]',
						start: 'top 85%',
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
	id="fleet"
	data-landing-section
	bind:this={sectionEl}
	class="relative mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28"
>
	<!-- Section 03 editorial header -->
	<div class="mb-12">
		<div class="mb-5 flex flex-wrap items-center justify-between gap-3">
			<div class="flex items-center gap-3">
				<span class="landing-label text-primary">03</span>
				<span class="landing-label text-muted-foreground">{t('landing.fleet.eyebrow')}</span>
			</div>
			<span
				class="landing-mono inline-flex items-center gap-1.5 rounded-full border border-border/60 bg-muted/40 px-2.5 py-0.5 text-[11px] text-muted-foreground"
			>
				{t('landing.fleet.sample_badge')}
			</span>
		</div>
		<div data-fleet-hairline class="mb-8 h-px w-full bg-border/60"></div>

		<div class="grid gap-6 lg:grid-cols-12 lg:items-end">
			<h2
				data-fleet-intro
				class="min-w-0 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl lg:col-span-7"
			>
				{t('landing.fleet.title')}
			</h2>
			<p
				data-fleet-intro
				class="min-w-0 text-sm leading-relaxed text-muted-foreground sm:text-base lg:col-span-5"
			>
				{t('landing.fleet.lead')}
			</p>
		</div>
	</div>

	<!-- Part A: Bots Lounge Fleet Cards -->
	<div data-fleet-grid class="grid gap-4 md:grid-cols-3">
		{#each FLEET_AGENTS as agent (agent.name)}
			{@const isHovered = hoveredAgent === agent.name}
			{@const bloubColor = agent.color === 'encre' && dark ? 'orange' : agent.color}
			<article
				data-fleet-card
				data-slot="card"
				use:motionCard
				class="frost-card group flex min-w-0 flex-col justify-between rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
				onmouseenter={() => (hoveredAgent = agent.name)}
				onmouseleave={() => (hoveredAgent = '')}
				onfocusin={() => (hoveredAgent = agent.name)}
				onfocusout={() => (hoveredAgent = '')}
			>
				<div>
					<div class="flex items-start justify-between gap-3">
						<div class="flex min-w-0 items-center gap-3">
							<div
								class="flex size-12 shrink-0 items-center justify-center rounded-xl border border-border/60 bg-muted/40"
							>
								<Bloub
									mood={isHovered ? agent.hover : agent.mood}
									color={bloubColor}
									size={44}
									fade
								/>
							</div>
							<div class="min-w-0">
								<h3 class="truncate text-base font-semibold text-foreground">{agent.name}</h3>
								<p class="truncate text-xs text-muted-foreground">{t(agent.role)}</p>
							</div>
						</div>

						<span
							class="landing-mono inline-flex shrink-0 items-center gap-1.5 rounded-full border border-border/60 bg-muted/40 px-2 py-0.5 text-[10px] tracking-wider text-foreground uppercase"
						>
							<span
								class="size-1.5 rounded-full {agent.status === 'idle'
									? 'bg-muted-foreground/60'
									: 'bg-primary'}"
							></span>
							{t(`landing.fleet.status_${agent.status}`)}
						</span>
					</div>

					<div class="mt-4 flex flex-wrap items-center justify-between gap-2">
						<span
							class="landing-mono inline-flex items-center rounded-md bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary"
						>
							{agent.runtime}
						</span>
						<span class="landing-mono text-[11px] text-muted-foreground">
							:{agent.port} · pid {agent.pid}
						</span>
					</div>

					<dl
						class="mt-4 grid grid-cols-3 gap-2 rounded-lg border border-border/60 bg-muted/40 p-2.5"
					>
						<div class="min-w-0">
							<dt class="landing-mono truncate text-[10px] text-muted-foreground uppercase">
								{t('landing.fleet.runs')}
							</dt>
							<dd class="landing-mono mt-0.5 text-sm font-semibold text-foreground">
								{agent.runs}
							</dd>
						</div>
						<div class="min-w-0 border-l border-border/60 pl-2">
							<dt class="landing-mono truncate text-[10px] text-muted-foreground uppercase">
								{t('landing.fleet.skills')}
							</dt>
							<dd class="landing-mono mt-0.5 text-sm font-semibold text-foreground">
								{agent.skills}
							</dd>
						</div>
						<div class="min-w-0 border-l border-border/60 pl-2">
							<dt class="landing-mono truncate text-[10px] text-muted-foreground uppercase">
								{t('landing.fleet.uptime')}
							</dt>
							<dd class="landing-mono mt-0.5 truncate text-sm font-semibold text-foreground">
								{agent.uptime}
							</dd>
						</div>
					</dl>
				</div>

				<div class="mt-4 flex flex-wrap items-center gap-1.5 border-t border-border/60 pt-3">
					{#each agent.channels as channel (channel)}
						<span
							class="landing-mono rounded-md border border-border/60 bg-muted/40 px-2 py-0.5 text-[10px] text-muted-foreground"
						>
							{channel}
						</span>
					{/each}
				</div>
			</article>
		{/each}
	</div>

	<div
		class="mt-4 flex flex-col gap-1 text-xs text-muted-foreground sm:flex-row sm:justify-between"
	>
		<p class="min-w-0">{t('landing.fleet.body')}</p>
		<p class="landing-mono shrink-0 text-[11px] opacity-80">{t('landing.fleet.example_note')}</p>
	</div>

	<!-- Part B: Multi-Agent Group Chat Room Interactive Preview -->
	<div
		data-fleet-block
		class="mt-16 grid gap-8 border-t border-border/60 pt-14 lg:grid-cols-12 lg:items-start"
	>
		<div class="min-w-0 lg:col-span-5">
			<div class="flex items-center gap-2 text-primary">
				<MessageSquare class="size-4 shrink-0" />
				<span class="landing-label">{t('landing.fleet.group_eyebrow')}</span>
			</div>
			<h3 class="mt-3 text-2xl font-semibold tracking-tight text-foreground">
				{t('landing.fleet.group_title')}
			</h3>
			<p class="mt-3 text-sm leading-relaxed text-muted-foreground">
				{t('landing.fleet.group_body')}
			</p>

			<div
				class="mt-6 flex flex-wrap gap-2"
				role="tablist"
				aria-label={t('landing.fleet.group_eyebrow')}
			>
				{#each GROUP_MODES as modeOption (modeOption)}
					{@const active = groupMode === modeOption}
					<button
						type="button"
						role="tab"
						aria-selected={active}
						use:motionButton
						onclick={() => (groupMode = modeOption)}
						class="rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors {active
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border/60 bg-card text-muted-foreground hover:text-foreground'}"
					>
						{t(`landing.fleet.group_modes.${modeOption}`)}
					</button>
				{/each}
			</div>

			{#key groupMode}
				<p
					use:motionPop={{ y: 6, duration: 0.24 }}
					class="mt-3 rounded-lg border border-border/60 bg-muted/40 px-3.5 py-2.5 text-xs leading-relaxed text-muted-foreground"
				>
					{t(`landing.fleet.group_modes.${groupMode}_desc`)}
				</p>
			{/key}
		</div>

		<div
			data-slot="card"
			class="frost-card min-w-0 rounded-xl bg-card text-card-foreground ring-1 ring-(--surface-ring) lg:col-span-7"
		>
			<div
				class="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 px-4 py-3"
			>
				<div class="min-w-0">
					<p class="landing-mono truncate text-xs font-semibold text-foreground">
						{t('landing.fleet.group_demo.room_name')}
					</p>
					<p class="landing-mono truncate text-[11px] text-muted-foreground">
						{t('landing.fleet.group_demo.participants')}
					</p>
				</div>
				<span
					class="landing-mono inline-flex items-center gap-1.5 rounded-full border border-border/60 bg-muted/40 px-2.5 py-0.5 text-[10px] text-foreground"
				>
					<span class="size-1.5 rounded-full bg-primary"></span>
					{t('landing.fleet.group_demo.status_live')}
				</span>
			</div>

			{#key groupMode}
				<div use:motionPop={{ y: 8, duration: 0.28 }} class="space-y-3 p-4">
					<!-- User prompt -->
					<div class="rounded-lg border border-border/60 bg-muted/40 p-3">
						<div class="flex items-center justify-between gap-2">
							<span class="landing-mono text-[11px] font-semibold text-foreground">You</span>
							<span class="landing-mono text-[10px] text-muted-foreground">
								{t(`landing.fleet.group_modes.${groupMode}`)}
							</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-foreground">
							{t('landing.fleet.group_demo.msg_user')}
						</p>
					</div>

					<!-- Codex Dev message -->
					<div class="flex items-start gap-2.5">
						<div
							class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-muted/40"
						>
							<Bloub mood="curieux" color="bleu" size={28} />
						</div>
						<div class="min-w-0 flex-1 rounded-lg border border-border/60 bg-card/60 p-3">
							<div class="flex flex-wrap items-center justify-between gap-2">
								<span class="text-xs font-semibold text-foreground">Codex Dev</span>
								<span
									class="landing-mono rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary"
								>
									OpenAI Codex
								</span>
							</div>
							<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
								{t('landing.fleet.group_demo.msg_codex')}
							</p>
						</div>
					</div>

					<!-- Nightshift message (visible in autonomous & all, or shown as @mentioned in mention mode) -->
					<div class="flex items-start gap-2.5">
						<div
							class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-muted/40"
						>
							<Bloub mood="attentif" color="violet" size={28} />
						</div>
						<div class="min-w-0 flex-1 rounded-lg border border-border/60 bg-card/60 p-3">
							<div class="flex flex-wrap items-center justify-between gap-2">
								<span class="text-xs font-semibold text-foreground">Nightshift</span>
								<span
									class="landing-mono rounded bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground"
								>
									Claude Code
								</span>
							</div>
							<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
								{t('landing.fleet.group_demo.msg_nightshift')}
							</p>
						</div>
					</div>

					<!-- Myaaw summary message -->
					{#if groupMode !== 'mention'}
						<div class="flex items-start gap-2.5">
							<div
								class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-muted/40"
							>
								<Bloub mood="heureux" color={dark ? 'orange' : 'encre'} size={28} />
							</div>
							<div class="min-w-0 flex-1 rounded-lg border border-primary/30 bg-primary/5 p-3">
								<div class="flex flex-wrap items-center justify-between gap-2">
									<span class="text-xs font-semibold text-foreground">Myaaw</span>
									<span
										class="landing-mono rounded bg-primary/15 px-1.5 py-0.5 text-[10px] text-primary"
									>
										Myaaw Native
									</span>
								</div>
								<p class="mt-1 text-xs leading-relaxed text-foreground">
									{t('landing.fleet.group_demo.msg_myaaw')}
								</p>
							</div>
						</div>
					{/if}
				</div>
			{/key}

			<div
				class="flex flex-wrap items-center justify-between gap-2 border-t border-border/60 px-4 py-2.5 text-[11px] text-muted-foreground"
			>
				<span class="landing-mono">{t('landing.fleet.group_demo.pass_signal')}</span>
				<span class="landing-mono text-primary">
					{t(`landing.fleet.group_modes.${groupMode}`)}
				</span>
			</div>
		</div>
	</div>

	<!-- Part C: 4 WebUI Shell Modes Switcher -->
	<div data-fleet-block class="mt-16 border-t border-border/60 pt-14">
		<div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
			<div class="min-w-0">
				<div class="flex items-center gap-2 text-primary">
					<LayoutGrid class="size-4 shrink-0" />
					<span class="landing-label">{t('landing.fleet.modes_eyebrow')}</span>
				</div>
				<h3 class="mt-3 text-2xl font-semibold tracking-tight text-foreground">
					{t('landing.fleet.modes_title')}
				</h3>
				<p class="mt-2 max-w-2xl text-sm text-muted-foreground">
					{t('landing.fleet.modes_lead')}
				</p>
			</div>

			<div
				class="flex flex-wrap gap-2"
				role="tablist"
				aria-label={t('landing.fleet.modes_eyebrow')}
			>
				{#each SHELL_MODES as shellMode (shellMode.id)}
					{@const active = activeMode === shellMode.id}
					<button
						type="button"
						role="tab"
						aria-selected={active}
						use:motionButton
						onclick={() => (activeMode = shellMode.id)}
						class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors {active
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border/60 bg-card text-muted-foreground hover:text-foreground'}"
					>
						<span>{t(shellMode.titleKey)}</span>
						{#if shellMode.isDefault}
							<span
								class="landing-mono rounded px-1 py-0.2 text-[9px] uppercase {active
									? 'bg-primary-foreground/20 text-primary-foreground'
									: 'bg-muted/40 text-muted-foreground'}"
							>
								{t('landing.fleet.default_badge')}
							</span>
						{/if}
					</button>
				{/each}
			</div>
		</div>

		{#key activeMode}
			<div
				data-slot="card"
				use:motionPop={{ y: 10, duration: 0.3 }}
				class="frost-card mt-6 grid gap-6 rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring) lg:grid-cols-12 lg:items-center lg:p-6"
			>
				<div class="min-w-0 lg:col-span-5">
					<div class="flex items-center gap-2">
						<Layers class="size-4 text-primary" />
						<h4 class="text-lg font-semibold text-foreground">{t(currentShellMode.titleKey)}</h4>
						{#if currentShellMode.isDefault}
							<span
								class="landing-mono rounded-full bg-primary/10 px-2 py-0.5 text-[10px] text-primary"
							>
								{t('landing.fleet.default_badge')}
							</span>
						{/if}
					</div>
					<p class="mt-1.5 text-sm font-medium text-foreground">
						{t(currentShellMode.taglineKey)}
					</p>
					<p class="mt-2.5 text-xs leading-relaxed text-muted-foreground sm:text-sm">
						{t(currentShellMode.descKey)}
					</p>
				</div>

				<!-- Abstract semantic CSS schematic for each shell mode -->
				<div
					class="min-w-0 rounded-xl border border-border/60 bg-muted/40 p-3 lg:col-span-7"
					aria-hidden="true"
				>
					<div class="mb-2.5 flex items-center justify-between border-b border-border/60 pb-2">
						<div class="flex items-center gap-1.5">
							<span class="size-2 rounded-full bg-primary/60"></span>
							<span class="size-2 rounded-full bg-muted-foreground/30"></span>
							<span class="size-2 rounded-full bg-muted-foreground/30"></span>
						</div>
						<span class="landing-mono text-[10px] text-muted-foreground uppercase">
							{t(currentShellMode.titleKey)}
						</span>
					</div>

					{#if activeMode === 'bots'}
						<div class="grid h-40 grid-cols-12 gap-2">
							<div
								class="col-span-3 flex flex-col gap-1.5 rounded-lg border border-border/60 bg-card/70 p-2"
							>
								<div class="h-2.5 w-3/4 rounded bg-primary/30"></div>
								<div class="h-2 w-full rounded bg-muted/60"></div>
								<div class="h-2 w-4/5 rounded bg-muted/60"></div>
								<div class="mt-auto h-2 w-2/3 rounded bg-muted/60"></div>
							</div>
							<div class="col-span-9 flex flex-col gap-2">
								<div
									class="flex items-center justify-between rounded-lg border border-primary/30 bg-primary/10 px-2.5 py-2"
								>
									<div class="h-2 w-1/3 rounded bg-primary/50"></div>
									<div class="h-3.5 w-12 rounded bg-primary/30"></div>
								</div>
								<div class="grid flex-1 grid-cols-3 gap-2">
									{#each [0, 1, 2] as cardIdx (cardIdx)}
										<div
											class="flex flex-col justify-between rounded-lg border border-border/60 bg-card/80 p-2"
										>
											<div class="flex items-center gap-1.5">
												<span class="size-4 rounded-md bg-primary/20"></span>
												<span class="h-2 w-10 rounded bg-foreground/20"></span>
											</div>
											<div class="space-y-1">
												<div class="h-1.5 w-full rounded bg-muted/70"></div>
												<div class="h-1.5 w-2/3 rounded bg-muted/70"></div>
											</div>
										</div>
									{/each}
								</div>
							</div>
						</div>
					{:else if activeMode === 'studio'}
						<div class="grid h-40 grid-cols-12 gap-2">
							<div
								class="col-span-2 flex flex-col gap-1.5 rounded-lg border border-border/60 bg-card/70 p-2"
							>
								<div class="h-2.5 w-full rounded bg-primary/30"></div>
								<div class="h-2 w-full rounded bg-muted/60"></div>
								<div class="h-2 w-full rounded bg-muted/60"></div>
								<div class="h-2 w-full rounded bg-muted/60"></div>
							</div>
							<div class="col-span-7 flex flex-col gap-2">
								<div class="flex-1 rounded-lg border border-border/60 bg-card/80 p-2">
									<div class="h-2 w-1/3 rounded bg-foreground/20"></div>
									<div class="mt-2 h-2 w-5/6 rounded bg-muted/70"></div>
									<div class="mt-1.5 h-2 w-2/3 rounded bg-muted/70"></div>
								</div>
								<div class="grid h-14 grid-cols-2 gap-2">
									<div class="rounded-lg border border-border/60 bg-card/70 p-2">
										<div class="h-1.5 w-1/2 rounded bg-primary/40"></div>
									</div>
									<div class="rounded-lg border border-border/60 bg-card/70 p-2">
										<div class="h-1.5 w-1/2 rounded bg-muted-foreground/30"></div>
									</div>
								</div>
							</div>
							<div
								class="col-span-3 flex flex-col gap-1.5 rounded-lg border border-border/60 bg-card/70 p-2"
							>
								<div class="h-2 w-2/3 rounded bg-primary/30"></div>
								<div class="h-10 w-full rounded bg-muted/60"></div>
								<div class="mt-auto h-10 w-full rounded bg-muted/60"></div>
							</div>
						</div>
					{:else if activeMode === 'focus'}
						<div class="grid h-40 grid-cols-12 gap-2">
							<div
								class="col-span-1 flex flex-col items-center gap-2 rounded-lg border border-border/60 bg-card/70 py-2"
							>
								<span class="size-3 rounded bg-primary/40"></span>
								<span class="size-3 rounded bg-muted/70"></span>
								<span class="size-3 rounded bg-muted/70"></span>
							</div>
							<div
								class="col-span-11 flex flex-col justify-between rounded-lg border border-border/60 bg-card/80 p-3"
							>
								<div class="mx-auto w-full max-w-sm space-y-2">
									<div class="h-2.5 w-1/3 rounded bg-primary/30"></div>
									<div class="h-2 w-full rounded bg-muted/70"></div>
									<div class="h-2 w-5/6 rounded bg-muted/70"></div>
								</div>
								<div
									class="mx-auto h-7 w-full max-w-sm rounded-lg border border-border/60 bg-muted/50"
								></div>
							</div>
						</div>
					{:else}
						<div class="grid h-40 grid-cols-12 gap-2">
							<div
								class="col-span-4 flex flex-col gap-1.5 rounded-lg border border-border/60 bg-card/70 p-2"
							>
								<div class="h-5 w-full rounded bg-primary/15"></div>
								<div class="h-4 w-full rounded bg-muted/60"></div>
								<div class="h-4 w-full rounded bg-muted/60"></div>
								<div class="h-4 w-full rounded bg-muted/60"></div>
							</div>
							<div
								class="col-span-8 flex flex-col justify-between rounded-lg border border-border/60 bg-card/80 p-3"
							>
								<div class="space-y-2">
									<div class="ml-auto h-5 w-2/3 rounded-lg bg-primary/15"></div>
									<div class="h-8 w-4/5 rounded-lg bg-muted/60"></div>
								</div>
								<div class="h-6 w-full rounded-lg border border-border/60 bg-muted/50"></div>
							</div>
						</div>
					{/if}
				</div>
			</div>
		{/key}
	</div>

	<!-- Part D: Compact Capabilities Grid -->
	<div class="mt-16 border-t border-border/60 pt-14">
		<div class="mb-8 flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
			<div class="min-w-0">
				<h3 class="text-2xl font-semibold tracking-tight text-foreground">
					{t('landing.capabilities.title')}
				</h3>
				<p class="mt-2 max-w-2xl text-sm text-muted-foreground">
					{t('landing.capabilities.body')}
				</p>
			</div>
		</div>

		<div data-cap-grid class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each CAPABILITY_ITEMS as item (item.id)}
				{@const Icon = capIcon(item.id)}
				<article
					data-cap-card
					data-slot="card"
					use:motionCard
					class="frost-card flex min-w-0 flex-col justify-between rounded-xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring)"
				>
					<div>
						<div class="flex items-center justify-between gap-3">
							<span
								class="grid size-9 shrink-0 place-items-center rounded-lg border border-border/60 bg-muted/40 text-foreground"
							>
								<Icon class="size-4" />
							</span>
							<span
								class="landing-mono rounded-full border border-border/60 bg-muted/40 px-2 py-0.5 text-[10px] tracking-wider uppercase {item.badge ===
								'available'
									? 'text-primary'
									: 'text-muted-foreground'}"
							>
								{t(`landing.capabilities.${item.badge}`)}
							</span>
						</div>
						<h4 class="mt-4 text-sm font-semibold text-foreground">{t(item.titleKey)}</h4>
						<p class="mt-2 text-xs leading-relaxed text-muted-foreground">
							{t(item.bodyKey)}
						</p>
					</div>
				</article>
			{/each}
		</div>

		<p class="mt-5 text-xs leading-relaxed text-muted-foreground">
			{t('landing.capabilities.computer_note')}
		</p>
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
</style>
