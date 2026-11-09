<script lang="ts">
	import { onMount } from 'svelte';
	import { PawPrint } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { preferences } from '$lib/stores/preferences.svelte';

	/**
	 * The trail: instead of a ruler, the landing page leaves a path of paw prints
	 * down the right margin. Prints behind the reader are walked and solid; the
	 * ones ahead wait as faint ghosts, and the current position is a live paw.
	 *
	 * Each section is a larger print with its name, so the trail doubles as a
	 * section nav. It only appears when the margin is wide enough (1440px and up);
	 * smaller screens get a hairline progress bar under the header.
	 */
	const { t } = useTranslation();

	const SECTIONS = [
		{ id: 'loop', labelKey: 'landing.nav.loop' },
		{ id: 'fleet', labelKey: 'landing.nav.fleet' },
		{ id: 'doors', labelKey: 'landing.nav.doors' },
		{ id: 'rules', labelKey: 'landing.nav.rules' },
		{ id: 'closing', labelKey: 'landing.nav.install' }
	];

	/** Footprints every 4% of the page, alternating left and right like a walk. */
	const PRINTS = Array.from({ length: 26 }, (_, index) => index / 25);

	interface Mark {
		id: string;
		labelKey: string;
		at: number;
	}

	let progress = $state(0);
	let marks = $state<Mark[]>([]);
	let active = $state('');
	let direction = $state<'down' | 'up'>('down');
	let track: HTMLDivElement | null = $state(null);
	let dragging = $state(false);

	const percent = $derived(String(Math.round(progress * 100)).padStart(3, '0'));

	onMount(() => {
		const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		const reduce = () => preferences.value.reduceMotion || motionQuery.matches;

		let offsets: { id: string; top: number }[] = [];
		let target = 0;
		let shown = 0;
		let frame = 0;
		let lastY = window.scrollY;

		function paint() {
			progress = shown;
			updateActive();
		}

		function tick() {
			const diff = target - shown;
			if (Math.abs(diff) < 0.0004) {
				shown = target;
				paint();
				frame = 0;
				return;
			}
			shown += diff * 0.2;
			paint();
			frame = requestAnimationFrame(tick);
		}

		function updateActive() {
			const line = window.scrollY + window.innerHeight * 0.35;
			let current = '';
			for (const offset of offsets) {
				if (line >= offset.top) current = offset.id;
			}
			if (current !== active) active = current;
		}

		function measure() {
			const scrollable = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
			offsets = SECTIONS.map((section) => {
				const el = document.getElementById(section.id);
				const top = el ? el.getBoundingClientRect().top + window.scrollY : 0;
				return { id: section.id, top };
			});
			marks = SECTIONS.map((section, index) => ({
				id: section.id,
				labelKey: section.labelKey,
				at: Math.min(1, Math.max(0, (offsets[index]?.top ?? 0) / scrollable))
			}));
			readProgress();
		}

		function readProgress() {
			const y = window.scrollY;
			if (y > lastY + 1) direction = 'down';
			else if (y < lastY - 1) direction = 'up';
			lastY = y;

			const documentHeight = document.documentElement.scrollHeight;
			const scrollable = Math.max(1, documentHeight - window.innerHeight);
			target = Math.min(1, Math.max(0, y / scrollable));
			if (reduce() || dragging) {
				shown = target;
				paint();
				return;
			}
			if (!frame) frame = requestAnimationFrame(tick);
		}

		const resizeObserver = new ResizeObserver(measure);
		resizeObserver.observe(document.body);

		// The paw trail replaces the native scrollbar while this page is open.
		document.documentElement.dataset.landing = 'true';

		window.addEventListener('scroll', readProgress, { passive: true });
		window.addEventListener('resize', measure);
		measure();

		return () => {
			cancelAnimationFrame(frame);
			resizeObserver.disconnect();
			window.removeEventListener('scroll', readProgress);
			window.removeEventListener('resize', measure);
			delete document.documentElement.dataset.landing;
		};
	});

	function jump(event: MouseEvent, id: string) {
		event.preventDefault();
		const el = document.getElementById(id);
		if (!el) return;
		el.scrollIntoView({
			behavior: preferences.value.reduceMotion ? 'auto' : 'smooth',
			block: 'start'
		});
		history.replaceState(null, '', `#${id}`);
	}

	function seek(clientY: number) {
		if (!track) return;
		const rect = track.getBoundingClientRect();
		const ratio = Math.min(1, Math.max(0, (clientY - rect.top) / rect.height));
		const scrollable = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
		window.scrollTo(0, ratio * scrollable);
	}

	function onPointerDown(event: PointerEvent) {
		if (event.pointerType === 'mouse' && event.button !== 0) return;
		event.preventDefault();
		dragging = true;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		seek(event.clientY);
	}

	function onPointerMove(event: PointerEvent) {
		if (!dragging) return;
		seek(event.clientY);
	}

	function onPointerUp(event: PointerEvent) {
		dragging = false;
		const target = event.currentTarget as HTMLElement;
		if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId);
	}

	function onKeydown(event: KeyboardEvent) {
		const behavior = preferences.value.reduceMotion ? 'auto' : 'smooth';
		const page = window.innerHeight * 0.9;
		const scrollable = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);

		switch (event.key) {
			case 'ArrowDown':
				window.scrollBy({ top: 80, behavior });
				break;
			case 'ArrowUp':
				window.scrollBy({ top: -80, behavior });
				break;
			case 'PageDown':
				window.scrollBy({ top: page, behavior });
				break;
			case 'PageUp':
				window.scrollBy({ top: -page, behavior });
				break;
			case 'Home':
				window.scrollTo({ top: 0, behavior });
				break;
			case 'End':
				window.scrollTo({ top: scrollable, behavior });
				break;
			default:
				return;
		}
		event.preventDefault();
	}
