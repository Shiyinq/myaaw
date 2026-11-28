<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
</script>

{#each Array.from({ length: 3 }) as _, index (index)}
	<Skeleton class="h-12 w-full" />
{/each}
