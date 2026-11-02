<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { formatChatTimestamp } from '$lib/utils/time';
	import { cn } from '$lib/utils';

	interface Props {
		/** UTC timestamp from the agent, or an ISO string recorded in the tab. */
		value?: string;
		class?: string;
	}

	let { value, class: className = '' }: Props = $props();
	const { t, locale } = useTranslation();

	const label = $derived(formatChatTimestamp(value, locale.value, t('myaaw.chat.yesterday')));
</script>

{#if label}
	<time datetime={value} class={cn('text-[11px] text-muted-foreground tabular-nums', className)}>
		{label}
	</time>
{/if}
