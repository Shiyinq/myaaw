<script lang="ts">
	import { onMount } from 'svelte';
	import { mode } from 'mode-watcher';
	import { ArrowUpRight, Cpu, Plug, Radio, Sparkles, Terminal, Users } from 'lucide-svelte';
	import type { BloubMood } from '$lib/bloub';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { locale } from '$lib/i18n';
	import { createReducedMotionState, loadGsap, motionButton, motionPop } from '$lib/landing/motion';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';
	import type { LucideIcon } from '$lib/types/icons';
	import { cn } from '$lib/utils';

	let { class: className = '' }: { class?: string } = $props();

	const uid = $props.id();
	const dark = $derived(mode.current === 'dark');
	const isId = $derived(locale.value === 'id');
	const reducedMotion = createReducedMotionState();
	const reduced = $derived(reducedMotion.current);

	interface OrbNode {
		id: string;
		label: string;
		labelId: string;
		tag: string;
		tagId: string;
		summary: string;
		summaryId: string;
		angleDeg: number;
		mood: BloubMood;
		icon: LucideIcon;
	}

	const CX = 220;
	const CY = 220;
	const ORBIT_R = 152;

	const NODES: OrbNode[] = [
		{
			id: 'runtimes',
			label: 'AI Engines',
			labelId: 'Mesin AI',
			tag: '9 Engines',
			tagId: '9 Mesin',
			summary:
				"Use Myaaw's built-in engine or plug in Claude Code, Codex, Antigravity, OpenCode, and more.",
			summaryId:
				'Pakai mesin bawaan Myaaw atau hubungkan Claude Code, Codex, Antigravity, OpenCode, dan lainnya.',
			angleDeg: -90,
			mood: 'excite',
			icon: Cpu
		},
		{
			id: 'mcp',
			label: 'Shared Tools',
			labelId: 'Alat Bersama',
			tag: 'Plug & play',
			tagId: 'Siap sambung',
			summary:
				'Share schedules, interactive chat cards, and your custom tools across every connected assistant.',
			summaryId:
				'Bagikan jadwal, kartu interaktif di chat, dan alat kustom ke semua asisten yang terhubung.',
			angleDeg: -30,
			mood: 'curieux',
			icon: Plug
		},
		{
			id: 'workspace',
			label: 'Workspace',
			labelId: 'Ruang Kerja',
			tag: 'Files & Git',
			tagId: 'File & Git',
			summary:
				'Browse project files, review Git changes, and open split terminals right next to your chat.',
			summaryId:
				'Buka file proyek, cek perubahan Git, dan jalankan terminal terpisah tepat di sebelah obrolan.',
			angleDeg: 30,
			mood: 'attentif',
			icon: Terminal
		},
		{
			id: 'groups',
			label: 'Group Rooms',
			labelId: 'Ruang Grup',
			tag: 'Multi-agent',
			tagId: 'Multi-agen',
			summary:
				'Bring several assistants together in one room with a shared project folder and tidy handoffs.',
			summaryId:
				'Kumpulkan beberapa asisten di satu ruangan dengan folder proyek bersama yang saling melengkapi.',
			angleDeg: 90,
			mood: 'heureux',
			icon: Users
		},
		{
			id: 'artifacts',
			label: 'Cards & Files',
			labelId: 'Kartu & File',
			tag: 'Gallery',
			tagId: 'Galeri',
			summary:
				'Clickable forms, checklists, diagrams, math, and built-in previews for images, videos, and PDFs.',
			summaryId:
				'Formulir klik, checklist, diagram, rumus, serta pratinjau langsung untuk gambar, video, dan PDF.',
			angleDeg: 150,
			mood: 'fier',
			icon: Sparkles
		},
		{
			id: 'doors',
			label: '6 Channels',
			labelId: '6 Akses',
			tag: 'Anywhere',
			tagId: 'Dari mana saja',
			summary:
				'Reach your assistants from the web dashboard, Telegram, Discord, terminal, local API, or schedules.',
			summaryId:
				'Hubungi asistenmu lewat dashboard web, Telegram, Discord, terminal, API lokal, atau jadwal rutin.',
			angleDeg: 210,
			mood: 'heureux',
			icon: Radio
		}
	];

	interface ComputedNode extends OrbNode {
		x: number;
		y: number;
		ctrlX: number;
		ctrlY: number;
		pathD: string;
		leftPct: string;
		topPct: string;
	}

	function polar(angleDeg: number, radius: number) {
		const rad = (angleDeg * Math.PI) / 180;
		return {
			x: CX + Math.cos(rad) * radius,
			y: CY + Math.sin(rad) * radius
		};
	}

	const COMPUTED_NODES: ComputedNode[] = NODES.map((node, index) => {
		const end = polar(node.angleDeg, ORBIT_R);
		// Slight tangential curvature so every conduit arcs gracefully out of the core.
		const bend = index % 2 === 0 ? 18 : -18;
		const ctrl = polar(node.angleDeg + bend, ORBIT_R * 0.56);
		return {
			...node,
			x: Number(end.x.toFixed(1)),
			y: Number(end.y.toFixed(1)),
			ctrlX: Number(ctrl.x.toFixed(1)),
			ctrlY: Number(ctrl.y.toFixed(1)),
			pathD: `M ${CX} ${CY} Q ${ctrl.x.toFixed(1)} ${ctrl.y.toFixed(1)} ${end.x.toFixed(1)} ${end.y.toFixed(1)}`,
			leftPct: `${((end.x / 440) * 100).toFixed(2)}%`,
			topPct: `${((end.y / 440) * 100).toFixed(2)}%`
		};
	});

	// 36 precision compass ticks around the outer perimeter ring.
	const COMPASS_TICKS = Array.from({ length: 36 }, (_, index) => {
		const angle = index * 10;
		const major = index % 3 === 0;
		const inner = polar(angle, major ? 176 : 181);
		const outer = polar(angle, 189);
		return {
			index,
			major,
			x1: inner.x.toFixed(1),
			y1: inner.y.toFixed(1),
			x2: outer.x.toFixed(1),
			y2: outer.y.toFixed(1)
		};
	});

	let rootEl: HTMLDivElement | null = $state(null);
	let stageEl: HTMLDivElement | null = $state(null);
	let shockwaveEl: SVGCircleElement | null = $state(null);
	let corePodEl: HTMLButtonElement | null = $state(null);

	let selectedId = $state<string>('core');
	let coreMood = $state<BloubMood>('attentif');
	let pulseCount = $state(0);

	// GSAP handles for imperative interactive feedback & pointer parallax.
	let gsapRef: typeof import('gsap').gsap | null = null;
	let quickLayers: {
		outerX: (v: number) => void;
		outerY: (v: number) => void;
		midX: (v: number) => void;
		midY: (v: number) => void;
		innerX: (v: number) => void;
		innerY: (v: number) => void;
	} | null = null;

	const activeNode = $derived(COMPUTED_NODES.find((node) => node.id === selectedId) ?? null);

	const readoutTitle = $derived(
		activeNode
			? isId
				? activeNode.labelId
				: activeNode.label
			: isId
				? 'Inti Lokal Myaaw'
				: 'Myaaw Local Hub'
	);

	const readoutTag = $derived(
		activeNode
			? isId
				? activeNode.tagId
				: activeNode.tag
			: isId
				? 'Berjalan lokal'
				: 'Runs locally'
	);

	const readoutSummary = $derived(
		activeNode
			? isId
				? activeNode.summaryId
				: activeNode.summary
			: isId
				? 'Satu aplikasi lokal yang menyatukan semua mesin AI, alat bersama, ruang kerja proyek, dan 6 jalur akses.'
				: 'One lightweight local app connecting your AI engines, shared tools, project workspaces, and 6 ways to chat.'
	);

	const activeSectionTarget = $derived.by(() => {
		switch (selectedId) {
			case 'runtimes':
			case 'mcp':
				return '#runtimes';
			case 'workspace':
			case 'artifacts':
				return '#workspace';
			case 'groups':
				return '#fleet';
			case 'doors':
				return '#doors';
			default:
				return '';
		}
	});

	function jumpToActiveSection() {
		if (!activeSectionTarget) return;
		landingScroll.scrollTo(activeSectionTarget, { offset: -64 });
	}

	function fireShockwave(targetBtn?: HTMLElement | null) {
		pulseCount += 1;
		if (reduced || !gsapRef) return;

		if (shockwaveEl) {
			gsapRef.killTweensOf(shockwaveEl);
			gsapRef.fromTo(
				shockwaveEl,
				{
					scale: 0.48,
					opacity: 0.9,
					svgOrigin: `${CX} ${CY}`
				},
				{
					scale: 2.35,
					opacity: 0,
					duration: 0.78,
					ease: 'power3.out',
					svgOrigin: `${CX} ${CY}`
				}
			);
		}

		if (targetBtn) {
			gsapRef.fromTo(
				targetBtn,
				{ scale: 0.92 },
				{ scale: 1, duration: 0.42, ease: 'back.out(2.2)', overwrite: 'auto' }
			);
		}
	}

	function activateNode(node: ComputedNode, triggerEl?: HTMLElement | null) {
		selectedId = node.id;
		coreMood = node.mood;
		fireShockwave(triggerEl);
	}

	function activateCore() {
		selectedId = 'core';
		coreMood = coreMood === 'excite' ? 'heureux' : 'excite';
		fireShockwave(corePodEl);
	}

	function handlePointerMove(event: PointerEvent) {
		if (reduced || !stageEl || !quickLayers || event.pointerType !== 'mouse') return;
		const rect = stageEl.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return;
		const nx = ((event.clientX - rect.left) / rect.width - 0.5) * 2;
		const ny = ((event.clientY - rect.top) / rect.height - 0.5) * 2;
		quickLayers.outerX(nx * -7);
		quickLayers.outerY(ny * -7);
		quickLayers.midX(nx * 5);
		quickLayers.midY(ny * 5);
		quickLayers.innerX(nx * 10);
		quickLayers.innerY(ny * 10);
	}

	function resetPointerParallax() {
		if (!quickLayers) return;
		quickLayers.outerX(0);
		quickLayers.outerY(0);
		quickLayers.midX(0);
		quickLayers.midY(0);
		quickLayers.innerX(0);
		quickLayers.innerY(0);
	}

	let gsapReady = $state(false);
	let syncMotionRunner: ((_isReduced: boolean) => void) | null = null;

	onMount(() => {
		let cancelled = false;
		let ctx: { revert: () => void } | null = null;
		let activeGsap: typeof import('gsap').gsap | null = null;

		function teardownGsap() {
			ctx?.revert();
			ctx = null;
			quickLayers = null;
		}

		function setupAnimations(gsap: typeof import('gsap').gsap) {
			const container = rootEl;
			if (cancelled || !container || reducedMotion.current) return;
			teardownGsap();

			ctx = gsap.context(() => {
				// 1. Continuous counter-rotating orbital precision rings.
				gsap.to('.orb-ring-outer', {
					rotation: 360,
					duration: 68,
					repeat: -1,
					ease: 'none',
					svgOrigin: `${CX} ${CY}`
				});
				gsap.to('.orb-ring-mid', {
					rotation: -360,
					duration: 42,
					repeat: -1,
					ease: 'none',
					svgOrigin: `${CX} ${CY}`
				});
				gsap.to('.orb-ring-inner', {
					rotation: 360,
					duration: 28,
					repeat: -1,
					ease: 'none',
					svgOrigin: `${CX} ${CY}`
				});

				// 2. Traveling stroke-dashoffset conduit beams.
				const beams = gsap.utils.toArray<SVGPathElement>('.orb-beam');
				beams.forEach((beam, index) => {
					gsap.fromTo(
						beam,
						{ strokeDashoffset: 180 },
						{
							strokeDashoffset: 0,
							duration: 2.3 + (index % 3) * 0.45,
							delay: index * 0.22,
							repeat: -1,
							ease: 'none'
						}
					);
				});

				// 3. Continuous glowing data packets along each quadratic Bezier conduit.
				const packets = gsap.utils.toArray<SVGCircleElement>('.orb-packet');
				packets.forEach((packet, index) => {
					const node = COMPUTED_NODES[index];
					if (!node) return;
					const outbound = index % 2 === 0;
					const state = { t: outbound ? 0 : 1 };
					gsap.to(state, {
						t: outbound ? 1 : 0,
						duration: 2.4 + (index % 3) * 0.4,
						delay: index * 0.35,
						repeat: -1,
						ease: 'power1.inOut',
						onUpdate: () => {
							const t = state.t;
							const inv = 1 - t;
							const px = inv * inv * CX + 2 * inv * t * node.ctrlX + t * t * node.x;
							const py = inv * inv * CY + 2 * inv * t * node.ctrlY + t * t * node.y;
							packet.setAttribute('cx', px.toFixed(1));
							packet.setAttribute('cy', py.toFixed(1));
							const edgeFade = Math.sin(t * Math.PI);
							packet.setAttribute('opacity', (0.2 + edgeFade * 0.8).toFixed(2));
						}
					});
				});

				// 4. Staggered sonar ripple rings radiating from the central core.
				const sonars = gsap.utils.toArray<SVGCircleElement>('.orb-sonar');
				sonars.forEach((ring, index) => {
					gsap.fromTo(
						ring,
						{ scale: 0.72, opacity: 0.42, svgOrigin: `${CX} ${CY}` },
						{
							scale: 1.95,
							opacity: 0,
							duration: 3.2,
							delay: index * 1.05,
							repeat: -1,
							ease: 'power2.out',
							svgOrigin: `${CX} ${CY}`
						}
					);
				});

				// 5. Breathing & floating central Bloub pod + subtle satellite float.
				gsap.to('.orb-core-float', {
					y: -6,
					duration: 2.7,
					repeat: -1,
					yoyo: true,
					ease: 'sine.inOut'
				});

				const satellites = gsap.utils.toArray<HTMLElement>('.orb-satellite-float');
				satellites.forEach((sat, index) => {
					gsap.to(sat, {
						y: index % 2 === 0 ? -3.5 : 3.5,
						duration: 2.4 + (index % 3) * 0.35,
						delay: index * 0.25,
						repeat: -1,
						yoyo: true,
						ease: 'sine.inOut'
					});
				});

				// 6. ScrollTrigger scrubbed articulation so scroll physically drives the orb.
				gsap.fromTo(
					'.orb-scrub-ring',
					{ rotation: -48, scale: 0.94, svgOrigin: `${CX} ${CY}` },
					{
						rotation: 52,
						scale: 1.04,
						ease: 'none',
						svgOrigin: `${CX} ${CY}`,
						scrollTrigger: {
							trigger: rootEl,
							start: 'top bottom',
							end: 'bottom top',
							scrub: 0.75
						}
					}
				);

				gsap.fromTo(
					'.orb-parallax-depth',
					{ y: 14 },
					{
						y: -14,
						ease: 'none',
						scrollTrigger: {
							trigger: rootEl,
							start: 'top bottom',
							end: 'bottom top',
							scrub: 0.65
						}
					}
				);

				// 7. Pointer quickTo parallax across outer/mid/inner orbital layers.
				const outerLayer = container.querySelector<HTMLElement>('.orb-layer-outer');
				const midLayer = container.querySelector<HTMLElement>('.orb-layer-mid');
				const innerLayer = container.querySelector<HTMLElement>('.orb-layer-inner');
				if (outerLayer && midLayer && innerLayer) {
					quickLayers = {
						outerX: gsap.quickTo(outerLayer, 'x', { duration: 0.55, ease: 'power3.out' }),
						outerY: gsap.quickTo(outerLayer, 'y', { duration: 0.55, ease: 'power3.out' }),
						midX: gsap.quickTo(midLayer, 'x', { duration: 0.45, ease: 'power3.out' }),
						midY: gsap.quickTo(midLayer, 'y', { duration: 0.45, ease: 'power3.out' }),
						innerX: gsap.quickTo(innerLayer, 'x', { duration: 0.35, ease: 'power3.out' }),
						innerY: gsap.quickTo(innerLayer, 'y', { duration: 0.35, ease: 'power3.out' })
					};
				}
			}, container);
		}

		syncMotionRunner = (isNowReduced: boolean) => {
			if (isNowReduced) {
				resetPointerParallax();
				teardownGsap();
			} else if (activeGsap) {
				setupAnimations(activeGsap);
			}
		};

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle) return;
			gsapRef = bundle.gsap;
			activeGsap = bundle.gsap;
			gsapReady = true;
			if (!reducedMotion.current) {
				setupAnimations(bundle.gsap);
			}
		});

		return () => {
			cancelled = true;
			syncMotionRunner = null;
			teardownGsap();
			gsapRef = null;
			reducedMotion.destroy();
		};
	});

	$effect(() => {
		// Reacts live to both OS media query and in-app `preferences.value.reduceMotion`.
		if (!gsapReady) return;
		syncMotionRunner?.(reduced);
	});
