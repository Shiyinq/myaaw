<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed space-y-3 px-1 py-1"
>
	{#each [0, 1] as section (section)}
		<div class="space-y-2" aria-hidden="true">
			<div class="h-2.5 w-20 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
			{#each [0, 1, 2] as row (row)}
				<div class="flex items-center gap-2 py-1">
					<div
						class="size-10 shrink-0 animate-pulse rounded-xl bg-muted motion-reduce:animate-none"
					></div>
					<div class="flex min-w-0 flex-1 flex-col gap-2">
						<div class="h-3 w-1/2 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
						<div
							class="h-2.5 w-4/5 animate-pulse rounded bg-muted motion-reduce:animate-none"
						></div>
					</div>
				</div>
			{/each}
		</div>
	{/each}
</div>
