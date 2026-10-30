<script lang="ts">
	import type { ComponentType } from 'svelte';
	import type { Icon } from 'lucide-svelte';
	import * as Empty from '$lib/components/ui/empty';
	import { cn } from '$lib/utils';

	interface Props {
		title: string;
		description: string;
		icon?: ComponentType<Icon>;
		class?: string;
		children?: any;
	}

	let { title, description, icon: IconComponent, class: className, children }: Props = $props();
</script>

<Empty.Root
	class={cn(
		'border-2 border-dashed bg-muted/5 min-h-[350px] animate-in fade-in zoom-in-95 duration-500',
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
		<Empty.Description class="max-w-[280px] leading-relaxed">
			{description}
		</Empty.Description>
	</Empty.Header>

	{#if children}
		<Empty.Content class="mt-8">
			{@render children()}
		</Empty.Content>
	{/if}
</Empty.Root>