</script>

<div
	bind:this={rootEl}
	data-slot="card"
	data-gsap-orb
	class={cn(
		'bg-card frost-card text-card-foreground ring-(--surface-ring) relative grid w-full max-w-full min-w-0 overflow-hidden rounded-2xl ring-1 lg:grid-cols-[1.02fr_0.98fr]',
		className
	)}
>
	<!-- Left column: Editorial overview, dynamic node spotlight & interactive capability index -->
	<div class="flex min-w-0 flex-col justify-between gap-6 p-6 sm:p-8">
		<div class="space-y-5">
			<!-- Eyebrow & local badge -->
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div class="flex min-w-0 items-center gap-2">
					<span
						class="bg-primary inline-block size-2 shrink-0 rounded-full"
						class:animate-pulse={!reduced}
						aria-hidden="true"
					></span>
					<span
						class="text-muted-foreground truncate font-mono text-[11px] font-medium tracking-wider uppercase"
					>
						{isId ? 'Myaaw · Inti Lokal' : 'Myaaw · Local Hub'}
					</span>
				</div>
				<span
					class="bg-muted/60 text-muted-foreground border-border/70 shrink-0 rounded-full border px-2.5 py-0.5 font-mono text-[10px]"
				>
					{isId ? 'Berjalan lokal' : 'Runs locally'}
				</span>
			</div>

			<!-- Headline -->
			<h2 class="text-2xl leading-[1.15] font-bold tracking-tight text-foreground sm:text-3xl">
				{isId
					? 'Semua terhubung ke satu pusat lokal.'
					: 'Everything connects to one quiet local hub.'}
			</h2>

			<!-- Active node spotlight card -->
			{#key selectedId}
				<div
					use:motionPop={{ y: 6, duration: 0.24 }}
					class="rounded-xl border border-border/70 bg-muted/35 p-4"
				>
					<div class="flex flex-wrap items-center justify-between gap-2">
						<div class="flex min-w-0 items-center gap-2">
							{#if activeNode}
								{@const ActiveIcon = activeNode.icon}
								<span
									class="grid size-7 shrink-0 place-items-center rounded-lg bg-primary/12 text-primary"
								>
									<ActiveIcon class="size-3.5" aria-hidden="true" />
								</span>
							{:else}
								<span
									class="grid size-7 shrink-0 place-items-center rounded-lg bg-primary/12 text-primary"
								>
									<Bloub mood={coreMood} color={dark ? 'orange' : 'encre'} size={20} tight />
								</span>
							{/if}
							<span class="truncate text-sm font-semibold text-foreground">{readoutTitle}</span>
						</div>
						<span
							class="border-border/80 bg-background/80 text-muted-foreground shrink-0 rounded-md border px-2 py-0.5 font-mono text-[10px]"
						>
							{readoutTag}
						</span>
					</div>

					<p class="mt-2.5 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
						{readoutSummary}
					</p>

					{#if activeSectionTarget}
						<button
							type="button"
							onclick={jumpToActiveSection}
							class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-primary transition-opacity hover:opacity-80"
						>
							<span>{isId ? 'Lihat detail bagian ini' : 'Jump to this section'}</span>
							<ArrowUpRight class="size-3.5" aria-hidden="true" />
						</button>
					{/if}
				</div>
			{/key}
		</div>

		<!-- Interactive capability index (synced 1:1 with the orbital diagram) -->
		<div class="space-y-2.5">
			<div class="flex items-center justify-between gap-2">
				<span class="font-mono text-[10px] tracking-wider text-muted-foreground uppercase">
					{isId ? 'Jelajahi komponen' : 'Explore modules'}
				</span>
				<button
					type="button"
					onclick={activateCore}
					class={cn(
						'font-mono text-[10px] tracking-wider uppercase transition-colors',
						selectedId === 'core'
							? 'font-semibold text-primary'
							: 'text-muted-foreground hover:text-foreground'
					)}
				>
					{isId ? 'Inti Myaaw' : 'Myaaw Core'}
				</button>
			</div>

			<div
				class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-2"
				role="group"
				aria-label={readoutTitle}
			>
				{#each COMPUTED_NODES as node (node.id)}
					{@const Icon = node.icon}
					{@const isSelected = selectedId === node.id}
					<button
						type="button"
						use:motionButton={{ scale: 0.97, hoverScale: 1.01 }}
						onclick={() => activateNode(node)}
						aria-pressed={isSelected}
						class={cn(
							'flex min-w-0 items-center gap-2 rounded-lg border px-2.5 py-2 text-left transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
							isSelected
								? 'border-primary bg-primary text-primary-foreground'
								: 'border-border/70 bg-background/60 text-foreground hover:border-foreground/25 hover:bg-muted/50'
						)}
					>
						<Icon class="size-3.5 shrink-0 opacity-85" aria-hidden="true" />
						<span class="truncate text-xs font-medium">{isId ? node.labelId : node.label}</span>
					</button>
				{/each}
			</div>
		</div>
	</div>

	<!-- Right column: Dedicated orbital instrument stage (GSAP orbital animation unchanged) -->
	<div
		class="relative flex min-w-0 items-center justify-center border-t border-border/60 bg-muted/20 p-4 sm:p-6 lg:border-t-0 lg:border-l"
	>
		<!-- Subtle architectural corner registration ticks -->
		<span
			aria-hidden="true"
			class="pointer-events-none absolute top-3 left-3 size-3 border-t border-l border-border/80"
		></span>
		<span
			aria-hidden="true"
			class="pointer-events-none absolute top-3 right-3 size-3 border-t border-r border-border/80"
		></span>
		<span
			aria-hidden="true"
			class="pointer-events-none absolute bottom-3 left-3 size-3 border-b border-l border-border/80"
		></span>
		<span
			aria-hidden="true"
			class="pointer-events-none absolute right-3 bottom-3 size-3 border-r border-b border-border/80"
		></span>

		<!-- Interactive GSAP orbital stage -->
		<div
			bind:this={stageEl}
			role="group"
			aria-label={isId ? 'Diagram orbit interaktif Myaaw' : 'Interactive Myaaw orbital diagram'}
			class="orb-parallax-depth landing-gpu relative mx-auto aspect-square w-full max-w-106 overflow-hidden select-none"
			onpointermove={handlePointerMove}
			onpointerleave={resetPointerParallax}
		>
			<!-- Layer 1 & 2: Precision orbital rings, conduits, and traveling packets -->
			<div class="orb-layer-outer landing-gpu pointer-events-none absolute inset-0">
				<svg
					viewBox="0 0 440 440"
					class="h-full w-full text-foreground"
					aria-hidden="true"
					focusable="false"
				>
					<!-- Subtle radial focus glow using semantic currentColor / primary -->
					<defs>
						<radialGradient id={`${uid}-core-glow`} cx="50%" cy="50%" r="50%">
							<stop offset="0%" stop-color="var(--primary)" stop-opacity="0.14" />
							<stop offset="58%" stop-color="var(--primary)" stop-opacity="0.04" />
							<stop offset="100%" stop-color="var(--primary)" stop-opacity="0" />
						</radialGradient>
					</defs>
					<circle cx={CX} cy={CY} r="192" fill={`url(#${uid}-core-glow)`} />

					<!-- Outer precision compass ring (GSAP continuous rotation) -->
					<g class="orb-ring-outer">
						<circle
							cx={CX}
							cy={CY}
							r="186"
							fill="none"
							stroke="var(--border)"
							stroke-width="1"
							stroke-opacity="0.75"
						/>
						{#each COMPASS_TICKS as tick (tick.index)}
							<line
								x1={tick.x1}
								y1={tick.y1}
								x2={tick.x2}
								y2={tick.y2}
								stroke="var(--muted-foreground)"
								stroke-width={tick.major ? 1.25 : 0.75}
								stroke-opacity={tick.major ? 0.55 : 0.28}
							/>
						{/each}
					</g>

					<!-- ScrollTrigger scrubbed mechanical bracket ring -->
					<g class="orb-scrub-ring">
						<circle
							cx={CX}
							cy={CY}
							r="168"
							fill="none"
							stroke="var(--primary)"
							stroke-width="1.5"
							stroke-opacity="0.32"
							stroke-dasharray="32 56 12 56"
							stroke-linecap="round"
						/>
					</g>
				</svg>
			</div>

			<!-- Mid Layer: Counter-rotating telemetry ring, conduits & data packets -->
			<div class="orb-layer-mid landing-gpu pointer-events-none absolute inset-0">
				<svg
					viewBox="0 0 440 440"
					class="h-full w-full text-foreground"
					aria-hidden="true"
					focusable="false"
				>
					<!-- Middle dashed telemetry ring -->
					<g class="orb-ring-mid">
						<circle
							cx={CX}
							cy={CY}
							r="134"
							fill="none"
							stroke="var(--border)"
							stroke-width="1.2"
							stroke-dasharray="4 8"
							stroke-opacity="0.85"
						/>
						<circle cx={CX} cy={CY - 134} r="2.5" fill="var(--primary)" opacity="0.65" />
						<circle cx={CX} cy={CY + 134} r="2.5" fill="var(--primary)" opacity="0.65" />
					</g>

					<!-- Inner precision ring -->
					<g class="orb-ring-inner">
						<circle
							cx={CX}
							cy={CY}
							r="82"
							fill="none"
							stroke="var(--border)"
							stroke-width="1"
							stroke-dasharray="14 10"
							stroke-opacity="0.7"
						/>
					</g>

					<!-- Curved signal conduits + animated stroke beams + traveling packet dots -->
					{#each COMPUTED_NODES as node (node.id)}
						{@const isActive = selectedId === node.id || selectedId === 'core'}
						<path
							d={node.pathD}
							fill="none"
							stroke="var(--border)"
							stroke-width={selectedId === node.id ? 1.8 : 1.1}
							stroke-opacity={isActive ? 0.85 : 0.45}
						/>
						<path
							class="orb-beam"
							d={node.pathD}
							fill="none"
							stroke="var(--primary)"
							stroke-width={selectedId === node.id ? 2.1 : 1.35}
							stroke-opacity={selectedId === node.id ? 0.9 : 0.48}
							stroke-dasharray="20 160"
							stroke-linecap="round"
						/>
						<circle
							class="orb-packet"
							cx={node.ctrlX}
							cy={node.ctrlY}
							r={selectedId === node.id ? 3.8 : 2.9}
							fill="var(--primary)"
							opacity={reduced ? 0.65 : 0.85}
						/>
					{/each}

					<!-- Continuous staggered sonar ripple rings -->
					<circle
						class="orb-sonar"
						cx={CX}
						cy={CY}
						r="46"
						fill="none"
						stroke="var(--primary)"
						stroke-width="1"
						opacity={reduced ? 0.16 : 0}
					/>
					<circle
						class="orb-sonar"
						cx={CX}
						cy={CY}
						r="46"
						fill="none"
						stroke="var(--primary)"
						stroke-width="1"
						opacity="0"
					/>
					<circle
						class="orb-sonar"
						cx={CX}
						cy={CY}
						r="46"
						fill="none"
						stroke="var(--primary)"
						stroke-width="1"
						opacity="0"
					/>

					<!-- Interactive click/hover shockwave ring -->
					<circle
						bind:this={shockwaveEl}
						cx={CX}
						cy={CY}
						r="48"
						fill="none"
						stroke="var(--primary)"
						stroke-width="2"
						opacity="0"
						data-pulse-count={pulseCount}
					/>
				</svg>
			</div>

			<!-- Inner Layer: Interactive central Bloub pod + 6 orbiting capability satellites -->
			<div class="orb-layer-inner landing-gpu absolute inset-0">
				<!-- Central levitating Bloub Core Pod -->
				<div class="orb-core-float absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
					<button
						bind:this={corePodEl}
						type="button"
						use:motionButton={{ scale: 0.94, hoverScale: 1.04 }}
						onclick={activateCore}
						aria-pressed={selectedId === 'core'}
						aria-label={isId ? 'Inti Control Plane Myaaw' : 'Myaaw Control Plane Core'}
						class={cn(
							'bg-card/95 frost-card ring-(--surface-ring) hover:ring-primary/50 focus-visible:ring-ring flex size-22 flex-col items-center justify-center rounded-full shadow-sm ring-1 transition-shadow focus-visible:ring-2 focus-visible:outline-none sm:size-24',
							selectedId === 'core' && 'ring-primary/65 ring-2'
						)}
					>
						<Bloub mood={coreMood} color={dark ? 'orange' : 'encre'} size={54} fade eager tight />
						<span
							class="text-muted-foreground font-mono text-[9px] font-semibold tracking-wider uppercase"
						>
							myaaw
						</span>
					</button>
				</div>

				<!-- 6 interactive orbiting capability satellite nodes -->
				{#each COMPUTED_NODES as node (node.id)}
					{@const Icon = node.icon}
					{@const isSelected = selectedId === node.id}
					<div
						class="absolute -translate-x-1/2 -translate-y-1/2"
						style:left={node.leftPct}
						style:top={node.topPct}
					>
						<div class="orb-satellite-float">
							<button
								type="button"
								use:motionButton={{ scale: 0.95, hoverScale: 1.04 }}
								onclick={(event) => activateNode(node, event.currentTarget)}
								onmouseenter={(event) => {
									if (selectedId !== node.id) activateNode(node, event.currentTarget);
								}}
								aria-pressed={isSelected}
								class={cn(
									'bg-card/95 frost-card ring-(--surface-ring) hover:ring-primary/50 focus-visible:ring-ring flex items-center gap-1 rounded-full px-2 py-1 text-[10px] font-medium whitespace-nowrap shadow-xs ring-1 transition-colors focus-visible:ring-2 focus-visible:outline-none sm:gap-1.5 sm:px-2.5 sm:py-1.5 sm:text-[11px]',
									isSelected ? 'bg-primary text-primary-foreground ring-primary' : 'text-foreground'
								)}
							>
								<Icon class="size-3 shrink-0 sm:size-3.5" aria-hidden="true" />
								<span class="leading-none">{isId ? node.labelId : node.label}</span>
							</button>
						</div>
					</div>
				{/each}
			</div>
		</div>
	</div>
</div>
