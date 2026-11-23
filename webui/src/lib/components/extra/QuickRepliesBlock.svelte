<script lang="ts">
	import { Check, Loader2 } from 'lucide-svelte';
	import type {
		ExtraBlock,
		ExtraQuickRepliesBlockData,
		ExtraQuickReplyOption,
		InteractionRequest
	} from '$lib/types';

	interface Props {
		block: ExtraBlock;
		data: ExtraQuickRepliesBlockData;
		disabled?: boolean;
		submitting?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { block, data, disabled = false, submitting = false, onInteract }: Props = $props();

	let singleUse = $derived(block.single_use !== false);
	let isSubmitted = $derived(block.state?.status === 'submitted');
	let isLocked = $derived((singleUse && isSubmitted) || disabled || submitting);
	let selectedId = $derived(
		typeof block.state?.payload?.action_id === 'string' ? block.state.payload.action_id : ''
	);
	let pendingId = $state('');

	async function pick(opt: ExtraQuickReplyOption) {
		if (isLocked || !onInteract) return;
		pendingId = opt.id;
		try {
			await onInteract({
				block_id: block.id,
				kind: 'quick_reply',
				action_id: opt.id,
				label: opt.label,
				value: opt.value ?? opt.label,
				display_text: opt.label
			});
		} finally {
			pendingId = '';
		}
	}
</script>

<div class="space-y-1.5">
	{#if data.prompt}
		<p class="text-xs font-medium text-muted-foreground">{data.prompt}</p>
	{/if}
	<div class="flex flex-wrap items-center gap-1.5">
		{#each data.options ?? [] as opt (opt.id)}
			{@const chosen = isSubmitted && selectedId === opt.id}
			{@const loading = submitting && pendingId === opt.id}
			<button
				type="button"
				disabled={isLocked}
				onclick={() => pick(opt)}
				class={[
					'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors',
					chosen
						? 'border-primary bg-primary/12 text-primary'
						: 'border-border bg-card text-foreground hover:border-foreground/30 hover:bg-muted/60',
					isLocked && !chosen && 'cursor-not-allowed opacity-55'
				]}
			>
				{#if loading}
					<Loader2 class="h-3 w-3 shrink-0 animate-spin" />
				{:else if chosen}
					<Check class="h-3 w-3 shrink-0" />
				{/if}
				<span>{opt.label}</span>
			</button>
		{/each}
	</div>
</div>
