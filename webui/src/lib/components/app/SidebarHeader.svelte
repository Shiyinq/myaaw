<script lang="ts">
	let {
		label,
		count = 0
	}: {
		label: string;
		count?: number;
	} = $props();
</script>

<div class="flex items-center justify-between gap-2">
	<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</p>
	{#if count > 0}
		<span class="text-[10px] tabular-nums text-muted-foreground">{count}</span>
	{/if}
</div>
