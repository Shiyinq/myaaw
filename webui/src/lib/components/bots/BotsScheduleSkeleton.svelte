<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed space-y-1">
	{#each [0, 1] as row (row)}
		<div aria-hidden="true" class="flex items-center gap-2 rounded-lg border px-2 py-1.5">
			<div class="size-7 shrink-0 rounded-md bg-muted"></div>
			<div class="min-w-0 flex-1 space-y-1">
				<div class="h-3 w-24 max-w-full rounded bg-muted"></div>
				<div class="h-2.5 w-16 max-w-full rounded bg-muted"></div>
			</div>
		</div>
	{/each}
</div>
