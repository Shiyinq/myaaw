<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed space-y-6">
	{#each [0, 1] as section (section)}
		<section class="space-y-3" aria-hidden="true">
			<div class="h-3 w-28 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each [0, 1, 2] as card (card)}
					<div class="space-y-4 rounded-2xl border bg-card/70 p-4">
						<div class="flex items-center gap-3">
							<div
								class="size-11 shrink-0 animate-pulse rounded-xl bg-muted motion-reduce:animate-none"
							></div>
							<div class="flex flex-1 flex-col gap-2">
								<div
									class="h-3.5 w-1/2 animate-pulse rounded bg-muted motion-reduce:animate-none"
								></div>
								<div
									class="h-2.5 w-1/3 animate-pulse rounded bg-muted motion-reduce:animate-none"
								></div>
							</div>
						</div>
						<div class="h-3 w-4/5 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
					</div>
				{/each}
			</div>
		</section>
	{/each}
</div>
