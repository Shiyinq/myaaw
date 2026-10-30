<script lang="ts">
	interface Props {
		title: string;
		description?: string;
		children?: import('svelte').Snippet;
	}

	let { title, description, children }: Props = $props();
</script>

<div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
	<div class="space-y-1">
		<h2 class="text-2xl font-bold tracking-tight">{title}</h2>
		{#if description}
			<p class="text-sm text-muted-foreground">{description}</p>
		{/if}
	</div>
	{#if children}
		<div class="flex flex-wrap items-center gap-2">
			{@render children()}
		</div>
	{/if}
</div>
