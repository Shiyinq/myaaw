<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
</script>

<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
	{#each { length: 10 } as _, idx (idx)}
		<div class="overflow-hidden rounded-2xl bg-card ring-1 ring-(--surface-ring) frost-card">
			<Skeleton class="aspect-4/3 w-full rounded-none" />
			<div class="space-y-2 p-3">
				<Skeleton class="h-3.5 w-3/4 rounded-full" />
				<Skeleton class="h-2.5 w-1/2 rounded-full" />
			</div>
		</div>
	{/each}
</div>
