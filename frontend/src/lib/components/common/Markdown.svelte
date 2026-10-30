<script lang="ts">
	import { marked } from 'marked';
	import DOMPurify from 'isomorphic-dompurify';

	interface Props {
		content: string;
	}

	let { content }: Props = $props();

	const html = $derived(DOMPurify.sanitize(marked.parse(content ?? '') as string));
</script>

<div class="prose prose-sm dark:prose-invert max-w-none break-words">
	{@html html}
</div>
