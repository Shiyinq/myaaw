<script lang="ts">
	import { Search } from 'lucide-svelte';
	import { Input } from '$lib/components/ui/input';

	let {
		placeholder,
		value = $bindable(''),
		inputRef = $bindable(null)
	}: {
		placeholder: string;
		value?: string;
		inputRef?: HTMLInputElement | null;
	} = $props();
</script>

<div class="relative">
	<Search
		class="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
	/>
	<Input
		bind:ref={inputRef}
		bind:value
		class="h-7 pl-7 text-xs"
		{placeholder}
		aria-label={placeholder}
		aria-keyshortcuts="Control+Shift+S Meta+Shift+S"
	/>
</div>
