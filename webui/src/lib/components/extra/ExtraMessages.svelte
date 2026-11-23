<script lang="ts">
	import type {
		ExtraButtonsBlockData,
		ExtraCalloutBlockData,
		ExtraCarouselBlockData,
		ExtraChecklistBlockData,
		ExtraFormBlockData,
		ExtraLinkPreviewBlockData,
		ExtraMediaBlockData,
		ExtraMessage,
		ExtraQuickRepliesBlockData,
		ExtraSummaryBlockData,
		InteractionRequest
	} from '$lib/types';
	import ButtonsBlock from './ButtonsBlock.svelte';
	import CalloutBlock from './CalloutBlock.svelte';
	import CarouselBlock from './CarouselBlock.svelte';
	import ChecklistBlock from './ChecklistBlock.svelte';
	import FormBlock from './FormBlock.svelte';
	import LinkPreviewBlock from './LinkPreviewBlock.svelte';
	import MediaBlock from './MediaBlock.svelte';
	import QuickRepliesBlock from './QuickRepliesBlock.svelte';
	import SummaryBlock from './SummaryBlock.svelte';

	interface Props {
		agent: string;
		extraMessages: ExtraMessage[];
		messageId?: string;
		disabled?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { agent, extraMessages, messageId = '', disabled = false, onInteract }: Props = $props();

	let submittingBlocks = $state<Record<string, boolean>>({});

	async function dispatchInteract(req: InteractionRequest) {
		if (!onInteract || !req.block_id) return;
		if (submittingBlocks[req.block_id]) return;
		submittingBlocks = { ...submittingBlocks, [req.block_id]: true };
		try {
			await onInteract({
				...req,
				...(messageId ? { message_id: messageId } : {})
			});
		} finally {
			const next = { ...submittingBlocks };
			delete next[req.block_id];
			submittingBlocks = next;
		}
	}
</script>

{#if extraMessages.length > 0}
	<div class="mt-2.5 space-y-2.5">
		{#each extraMessages as extraMsg (extraMsg.id)}
			{#each extraMsg.blocks ?? [] as block (block.id)}
				{@const isBusy = Boolean(submittingBlocks[block.id])}
				{#if block.type === 'media'}
					<MediaBlock {agent} data={block.data as ExtraMediaBlockData} />
				{:else if block.type === 'carousel'}
					<CarouselBlock
						{agent}
						{block}
						data={block.data as ExtraCarouselBlockData}
						{disabled}
						submitting={isBusy}
						onInteract={dispatchInteract}
					/>
				{:else if block.type === 'buttons'}
					<ButtonsBlock
						{block}
						data={block.data as ExtraButtonsBlockData}
						{disabled}
						submitting={isBusy}
						onInteract={dispatchInteract}
					/>
				{:else if block.type === 'quick_replies'}
					<QuickRepliesBlock
						{block}
						data={block.data as ExtraQuickRepliesBlockData}
						{disabled}
						submitting={isBusy}
						onInteract={dispatchInteract}
					/>
				{:else if block.type === 'form'}
					<FormBlock
						{block}
						data={block.data as ExtraFormBlockData}
						{disabled}
						submitting={isBusy}
						onInteract={dispatchInteract}
					/>
				{:else if block.type === 'checklist'}
					<ChecklistBlock
						{block}
						data={block.data as ExtraChecklistBlockData}
						{disabled}
						submitting={isBusy}
						onInteract={dispatchInteract}
					/>
				{:else if block.type === 'callout'}
					<CalloutBlock data={block.data as ExtraCalloutBlockData} />
				{:else if block.type === 'summary'}
					<SummaryBlock data={block.data as ExtraSummaryBlockData} />
				{:else if block.type === 'link_preview'}
					<LinkPreviewBlock data={block.data as ExtraLinkPreviewBlockData} />
				{/if}
			{/each}
		{/each}
	</div>
{/if}
