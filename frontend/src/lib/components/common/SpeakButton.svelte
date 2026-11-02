<script lang="ts">
	import { Loader2, Square, Volume2 } from 'lucide-svelte';
	import { cn } from '$lib/utils';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { tts } from '$lib/speech/tts.svelte';
	import { speakableText } from '$lib/speech/text';

	interface Props {
		/** Row identity, so two answers never share one playback session. */
		id: string;
		text: string;
		class?: string;
	}

	let { id, text, class: className = '' }: Props = $props();
	const { t } = useTranslation();

	// Answers made only of code blocks have nothing to read.
	const usable = $derived(speakableText(text).length > 0);
	const active = $derived(tts.activeId === id && tts.phase !== 'idle');
	const starting = $derived(active && tts.phase === 'starting');
	const label = $derived(active ? t('myaaw.chat.speak_stop') : t('myaaw.chat.speak'));
</script>

{#if tts.supported && usable}
	<button
		type="button"
		class={cn(
			'inline-flex items-center gap-1 rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground',
			className
		)}
		onclick={() => tts.toggle(id, text)}
		title={label}
		aria-label={label}
		aria-pressed={active}
	>
		{#if starting}
			<Loader2 class="h-3.5 w-3.5 animate-spin" />
		{:else if active}
			<Square class="h-3.5 w-3.5" />
		{:else}
			<Volume2 class="h-3.5 w-3.5" />
		{/if}
	</button>
{/if}
