<script lang="ts">
	import { cn } from '$lib/utils';

	/**
	 * The Myaaw mark: a cat head with pointed ears, and a paw raised beside its
	 * right cheek. The head is one even-odd path so the eyes are holes, and the
	 * paw is drawn in front of it with a ring of background carved around the
	 * overlap, so it reads as raised instead of growing out of the cheek. The
	 * viewBox is cropped to the pair, so the glyph fills whatever size the caller
	 * gives it.
	 */
	let { class: className = '' }: { class?: string } = $props();

	// One mask per instance: two logos on the same page must not share the id.
	const instanceId = $props.id();
	const maskId = `myaaw-paw-${instanceId}`;

	const MARK =
		'M240 430 L268 210 C272 190 296 184 312 198 L430 296 C452 290 482 286 512 286 C542 286 572 290 594 296 L712 198 C728 184 752 190 756 210 L784 430 C784 600 700 700 512 700 C324 700 240 600 240 430 Z ' +
		'M402 470 a40 40 0 1 0 80 0 a40 40 0 1 0 -80 0 Z ' +
		'M542 470 a40 40 0 1 0 80 0 a40 40 0 1 0 -80 0 Z';

	// A palm and three toes, tilted outwards like a wave. It sits between the
	// cheek and the chin, and the stroke-width on the masked copy is the gap the
	// head gives up around it.
	const PAW = 'translate(752 600) rotate(22) scale(2)';
	const PAW_GAP = 22;
</script>

{#snippet paw()}
	<ellipse cx="0" cy="10" rx="30" ry="27" />
	<circle cx="-20" cy="-18" r="13" />
	<circle cx="0" cy="-26" r="13.5" />
	<circle cx="20" cy="-18" r="13" />
{/snippet}

<svg viewBox="226 130 632 632" class={cn('fill-current', className)} aria-hidden="true">
	<defs>
		<mask id={maskId} maskUnits="userSpaceOnUse" x="0" y="0" width="1100" height="1100">
			<rect x="0" y="0" width="1100" height="1100" fill="#fff" />
			<g transform={PAW} fill="#000" stroke="#000" stroke-width={PAW_GAP}>
				{@render paw()}
			</g>
		</mask>
	</defs>
	<path fill-rule="evenodd" clip-rule="evenodd" d={MARK} mask="url(#{maskId})" />
	<g transform={PAW}>
		{@render paw()}
	</g>
</svg>
