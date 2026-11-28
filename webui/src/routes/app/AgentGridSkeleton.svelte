<script lang="ts">
	let { label }: { label: string } = $props();
</script>

<div
	role="status"
	aria-label={label}
	class="skeleton-group-delayed grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
>
	{#each [0, 1, 2, 3] as card (card)}
		<div aria-hidden="true" class="flex flex-col items-center gap-3 rounded-xl border bg-card p-5">
			<div class="size-10 animate-pulse rounded-full bg-muted motion-reduce:animate-none"></div>
			<div class="h-3.5 w-24 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
			<div class="h-5 w-16 animate-pulse rounded-full bg-muted motion-reduce:animate-none"></div>
		</div>
	{/each}
</div>
