<script lang="ts">
	import type { ExtraSummaryBlockData } from '$lib/types';

	interface Props {
		data: ExtraSummaryBlockData;
	}

	let { data }: Props = $props();

	function toneClass(tone?: string): string {
		switch ((tone || 'default').toLowerCase()) {
			case 'positive':
				return 'text-emerald-600 dark:text-emerald-400 font-semibold';
			case 'warning':
				return 'text-amber-600 dark:text-amber-400 font-semibold';
			case 'danger':
				return 'text-destructive font-semibold';
			default:
				return 'text-foreground font-medium';
		}
	}
</script>

<div class="w-full max-w-lg overflow-hidden rounded-xl border bg-card">
	{#if data.title || data.subtitle}
		<div class="border-b px-3.5 py-2.5">
			{#if data.title}
				<p class="text-sm font-semibold leading-snug text-foreground">{data.title}</p>
			{/if}
			{#if data.subtitle}
				<p class="mt-0.5 text-xs text-muted-foreground">{data.subtitle}</p>
			{/if}
		</div>
	{/if}
	<dl class="divide-y divide-border/70">
		{#each data.items ?? [] as item, idx (`${item.label}-${idx}`)}
			<div class="flex items-baseline justify-between gap-4 px-3.5 py-2 text-xs">
				<dt class="min-w-0 text-muted-foreground">{item.label}</dt>
				<dd class={['min-w-0 text-right wrap-break-word', toneClass(item.tone)]}>
					{item.value}
				</dd>
			</div>
		{/each}
	</dl>
</div>
