<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { CapabilityEntry } from '$lib/types';

	interface Props {
		items: CapabilityEntry[];
		/** `name -> selected`, so the page owns the state. */
		selection: Record<string, boolean>;
		emptyLabel: string;
	}

	let { items, selection, emptyLabel }: Props = $props();
	const { t } = useTranslation();

	let selectedCount = $derived(items.filter((item) => selection[item.name]).length);

	function setAll(value: boolean) {
		for (const item of items) selection[item.name] = value;
	}
</script>

<div class="space-y-2">
	<div class="flex items-center justify-between gap-2">
		<span class="text-[11px] font-medium text-muted-foreground">
			{selectedCount}/{items.length}
		</span>
		<div class="flex items-center gap-1">
			<Button variant="ghost" size="xs" onclick={() => setAll(true)}>
				{t('myaaw.onboard.select_all')}
			</Button>
			<Button variant="ghost" size="xs" onclick={() => setAll(false)}>
				{t('myaaw.onboard.clear_all')}
			</Button>
		</div>
	</div>

	{#if items.length === 0}
		<p class="text-xs text-muted-foreground">{emptyLabel}</p>
	{:else}
		<div class="max-h-72 space-y-0.5 overflow-y-auto pr-1">
			{#each items as item (item.name)}
				<label
					class="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-muted/60"
				>
					<Checkbox
						class="mt-0.5"
						checked={!!selection[item.name]}
						onCheckedChange={(value) => (selection[item.name] = value === true)}
					/>
					<span class="min-w-0">
						<span class="block font-mono text-xs font-medium">{item.name}</span>
						{#if item.description}
							<span class="block text-[11px] leading-snug text-muted-foreground">
								{item.description}
							</span>
						{/if}
					</span>
				</label>
			{/each}
		</div>
	{/if}
</div>
