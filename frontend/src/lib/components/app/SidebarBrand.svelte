<script lang="ts">
	import { PanelLeftClose, PanelLeftOpen, X } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { sidebar } from '$lib/stores';
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
	One header for both sidebar widths: the brand block keeps its position while
	the labels fade and the width animates, so collapsing does not swap the whole
	header in one frame.
-->
<div
	class={[
		'relative flex h-14 shrink-0 items-center overflow-hidden border-b',
		compact ? 'justify-center' : 'pr-2 pl-4 md:pr-0'
	]}
>
	<Bloub mood="attentif" size={compact ? 40 : 48} />
	<div
		class={[
			'flex min-w-0 flex-col overflow-hidden leading-tight whitespace-nowrap transition-all duration-200',
			compact ? 'max-w-0 opacity-0' : 'max-w-44 opacity-100'
		]}
	>
		<span class="text-sm font-bold tracking-tight">Myaaw</span>
		<span class="text-[10px] uppercase tracking-widest text-muted-foreground"> Control plane </span>
	</div>
	<button
		class={[
			'ml-auto w-10 shrink-0 items-center justify-end self-stretch rounded-l-md pr-1 text-muted-foreground transition-[color,opacity] hover:bg-muted hover:text-foreground',
			compact ? 'hidden' : 'hidden md:flex'
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

{#if compact}
	<button
		class="absolute top-4 -right-3 z-50 flex h-6 w-6 items-center justify-center rounded-full border bg-card text-muted-foreground shadow-sm frost-card transition-colors hover:bg-muted hover:text-foreground"
		onclick={() => sidebar.toggle()}
		title={t('myaaw.sidebar.expand')}
		aria-label={t('myaaw.sidebar.expand')}
		aria-keyshortcuts="Control+\\ Meta+\\"
	>
		<PanelLeftOpen class="h-3.5 w-3.5" />
	</button>
{/if}
