<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed mx-auto w-full max-w-3xl space-y-5 p-4"
>
	<div aria-hidden="true" class="h-5 w-1/3 rounded bg-muted"></div>
	{#each [0, 1] as paragraph (paragraph)}
		<div aria-hidden="true" class="space-y-2">
			<div class="h-3 w-full rounded bg-muted"></div>
			<div class="h-3 w-11/12 rounded bg-muted"></div>
			<div class="h-3 w-2/3 rounded bg-muted"></div>
		</div>
	{/each}
</div>
