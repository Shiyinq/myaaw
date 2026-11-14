<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';

	interface Props {
		rows?: number;
		class?: string;
	}

	let { rows = 4, class: className = 'h-9' }: Props = $props();
</script>

<div class="space-y-1.5 px-1 py-1">
	{#each Array.from({ length: rows }, (_, index) => index) as index (index)}
		<Skeleton class={`w-full ${className}`} />
	{/each}
</div>
