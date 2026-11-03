<script lang="ts">
	import { Toaster as Sonner, type ToasterProps as SonnerProps } from 'svelte-sonner';
	import { mode } from 'mode-watcher';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import Bloub from '$lib/components/common/Bloub.svelte';

	let { ...restProps }: SonnerProps = $props();
</script>

<Sonner
	theme={mode.current}
	class="toaster group"
	style="--normal-bg: var(--color-popover); --normal-text: var(--color-popover-foreground); --normal-border: var(--color-border);"
	{...restProps}
>
	{#snippet loadingIcon()}
		<Loader2Icon class="size-4 animate-spin" />
	{/snippet}
	{#snippet successIcon()}
		<Bloub mood="heureux" size={16} />
	{/snippet}
	{#snippet errorIcon()}
		<Bloub mood="triste" size={16} />
	{/snippet}
	{#snippet infoIcon()}
		<Bloub mood="curieux" size={16} />
	{/snippet}
	{#snippet warningIcon()}
		<Bloub mood="mefiant" size={16} />
	{/snippet}
</Sonner>
