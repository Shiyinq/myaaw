<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
</script>

<div class="space-y-1.5" aria-hidden="true">
	{#each { length: 6 } as _, index (index)}
		<div class="flex items-center gap-2.5 rounded-lg px-2 py-1.5">
			<Skeleton class="h-10 w-10 shrink-0 rounded-md" />
			<div class="min-w-0 flex-1 space-y-1.5">
				<Skeleton class="h-2.5 w-2/3 rounded-full" />
				<Skeleton class="h-2 w-1/3 rounded-full" />
			</div>
		</div>
	{/each}
</div>
