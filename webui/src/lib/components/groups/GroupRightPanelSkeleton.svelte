<script lang="ts">
	import AttachmentPanelSkeleton from '$lib/components/app/AttachmentPanelSkeleton.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Users, Paperclip, Settings2 } from 'lucide-svelte';

	let { tab }: { tab: 'members' | 'attachments' | 'settings' } = $props();
	const { t } = useTranslation();
	const tabs = [
		{ id: 'members', icon: Users, label: 'myaaw.groups.tab_members' },
		{ id: 'attachments', icon: Paperclip, label: 'myaaw.groups.tab_attachments' },
		{ id: 'settings', icon: Settings2, label: 'myaaw.groups.tab_settings' }
	] as const;
</script>

<div aria-busy="true" aria-label={t('common.loading')}>
	<div class="flex shrink-0 flex-col items-center gap-1.5 px-4 pt-4 pb-3 lg:pt-16">
		<span
			class="grid size-10 place-items-center rounded-full bg-muted/80 text-muted-foreground ring-2 ring-card"
		>
			<Users class="size-5" aria-hidden="true" />
		</span>
		<div class="skeleton-group-delayed mt-1 grid h-5 place-items-center" aria-hidden="true">
			<div class="h-3 w-20 rounded bg-muted"></div>
		</div>
		<div class="skeleton-group-delayed grid h-4 place-items-center" aria-hidden="true">
			<div class="h-2.5 w-28 rounded bg-muted"></div>
		</div>
	</div>
	<div class="mx-3 mb-2 grid grid-cols-3 rounded-lg border p-0.5">
		{#each tabs as entry (entry.id)}
			<span
				class={[
					'flex items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-xs font-medium',
					tab === entry.id ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'
				]}
			>
				<entry.icon class="size-3.5 shrink-0" />
				<span class="truncate">{t(entry.label)}</span>
			</span>
		{/each}
	</div>
	<div class="px-3">
		{#if tab === 'attachments'}
			<AttachmentPanelSkeleton />
		{:else}
			<div class="skeleton-group-delayed space-y-2" aria-hidden="true">
				{#each [0, 1, 2] as row (row)}
					<div class="flex items-center gap-3 rounded-xl border p-3">
						<div class="size-8 rounded-full bg-muted"></div>
						<div class="h-3 w-24 rounded bg-muted"></div>
						<div class="ml-auto h-6 w-12 rounded-md bg-muted"></div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
