<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	let { rows = 4 }: { rows?: number } = $props();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed space-y-3 px-2 py-2"
>
	{#each Array.from({ length: rows }, (_, index) => index) as row (row)}
		<div aria-hidden="true" class="space-y-2 py-1">
			<div class="h-3.5 w-4/5 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
			<div class="h-2.5 w-3/5 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
		</div>
	{/each}
</div>
