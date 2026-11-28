<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	let { variant }: { variant: 'files' | 'git' | 'chats' | 'projects' } = $props();
	const { t } = useTranslation();
	const block = 'rounded bg-muted animate-pulse motion-reduce:animate-none';
</script>

<div role="status" aria-label={t('common.loading')} class="skeleton-group-delayed">
	{#if variant === 'files'}
		<div aria-hidden="true" class="space-y-0.5">
			{#each [0, 1, 1, 2, 0, 0] as depth, row (row)}
				<div class="flex h-6 items-center gap-1.5" style:padding-left={`${6 + depth * 12}px`}>
					<div class={[block, 'size-3.5 shrink-0']}></div>
					<div class={[block, 'h-2.5 min-w-0', row % 2 ? 'w-24' : 'w-16']}></div>
				</div>
			{/each}
		</div>
	{:else if variant === 'git'}
		<div aria-hidden="true">
			<div class="border-b bg-muted/25 px-3 py-2">
				<div class={[block, 'h-3 w-20']}></div>
			</div>
			<div class="space-y-2 px-2 py-2">
				<div class={[block, 'h-14 w-full']}></div>
				<div class={[block, 'h-7 w-full']}></div>
				{#each [0, 1, 2] as row (row)}
					<div class="flex h-5 items-center gap-2 px-1">
						<div class={[block, 'size-3 shrink-0']}></div>
						<div class={[block, 'h-2.5 min-w-0', row % 2 ? 'w-28' : 'w-20']}></div>
						<div class={[block, 'ml-auto size-3 shrink-0']}></div>
					</div>
				{/each}
			</div>
		</div>
	{:else if variant === 'chats'}
		<div aria-hidden="true" class="space-y-1 px-2 py-1">
			<div class={[block, 'mb-2 h-2 w-12']}></div>
			{#each [0, 1, 2] as row (row)}
				<div class="flex h-8 items-center gap-2">
					<div class={[block, 'size-3.5 shrink-0']}></div>
					<div class={[block, 'h-3 min-w-0', row % 2 ? 'w-24' : 'w-36']}></div>
					<div class={[block, 'ml-auto h-2 w-7 shrink-0']}></div>
				</div>
			{/each}
		</div>
	{:else}
		<div aria-hidden="true" class="space-y-2 pt-1">
			{#each [0, 1, 2] as row (row)}
				<div class="flex items-center gap-2 rounded-lg border px-2 py-2">
					<div class={[block, 'size-4 shrink-0']}></div>
					<div class="min-w-0 flex-1 space-y-1.5">
						<div class={[block, 'h-3 max-w-full', row % 2 ? 'w-24' : 'w-32']}></div>
						<div class={[block, 'h-2 w-40 max-w-full']}></div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
