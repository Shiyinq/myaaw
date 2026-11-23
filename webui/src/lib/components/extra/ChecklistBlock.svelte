<script lang="ts">
	import { Check, CheckCircle2, Loader2 } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ExtraBlock, ExtraChecklistBlockData, InteractionRequest } from '$lib/types';

	interface Props {
		block: ExtraBlock;
		data: ExtraChecklistBlockData;
		disabled?: boolean;
		submitting?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { block, data, disabled = false, submitting = false, onInteract }: Props = $props();

	const { t } = useTranslation();

	let singleUse = $derived(block.single_use !== false);
	let isSubmitted = $derived(block.state?.status === 'submitted');
	let isLocked = $derived((singleUse && isSubmitted) || disabled || submitting);

	let checkedIds = $state<string[]>([]);
	let initializedFor = $state('');

	$effect(() => {
		if (initializedFor !== block.id) {
			initializedFor = block.id;
			checkedIds = (data.items ?? []).filter((it) => it.checked).map((it) => it.id);
		}
	});

	let effectiveCheckedIds = $derived.by<string[]>(() => {
		if (isSubmitted) {
			const payloadValues = block.state?.payload?.values as Record<string, unknown> | undefined;
			if (payloadValues && Array.isArray(payloadValues.checked_ids)) {
				return payloadValues.checked_ids.map(String);
			}
		}
		return checkedIds;
	});

	function toggle(id: string) {
		if (isLocked) return;
		checkedIds = checkedIds.includes(id)
			? checkedIds.filter((item) => item !== id)
			: [...checkedIds, id];
	}

	async function handleSubmit() {
		if (isLocked || !onInteract) return;
		const chosenItems = (data.items ?? []).filter((it) => checkedIds.includes(it.id));
		const checkedLabels = chosenItems.map((it) => it.label);
		const summary =
			checkedLabels.length > 0
				? `${data.title ? `${data.title}: ` : ''}${checkedLabels.join(', ')}`
				: `${data.title ? `${data.title}: ` : ''}${t('myaaw.extra.none_selected')}`;

		await onInteract({
			block_id: block.id,
			kind: 'checklist',
			label: data.title || t('myaaw.chat.interaction_checklist'),
			values: {
				checked_ids: checkedIds,
				checked_labels: checkedLabels
			},
			display_text: summary
		});
	}
</script>

<div class="w-full max-w-lg overflow-hidden rounded-xl border bg-card">
	{#if data.title || data.description || isSubmitted}
		<div class="flex items-start justify-between gap-2 border-b px-3.5 py-2.5">
			<div class="min-w-0">
				{#if data.title}
					<p class="text-sm font-semibold leading-snug text-foreground">{data.title}</p>
				{/if}
				{#if data.description}
					<p class="mt-0.5 text-xs text-muted-foreground">{data.description}</p>
				{/if}
			</div>
			{#if isSubmitted}
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-medium text-emerald-600 dark:text-emerald-400"
				>
					<CheckCircle2 class="h-3 w-3" />
					{t('myaaw.extra.submitted')}
				</span>
			{/if}
		</div>
	{/if}

	<div class="divide-y divide-border/60">
		{#each data.items ?? [] as item (item.id)}
			{@const isChecked = effectiveCheckedIds.includes(item.id)}
			<button
				type="button"
				disabled={isLocked}
				onclick={() => toggle(item.id)}
				class={[
					'flex w-full items-start gap-2.5 px-3.5 py-2 text-left transition-colors',
					!isLocked && 'hover:bg-muted/40',
					isLocked && 'cursor-default'
				]}
			>
				<span
					class={[
						'mt-0.5 grid size-4 shrink-0 place-items-center rounded border transition-colors',
						isChecked
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-muted-foreground/40 bg-background'
					]}
				>
					{#if isChecked}
						<Check class="h-3 w-3" />
					{/if}
				</span>
				<div class="min-w-0 flex-1">
					<p class="text-xs font-medium text-foreground">{item.label}</p>
					{#if item.description}
						<p class="text-[11px] text-muted-foreground">{item.description}</p>
					{/if}
				</div>
			</button>
		{/each}
	</div>

	{#if !(isSubmitted && singleUse)}
		<div class="flex items-center justify-between border-t px-3.5 py-2">
			<span class="text-[11px] text-muted-foreground">
				{t('myaaw.extra.selected_count', { count: checkedIds.length })}
			</span>
			<button
				type="button"
				disabled={isLocked}
				onclick={handleSubmit}
				class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-55"
			>
				{#if submitting}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{/if}
				<span>{data.submit_label || t('myaaw.extra.confirm_selection')}</span>
			</button>
		</div>
	{/if}
</div>
