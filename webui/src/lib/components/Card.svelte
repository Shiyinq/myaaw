<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		children?: Snippet;
		[key: string]: any;
	}

	// svelte-ignore custom_element_props_identifier
	let { children, ...rest }: Props = $props();
</script>

<div class="card glass-panel {rest.class || ''}" {...rest}>
	{@render children?.()}
</div>

<style>
	.card {
		padding: 2rem;
		width: 100%;
		margin: 0 auto;
	}

	@media (min-width: 640px) {
		.card {
			padding: 3rem;
		}
	}
</style>
