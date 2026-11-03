<script lang="ts">
	import * as Empty from '$lib/components/ui/empty';
	import { cn } from '$lib/utils';
	import type { LucideIcon } from '$lib/types/icons';

	interface Props {
		title: string;
		description: string;
		icon?: LucideIcon;
		class?: string;
		children?: any;
	}

	let { title, description, icon: IconComponent, class: className, children }: Props = $props();
</script>

<Empty.Root
	class={cn(
		'border-2 border-dashed bg-muted/5 min-h-87.5 animate-in fade-in zoom-in-95 duration-500',
		className
	)}
>
	<Empty.Header>
		{#if IconComponent}
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
