<script lang="ts">
	import { Check, ExternalLink, Loader2 } from 'lucide-svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type {
		ExtraBlock,
		ExtraButton,
		ExtraButtonsBlockData,
		InteractionRequest
	} from '$lib/types';

	interface Props {
		block: ExtraBlock;
		data: ExtraButtonsBlockData;
		disabled?: boolean;
		submitting?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { block, data, disabled = false, submitting = false, onInteract }: Props = $props();

	const { t } = useTranslation();

	let singleUse = $derived(block.single_use !== false);
	let isSubmitted = $derived(block.state?.status === 'submitted');
	let isLocked = $derived((singleUse && isSubmitted) || disabled || submitting);
	let selectedActionId = $derived(
		typeof block.state?.payload?.action_id === 'string' ? block.state.payload.action_id : ''
	);
	let pendingButtonId = $state('');

	function buttonClass(btn: ExtraButton, chosen: boolean): string {
		if (chosen) {
			return 'border-primary bg-primary/12 text-primary ring-1 ring-primary/30';
		}
		switch (btn.style) {
			case 'primary':
				return 'border-primary bg-primary text-primary-foreground hover:bg-primary/90';
			case 'danger':
				return 'border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/15';
			default:
				return 'border-border bg-background text-foreground hover:bg-muted/70';
		}
	}

	async function handleAction(btn: ExtraButton) {
		if (isLocked || !onInteract) return;
		if (btn.confirm) {
			const ok = await askConfirm({
				title: btn.label,
				message: btn.confirm,
				confirmText: btn.label,
				variant: btn.style === 'danger' ? 'destructive' : 'default'
			});
			if (!ok) return;
		}
		pendingButtonId = btn.id;
		try {
			await onInteract({
				block_id: block.id,
				kind: 'button',
				action_id: btn.id,
				label: btn.label,
				value: btn.value ?? btn.label,
				display_text: btn.label
			});
		} finally {
			pendingButtonId = '';
		}
	}
</script>

<div
	class={[
		'w-fit max-w-full space-y-2 rounded-xl border bg-card/70 p-3',
		data.layout === 'stack' && 'min-w-52'
	]}
>
	{#if data.prompt}
		<p class="text-xs font-medium text-foreground">{data.prompt}</p>
	{/if}
	<div
		class={data.layout === 'stack' ? 'flex flex-col gap-1.5' : 'flex flex-wrap items-center gap-2'}
	>
		{#each data.buttons ?? [] as btn (btn.id)}
			{#if btn.type === 'link' && btn.url}
				<a
					href={btn.url}
					target="_blank"
					rel="noopener noreferrer"
					class={[
						'inline-flex items-center justify-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors',
						data.layout === 'stack' && 'w-full',
						buttonClass(btn, false)
					]}
				>
					<span>{btn.label}</span>
					<ExternalLink class="h-3 w-3 shrink-0 opacity-75" />
				</a>
			{:else}
				{@const chosen = isSubmitted && selectedActionId === btn.id}
				{@const loading = submitting && pendingButtonId === btn.id}
				<button
					type="button"
					disabled={isLocked}
					onclick={() => handleAction(btn)}
					class={[
						'inline-flex items-center justify-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors',
						data.layout === 'stack' && 'w-full',
						isLocked && !chosen && 'cursor-not-allowed opacity-55',
						buttonClass(btn, chosen)
					]}
				>
					{#if loading}
						<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
					{:else if chosen}
						<Check class="h-3.5 w-3.5 shrink-0" />
					{/if}
					<span>{btn.label}</span>
				</button>
			{/if}
		{/each}
	</div>
	{#if isSubmitted && block.state?.summary}
		<p class="flex items-center gap-1 text-[11px] text-muted-foreground">
			<Check class="h-3 w-3 text-emerald-500" />
			<span>{t('myaaw.extra.selected_summary', { value: block.state.summary })}</span>
		</p>
	{/if}
</div>
