<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed flex flex-col gap-3 md:flex-row md:items-center md:justify-between"
>
	<div aria-hidden="true" class="flex gap-2 overflow-hidden">
		{#each [0, 1, 2, 3] as pill (pill)}
			<div
				class="h-8 w-20 shrink-0 animate-pulse rounded-full bg-muted motion-reduce:animate-none"
			></div>
		{/each}
	</div>
	<div
		aria-hidden="true"
		class="h-9 w-full animate-pulse rounded-xl bg-muted motion-reduce:animate-none md:w-64"
	></div>
</div>
