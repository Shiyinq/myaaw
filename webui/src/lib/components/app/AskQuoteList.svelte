<script lang="ts">
	import { Brain, ChevronDown, Quote, Wrench, X } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { AskQuote } from '$lib/types';
	import { cn } from '$lib/utils';

	interface Props {
		quotes: AskQuote[];
		/** Omit to render a read-only list (e.g. the sent-message preview). */
		onremove?: (id: string) => void;
		onclear?: () => void;
		/** Clicking the body jumps back to the quoted text in the transcript. */
		onlocate?: (quote: AskQuote) => void;
		class?: string;
	}

	let { quotes, onremove, onclear, onlocate, class: className }: Props = $props();

	const { t } = useTranslation();

	// Cards start collapsed to three lines; the footer reveals the whole excerpt
	// without leaving the composer.
	let expanded = $state<Record<string, boolean>>({});

	function toggle(id: string) {
		expanded = { ...expanded, [id]: !expanded[id] };
	}

	function sourceLabel(quote: AskQuote): string {
		if (quote.source === 'tool') {
			return quote.label
				? `${t('myaaw.chat.ask_source_tool')} · ${quote.label}`
				: t('myaaw.chat.ask_source_tool');
		}
		return quote.source === 'thinking'
			? t('myaaw.chat.ask_source_thinking')
			: t('myaaw.chat.ask_source_answer');
	}

	function sourceIcon(quote: AskQuote) {
		return quote.source === 'tool' ? Wrench : quote.source === 'thinking' ? Brain : Quote;
	}
</script>

<div class={cn('space-y-2', className)}>
	<div class="flex items-center justify-between gap-2 px-1 text-[11px] text-muted-foreground">
		<span class="inline-flex items-center gap-1.5 font-medium">
			<Quote class="h-3.5 w-3.5" />
			{quotes.length === 1
				? t('myaaw.chat.ask_selected_one')
				: t('myaaw.chat.ask_selected', { count: quotes.length })}
		</span>
		{#if onclear && quotes.length > 1}
			<button
				type="button"
				class="rounded-md px-1 py-0.5 transition-colors hover:bg-muted hover:text-foreground"
				onclick={onclear}
			>
				{t('myaaw.chat.ask_clear')}
			</button>
		{/if}
	</div>

	<div class="flex items-start gap-2 overflow-x-auto pb-1">
		{#each quotes as quote (quote.id)}
			{@const Icon = sourceIcon(quote)}
			{@const open = !!expanded[quote.id]}
			<div
				class={cn(
					'relative w-52 shrink-0 overflow-hidden rounded-xl border bg-muted/30',
					open && 'bg-muted/50'
				)}
				data-ask-quote={quote.id}
				data-ask-source-id={quote.sourceId ?? ''}
			>
				<button
					type="button"
					class="block w-full text-left"
					onclick={() => (onlocate ? onlocate(quote) : toggle(quote.id))}
					title={onlocate ? t('myaaw.chat.ask_locate') : undefined}
					aria-label={onlocate ? t('myaaw.chat.ask_locate') : undefined}
				>
					<p
						class={cn(
							'px-2.5 pt-2 pb-1.5 text-xs text-foreground/80',
							open ? 'max-h-44 overflow-y-auto whitespace-pre-wrap' : 'line-clamp-3'
						)}
					>
						{quote.text}
					</p>
				</button>
				<button
					type="button"
					class="flex w-full items-center gap-1 border-t bg-muted/30 px-2.5 py-1.5 text-[10px] font-medium text-muted-foreground transition-colors hover:text-foreground"
					onclick={() => toggle(quote.id)}
					aria-expanded={open}
					aria-label={t('myaaw.chat.ask_expand')}
					title={t('myaaw.chat.ask_expand')}
				>
					<Icon class="h-3 w-3 shrink-0" />
					<span class="min-w-0 truncate">{sourceLabel(quote)}</span>
					<ChevronDown
						class={cn('ml-auto h-3 w-3 shrink-0 transition-transform', open && 'rotate-180')}
					/>
				</button>
				{#if onremove}
					<button
						type="button"
						class="absolute top-1 right-1 grid size-5 place-items-center rounded-full border bg-background text-muted-foreground shadow-sm transition-colors hover:text-foreground"
						onclick={() => onremove?.(quote.id)}
						aria-label={t('myaaw.chat.ask_remove')}
						title={t('myaaw.chat.ask_remove')}
					>
						<X class="h-3 w-3" />
					</button>
				{/if}
			</div>
		{/each}
	</div>
</div>
