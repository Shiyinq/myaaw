<script lang="ts">
	import { Toaster as Sonner, type ToasterProps as SonnerProps } from 'svelte-sonner';
	import { mode } from 'mode-watcher';

	let { ...restProps }: SonnerProps = $props();
</script>

<!--
	Toasts say what happened in words and colour, so they carry no icon: a face
	there competed with the message and never matched the agent's own colour.

	sonner still renders an icon column and styles it with
	`[data-sonner-toast] [data-icon]`, which outranks a utility class, so `app.css`
	hides it once for every theme instead. See the `.toaster` rule there.
-->
<Sonner
	theme={mode.current}
	class="toaster group"
	style="--normal-bg: var(--color-popover); --normal-text: var(--color-popover-foreground); --normal-border: var(--color-border);"
	{...restProps}
/>
