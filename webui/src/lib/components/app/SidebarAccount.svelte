<script lang="ts">
	import AccountMenu from '$lib/components/app/AccountMenu.svelte';

	interface Props {
		/** Icon-only rail: the profile shrinks to its initials, the menu opens right. */
		compact?: boolean;
	}

	let { compact = false }: Props = $props();
</script>

<div class={compact ? 'shrink-0 border-t p-2' : 'shrink-0 border-t p-3'}>
	{#if compact}
		<div class="flex justify-center"><AccountMenu compact align="start" /></div>
	{:else}
		<AccountMenu />
	{/if}
</div>
