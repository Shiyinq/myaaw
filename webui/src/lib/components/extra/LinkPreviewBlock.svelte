<script lang="ts">
	import { ExternalLink, Globe } from 'lucide-svelte';
	import type { ExtraLinkPreviewBlockData } from '$lib/types';

	interface Props {
		data: ExtraLinkPreviewBlockData;
	}

	let { data }: Props = $props();

	let imageFailed = $state(false);
</script>

<a
	href={data.url}
	target="_blank"
	rel="noopener noreferrer"
	class="group flex w-full max-w-lg overflow-hidden rounded-xl border bg-card transition-colors hover:border-foreground/25 hover:bg-muted/30"
>
	{#if data.image_url && !imageFailed}
		<div class="hidden w-28 shrink-0 bg-muted/40 sm:block">
			<img
				src={data.image_url}
				alt={data.title}
				loading="lazy"
				referrerpolicy="no-referrer"
				onerror={() => (imageFailed = true)}
				class="h-full w-full object-cover"
			/>
		</div>
	{/if}
	<div class="flex min-w-0 flex-1 flex-col justify-center gap-1 px-3.5 py-2.5">
		<div class="flex items-center gap-1.5 text-[11px] text-muted-foreground">
			<Globe class="h-3 w-3 shrink-0" />
			<span class="truncate">{data.domain || data.url}</span>
			<ExternalLink class="ml-auto h-3 w-3 shrink-0 opacity-60 group-hover:opacity-100" />
		</div>
		<p class="line-clamp-2 text-sm font-medium leading-snug text-foreground">
			{data.title}
		</p>
		{#if data.description}
			<p class="line-clamp-2 text-xs leading-relaxed text-muted-foreground">
				{data.description}
			</p>
		{/if}
	</div>
</a>
