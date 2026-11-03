<script lang="ts">
	import * as Empty from '$lib/components/ui/empty';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import type { BloubMood } from '$lib/bloub';
	import { cn } from '$lib/utils';
	import type { LucideIcon } from '$lib/types/icons';

	interface Props {
		title: string;
		description: string;
		icon?: LucideIcon;
		/** Replaces the lucide icon with the mascot when set. */
		bloub?: BloubMood;
		class?: string;
		children?: any;
	}

	let {
		title,
		description,
		icon: IconComponent,
		bloub,
		class: className,
		children
	}: Props = $props();
</script>

<Empty.Root
	class={cn(
		'border-2 border-dashed bg-muted/5 min-h-87.5 animate-in fade-in zoom-in-95 duration-500',
		className
	)}
>
	<Empty.Header>
		{#if bloub}
			<Bloub mood={bloub} size={96} tight />
		{:else if IconComponent}
			<Empty.Media variant="icon" class="bg-muted/50 h-20 w-20">
				<IconComponent class="h-10 w-10 text-muted-foreground/70" />
			</Empty.Media>
		{/if}
		<Empty.Title class="text-xl font-bold">{title}</Empty.Title>
		<Empty.Description class="max-w-70 leading-relaxed">
			{description}
		</Empty.Description>
	</Empty.Header>

	{#if children}
		<Empty.Content class="mt-8">
			{@render children()}
		</Empty.Content>
	{/if}
</Empty.Root>
