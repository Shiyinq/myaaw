<script lang="ts">
	import type { Snippet } from 'svelte';
	import { ChevronDown, Loader2 } from 'lucide-svelte';
	import type { LucideIcon } from '$lib/types/icons';
	import { cn } from '$lib/utils';
	import CopyButton from '$lib/components/common/CopyButton.svelte';

	interface Props {
		title: string;
		summary?: string;
		icon: LucideIcon;
		expanded: boolean;
		ontoggle: () => void;
		running?: boolean;
		error?: boolean;
		/** Text the Copy button places on the clipboard. */
		copyText?: string;
		children: Snippet;
	}

	let {
		title,
		summary = '',
		icon: IconComponent,
		expanded,
		ontoggle,
		running = false,
		error = false,
		copyText = '',
		children
	}: Props = $props();
</script>

<div class="text-xs">
	<button
		type="button"
		class={cn(
			'flex w-full items-center gap-2 rounded-md px-1 py-1 text-left transition-colors hover:bg-muted/60',
			error && 'text-destructive'
		)}
		onclick={ontoggle}
		aria-expanded={expanded}
	>
		<IconComponent
			class={cn('h-3.5 w-3.5 shrink-0', error ? 'text-destructive' : 'text-muted-foreground')}
		/>
		<span class="shrink-0 font-medium">{title}</span>
		{#if summary}
			<span class="text-muted-foreground">·</span>
			<span class="min-w-0 flex-1 truncate text-muted-foreground">{summary}</span>
		{:else}
			<span class="min-w-0 flex-1"></span>
		{/if}
		{#if running}
			<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />
		{/if}
		<ChevronDown
			class={cn(
				'h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform',
				expanded && 'rotate-180'
			)}
		/>
	</button>
	{#if expanded}
		<div class="mt-1 ml-5.5">
			{@render children()}
			{#if copyText}
				<CopyButton text={copyText} label class="mt-1" />
			{/if}
		</div>
	{/if}
</div>
