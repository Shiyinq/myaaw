<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		section: 'context' | 'server' | 'status';
	}

	let { section }: Props = $props();
	const { t } = useTranslation();
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed">
	{#if section === 'context'}
		<div aria-hidden="true" class="space-y-2">
			{#each [0, 1, 2] as row (row)}
				<div class="flex items-center gap-3 rounded-xl border p-3">
					<div class="size-9 shrink-0 rounded-lg bg-muted"></div>
					<div class="min-w-0 flex-1 space-y-2">
						<div class="h-3 w-20 max-w-full rounded bg-muted"></div>
						<div class="h-2.5 w-32 max-w-full rounded bg-muted"></div>
					</div>
					<div class="size-3 shrink-0 rounded bg-muted"></div>
				</div>
			{/each}
		</div>
	{:else if section === 'server'}
		<div aria-hidden="true" class="rounded-xl border p-3">
			<div class="flex h-5 items-center gap-2">
				<div class="size-2 rounded-full bg-muted"></div>
				<div class="h-3 w-24 rounded bg-muted"></div>
			</div>
			<div class="mt-1 h-4 w-3/4 rounded bg-muted"></div>
			<div class="mt-3 h-8 rounded-md bg-muted"></div>
		</div>
	{:else}
		<div aria-hidden="true" class="flex h-4 items-center gap-1.5">
			<div class="size-1.5 rounded-full bg-muted"></div>
			<div class="h-2.5 w-16 rounded bg-muted"></div>
		</div>
	{/if}
</div>
