<script lang="ts">
	import { Check, Copy } from 'lucide-svelte';
	import { cn } from '$lib/utils';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		text: string;
		/** Show the Copy/Copied wording next to the icon. */
		label?: boolean;
		class?: string;
	}

	let { text, label = false, class: className = '' }: Props = $props();

	const { t } = useTranslation();
	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function copy() {
		if (!text) return;
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 1500);
		} catch {
			// Clipboard access can be denied; leave the button as it is.
		}
	}
</script>

<button
	type="button"
	class={cn(
		'inline-flex items-center gap-1 rounded-md text-muted-foreground transition-colors hover:text-foreground',
		label ? 'text-[11px]' : 'p-1',
		className
	)}
	onclick={copy}
	title={copied ? t('myaaw.chat.copied') : t('myaaw.chat.copy')}
	aria-label={copied ? t('myaaw.chat.copied') : t('myaaw.chat.copy')}
>
	{#if copied}
		<Check class="h-3.5 w-3.5" />
	{:else}
		<Copy class="h-3.5 w-3.5" />
	{/if}
	{#if label}
		{copied ? t('myaaw.chat.copied') : t('myaaw.chat.copy')}
	{/if}
</button>
