<script module lang="ts">
	import { marked } from 'marked';
	import { highlightCode } from '$lib/utils/highlight';

	// Code fences get the same highlighter as Files/Skills, plus a small header
	// with the language, a copy action, and a download action. Marked escapes the
	// source before wrapping tokens; DOMPurify sanitises the final HTML.
	marked.use({
		renderer: {
			code({ text, lang }) {
				const language = (lang ?? '').trim().split(/\s+/)[0] || 'plaintext';
				const label = language === 'plaintext' ? 'text' : language;
				const body = highlightCode(text ?? '', language);
				return `<div class="md-code not-prose my-3 overflow-hidden rounded-md border border-input">
	<div class="flex items-center justify-between gap-2 border-b bg-muted/40 px-3 py-1.5 text-[11px] text-muted-foreground">
		<span class="font-mono">${label}</span>
		<span class="flex items-center gap-3">
			<button type="button" data-code-action="copy" class="hover:text-foreground">Copy</button>
			<button type="button" data-code-action="download" data-code-lang="${language}" class="hover:text-foreground">Download</button>
		</span>
	</div>
	<pre class="hljs overflow-auto bg-muted/20 p-3 font-mono text-xs leading-relaxed"><code class="language-${language}">${body}</code></pre>
</div>`;
			}
		}
	});
</script>

<script lang="ts">
	import DOMPurify from 'isomorphic-dompurify';

	interface Props {
		content: string;
	}

	let { content }: Props = $props();

	// The rendered HTML is produced by marked and then sanitised with DOMPurify,
	// so injecting it is safe.
	const html = $derived(DOMPurify.sanitize(marked.parse(content ?? '') as string));

	const extensions: Record<string, string> = {
		bash: 'sh',
		css: 'css',
		go: 'go',
		ini: 'ini',
		javascript: 'js',
		json: 'json',
		markdown: 'md',
		python: 'py',
		rust: 'rs',
		sql: 'sql',
		typescript: 'ts',
		xml: 'xml',
		yaml: 'yml'
	};

	let root = $state<HTMLDivElement | null>(null);

	// Injected HTML cannot carry Svelte handlers, so the wrapper delegates clicks
	// for every code block it contains.
	$effect(() => {
		const node = root;
		if (!node) return;
		const onClick = (event: MouseEvent) => {
			const button = (event.target as HTMLElement).closest<HTMLButtonElement>(
				'button[data-code-action]'
			);
			if (!button) return;
			const codeElement = button.closest('.md-code')?.querySelector('code');
			const code = codeElement?.textContent ?? '';
			if (button.dataset.codeAction === 'copy') {
				void navigator.clipboard?.writeText(code);
				const previous = button.textContent;
				button.textContent = 'Copied';
				setTimeout(() => (button.textContent = previous), 1200);
				return;
			}
			const language = button.dataset.codeLang || 'plaintext';
			const url = URL.createObjectURL(new Blob([code], { type: 'text/plain' }));
			const anchor = document.createElement('a');
			anchor.href = url;
			anchor.download = `code.${extensions[language] ?? 'txt'}`;
			document.body.append(anchor);
			anchor.click();
			anchor.remove();
			setTimeout(() => URL.revokeObjectURL(url), 1000);
		};
		node.addEventListener('click', onClick);
		return () => node.removeEventListener('click', onClick);
	});
</script>

<div bind:this={root} class="prose prose-sm dark:prose-invert max-w-none wrap-break-word">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html html}
</div>
