<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed space-y-5">
	<div
		aria-hidden="true"
		class="flex flex-col gap-5 rounded-xl border bg-card p-6 sm:flex-row sm:items-center"
	>
		<div
			class="size-16 shrink-0 animate-pulse rounded-2xl bg-muted motion-reduce:animate-none"
		></div>
		<div class="flex flex-1 flex-col gap-3">
			<div
				class="h-5 w-40 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
			></div>
			<div
				class="h-3 w-72 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
			></div>
		</div>
		<div class="hidden gap-2 sm:flex">
			<div class="h-9 w-20 animate-pulse rounded-md bg-muted motion-reduce:animate-none"></div>
			<div class="h-9 w-24 animate-pulse rounded-md bg-muted motion-reduce:animate-none"></div>
		</div>
	</div>
	<div aria-hidden="true" class="grid gap-3 sm:grid-cols-3">
		{#each [0, 1, 2] as card (card)}
			<div class="space-y-3 rounded-xl border bg-card p-4">
				<div class="h-3 w-20 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
				<div class="h-5 w-28 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
			</div>
		{/each}
	</div>
	<div aria-hidden="true" class="space-y-4 rounded-xl border bg-card p-6">
		<div class="h-4 w-32 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
		<div
			class="h-3 w-52 max-w-full animate-pulse rounded bg-muted motion-reduce:animate-none"
		></div>
		<div class="grid gap-2 sm:grid-cols-2">
			{#each [0, 1, 2, 3] as chat (chat)}
				<div class="flex items-center gap-3 rounded-xl border p-3">
					<div
						class="size-8 shrink-0 animate-pulse rounded-lg bg-muted motion-reduce:animate-none"
					></div>
					<div class="flex flex-1 flex-col gap-2">
						<div class="h-3 w-3/4 animate-pulse rounded bg-muted motion-reduce:animate-none"></div>
						<div
							class="h-2.5 w-1/2 animate-pulse rounded bg-muted motion-reduce:animate-none"
						></div>
					</div>
				</div>
			{/each}
		</div>
	</div>
</div>
