<script lang="ts">
	import { onMount } from 'svelte';
	import { createReducedMotionState, loadGsap } from '$lib/landing/motion';
	import { cn } from '$lib/utils';

	/**
	 * Architectural Agent Process Trace Field (GSAP-choreographed).
	 *
	 * Replaces generic particle/constellation webs with an orthographic AI-agent
	 * telemetry blueprint: isolated process swimlanes, live expanding/collapsing
	 * execution span capsules (`think`, `mcp:bridge`, `git.diff`, `acp.rpc`, ...),
	 * 90-degree signal handoff buses with traveling token packets, rotating
	 * junction reticles, and multi-depth `gsap.quickTo` pointer parallax.
	 *
	 * Uses only semantic theme tokens so it stays crisp across all themes, pauses
	 * off-screen and on hidden tabs, and renders a calm static blueprint under
	 * reduced motion.
	 */
	let { class: className = '' }: { class?: string } = $props();

	const reducedMotion = createReducedMotionState();
	const reduced = $derived(reducedMotion.current);

	interface ProcessLane {
		id: string;
		label: string;
		y: number;
		dashed?: boolean;
	}

	interface ExecutionSpan {
		id: string;
		laneY: number;
		x: number;
		width: number;
		tag: string;
		accent?: boolean;
		delay: number;
		duration: number;
		hold: number;
	}

	interface BusWaypoint {
		x: number;
		y: number;
	}

	interface SignalBus {
		id: string;
		path: string;
		length: number;
		waypoints: BusWaypoint[];
		junction: BusWaypoint;
		delay: number;
		duration: number;
	}

	interface TelemetryBracket {
		id: string;
		x: number;
		y: number;
		code: string;
		driftY: number;
		duration: number;
		delay: number;
	}

	// Six orthographic process rails across a 1440x820 blueprint coordinate frame.
	const LANES: readonly ProcessLane[] = [
		{ id: 'p0', label: 'PID:01', y: 92 },
		{ id: 'p1', label: 'PID:02', y: 214, dashed: true },
		{ id: 'p2', label: 'PID:03', y: 368 },
		{ id: 'p3', label: 'PID:04', y: 522, dashed: true },
		{ id: 'p4', label: 'PID:05', y: 656 },
		{ id: 'p5', label: 'PID:06', y: 760, dashed: true }
	];

	// Execution span capsules positioned around the periphery and natural pauses
	// so they frame the hero typography and RunStream card cleanly.
	const SPANS: readonly ExecutionSpan[] = [
		{
			id: 's1',
			laneY: 92,
			x: 112,
			width: 174,
			tag: 'acp.rpc',
			accent: true,
			delay: 0.2,
			duration: 1.5,
			hold: 2.1
		},
		{
			id: 's2',
			laneY: 92,
			x: 560,
			width: 196,
			tag: 'mcp:bridge',
			delay: 1.4,
			duration: 1.7,
			hold: 1.8
		},
		{
			id: 's3',
			laneY: 92,
			x: 1090,
			width: 184,
			tag: 'sqlite.wal',
			accent: true,
			delay: 0.8,
			duration: 1.6,
			hold: 2.2
		},
		{
			id: 's4',
			laneY: 214,
			x: 36,
			width: 138,
			tag: 'think',
			delay: 2.1,
			duration: 1.4,
			hold: 1.9
		},
		{
			id: 's5',
			laneY: 214,
			x: 642,
			width: 146,
			tag: 'tool.exec',
			delay: 0.5,
			duration: 1.35,
			hold: 2.0
		},
		{
			id: 's6',
			laneY: 214,
			x: 1228,
			width: 148,
			tag: 'pty:02',
			accent: true,
			delay: 1.9,
			duration: 1.55,
			hold: 1.7
		},
		{
			id: 's7',
			laneY: 368,
			x: 1244,
			width: 144,
			tag: 'git.diff',
			delay: 1.1,
			duration: 1.5,
			hold: 2.3
		},
		{
			id: 's8',
			laneY: 522,
			x: 28,
			width: 146,
			tag: 'a2a.sync',
			accent: true,
			delay: 0.9,
			duration: 1.6,
			hold: 2.1
		},
		{
			id: 's9',
			laneY: 522,
			x: 618,
			width: 152,
			tag: 'ctx.cache',
			delay: 2.5,
			duration: 1.45,
			hold: 1.8
		},
		{
			id: 's10',
			laneY: 656,
			x: 68,
			width: 162,
			tag: 'group.room',
			delay: 1.7,
			duration: 1.55,
			hold: 2.0
		},
		{
			id: 's11',
			laneY: 656,
			x: 1208,
			width: 170,
			tag: 'cron.tick',
			accent: true,
			delay: 0.4,
			duration: 1.5,
			hold: 2.4
		},
		{
			id: 's12',
			laneY: 760,
			x: 492,
			width: 214,
			tag: 'run.stream',
			accent: true,
			delay: 1.2,
			duration: 1.8,
			hold: 1.9
		}
	];

	// Orthogonal 90-degree agent handoff conduits (PCB / scheduler trace geometry).
	const BUSES: readonly SignalBus[] = [
		{
			id: 'b1',
			path: 'M 196 92 H 432 V 214 H 642',
			length: 568,
			waypoints: [
				{ x: 196, y: 92 },
				{ x: 432, y: 92 },
				{ x: 432, y: 214 },
				{ x: 642, y: 214 }
			],
			junction: { x: 432, y: 214 },
			delay: 0.3,
			duration: 4.4
		},
		{
			id: 'b2',
			path: 'M 756 92 H 1018 V 214 H 1228',
			length: 594,
			waypoints: [
				{ x: 756, y: 92 },
				{ x: 1018, y: 92 },
				{ x: 1018, y: 214 },
				{ x: 1228, y: 214 }
			],
			junction: { x: 1018, y: 92 },
			delay: 1.6,
			duration: 4.8
		},
		{
			id: 'b3',
			path: 'M 1316 214 V 368 H 1244 V 522 H 1368',
			length: 504,
			waypoints: [
				{ x: 1316, y: 214 },
				{ x: 1316, y: 368 },
				{ x: 1244, y: 368 },
				{ x: 1244, y: 522 },
				{ x: 1368, y: 522 }
			],
			junction: { x: 1244, y: 368 },
			delay: 0.9,
			duration: 5.1
		},
		{
			id: 'b4',
			path: 'M 102 522 H 264 V 656 H 516 V 760',
			length: 652,
			waypoints: [
				{ x: 102, y: 522 },
				{ x: 264, y: 522 },
				{ x: 264, y: 656 },
				{ x: 516, y: 656 },
				{ x: 516, y: 760 }
			],
			junction: { x: 264, y: 656 },
			delay: 2.0,
			duration: 5.4
		},
		{
			id: 'b5',
			path: 'M 706 760 H 1066 V 656 H 1208',
			length: 606,
			waypoints: [
				{ x: 706, y: 760 },
				{ x: 1066, y: 760 },
				{ x: 1066, y: 656 },
				{ x: 1208, y: 656 }
			],
			junction: { x: 1066, y: 656 },
			delay: 1.1,
			duration: 4.6
		}
	];

	// Floating precision registration brackets (depth accent layer).
	const BRACKETS: readonly TelemetryBracket[] = [
		{ id: 'k1', x: 74, y: 146, code: '01//PROC', driftY: 9, duration: 5.8, delay: 0 },
		{ id: 'k2', x: 598, y: 148, code: '02//LOOP', driftY: -8, duration: 6.4, delay: 0.7 },
		{ id: 'k3', x: 1272, y: 150, code: '03//PTY', driftY: 10, duration: 5.5, delay: 1.2 },
		{ id: 'k4', x: 84, y: 586, code: '04//A2A', driftY: -9, duration: 6.2, delay: 0.4 },
		{ id: 'k5', x: 1266, y: 590, code: '05//WAL', driftY: 8, duration: 6.0, delay: 1.5 }
	];

	let rootEl: HTMLDivElement | null = $state(null);
	let gsapReady = $state(false);
	let syncMotionRunner: ((_isReduced: boolean) => void) | null = null;

	onMount(() => {
		let cancelled = false;
		let visible = true;
		let tabVisible = !document.hidden;
		let ctx: { revert: () => void } | null = null;
		let activeGsap: typeof import('gsap').gsap | null = null;
		let activeTimelines: Array<{ pause: () => void; play: () => void }> = [];
		let pointerMoveHandler: ((event: PointerEvent) => void) | null = null;
		let pointerLeaveHandler: (() => void) | null = null;

		function updatePlayback() {
			const shouldPlay = visible && tabVisible && !reducedMotion.current;
			for (const tl of activeTimelines) {
				if (shouldPlay) tl.play();
				else tl.pause();
			}
		}

		function teardown() {
			if (pointerMoveHandler) {
				window.removeEventListener('pointermove', pointerMoveHandler);
				pointerMoveHandler = null;
			}
			if (pointerLeaveHandler) {
				window.removeEventListener('pointerleave', pointerLeaveHandler);
				pointerLeaveHandler = null;
			}
			activeTimelines = [];
			ctx?.revert();
			ctx = null;
		}

		function setupAnimations(gsap: typeof import('gsap').gsap) {
			const container = rootEl;
			if (cancelled || !container || reducedMotion.current) return;
			teardown();

			ctx = gsap.context(() => {
				const timelines: Array<{ pause: () => void; play: () => void }> = [];

				// 1. Live Execution Span Capsules: expand left->right, hold with cursor blink, collapse right.
				SPANS.forEach((span) => {
					const bar = container.querySelector<SVGRectElement>(`[data-span-bar="${span.id}"]`);
					const cap = container.querySelector<SVGRectElement>(`[data-span-cap="${span.id}"]`);
					const label = container.querySelector<SVGTextElement>(`[data-span-tag="${span.id}"]`);
					if (!bar) return;

					const tl = gsap.timeline({
						repeat: -1,
						repeatDelay: 1.1 + (span.delay % 1.4),
						delay: span.delay
					});

					tl.set(bar, {
						scaleX: 0,
						opacity: 0,
						svgOrigin: `${span.x} ${span.laneY}`
					})
						.to(bar, {
							scaleX: 1,
							opacity: span.accent ? 0.24 : 0.14,
							duration: span.duration,
							ease: 'power2.inOut'
						})
						.to(
							label,
							{
								opacity: span.accent ? 0.56 : 0.36,
								duration: 0.45,
								ease: 'power2.out'
							},
							'-=0.85'
						);

					if (cap) {
						tl.fromTo(
							cap,
							{ opacity: 0.7 },
							{
								opacity: 0.12,
								duration: 0.38,
								repeat: 3,
								yoyo: true,
								ease: 'sine.inOut'
							},
							'-=0.25'
						);
					}

					tl.to({}, { duration: span.hold })
						.to(label, {
							opacity: 0,
							duration: 0.35,
							ease: 'power1.in'
						})
						.set(bar, {
							svgOrigin: `${span.x + span.width} ${span.laneY}`
						})
						.to(
							bar,
							{
								scaleX: 0,
								opacity: 0,
								duration: span.duration * 0.72,
								ease: 'power2.in'
							},
							'-=0.2'
						);

					timelines.push(tl);
				});

				// 2. Orthogonal Signal Bus Conduits, Waypoint Packets & Junction Reticle Lock-Ons.
				BUSES.forEach((bus) => {
					const trace = container.querySelector<SVGPathElement>(`[data-bus-trace="${bus.id}"]`);
					const packet = container.querySelector<SVGRectElement>(`[data-bus-packet="${bus.id}"]`);
					const reticle = container.querySelector<SVGGElement>(`[data-bus-reticle="${bus.id}"]`);
					const ping = container.querySelector<SVGCircleElement>(`[data-bus-ping="${bus.id}"]`);

					if (trace) {
						const dashTl = gsap.timeline({ repeat: -1, delay: bus.delay });
						dashTl.fromTo(
							trace,
							{ strokeDashoffset: bus.length + 140 },
							{
								strokeDashoffset: -140,
								duration: bus.duration,
								ease: 'none'
							}
						);
						timelines.push(dashTl);
					}

					if (packet && bus.waypoints.length >= 2) {
						const start = bus.waypoints[0];
						const segLengths: number[] = [];
						let totalDist = 0;
						for (let i = 1; i < bus.waypoints.length; i += 1) {
							const prev = bus.waypoints[i - 1];
							const next = bus.waypoints[i];
							const dist = Math.hypot(next.x - prev.x, next.y - prev.y);
							segLengths.push(dist);
							totalDist += dist;
						}

						const pktTl = gsap.timeline({
							repeat: -1,
							repeatDelay: 0.85,
							delay: bus.delay
						});
						pktTl.set(packet, {
							attr: { x: start.x - 4, y: start.y - 4 },
							opacity: 0
						});
						pktTl.to(packet, { opacity: 0.82, duration: 0.2, ease: 'power1.out' });

						for (let i = 1; i < bus.waypoints.length; i += 1) {
							const pt = bus.waypoints[i];
							const segDur = (segLengths[i - 1] / Math.max(totalDist, 1)) * (bus.duration * 0.82);
							pktTl.to(packet, {
								attr: { x: pt.x - 4, y: pt.y - 4 },
								duration: segDur,
								ease: 'none'
							});

							// Trigger reticle rotation + sonar ping when packet hits the bus junction.
							if (
								Math.abs(pt.x - bus.junction.x) < 1 &&
								Math.abs(pt.y - bus.junction.y) < 1 &&
								reticle
							) {
								pktTl.to(
									reticle,
									{
										rotation: '+=90',
										scale: 1.28,
										duration: 0.36,
										ease: 'back.out(2)',
										svgOrigin: `${bus.junction.x} ${bus.junction.y}`
									},
									'>-0.05'
								);
								pktTl.to(reticle, {
									scale: 1,
									duration: 0.32,
									ease: 'power2.out',
									svgOrigin: `${bus.junction.x} ${bus.junction.y}`
								});
								if (ping) {
									pktTl.fromTo(
										ping,
										{
											scale: 0.6,
											opacity: 0.55,
											svgOrigin: `${bus.junction.x} ${bus.junction.y}`
										},
										{
											scale: 2.6,
											opacity: 0,
											duration: 0.9,
											ease: 'power2.out',
											svgOrigin: `${bus.junction.x} ${bus.junction.y}`
										},
										'<-0.3'
									);
								}
							}
						}

						pktTl.to(packet, { opacity: 0, duration: 0.25, ease: 'power1.in' });
						timelines.push(pktTl);
					}
				});

				// 3. Soft floating architectural registration brackets.
				BRACKETS.forEach((bracket) => {
					const el = container.querySelector<SVGGElement>(`[data-bracket="${bracket.id}"]`);
					if (!el) return;
					const floatTl = gsap.timeline({ repeat: -1, yoyo: true, delay: bracket.delay });
					floatTl.to(el, {
						y: bracket.driftY,
						opacity: 0.42,
						duration: bracket.duration,
						ease: 'sine.inOut'
					});
					timelines.push(floatTl);
				});

				// 4. Sweeping vertical telemetry scan hairline (subtle oscilloscope beat).
				const scanLine = container.querySelector<SVGLineElement>('[data-scan-beam]');
				if (scanLine) {
					const scanTl = gsap.timeline({ repeat: -1, repeatDelay: 2.6 });
					scanTl
						.fromTo(scanLine, { x: -40, opacity: 0 }, { x: 140, opacity: 0.16, duration: 1.1 })
						.to(scanLine, { x: 1320, opacity: 0.14, duration: 7.4, ease: 'none' })
						.to(scanLine, { x: 1480, opacity: 0, duration: 1.1 });
					timelines.push(scanTl);
				}

				// 5. Multi-depth `gsap.quickTo` pointer parallax + optical cursor reticle (desktop mouse).
				const farLayer = container.querySelector<HTMLElement>('[data-depth="far"]');
				const midLayer = container.querySelector<HTMLElement>('[data-depth="mid"]');
				const nearLayer = container.querySelector<HTMLElement>('[data-depth="near"]');
				const reticleEl = container.querySelector<HTMLElement>('[data-cursor-reticle]');

				if (farLayer && midLayer && nearLayer) {
					const farX = gsap.quickTo(farLayer, 'x', { duration: 0.85, ease: 'power3.out' });
					const farY = gsap.quickTo(farLayer, 'y', { duration: 0.85, ease: 'power3.out' });
					const midX = gsap.quickTo(midLayer, 'x', { duration: 0.65, ease: 'power3.out' });
					const midY = gsap.quickTo(midLayer, 'y', { duration: 0.65, ease: 'power3.out' });
					const nearX = gsap.quickTo(nearLayer, 'x', { duration: 0.5, ease: 'power3.out' });
					const nearY = gsap.quickTo(nearLayer, 'y', { duration: 0.5, ease: 'power3.out' });
					const retX = reticleEl
						? gsap.quickTo(reticleEl, 'x', { duration: 0.38, ease: 'power3.out' })
						: null;
					const retY = reticleEl
						? gsap.quickTo(reticleEl, 'y', { duration: 0.38, ease: 'power3.out' })
						: null;

					pointerMoveHandler = (event: PointerEvent) => {
						if (!visible || event.pointerType !== 'mouse') return;
						const rect = container.getBoundingClientRect();
						if (rect.width <= 0 || rect.height <= 0) return;
						const inside =
							event.clientX >= rect.left &&
							event.clientX <= rect.right &&
							event.clientY >= rect.top &&
							event.clientY <= rect.bottom;
						if (!inside) {
							if (reticleEl) gsap.to(reticleEl, { opacity: 0, duration: 0.35, overwrite: 'auto' });
							return;
						}
						const nx = ((event.clientX - rect.left) / rect.width - 0.5) * 2;
						const ny = ((event.clientY - rect.top) / rect.height - 0.5) * 2;
						farX(nx * -8);
						farY(ny * -6);
						midX(nx * 6);
						midY(ny * 5);
						nearX(nx * 14);
						nearY(ny * 10);

						if (reticleEl && retX && retY) {
							retX(event.clientX - rect.left);
							retY(event.clientY - rect.top);
							gsap.to(reticleEl, { opacity: 0.22, duration: 0.3, overwrite: 'auto' });
						}
					};

					pointerLeaveHandler = () => {
						farX(0);
						farY(0);
						midX(0);
						midY(0);
						nearX(0);
						nearY(0);
						if (reticleEl) gsap.to(reticleEl, { opacity: 0, duration: 0.35, overwrite: 'auto' });
					};

					window.addEventListener('pointermove', pointerMoveHandler, { passive: true });
					window.addEventListener('pointerleave', pointerLeaveHandler, { passive: true });
				}

				activeTimelines = timelines;
				updatePlayback();
			}, container);
		}

		const observer = new IntersectionObserver(
			(entries) => {
				visible = entries.some((entry) => entry.isIntersecting);
				updatePlayback();
			},
			{ threshold: 0.02 }
		);
		if (rootEl) observer.observe(rootEl);

		const onVisibility = () => {
			tabVisible = !document.hidden;
			updatePlayback();
		};
		document.addEventListener('visibilitychange', onVisibility);

		syncMotionRunner = (isNowReduced: boolean) => {
			if (isNowReduced) {
				teardown();
			} else if (activeGsap) {
				setupAnimations(activeGsap);
			}
		};

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle) return;
			activeGsap = bundle.gsap;
			gsapReady = true;
			if (!reducedMotion.current) {
				setupAnimations(bundle.gsap);
			}
		});

		return () => {
			cancelled = true;
			syncMotionRunner = null;
			observer.disconnect();
			document.removeEventListener('visibilitychange', onVisibility);
			teardown();
			reducedMotion.destroy();
		};
	});

	$effect(() => {
		if (!gsapReady) return;
		syncMotionRunner?.(reduced);
	});
