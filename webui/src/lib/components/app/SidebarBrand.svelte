<script lang="ts">
	import { PanelLeftClose, PanelLeftOpen, X } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { agentSelection, sidebar } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: the wordmark fades out and a re-expand handle appears. */
		compact?: boolean;
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, onClose }: Props = $props();
</script>

<!--
	One header for both sidebar widths. The logo box stays fixed while the rail
	animates; the wordmark fades. The collapse handle is pinned to the header's
	right edge (not `ml-auto`, which slid it over the logo mid-transition) and only
	fades in after the rail has finished widening.
-->
<div class="relative flex h-14 shrink-0 items-center overflow-hidden border-b pr-2 pl-4 md:pr-0">
	<div
		class="flex h-12 w-12 shrink-0 items-center justify-center transition-transform duration-200 {compact
			? '-translate-x-3 scale-[0.833]'
			: 'translate-x-0 scale-100'}"
		data-brand-logo
	>
		<Bloub mood="attentif" size={48} color={agentSelection.color} />
	</div>
	<div
		class={[
			'flex min-w-0 flex-col overflow-hidden leading-tight whitespace-nowrap transition-all duration-200',
			compact ? 'max-w-0 opacity-0 delay-0' : 'max-w-44 opacity-100 delay-200'
		]}
	>
		<span class="text-sm font-bold tracking-tight">Myaaw</span>
		<span class="text-[10px] uppercase tracking-widest text-muted-foreground"> Control plane </span>
	</div>
	<button
		class={[
			'absolute inset-y-0 right-0 hidden w-10 items-center justify-end self-stretch rounded-l-md pr-1 text-muted-foreground hover:bg-muted hover:text-foreground md:flex',
			compact
				? 'pointer-events-none opacity-0 transition-none'
				: 'opacity-100 transition-[opacity,color,background-color] duration-200 delay-200'
		]}
		onclick={() => sidebar.toggle()}
		title={t('myaaw.sidebar.collapse')}
		aria-label={t('myaaw.sidebar.collapse')}
		aria-keyshortcuts="Control+\\ Meta+\\"
	>
		<PanelLeftClose class="h-4 w-4" />
	</button>
	<button
		class="ml-auto flex w-10 shrink-0 items-center justify-end self-stretch rounded-l-md pr-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground md:hidden"
		onclick={onClose}
		title={t('myaaw.sidebar.close')}
		aria-label={t('myaaw.sidebar.close')}
	>
		<X class="h-4 w-4" />
	</button>
</div>

<button
	class={[
		'absolute top-4 -right-3 z-50 flex h-6 w-6 items-center justify-center rounded-full border bg-card text-muted-foreground shadow-sm frost-card hover:bg-muted hover:text-foreground',
		compact
			? 'opacity-100 transition-opacity duration-200 delay-200'
			: 'pointer-events-none opacity-0 transition-none'
	]}
	inert={!compact}
	aria-hidden={!compact}
	onclick={() => sidebar.toggle()}
	title={t('myaaw.sidebar.expand')}
	aria-label={t('myaaw.sidebar.expand')}
	aria-keyshortcuts="Control+\\ Meta+\\"
>
	<PanelLeftOpen class="h-3.5 w-3.5" />
</button>
