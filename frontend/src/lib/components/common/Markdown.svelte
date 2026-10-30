<script lang="ts">
	import { marked } from 'marked';
	import DOMPurify from 'isomorphic-dompurify';

	interface Props {
		content: string;
	}

	let { content }: Props = $props();

	// The rendered HTML is produced by marked and then sanitised with DOMPurify,
	// so injecting it is safe.
	const html = $derived(DOMPurify.sanitize(marked.parse(content ?? '') as string));
</script>

<div class="prose prose-sm dark:prose-invert max-w-none break-words">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html html}
</div>