</script>

<div
	bind:this={rootEl}
	data-agent-trace-field
	aria-hidden="true"
	class={cn('pointer-events-none absolute inset-0 overflow-hidden select-none', className)}
>
	<!-- Depth Layer 1 (Far): Orthographic Process Swimlanes & Edge PID Ticks -->
	<div data-depth="far" class="landing-gpu absolute -inset-4">
		<svg
			viewBox="0 0 1440 820"
			preserveAspectRatio="xMidYMid slice"
			class="h-full w-full text-foreground"
			focusable="false"
		>
			<!-- Subtle architectural column guides -->
			{#each [120, 432, 720, 1018, 1316] as colX (colX)}
				<line
					x1={colX}
					y1="0"
					x2={colX}
					y2="820"
					stroke="var(--border)"
					stroke-width="1"
					stroke-opacity="0.24"
					stroke-dasharray="2 10"
				/>
			{/each}

			<!-- Horizontal isolated agent process swimlanes -->
			{#each LANES as lane (lane.id)}
				<line
					x1="0"
					y1={lane.y}
					x2="1440"
					y2={lane.y}
					stroke="var(--border)"
					stroke-width="1"
					stroke-opacity={lane.dashed ? 0.34 : 0.45}
					stroke-dasharray={lane.dashed ? '4 8' : undefined}
				/>
				<text
					x="18"
					y={lane.y - 6}
					fill="var(--muted-foreground)"
					fill-opacity="0.34"
					font-family="var(--font-ui-mono)"
					font-size="9"
					letter-spacing="0.14em"
				>
					{lane.label}
				</text>
			{/each}

			<!-- Subtle sweeping telemetry scanline -->
			<line
				data-scan-beam
				x1="0"
				y1="0"
				x2="0"
				y2="820"
				stroke="var(--primary)"
				stroke-width="1.25"
				opacity="0"
			/>
		</svg>
	</div>

	<!-- Depth Layer 2 (Mid): Orthogonal Signal Buses, Live Spans & Junction Reticles -->
	<div data-depth="mid" class="landing-gpu absolute -inset-4">
		<svg
			viewBox="0 0 1440 820"
			preserveAspectRatio="xMidYMid slice"
			class="h-full w-full text-foreground"
			focusable="false"
		>
			<!-- 90-degree signal handoff conduits -->
			{#each BUSES as bus (bus.id)}
				<path
					d={bus.path}
					fill="none"
					stroke="var(--border)"
					stroke-width="1"
					stroke-opacity="0.45"
				/>
				<path
					data-bus-trace={bus.id}
					d={bus.path}
					fill="none"
					stroke="var(--primary)"
					stroke-width="1.4"
					stroke-opacity={reduced ? 0.2 : 0.42}
					stroke-dasharray="54 580"
					stroke-linecap="square"
				/>
				<rect
					data-bus-packet={bus.id}
					x={bus.waypoints[0].x - 4}
					y={bus.waypoints[0].y - 4}
					width="8"
					height="8"
					rx="1.5"
					fill="var(--primary)"
					opacity={reduced ? 0.25 : 0}
				/>

				<!-- Precision junction crosshair reticle + sonar ping -->
				<circle
					data-bus-ping={bus.id}
					cx={bus.junction.x}
					cy={bus.junction.y}
					r="7"
					fill="none"
					stroke="var(--primary)"
					stroke-width="1"
					opacity="0"
				/>
				<g data-bus-reticle={bus.id}>
					<circle
						cx={bus.junction.x}
						cy={bus.junction.y}
						r="6"
						fill="none"
						stroke="var(--foreground)"
						stroke-width="1"
						stroke-opacity="0.24"
						stroke-dasharray="5 4"
					/>
					<line
						x1={bus.junction.x - 4}
						y1={bus.junction.y}
						x2={bus.junction.x + 4}
						y2={bus.junction.y}
						stroke="var(--primary)"
						stroke-width="1"
						stroke-opacity="0.45"
					/>
					<line
						x1={bus.junction.x}
						y1={bus.junction.y - 4}
						x2={bus.junction.x}
						y2={bus.junction.y + 4}
						stroke="var(--primary)"
						stroke-width="1"
						stroke-opacity="0.45"
					/>
				</g>
			{/each}

			<!-- Expanding/collapsing live agent execution span capsules -->
			{#each SPANS as span (span.id)}
				<g>
					<rect
						data-span-bar={span.id}
						x={span.x}
						y={span.laneY - 7}
						width={span.width}
						height="14"
						rx="3"
						fill={span.accent ? 'var(--primary)' : 'var(--foreground)'}
						opacity={reduced ? (span.accent ? 0.12 : 0.08) : 0}
					/>
					<rect
						data-span-cap={span.id}
						x={span.x}
						y={span.laneY - 7}
						width="2.5"
						height="14"
						rx="1"
						fill="var(--primary)"
						opacity={reduced ? 0.28 : 0}
					/>
					<text
						data-span-tag={span.id}
						x={span.x + 8}
						y={span.laneY - 12}
						fill="var(--muted-foreground)"
						opacity={reduced ? 0.34 : 0}
						font-family="var(--font-ui-mono)"
						font-size="9.5"
						letter-spacing="0.12em"
					>
						{span.tag}
					</text>
				</g>
			{/each}
		</svg>
	</div>

	<!-- Depth Layer 3 (Near): Floating Registration Brackets -->
	<div data-depth="near" class="landing-gpu absolute -inset-4">
		<svg
			viewBox="0 0 1440 820"
			preserveAspectRatio="xMidYMid slice"
			class="h-full w-full text-foreground"
			focusable="false"
		>
			{#each BRACKETS as bracket (bracket.id)}
				<g data-bracket={bracket.id} opacity="0.22">
					<path
						d={`M ${bracket.x} ${bracket.y + 7} V ${bracket.y} H ${bracket.x + 7}`}
						fill="none"
						stroke="var(--foreground)"
						stroke-width="1"
					/>
					<path
						d={`M ${bracket.x + 76} ${bracket.y + 13} V ${bracket.y + 20} H ${bracket.x + 69}`}
						fill="none"
						stroke="var(--foreground)"
						stroke-width="1"
					/>
					<text
						x={bracket.x + 10}
						y={bracket.y + 13}
						fill="var(--muted-foreground)"
						font-family="var(--font-ui-mono)"
						font-size="8.5"
						letter-spacing="0.16em"
					>
						{bracket.code}
					</text>
				</g>
			{/each}
		</svg>
	</div>

	<!-- Subtle GSAP optical pointer tracking reticle -->
	<div
		data-cursor-reticle
		class="landing-gpu absolute top-0 left-0 -mt-3.5 -ml-3.5 size-7 opacity-0"
	>
		<svg viewBox="0 0 28 28" class="size-full text-primary" focusable="false">
			<path
				d="M 2 9 V 2 H 9 M 19 2 H 26 V 9 M 26 19 V 26 H 19 M 9 26 H 2 V 19"
				fill="none"
				stroke="currentColor"
				stroke-width="1.25"
			/>
			<circle cx="14" cy="14" r="1.5" fill="currentColor" />
		</svg>
	</div>
</div>
