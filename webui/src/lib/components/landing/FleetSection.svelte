<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Archive,
		ArrowUpRight,
		Brain,
		ChevronDown,
		Clock,
		Cpu,
		Layers,
		LayoutGrid,
		MessageSquare,
		MessageSquarePlus,
		Play,
		Puzzle,
		RotateCw,
		SlidersHorizontal,
		Sparkles,
		Square,
		Users
	} from 'lucide-svelte';
	import { mode } from 'mode-watcher';
	import type { BloubColor, BloubCycle, BloubMood } from '$lib/bloub';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import ChatStep from '$lib/components/common/ChatStep.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import {
		mentionPillClass,
		splitMentionSegments,
		type MentionTarget
	} from '$lib/components/groups/mentions';
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
		type FleetAgent,
		type ShellModeId
	} from '$lib/landing/sections';

	const { t } = useTranslation();

	type GroupMode = 'autonomous' | 'all' | 'mention';
	const GROUP_MODES: readonly GroupMode[] = ['autonomous', 'all', 'mention'];

	let sectionEl: HTMLElement | null = $state(null);
	let hoveredAgent = $state('');
	let groupMode = $state<GroupMode>('autonomous');
	let activeMode = $state<ShellModeId>('bots');
	let expandedThoughts = $state<Record<string, boolean>>({});
	let expandedThoughtSteps = $state<Record<string, boolean>>({});

	const GROUP_MENTION_TARGETS: MentionTarget[] = [
		{ name: 'myaaw', color: 'encre' },
		{ name: 'myaadex', color: 'rouge' },
		{ name: 'nightshift', color: 'rose' }
	];

	interface DummyGroupMessage {
		id: string;
		sender: string;
		color: BloubColor;
		mood: BloubMood;
		textKey: string;
		timeKey: string;
		thoughtKey?: string;
	}

	const GROUP_USER_TEXT_KEY = $derived(
		groupMode === 'all'
			? 'landing.fleet.group_demo.all_user'
			: groupMode === 'mention'
				? 'landing.fleet.group_demo.mention_user'
				: 'landing.fleet.group_demo.msg_user'
	);

	const GROUP_MESSAGES = $derived.by<DummyGroupMessage[]>(() => {
		if (groupMode === 'all') {
			return [
				{
					id: 'all-myaaw',
					sender: 'myaaw',
					color: 'encre',
					mood: 'heureux',
					textKey: 'landing.fleet.group_demo.all_myaaw',
					timeKey: 'landing.fleet.group_demo.time_1'
				},
				{
					id: 'all-codex',
					sender: 'myaadex',
					color: 'rouge',
					mood: 'attentif',
					textKey: 'landing.fleet.group_demo.all_codex',
					timeKey: 'landing.fleet.group_demo.time_1',
					thoughtKey: 'landing.fleet.group_demo.all_codex_thought'
				},
				{
					id: 'all-nightshift',
					sender: 'nightshift',
					color: 'rose',
					mood: 'attentif',
					textKey: 'landing.fleet.group_demo.all_nightshift',
					timeKey: 'landing.fleet.group_demo.time_2',
					thoughtKey: 'landing.fleet.group_demo.all_nightshift_thought'
				}
			];
		}

		if (groupMode === 'mention') {
			return [
				{
					id: 'mention-codex',
					sender: 'myaadex',
					color: 'rouge',
					mood: 'heureux',
					textKey: 'landing.fleet.group_demo.mention_codex',
					timeKey: 'landing.fleet.group_demo.time_1',
					thoughtKey: 'landing.fleet.group_demo.mention_codex_thought'
				}
			];
		}

		return [
			{
				id: 'auto-codex',
				sender: 'myaadex',
				color: 'rouge',
				mood: 'curieux',
				textKey: 'landing.fleet.group_demo.msg_codex',
				timeKey: 'landing.fleet.group_demo.time_1',
				thoughtKey: 'landing.fleet.group_demo.thought_codex'
			},
			{
				id: 'auto-nightshift',
				sender: 'nightshift',
				color: 'rose',
				mood: 'attentif',
				textKey: 'landing.fleet.group_demo.msg_nightshift',
				timeKey: 'landing.fleet.group_demo.time_2',
				thoughtKey: 'landing.fleet.group_demo.thought_nightshift'
			},
			{
				id: 'auto-myaaw',
				sender: 'myaaw',
				color: 'encre',
				mood: 'heureux',
				textKey: 'landing.fleet.group_demo.msg_myaaw',
				timeKey: 'landing.fleet.group_demo.time_2'
			}
		];
	});

	function toggleThought(id: string) {
		expandedThoughts = { ...expandedThoughts, [id]: !expandedThoughts[id] };
	}

	function toggleThoughtStep(id: string) {
		expandedThoughtSteps = { ...expandedThoughtSteps, [id]: !expandedThoughtSteps[id] };
	}

	// Interactive local dummy state for Start / Stop / Restart demo toggles (no API calls).
	let runningState = $state<Record<string, boolean>>(
		Object.fromEntries(FLEET_AGENTS.map((agent) => [agent.name, agent.initialRunning]))
	);
	let unreadState = $state<Record<string, number>>(
		Object.fromEntries(FLEET_AGENTS.map((agent) => [agent.name, agent.unreads ?? 0]))
	);

	function isAgentRunning(agent: FleetAgent): boolean {
		return runningState[agent.name] ?? agent.initialRunning;
	}

	function isAgentStreaming(agent: FleetAgent): boolean {
		return isAgentRunning(agent) && Boolean(agent.initialStreaming);
	}

	function toggleDummyAgent(name: string, nextRunning: boolean) {
		runningState[name] = nextRunning;
	}

	function clearDummyUnread(name: string) {
		if ((unreadState[name] ?? 0) > 0) unreadState[name] = 0;
	}

	function dummyBloubState(
		agent: FleetAgent,
		hovered: boolean
	): { mood: BloubMood; cycle?: BloubCycle } {
		if (isAgentStreaming(agent)) return { mood: 'attentif', cycle: 'loading' };
		if ((unreadState[agent.name] ?? 0) > 0) return { mood: 'excite' };
		if (!isAgentRunning(agent)) return { mood: 'somnolent' };
		return { mood: hovered ? 'heureux' : 'attentif' };
	}

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

	<!-- Part A: Bots Lounge Fleet Cards (matches app Bots mode card layout with dummy data) -->
	<div class="mb-3 flex items-center justify-between gap-2">
		<span
			class="landing-mono text-[11px] font-semibold tracking-wider text-muted-foreground uppercase"
		>
			{t('landing.fleet.cards.section_heading')}
		</span>
		<span class="landing-mono text-[11px] text-muted-foreground tabular-nums">
			{FLEET_AGENTS.length}
		</span>
	</div>

	<div data-fleet-grid class="grid gap-4 md:grid-cols-3">
		{#each FLEET_AGENTS as agent (agent.name)}
			{@const isHovered = hoveredAgent === agent.name}
			{@const running = isAgentRunning(agent)}
			{@const streaming = isAgentStreaming(agent)}
			{@const unreads = unreadState[agent.name] ?? 0}
			{@const bloubColor = agent.color === 'encre' && dark ? 'orange' : agent.color}
			{@const accent = groupAccent(agent.color)}
			{@const bloub = dummyBloubState(agent, isHovered)}
			<article
				data-fleet-card
				data-slot="card"
				use:motionCard={{ lift: 3, scale: 1.006, pressable: false }}
				onmouseenter={() => (hoveredAgent = agent.name)}
				onmouseleave={() => {
					if (hoveredAgent === agent.name) hoveredAgent = '';
				}}
				onfocusin={() => (hoveredAgent = agent.name)}
				onfocusout={() => {
					if (hoveredAgent === agent.name) hoveredAgent = '';
				}}
				class={[
					'frost-card group relative flex min-w-0 flex-col justify-between overflow-hidden rounded-2xl border border-border/80 bg-card text-card-foreground transition-all duration-200 hover:border-foreground/20 hover:shadow-md',
					!running ? 'opacity-85 hover:opacity-100' : ''
				]}
			>
				<!-- Color-matched top wash (matches BotsHome grid card) -->
				<div
					aria-hidden="true"
					class={['pointer-events-none h-1 w-full opacity-75', accent.dot]}
				></div>

				<!-- Main card area -->
				<button
					type="button"
					onclick={() => clearDummyUnread(agent.name)}
					class="flex flex-1 flex-col gap-3.5 p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
				>
					<div class="flex items-start justify-between gap-3">
						<div class="flex min-w-0 items-center gap-3">
							<span
								class={[
									'relative grid h-13 w-13 shrink-0 place-items-center rounded-2xl ring-1 transition-transform duration-200 group-hover:scale-105',
									accent.softBg,
									accent.ring
								]}
							>
								<Bloub mood={bloub.mood} cycle={bloub.cycle} size={44} tight color={bloubColor} />
								<RuntimeBadge
									runtimeType={agent.runtimeType}
									runtimePreset={agent.runtimePreset}
									size="md"
								/>
								<span
									aria-hidden="true"
									class={[
										'absolute -right-0.5 -bottom-0.5 h-3 w-3 rounded-full ring-2 ring-card',
										streaming
											? 'animate-pulse bg-primary'
											: running
												? 'bg-emerald-500'
												: 'bg-muted-foreground/40'
									]}
								></span>
							</span>

							<div class="min-w-0">
								<div class="flex items-center gap-1.5">
									<h3 class="truncate text-base font-bold tracking-tight text-foreground">
										{agent.name}
									</h3>
									{#if agent.isDefault}
										<span
											class="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
										>
											{t('landing.fleet.default_badge')}
										</span>
									{/if}
								</div>
								<div class="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px]">
									{#if streaming}
										<span class="font-medium text-primary">
											{t('myaaw.bots.thinking')}
										</span>
									{:else if running}
										<span class="text-muted-foreground">
											{agent.runtimeType === 'myaaw'
												? t('myaaw.onboard.runtime_builtin')
												: agent.runtimeLabel}
										</span>
									{:else}
										<span class="text-muted-foreground">{t('myaaw.bots.stopped')}</span>
									{/if}
									<span class="text-muted-foreground/50">·</span>
									<span class="truncate text-muted-foreground">{t(agent.sectionKey)}</span>
								</div>
							</div>
						</div>

						<div class="flex shrink-0 items-center gap-1">
							{#if unreads > 0}
								<span
									class="grid h-5 min-w-5 place-items-center rounded-full bg-primary px-1.5 text-[10px] font-semibold text-primary-foreground tabular-nums"
								>
									{unreads}
								</span>
							{/if}
							<ArrowUpRight
								class="h-4 w-4 text-muted-foreground/40 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-foreground"
							/>
						</div>
					</div>

					<!-- Accent-bordered activity quote -->
					<div class={['min-h-10 border-l-2 pl-3', accent.border]}>
						<p class="line-clamp-2 text-xs leading-relaxed text-foreground/85">
							{t(agent.previewKey)}
						</p>
					</div>
				</button>

				<!-- Quiet footer toolbar (interactive dummy controls, no backend calls) -->
				<div
					class="flex items-center justify-between gap-2 border-t border-border/50 bg-muted/20 px-3.5 py-2"
				>
					<span class="truncate text-[10px] text-muted-foreground tabular-nums">
						{t(agent.timeKey)}
					</span>

					<div class="flex shrink-0 items-center gap-1">
						{#if running}
							<button
								type="button"
								onclick={() => toggleDummyAgent(agent.name, true)}
								title={t('myaaw.server.restart')}
								aria-label={t('myaaw.server.restart')}
								class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
							>
								<RotateCw class="h-3.5 w-3.5" />
							</button>
							<button
								type="button"
								onclick={() => toggleDummyAgent(agent.name, false)}
								title={t('myaaw.server.stop')}
								aria-label={t('myaaw.server.stop')}
								class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/12 hover:text-destructive"
							>
								<Square class="h-3 w-3" />
							</button>
						{:else}
							<button
								type="button"
								onclick={() => toggleDummyAgent(agent.name, true)}
								class="inline-flex h-6.5 items-center gap-1 rounded-lg bg-emerald-500/12 px-2 text-[11px] font-medium text-emerald-700 transition-colors hover:bg-emerald-500/20 dark:text-emerald-300"
							>
								<Play class="h-3 w-3" />
								{t('myaaw.server.start')}
							</button>
						{/if}
						<span aria-hidden="true" class="mx-0.5 h-3.5 w-px bg-border/70"></span>
						<button
							type="button"
							onclick={() => clearDummyUnread(agent.name)}
							title={t('myaaw.chat.new')}
							aria-label={t('myaaw.chat.new')}
							class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
						>
							<MessageSquarePlus class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							onclick={() => toggleDummyAgent(agent.name, !running)}
							title={t('myaaw.bots.reconfigure')}
							aria-label={t('myaaw.bots.reconfigure')}
							class="grid h-6.5 w-6.5 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
						>
							<SlidersHorizontal class="h-3.5 w-3.5" />
						</button>
					</div>
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
				<div
					use:motionPop={{ y: 6, duration: 0.24 }}
					class="mt-3 space-y-2 rounded-lg border border-border/60 bg-muted/40 px-3.5 py-2.5 text-xs leading-relaxed text-muted-foreground"
				>
					<p>{t(`landing.fleet.group_modes.${groupMode}_desc`)}</p>
					{#if groupMode === 'mention'}
						<p class="text-[11px] text-muted-foreground">
							{#each splitMentionSegments(t('landing.fleet.group_demo.mention_quiet_note'), GROUP_MENTION_TARGETS) as segment, idx (idx)}
								{#if segment.mention}
									<span class={mentionPillClass(segment.mention.color, segment.mention.isAll)}
										>{segment.text}</span
									>
								{:else}
									{segment.text}
								{/if}
							{/each}
						</p>
					{/if}
				</div>
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
				<div use:motionPop={{ y: 8, duration: 0.28 }} class="space-y-4 bg-background/40 p-4 sm:p-5">
					<!-- Right-aligned user bubble (matches real /app/groups/[id] chat UI) -->
					<div class="ml-auto flex max-w-[85%] flex-col items-end gap-1">
						<div
							class="w-fit max-w-full rounded-2xl rounded-tr-sm bg-primary/10 px-3.5 py-2.5 text-xs leading-relaxed text-foreground ring-1 ring-primary/15 sm:text-sm"
						>
							<p class="whitespace-pre-wrap wrap-break-word">
								{#each splitMentionSegments(t(GROUP_USER_TEXT_KEY), GROUP_MENTION_TARGETS) as segment, idx (idx)}
									{#if segment.mention}
										<span class={mentionPillClass(segment.mention.color, segment.mention.isAll)}
											>{segment.text}</span
										>
									{:else}
										{segment.text}
									{/if}
								{/each}
							</p>
						</div>
						<span class="px-1 text-[11px] text-muted-foreground tabular-nums">
							{t('landing.fleet.group_demo.time_1')}
						</span>
					</div>

					<!-- Left-aligned agent replies (matches real /app/groups/[id] chat UI) -->
					{#each GROUP_MESSAGES as msg (msg.id)}
						{@const accent = groupAccent(msg.color)}
						{@const bloubColor = msg.color === 'encre' && dark ? 'orange' : msg.color}
						{@const thoughtOpen = Boolean(expandedThoughts[msg.id])}
						<div class="flex items-start gap-2.5">
							<div
								class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-full bg-muted/70 ring-1 ring-border/50"
							>
								<Bloub mood={msg.mood} size={24} tight color={bloubColor} />
							</div>

							<div class="min-w-0 flex-1">
								<div class="mb-1 flex flex-wrap items-center gap-2">
									<span class={['text-xs font-semibold', accent.text]}>
										{msg.sender}
									</span>
									<span class="text-[11px] text-muted-foreground tabular-nums">
										{t(msg.timeKey)}
									</span>
								</div>

								{#if msg.thoughtKey}
									{@const thoughtText = t(msg.thoughtKey)}
									{@const stepExpanded = Boolean(expandedThoughtSteps[msg.id])}
									<div class="mb-1.5 space-y-0.5">
										<button
											type="button"
											onclick={() => toggleThought(msg.id)}
											aria-expanded={thoughtOpen}
											class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
										>
											<Clock class="h-3.5 w-3.5 shrink-0" />
											<span class="font-medium">{t('landing.fleet.group_demo.thinking_label')}</span
											>
											<ChevronDown
												class="h-3.5 w-3.5 shrink-0 transition-transform {thoughtOpen
													? 'rotate-180'
													: ''}"
											/>
										</button>

										{#if thoughtOpen}
											<div class="space-y-0.5 pl-2">
												<ChatStep
													title={t('myaaw.chat.thinking')}
													summary={thoughtText}
													icon={Brain}
													expanded={stepExpanded}
													ontoggle={() => toggleThoughtStep(msg.id)}
													copyText={thoughtText}
												>
													<p class="whitespace-pre-wrap text-muted-foreground">
														{thoughtText}
													</p>
												</ChatStep>
											</div>
										{/if}
									</div>
								{/if}

								<div
									class="w-fit max-w-full rounded-2xl rounded-tl-sm border border-border/70 bg-card px-3.5 py-2.5 text-xs leading-relaxed text-foreground shadow-2xs sm:text-sm"
								>
									<p class="whitespace-pre-wrap wrap-break-word">
										{#each splitMentionSegments(t(msg.textKey), GROUP_MENTION_TARGETS) as segment, idx (idx)}
											{#if segment.mention}
												<span class={mentionPillClass(segment.mention.color, segment.mention.isAll)}
													>{segment.text}</span
												>
											{:else}
												{segment.text}
											{/if}
										{/each}
									</p>
								</div>
							</div>
						</div>
					{/each}
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
