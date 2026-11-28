<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div
	role="status"
	aria-label={t('common.loading')}
	class="skeleton-group-delayed rounded-3xl border bg-card/70 p-5 sm:p-6"
>
	<div aria-hidden="true" class="flex flex-col gap-4 sm:flex-row sm:items-center sm:gap-4">
		<div
			class="size-12 shrink-0 animate-pulse rounded-2xl bg-muted motion-reduce:animate-none"
		></div>
		<div class="flex flex-1 flex-col gap-2">
			<div
				class="h-4 w-44 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
			></div>
			<div
				class="h-3 w-80 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
			></div>
		</div>
		<div class="hidden gap-2 lg:flex">
			<div class="h-9 w-28 animate-pulse rounded-xl bg-muted motion-reduce:animate-none"></div>
			<div class="h-9 w-32 animate-pulse rounded-xl bg-muted motion-reduce:animate-none"></div>
			<div class="size-9 animate-pulse rounded-xl bg-muted motion-reduce:animate-none"></div>
		</div>
	</div>
</div>
