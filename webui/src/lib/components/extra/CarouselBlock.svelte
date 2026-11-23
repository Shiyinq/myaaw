<script lang="ts">
	import { Check, ChevronLeft, ChevronRight, Download, ExternalLink, Loader2 } from 'lucide-svelte';
	import { filesStore } from '$lib/stores/files.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type {
		ExtraBlock,
		ExtraButton,
		ExtraCarouselBlockData,
		ExtraCarouselCard,
		InteractionRequest,
		LightboxImage
	} from '$lib/types';
	import LightBox from '$lib/components/app/LightBox.svelte';

	interface Props {
		agent: string;
		block: ExtraBlock;
		data: ExtraCarouselBlockData;
		disabled?: boolean;
		submitting?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { agent, block, data, disabled = false, submitting = false, onInteract }: Props = $props();

	const { t } = useTranslation();

	let cards = $derived(data.cards ?? []);
	let singleUse = $derived(block.single_use !== false);
	let isSubmitted = $derived(block.state?.status === 'submitted');
	let isLocked = $derived((singleUse && isSubmitted) || disabled || submitting);
	let selectedActionId = $derived(
		typeof block.state?.payload?.action_id === 'string' ? block.state.payload.action_id : ''
	);
	let selectedCardId = $derived(
		typeof block.state?.payload?.card_id === 'string' ? block.state.payload.card_id : ''
	);
	let pendingKey = $state('');

	$effect(() => {
		if (!agent) return;
		const paths = cards.map((c) => c.image_path).filter((p): p is string => Boolean(p));
		if (paths.length > 0) {
			void ensureSigned(agent, paths);
		}
	});

	function imageSrc(card: ExtraCarouselCard, download = false): string {
		if (card.image_path && agent) {
			return filesStore.rawUrl(agent, card.image_path, { download });
		}
		return card.image_url ?? '';
	}

	let cardsWithImages = $derived(cards.filter((card) => Boolean(imageSrc(card))));
	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let lightboxItems = $derived<LightboxImage[]>(
		cardsWithImages.map((card) => ({
			src: imageSrc(card),
			name: card.title,
			downloadSrc: imageSrc(card, true)
		}))
	);

	let pressX = 0;
	let pressY = 0;

	function notePress(event: PointerEvent) {
		pressX = event.clientX;
		pressY = event.clientY;
	}

	function openCardImage(event: MouseEvent, card: ExtraCarouselCard) {
		if (Math.hypot(event.clientX - pressX, event.clientY - pressY) > 8) return;
		const idx = cardsWithImages.indexOf(card);
		lightboxIndex = idx < 0 ? 0 : idx;
		lightboxOpen = true;
	}

	function downloadCardImage(card: ExtraCarouselCard) {
		if (card.image_path && agent) {
			filesStore.download(agent, card.image_path);
			return;
		}
		const src = imageSrc(card);
		if (!src) return;
		const link = document.createElement('a');
		link.href = src;
		link.download = `${card.title || 'image'}`;
		link.target = '_blank';
		link.rel = 'noopener noreferrer';
		document.body.appendChild(link);
		link.click();
		link.remove();
	}

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

	async function handleAction(card: ExtraCarouselCard, btn: ExtraButton) {
		if (isLocked || !onInteract) return;
		if (btn.confirm) {
			const ok = await askConfirm({
				title: `${card.title} — ${btn.label}`,
				message: btn.confirm,
				confirmText: btn.label,
				variant: btn.style === 'danger' ? 'destructive' : 'default'
			});
			if (!ok) return;
		}
		const key = `${card.id}:${btn.id}`;
		pendingKey = key;
		try {
			await onInteract({
				block_id: block.id,
				kind: 'button',
				card_id: card.id,
				action_id: btn.id,
				label: `${card.title}: ${btn.label}`,
				value: btn.value ?? btn.label,
				display_text: `${card.title} — ${btn.label}`
			});
		} finally {
			pendingKey = '';
		}
	}

	let track = $state<HTMLDivElement | null>(null);
	let canPrev = $state(false);
	let canNext = $state(false);

	function syncNav() {
		const node = track;
		if (!node) return;
		canPrev = node.scrollLeft > 4;
		canNext = node.scrollLeft + node.clientWidth < node.scrollWidth - 4;
	}

	function stepScroll(dir: -1 | 1) {
		const node = track;
		if (!node) return;
		node.scrollBy({ left: dir * 280, behavior: 'smooth' });
	}

	$effect(() => {
		const node = track;
		if (!node || typeof ResizeObserver === 'undefined') return;
		syncNav();
		const observer = new ResizeObserver(() => syncNav());
		observer.observe(node);
		return () => observer.disconnect();
	});
</script>

<div class="relative">
	<div
		bind:this={track}
		onscroll={syncNav}
		class="flex snap-x snap-mandatory gap-2.5 overflow-x-auto pb-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
	>
		{#each cards as card (card.id)}
			{@const img = imageSrc(card)}
			<div class="flex w-64 shrink-0 snap-start flex-col overflow-hidden rounded-xl border bg-card">
				{#if img}
					<div
						class="relative flex w-full shrink-0 items-center justify-center border-b bg-muted/30"
					>
						<button
							type="button"
							class="flex w-full cursor-zoom-in items-center justify-center focus-visible:outline-2 focus-visible:outline-offset-2"
							aria-label={card.title}
							onpointerdown={notePress}
							onclick={(event) => openCardImage(event, card)}
						>
							<img
								src={img}
								alt={card.title}
								loading="lazy"
								draggable="false"
								referrerpolicy="no-referrer"
								class="max-h-56 w-auto max-w-full object-contain"
							/>
						</button>
						<button
							type="button"
							onclick={() => downloadCardImage(card)}
							title={t('myaaw.chat.download_file')}
							aria-label={t('myaaw.chat.download_file')}
							class="absolute right-1.5 bottom-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80"
						>
							<Download class="h-3.5 w-3.5" />
						</button>
					</div>
				{/if}
				<div class="flex flex-1 flex-col gap-1.5 p-3">
					{#if card.badges && card.badges.length > 0}
						<div class="flex flex-wrap gap-1">
							{#each card.badges as badge, idx (`${card.id}-b-${idx}`)}
								<span
									class="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground"
								>
									{badge}
								</span>
							{/each}
						</div>
					{/if}
					<div>
						<p class="line-clamp-2 text-sm font-semibold leading-snug text-foreground">
							{card.title}
						</p>
						{#if card.subtitle}
							<p class="mt-0.5 text-xs font-medium text-muted-foreground">{card.subtitle}</p>
						{/if}
					</div>
					{#if card.description}
						<p class="line-clamp-3 text-xs leading-relaxed text-muted-foreground">
							{card.description}
						</p>
					{/if}
					{#if card.buttons && card.buttons.length > 0}
						<div class="mt-auto flex flex-col gap-1.5 pt-2">
							{#each card.buttons as btn (btn.id)}
								{#if btn.type === 'link' && btn.url}
									<a
										href={btn.url}
										target="_blank"
										rel="noopener noreferrer"
										class={[
											'inline-flex w-full items-center justify-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition-colors',
											buttonClass(btn, false)
										]}
									>
										<span>{btn.label}</span>
										<ExternalLink class="h-3 w-3 shrink-0 opacity-75" />
									</a>
								{:else}
									{@const chosen =
										isSubmitted &&
										selectedActionId === btn.id &&
										(!selectedCardId || selectedCardId === card.id)}
									{@const loading = submitting && pendingKey === `${card.id}:${btn.id}`}
									<button
										type="button"
										disabled={isLocked}
										onclick={() => handleAction(card, btn)}
										class={[
											'inline-flex w-full items-center justify-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition-colors',
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
					{/if}
				</div>
			</div>
		{/each}
	</div>

	{#if canPrev}
		<button
			type="button"
			onclick={() => stepScroll(-1)}
			aria-label={t('myaaw.extra.prev')}
			class="absolute top-1/2 left-1.5 grid size-7 -translate-y-1/2 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80"
		>
			<ChevronLeft class="h-4 w-4" />
		</button>
	{/if}
	{#if canNext}
		<button
			type="button"
			onclick={() => stepScroll(1)}
			aria-label={t('myaaw.extra.next')}
			class="absolute top-1/2 right-1.5 grid size-7 -translate-y-1/2 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80"
		>
			<ChevronRight class="h-4 w-4" />
		</button>
	{/if}
</div>

<LightBox bind:open={lightboxOpen} items={lightboxItems} bind:index={lightboxIndex} />
