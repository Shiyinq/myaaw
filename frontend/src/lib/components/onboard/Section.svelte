<script lang="ts">
	import { ChevronDown } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils.js';

	interface Props {
		title: string;
		/** One-line state shown next to the title, e.g. `Telegram · polling`. */
		summary?: string;
		description?: string;
		open?: boolean;
		children: Snippet;
	}

	let { title, summary, description, open = $bindable(false), children }: Props = $props();
</script>

<div class="rounded-xl border bg-card">
	<button
		type="button"
		class="flex w-full items-center gap-3 p-4 text-left"
		aria-expanded={open}
		onclick={() => (open = !open)}
	>
		<div class="min-w-0 flex-1">
			<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
				<span class="text-sm font-semibold">{title}</span>
				{#if summary}
					<span class="truncate text-xs text-muted-foreground">{summary}</span>
				{/if}
			</div>
			{#if description}
				<p class="mt-0.5 text-xs text-muted-foreground">{description}</p>
			{/if}
		</div>
		<ChevronDown
			class={cn(
				'h-4 w-4 shrink-0 text-muted-foreground transition-transform',
				open && 'rotate-180'
			)}
		/>
	</button>
	{#if open}
		<div class="space-y-4 border-t p-4">
			{@render children()}
		</div>
	{/if}
</div>
