<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
</script>

{#each Array.from({ length: 4 }) as _, idx (idx)}
	<Skeleton class="h-28 w-full rounded-xl" />
{/each}
