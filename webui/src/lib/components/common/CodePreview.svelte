<script lang="ts">
	import { highlightCode } from '$lib/utils/highlight';

	interface Props {
		code: string;
		language?: string;
		lineNumbers?: boolean;
	}

	let { code, language = 'plaintext', lineNumbers = true }: Props = $props();

	// highlight.js escapes the source before wrapping tokens in spans, so the
	// markup never contains user-provided HTML.
	const html = $derived(highlightCode(code, language));
	const lineCount = $derived(code.length === 0 ? 1 : code.split('\n').length);
</script>

<div
	class="hljs flex h-full min-h-0 overflow-auto rounded-md border border-input bg-muted/20 font-mono text-xs leading-6"
>
	{#if lineNumbers}
		<div
			aria-hidden="true"
			class="sticky left-0 z-10 flex shrink-0 flex-col border-r border-border/50 bg-muted/40 py-3 pr-2.5 pl-3 text-right text-[11px] text-muted-foreground/70 select-none tabular-nums"
		>
			{#each { length: lineCount } as _, idx (idx)}
				<span class="leading-6">{idx + 1}</span>
			{/each}
		</div>
	{/if}
	<!-- prettier-ignore -->
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	<pre class="m-0 min-w-0 flex-1 py-3 pr-4 pl-3 leading-6 whitespace-pre"><code>{@html html}</code
		></pre>
</div>
