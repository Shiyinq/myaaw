<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { LucideIcon } from '$lib/types/icons';

	let {
		icon: Icon,
		label,
		count = 0,
		title,
		children
	}: {
		icon: LucideIcon;
		label: string;
		count?: number;
		title?: string;
		children?: Snippet;
	} = $props();
</script>

<section class="space-y-1">
	<div class="flex items-center gap-1.5 px-1">
		<Icon class="h-3 w-3 shrink-0 text-muted-foreground" />
		<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground" {title}>
			{label}
		</p>
		<span class="ml-auto text-[10px] tabular-nums text-muted-foreground">{count}</span>
	</div>
	{@render children?.()}
</section>
