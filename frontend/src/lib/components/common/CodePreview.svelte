<script lang="ts">
	import { highlightCode } from '$lib/utils/highlight';

	interface Props {
		code: string;
		language?: string;
	}

	let { code, language = 'plaintext' }: Props = $props();

	// highlight.js escapes the source before wrapping tokens in spans, so the
	// markup never contains user-provided HTML.
	const html = $derived(highlightCode(code, language));
</script>

<pre
	class="hljs h-full overflow-auto rounded-md border border-input bg-muted/20 p-3 font-mono text-xs leading-relaxed">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	<code>{@html html}</code>
</pre>
