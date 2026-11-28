<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';

	let { label }: { label: string } = $props();
</script>

<div class="w-full max-w-3xl space-y-4" aria-live="polite" aria-label={label}>
	<div class="flex justify-end">
		<Skeleton class="h-10 w-1/3 rounded-2xl" />
	</div>
	<div class="flex justify-start">
		<Skeleton class="h-28 w-3/4 rounded-2xl" />
	</div>
	<div class="flex justify-start">
		<Skeleton class="h-16 w-1/2 rounded-2xl" />
	</div>
</div>
