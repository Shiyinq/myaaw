<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';

	let { label }: { label: string } = $props();
</script>

<div class="flex flex-col gap-4" aria-live="polite" aria-label={label}>
	<Skeleton class="h-14 w-2/3 rounded-2xl" />
	<Skeleton class="ml-auto h-12 w-1/2 rounded-2xl" />
	<Skeleton class="h-24 w-3/4 rounded-2xl" />
</div>
