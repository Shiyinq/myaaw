<script lang="ts">
	import { cn } from '$lib/utils';

	/**
	 * The Myaaw mark: a cat head with pointed ears. Drawn as one even-odd path so
	 * the eyes are holes, and cropped tight so the glyph fills whatever size the
	 * caller gives it instead of floating in the drawing box.
	 */
	let { class: className = '' }: { class?: string } = $props();

	const MARK =
		'M240 430 L268 210 C272 190 296 184 312 198 L430 296 C452 290 482 286 512 286 C542 286 572 290 594 296 L712 198 C728 184 752 190 756 210 L784 430 C784 600 700 700 512 700 C324 700 240 600 240 430 Z ' +
		'M402 470 a40 40 0 1 0 80 0 a40 40 0 1 0 -80 0 Z ' +
		'M542 470 a40 40 0 1 0 80 0 a40 40 0 1 0 -80 0 Z';
</script>

<svg viewBox="224 161 576 576" class={cn('fill-current', className)} aria-hidden="true">
	<path fill-rule="evenodd" clip-rule="evenodd" d={MARK} />
</svg>
