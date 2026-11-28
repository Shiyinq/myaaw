<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed space-y-5">
	<div
		aria-hidden="true"
		class="flex flex-col items-center gap-3 rounded-xl border bg-card px-6 py-7"
	>
		<div class="size-20 animate-pulse rounded-full bg-muted motion-reduce:animate-none"></div>
		<div class="h-6 w-36 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
		<div
			class="h-3 w-64 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
		></div>
		<div class="mt-2 flex gap-2">
			{#each [0, 1, 2] as button (button)}
				<div class="h-9 w-20 animate-pulse rounded-md bg-muted motion-reduce:animate-none"></div>
			{/each}
		</div>
	</div>
	<div aria-hidden="true" class="grid gap-4 sm:grid-cols-2">
		{#each [0, 1, 2, 3] as card (card)}
			<div class="space-y-3 rounded-xl border bg-card p-6">
				<div class="h-4 w-32 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
				<div class="h-3 w-20 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
				<div class="pt-3 space-y-2">
					<div class="h-3 w-full animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
					<div class="h-3 w-3/4 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
				</div>
			</div>
		{/each}
	</div>
</div>
