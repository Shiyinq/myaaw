<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	let { compact }: { compact: boolean } = $props();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed flex h-9 items-center gap-3 rounded-md border border-input px-2"
>
	<div
		aria-hidden="true"
		class="size-5 shrink-0 animate-pulse rounded-full bg-muted motion-reduce:animate-none"
	></div>
	{#if !compact}
		<div
			aria-hidden="true"
			class="h-3 w-24 animate-pulse rounded bg-muted motion-reduce:animate-none"
		></div>
	{/if}
</div>
