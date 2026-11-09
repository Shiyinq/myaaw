<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import {
		BLOUB_DEFAULT_COLOR,
		CYCLE_FALLBACK,
		cycleSrc,
		moodSrc,
		type BloubColor,
		type BloubCycle,
		type BloubMood
	} from '$lib/bloub';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { cn } from '$lib/utils';

	interface Props {
		/** Static emotion. Ignored when `cycle` is set and motion is allowed. */
		mood?: BloubMood;
		/** Looping GIF. Freezes to `mood`/`CYCLE_FALLBACK` under reduced motion. */
		cycle?: BloubCycle;
		/** Palette key. Agents carry their own; everything else stays ink. */
		color?: BloubColor;
		size?: number;
		/** Accessible text. Omit for decorative instances (the default). */
		label?: string;
		/** Rendered instead of the image if the asset fails to load. */
		fallback?: Snippet;
		/** Set for above-the-fold instances such as the splash. */
		eager?: boolean;
		onmouseenter?: (event: MouseEvent) => void;
		onmouseleave?: (event: MouseEvent) => void;
		/**
		 * Pull the next element up over the art's transparent bottom padding (~24%
		 * of an emotion SVG, ~30% of a cycle GIF). Use when the icon sits above
		 * text; leave off for inline icons such as the sidebar brand or toasts.
		 */
		tight?: boolean;
		/** Cross-fade when the source changes instead of cutting straight to it. */
		fade?: boolean;
		class?: string;
	}

	let {
		mood = 'neutre',
		cycle,
		color = BLOUB_DEFAULT_COLOR,
		size = 64,
		label,
		fallback,
		eager = false,
		onmouseenter,
		onmouseleave,
		tight = false,
		fade = false,
		class: className = ''
	}: Props = $props();

	const reduced = $derived(preferences.value.reduceMotion);
	const usingCycle = $derived(Boolean(cycle) && !reduced);
	// The cat fills ~54% of a GIF frame but ~68% of an emotion SVG, so switching
	// from a cycle to a face would visibly change its size. Scale the GIF up to
	// match; a transform leaves the layout box (and the `tight` offset) untouched.
	const CYCLE_SCALE = 1.27;
	const artScale = $derived(usingCycle ? CYCLE_SCALE : 1);
	const src = $derived.by(() => {
		// A cycle is motion; reduced motion gets the closest static emotion.
		if (usingCycle && cycle) return cycleSrc(cycle, color);
		return moodSrc(cycle ? CYCLE_FALLBACK[cycle] : mood, color);
	});
	const tightMargin = $derived(tight ? `${-Math.round(size * 0.24)}px` : undefined);
	const isCycleSrc = (value: string) => value.endsWith('-cycle.gif');

	// Track which source failed, so a new source retries without an effect (and
	// without a one-frame fallback flash).
	let failedSrc = $state('');
	const failed = $derived(failedSrc === src);

	// The outgoing layer stays fully opaque underneath while the incoming layer
	// fades in on top, so the shared body never fades. Once the new layer is
	// opaque the old one is dropped: its parts outside the body (the notification
	// badge) must not linger behind the new face.
	const FADE_MS = 120;
	let layers = $state<[string, string]>([untrack(() => src), '']);
	let top = $state(0);
	let reveal = $state(true);
	let settled = $state(true);
	let lastSrc = untrack(() => src);
	$effect(() => {
		const next = src;
		if (next === lastSrc) return;
		lastSrc = next;
		untrack(() => {
			const slot = top === 0 ? 1 : 0;
			layers = slot === 0 ? [next, layers[1]] : [layers[0], next];
			top = slot;
			reveal = false;
			settled = false;
		});
		let inner = 0;
		let settle: ReturnType<typeof setTimeout> | undefined;
		// Two frames: paint the new layer at opacity 0, then transition it in.
		const outer = requestAnimationFrame(() => {
			inner = requestAnimationFrame(() => {
				reveal = true;
				// Drop the old layer only after the new one is fully opaque.
				settle = setTimeout(() => (settled = true), FADE_MS + 40);
			});
		});
		return () => {
			cancelAnimationFrame(outer);
			cancelAnimationFrame(inner);
			if (settle !== undefined) clearTimeout(settle);
		};
	});
</script>

{#if failed}
	<span
		class={cn('inline-flex shrink-0 items-center justify-center select-none', className)}
		style:width={`${size}px`}
		style:height={`${size}px`}
		style:margin-bottom={tightMargin}
		aria-hidden="true"
		{onmouseenter}
		{onmouseleave}
	>
		{@render fallback?.()}
	</span>
{:else if fade}
	<span
		class={cn('relative inline-block shrink-0 select-none', className)}
		style:width={`${size}px`}
		style:height={`${size}px`}
		style:margin-bottom={tightMargin}
		role="presentation"
		aria-hidden="true"
		{onmouseenter}
		{onmouseleave}
	>
		{#each layers as layerSrc, index (index)}
			{#if layerSrc}
				<img
					src={layerSrc}
					width={size}
					height={size}
					alt=""
					aria-hidden="true"
					loading={eager ? 'eager' : 'lazy'}
					decoding="async"
					draggable="false"
					class="absolute inset-0 h-full w-full object-contain"
					style:opacity={index === top ? (reveal ? 1 : 0) : settled ? 0 : 1}
					style:z-index={index === top ? 1 : 0}
					style:transition={`opacity ${index === top ? FADE_MS : 80}ms ease-out`}
					style:transform={isCycleSrc(layerSrc) ? `scale(${CYCLE_SCALE})` : undefined}
					onerror={() => (failedSrc = layerSrc)}
				/>
			{/if}
		{/each}
	</span>
{:else}
	<img
		{src}
		width={size}
		height={size}
		alt={label ?? ''}
		aria-hidden={label ? undefined : 'true'}
		loading={eager ? 'eager' : 'lazy'}
		decoding="async"
		draggable="false"
		class={cn('shrink-0 select-none', className)}
		style:margin-bottom={tightMargin}
		style:transform={artScale === 1 ? undefined : `scale(${artScale})`}
		onerror={() => (failedSrc = src)}
		{onmouseenter}
		{onmouseleave}
	/>
{/if}