</script>

<nav
	class="fixed top-24 right-2 bottom-10 z-30 hidden w-32 flex-col min-[1440px]:flex"
	aria-label={t('landing.trail.sections')}
>
	<div class="flex items-center justify-between gap-2 px-1 pb-3">
		<span class="trail-label">
			<PawPrint class="size-2.5" />
			{t('landing.trail.label')}
		</span>
		<span class="trail-readout">{percent}%</span>
	</div>

	<div class="relative flex-1">
		<div
			bind:this={track}
			class="trail-prints"
			class:dragging
			role="scrollbar"
			aria-controls="main"
			aria-label={t('landing.trail.scroll')}
			aria-orientation="vertical"
			aria-valuemin="0"
			aria-valuemax="100"
			aria-valuenow={Math.round(progress * 100)}
			tabindex="0"
			title={t('landing.trail.drag')}
			onpointerdown={onPointerDown}
			onpointermove={onPointerMove}
			onpointerup={onPointerUp}
			onpointercancel={onPointerUp}
			onkeydown={onKeydown}
		>
			{#each PRINTS as at, index (at)}
				<span
					class="print"
					class:left={index % 2 === 1}
					class:walked={progress >= at - 0.003}
					style:top={`${at * 100}%`}
				></span>
			{/each}

			<div class="playhead" class:up={direction === 'up'} style:top={`${progress * 100}%`}>
				<span class="playhead-ring"></span>
				<span class="playhead-paw"><PawPrint class="size-3.5" /></span>
			</div>
		</div>

		<div class="absolute inset-y-0 right-9 left-0">
			{#each marks as mark (mark.id)}
				<a
					href={`#${mark.id}`}
					class="trail-mark"
					class:on={active === mark.id}
					style:top={`${mark.at * 100}%`}
					onclick={(event) => jump(event, mark.id)}
				>
					<span class="mark-paw"><PawPrint class="size-3" /></span>
					<span class="mark-text">{t(mark.labelKey)}</span>
				</a>
			{/each}
		</div>
	</div>
</nav>

<div class="mobile-trail" class:up={direction === 'up'} aria-hidden="true">
	<div class="mobile-track">
		{#each PRINTS as at, index (at)}
			<span
				class="m-print"
				class:alt={index % 2 === 1}
				class:walked={progress >= at - 0.003}
				style:left={`${at * 100}%`}
			></span>
		{/each}

		<span class="m-playhead" style:left={`${progress * 100}%`}>
			<span class="m-ring"></span>
			<span class="m-paw"><PawPrint class="size-3" /></span>
		</span>
	</div>
</div>

<style>
	.trail-label {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-family: var(--font-ui-mono);
		font-size: 0.5625rem;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--muted-foreground);
	}

	.trail-readout {
		font-family: var(--font-ui-mono);
		font-size: 0.625rem;
		font-variant-numeric: tabular-nums;
		color: var(--foreground);
	}

	.trail-prints {
		position: absolute;
		inset-block: 0;
		right: 0;
		width: 1.75rem;
		cursor: grab;
		touch-action: none;
		user-select: none;
		outline: none;
	}

	.trail-prints.dragging {
		cursor: grabbing;
	}

	.trail-prints:focus-visible {
		outline: 1px solid var(--ring);
		outline-offset: 4px;
		border-radius: 9999px;
	}

	.print {
		--paw-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='-46 -48 92 92'%3E%3Cg fill='%23fff'%3E%3Cellipse cx='0' cy='10' rx='30' ry='27'/%3E%3Ccircle cx='-20' cy='-18' r='13'/%3E%3Ccircle cx='0' cy='-26' r='13.5'/%3E%3Ccircle cx='20' cy='-18' r='13'/%3E%3C/g%3E%3C/svg%3E");
		--turn: 180deg;
		position: absolute;
		right: 2px;
		width: 0.7rem;
		height: 0.7rem;
		background: currentColor;
		color: var(--foreground);
		opacity: 0.09;
		transform: translateY(-50%) rotate(calc(var(--turn) + 10deg));
		-webkit-mask: var(--paw-mask) center / contain no-repeat;
		mask: var(--paw-mask) center / contain no-repeat;
		transition:
			opacity 0.45s ease,
			transform 0.45s ease;
	}

	.print.left {
		right: 13px;
		transform: translateY(-50%) rotate(calc(var(--turn) - 10deg));
	}

	.print.walked {
		--turn: 0deg;
		opacity: 0.38;
	}

	.playhead {
		position: absolute;
		right: 5px;
		display: grid;
		place-items: center;
		color: var(--primary);
		transform: translateY(-50%);
	}

	.playhead-ring {
		position: absolute;
		inset: -0.3rem;
		border: 1px solid color-mix(in oklab, var(--primary) 60%, transparent);
		border-radius: 9999px;
		animation: trail-ping 2.4s ease-out infinite;
	}

	.playhead-paw {
		display: inline-flex;
		transform: rotate(180deg);
		transition: transform 0.45s ease;
	}

	.playhead.up .playhead-paw {
		transform: rotate(0deg);
	}

	@keyframes trail-ping {
		0% {
			transform: scale(0.6);
			opacity: 0.7;
		}
		70%,
		100% {
			transform: scale(1.3);
			opacity: 0;
		}
	}

	.trail-mark {
		position: absolute;
		right: 0;
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.25rem;
		width: 100%;
		padding: 0.125rem 0;
		color: var(--muted-foreground);
		text-align: right;
		text-decoration: none;
		opacity: 0.45;
		transform: translateY(-50%);
		transition:
			color 0.25s ease,
			opacity 0.25s ease,
			transform 0.25s ease;
	}

	.trail-mark:hover,
	.trail-mark.on {
		color: var(--foreground);
		opacity: 1;
	}

	.trail-mark.on {
		transform: translateY(-50%) translateX(-3px);
	}

	.mark-paw {
		display: inline-flex;
		flex: none;
		color: var(--primary);
		opacity: 0.3;
		transition: opacity 0.25s ease;
	}

	.trail-mark.on .mark-paw {
		opacity: 1;
	}

	.mark-text {
		font-family: var(--font-ui-mono);
		font-size: 0.5625rem;
		line-height: 1.2;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		white-space: nowrap;
	}

	.mobile-trail {
		--turn: 90deg;
		position: fixed;
		inset-inline: 0;
		top: 3.5625rem;
		z-index: 50;
		height: 1.35rem;
		pointer-events: none;
		background: color-mix(in oklab, var(--background) 80%, transparent);
		-webkit-backdrop-filter: blur(10px);
		backdrop-filter: blur(10px);
		border-bottom: 1px solid color-mix(in oklab, var(--border) 55%, transparent);
	}

	.mobile-trail.up {
		--turn: -90deg;
	}

	.mobile-track {
		position: absolute;
		inset-inline: 1rem;
		top: 50%;
		height: 1px;
		background: color-mix(in oklab, var(--border) 65%, transparent);
	}

	.m-print {
		--paw-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='-46 -48 92 92'%3E%3Cg fill='%23fff'%3E%3Cellipse cx='0' cy='10' rx='30' ry='27'/%3E%3Ccircle cx='-20' cy='-18' r='13'/%3E%3Ccircle cx='0' cy='-26' r='13.5'/%3E%3Ccircle cx='20' cy='-18' r='13'/%3E%3C/g%3E%3C/svg%3E");
		position: absolute;
		top: 50%;
		width: 0.5rem;
		height: 0.5rem;
		background: currentColor;
		color: var(--foreground);
		opacity: 0.16;
		transform: translate(-50%, -50%) rotate(calc(var(--turn) + 6deg));
		-webkit-mask: var(--paw-mask) center / contain no-repeat;
		mask: var(--paw-mask) center / contain no-repeat;
		transition:
			opacity 0.45s ease,
			transform 0.45s ease;
	}

	.m-print.alt {
		transform: translate(-50%, -50%) rotate(calc(var(--turn) - 6deg));
	}

	.m-print.walked {
		opacity: 0.55;
	}

	.m-playhead {
		position: absolute;
		top: 50%;
		display: grid;
		place-items: center;
		color: var(--primary);
		transform: translate(-50%, -50%);
	}

	.m-ring {
		position: absolute;
		inset: -0.28rem;
		border: 1px solid color-mix(in oklab, var(--primary) 60%, transparent);
		border-radius: 9999px;
		animation: trail-ping 2.4s ease-out infinite;
	}

	.m-paw {
		display: inline-flex;
		transform: rotate(var(--turn));
		transition: transform 0.45s ease;
	}

	@media (min-width: 1440px) {
		.mobile-trail {
			display: none;
		}

		:global(html[data-landing='true']) {
			scrollbar-width: none;
		}

		:global(html[data-landing='true'])::-webkit-scrollbar {
			display: none;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.playhead-ring,
		.m-ring {
			animation: none;
			opacity: 0.35;
		}

		.trail-mark,
		.print,
		.playhead-paw,
		.m-print,
		.m-paw {
			transition: none;
		}
	}

	:global(html[data-reduce-motion='true']) .playhead-ring,
	:global(html[data-reduce-motion='true']) .m-ring {
		animation: none;
		opacity: 0.35;
	}

	:global(html[data-reduce-motion='true']) .trail-mark,
	:global(html[data-reduce-motion='true']) .print,
	:global(html[data-reduce-motion='true']) .playhead-paw,
	:global(html[data-reduce-motion='true']) .m-print,
	:global(html[data-reduce-motion='true']) .m-paw {
		transition: none;
	}
</style>
