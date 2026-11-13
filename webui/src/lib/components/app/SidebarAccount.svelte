<script lang="ts">
	import AccountMenu from '$lib/components/app/AccountMenu.svelte';

	interface Props {
		/** Icon-only rail: the profile shrinks to its initials, the menu opens right. */
		compact?: boolean;
	}

	let { compact = false }: Props = $props();
</script>

<div
	class="relative grid w-full min-w-0 shrink-0 grid-cols-1 overflow-hidden border-t transition-[height] duration-200 {compact
		? 'h-14'
		: 'h-16'}"
>
	<!-- One cell for both states keeps the block height stable while the expanded
	     row and the initials crossfade, and the height animates with the sidebar. -->
	<div
		class="absolute inset-0 flex min-w-0 items-center px-3 transition-opacity duration-200 {compact
			? 'pointer-events-none opacity-0 delay-0'
			: 'opacity-100 delay-200'}"
		data-account-expanded
		inert={compact}
		aria-hidden={compact}
	>
		<AccountMenu />
	</div>
	<div
		class="absolute inset-0 flex min-w-0 items-center px-3 transition-opacity duration-200 {compact
			? 'opacity-100 delay-200'
			: 'pointer-events-none opacity-0 delay-0'}"
		data-account-compact
		inert={!compact}
		aria-hidden={!compact}
	>
		<AccountMenu compact align="start" />
	</div>
</div>
