<script lang="ts">
	let { label }: { label: string } = $props();
</script>

<div role="status" aria-label={label} class="skeleton-group-delayed divide-y rounded-xl border">
	{#each [0, 1, 2] as row (row)}
		<div aria-hidden="true" class="flex items-center justify-between gap-3 p-4">
			<div class="flex min-w-0 flex-1 flex-col gap-2">
				<div
					class="h-3.5 w-48 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
				></div>
				<div
					class="h-3 w-32 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
				></div>
			</div>
			<div
				class="h-8 w-20 shrink-0 animate-pulse rounded-md bg-muted motion-reduce:animate-none"
			></div>
		</div>
	{/each}
</div>
