<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { formatChatTimestamp } from '$lib/utils/time';
	import { cn } from '$lib/utils';

	interface Props {
		/** UTC timestamp from the agent, or an ISO string recorded in the tab. */
		at?: string;
		class?: string;
	}

	let { at, class: className = '' }: Props = $props();
	const { t, locale } = useTranslation();

	const label = $derived(formatChatTimestamp(at, locale.value, t('myaaw.chat.yesterday')));
</script>

{#if label}
	<time datetime={at} class={cn('text-[11px] text-muted-foreground tabular-nums', className)}>
		{label}
	</time>
{/if}
