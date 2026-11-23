<script lang="ts">
	import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-svelte';
	import type { ExtraCalloutBlockData } from '$lib/types';

	interface Props {
		data: ExtraCalloutBlockData;
	}

	let { data }: Props = $props();

	let variant = $derived((data.variant || 'info').toLowerCase());

	let styles = $derived.by(() => {
		switch (variant) {
			case 'success':
				return {
					box: 'border-emerald-500/30 bg-emerald-500/8 text-foreground',
					icon: 'text-emerald-600 dark:text-emerald-400',
					Icon: CheckCircle2
				};
			case 'warning':
				return {
					box: 'border-amber-500/35 bg-amber-500/8 text-foreground',
					icon: 'text-amber-600 dark:text-amber-400',
					Icon: AlertTriangle
				};
			case 'error':
				return {
					box: 'border-destructive/35 bg-destructive/8 text-foreground',
					icon: 'text-destructive',
					Icon: XCircle
				};
			default:
				return {
					box: 'border-sky-500/30 bg-sky-500/8 text-foreground',
					icon: 'text-sky-600 dark:text-sky-400',
					Icon: Info
				};
		}
	});
</script>

<div
	class={[
		'flex w-fit max-w-lg items-start gap-2.5 rounded-xl border px-3.5 py-2.5 text-sm',
		styles.box
	]}
>
	<styles.Icon class={`mt-0.5 h-4 w-4 shrink-0 ${styles.icon}`} />
	<div class="min-w-0 flex-1 space-y-0.5">
		{#if data.title}
			<p class="font-medium leading-snug">{data.title}</p>
		{/if}
		<p class="whitespace-pre-wrap text-xs leading-relaxed text-muted-foreground">{data.text}</p>
	</div>
</div>
