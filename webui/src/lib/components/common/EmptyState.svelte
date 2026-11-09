<script lang="ts">
	import * as Empty from '$lib/components/ui/empty';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import type { BloubColor, BloubMood } from '$lib/bloub';
	import { agentSelection } from '$lib/stores';
	import { cn } from '$lib/utils';
	import type { LucideIcon } from '$lib/types/icons';

	interface Props {
		title: string;
		description: string;
		icon?: LucideIcon;
		/** Replaces the lucide icon with the mascot when set. */
		bloub?: BloubMood;
		/** Overrides the palette; defaults to the selected agent's colour. */
		color?: BloubColor;
		class?: string;
		children?: any;
	}

	let {
		title,
		description,
		icon: IconComponent,
		bloub,
		color,
		class: className,
		children
	}: Props = $props();

	// An empty state is always about the agent in scope, so the mascot wears that
	// agent's colour unless a caller asks for something else.
	const mascotColor = $derived(color ?? agentSelection.color);
</script>

<Empty.Root
	class={cn(
		'border-2 border-dashed bg-muted/5 min-h-87.5 animate-in fade-in zoom-in-95 duration-500',
		className
	)}
>
	<Empty.Header>
		{#if bloub}
			<Bloub mood={bloub} size={96} tight color={mascotColor} />
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
