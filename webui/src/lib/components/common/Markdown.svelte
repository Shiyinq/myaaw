<script module lang="ts">
	import { Marked } from 'marked';
	import { highlightCode } from '$lib/utils/highlight';
	import { markdownMath } from '$lib/utils/math';

	export interface MarkdownLabels {
		copy: string;
		copied: string;
		download: string;
		source: string;
		showDiagram: string;
		renderFailed: string;
		expand: string;
	}

	function escapeHtml(value: string): string {
		return value.replace(
			/[&<>"']/g,
			(character) =>
				({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character] ??
				character
		);
	}

	// `data-mermaid-state` drives the stylesheet: pending shows the source, ready
	// swaps in the SVG, error keeps the source and adds a note. The source lives
	// in the DOM so copy, download and the Source toggle never need extra state.
	function mermaidBlock(source: string, labels: MarkdownLabels): string {
		const copy = escapeHtml(labels.copy);
		const download = escapeHtml(labels.download);
		const sourceLabel = escapeHtml(labels.source);
		return `<div class="md-mermaid not-prose my-3 overflow-hidden rounded-md border border-input" data-code-block data-mermaid data-mermaid-state="pending">
	<div class="flex items-center justify-between gap-2 border-b bg-muted/40 px-3 py-1.5 text-[11px] text-muted-foreground">
		<span class="font-mono">mermaid</span>
		<span class="flex items-center gap-3">
			<button type="button" data-code-action="copy" data-code-label="${copy}" data-copied-label="${escapeHtml(labels.copied)}" class="hover:text-foreground">${copy}</button>
			<button type="button" data-code-action="download" data-code-lang="mmd" class="hover:text-foreground">${download}</button>
			<button type="button" data-mermaid-action="source" data-source-label="${sourceLabel}" data-diagram-label="${escapeHtml(labels.showDiagram)}" class="hover:text-foreground">${sourceLabel}</button>
			<button type="button" data-mermaid-action="expand" title="${escapeHtml(labels.expand)}" aria-label="${escapeHtml(labels.expand)}" class="grid place-items-center hover:text-foreground"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5" aria-hidden="true"><polyline points="15 3 21 3 21 9"></polyline><polyline points="9 21 3 21 3 15"></polyline><line x1="21" y1="3" x2="14" y2="10"></line><line x1="3" y1="21" x2="10" y2="14"></line></svg></button>
		</span>
	</div>
	<div class="md-mermaid-body bg-muted/20 p-3">
		<div class="md-mermaid-canvas" data-mermaid-canvas></div>
		<p class="md-mermaid-error">${escapeHtml(labels.renderFailed)}</p>
		<pre class="md-mermaid-source" data-mermaid-source><code class="language-mermaid">${escapeHtml(source)}</code></pre>
	</div>
</div>`;
	}

	/**
	 * Builds a parser for one component. The code-fence toolbar reads its labels
	 * while parsing, so switching language simply parses the answer again.
	 * Marked escapes the source before wrapping tokens; DOMPurify sanitises the
	 * final HTML.
	 *
	 * Mermaid fences only become a container: the diagram itself is rendered by
	 * the component after mount, because the sanitizer would strip the SVG's
	 * <style> block and foreignObject labels.
	 */
	export function createParser(labels: () => MarkdownLabels): Marked {
		return new Marked({
			extensions: markdownMath,
			renderer: {
				code({ text, lang }) {
					const strings = labels();
					const language = (lang ?? '').trim().split(/\s+/)[0] || 'plaintext';
					if (language === 'mermaid' || language === 'mmd') {
						return mermaidBlock(text ?? '', strings);
					}
					const label = language === 'plaintext' ? 'text' : language;
					const body = highlightCode(text ?? '', language);
					return `<div class="md-code not-prose my-3 overflow-hidden rounded-md border border-input" data-code-block>
<div class="flex items-center justify-between gap-2 border-b bg-muted/40 px-3 py-1.5 text-[11px] text-muted-foreground">
	<span class="font-mono">${label}</span>
	<span class="flex items-center gap-3">
		<button type="button" data-code-action="copy" data-code-label="${escapeHtml(strings.copy)}" data-copied-label="${escapeHtml(strings.copied)}" class="hover:text-foreground">${escapeHtml(strings.copy)}</button>
		<button type="button" data-code-action="download" data-code-lang="${language}" class="hover:text-foreground">${escapeHtml(strings.download)}</button>
	</span>
</div>
<pre class="hljs overflow-auto bg-muted/20 p-3 font-mono text-xs leading-relaxed"><code class="language-${language}">${body}</code></pre>
</div>`;
				},
				// Answers link out to the web constantly, and following one in the same tab
				// would replace the conversation. External links open in their own tab;
				// in-page anchors keep the current one.
				link({ href, title, tokens }) {
					const titleAttribute = title ? ` title="${escapeHtml(title)}"` : '';
					const target = href.startsWith('#') ? '' : ' target="_blank" rel="noopener noreferrer"';
					const body = this.parser.parseInline(tokens);
					return `<a href="${escapeHtml(href)}"${titleAttribute}${target}>${body}</a>`;
				}
			}
		});
	}
</script>

<script lang="ts">
	import DOMPurify from 'isomorphic-dompurify';
	import { browser } from '$app/environment';
	import { mode } from 'mode-watcher';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import MermaidViewer from '$lib/components/common/MermaidViewer.svelte';
	import { renderMermaid, type MermaidMode } from '$lib/utils/mermaid';

	interface Props {
		content: string;
		/**
		 * True while this answer is still arriving. Diagrams wait for the final
		 * text, so a half-written fence is never parsed as a broken diagram.
		 */
		streaming?: boolean;
	}

	let { content, streaming = false }: Props = $props();

	// KaTeX emits MathML next to its styled HTML; the math profile keeps it (and
	// `semantics`/`annotation`, which DOMPurify does not know) so screen readers
	// still get the expression. The few SVG tags are what the Mermaid toolbar's
	// fullscreen icon needs: nothing else in a message is allowed to draw SVG.
	// `target` is not in DOMPurify's default attribute list, and links rely on it
	// to open outside the dashboard.
	const SANITIZE = {
		USE_PROFILES: { html: true, mathMl: true },
		ADD_TAGS: ['annotation', 'semantics', 'svg', 'path', 'polyline', 'line'],
		ADD_ATTR: [
			'target',
			'encoding',
			'viewBox',
			'xmlns',
			'fill',
			'stroke',
			'stroke-width',
			'stroke-linecap',
			'stroke-linejoin',
			'points',
			'x1',
			'y1',
			'x2',
			'y2'
		]
	};

	const { t } = useTranslation();
	const parser = createParser(() => ({
		copy: t('common.markdown.copy'),
		copied: t('common.markdown.copied'),
		download: t('common.markdown.download'),
		source: t('common.markdown.source'),
		showDiagram: t('common.markdown.show_diagram'),
		renderFailed: t('common.markdown.render_failed'),
		expand: t('common.markdown.expand')
	}));

	// The rendered HTML is produced by marked and then sanitised with DOMPurify,
	// so injecting it is safe.
	const html = $derived(DOMPurify.sanitize(parser.parse(content ?? '') as string, SANITIZE));

	let diagramMode: MermaidMode = $derived(mode.current === 'dark' ? 'dark' : 'light');

	const extensions: Record<string, string> = {
		bash: 'sh',
		css: 'css',
		go: 'go',
		ini: 'ini',
		javascript: 'js',
		json: 'json',
		markdown: 'md',
		mermaid: 'mmd',
		mmd: 'mmd',
		python: 'py',
		rust: 'rs',
		sql: 'sql',
		typescript: 'ts',
		xml: 'xml',
		yaml: 'yml'
	};

	let root = $state<HTMLDivElement | null>(null);

	// The fullscreen viewer is mounted only while it is open, so each message keeps
	// no zoom state of its own.
	let viewerSource = $state('');
	let viewerOpen = $state(false);

	// Injected HTML cannot carry Svelte handlers, so the wrapper delegates clicks
	// for every code block it contains.
	$effect(() => {
		const node = root;
		if (!node) return;
		const onClick = (event: MouseEvent) => {
			const target = event.target as HTMLElement;
			const sourceButton = target.closest<HTMLButtonElement>('button[data-mermaid-action]');
			if (sourceButton) {
				const block = sourceButton.closest<HTMLElement>('[data-mermaid]');
				if (!block) return;
				if (sourceButton.dataset.mermaidAction === 'expand') {
					const source = block.querySelector<HTMLElement>(
						'[data-mermaid-source] code'
					)?.textContent;
					if (!source?.trim()) return;
					viewerSource = source.trim();
					viewerOpen = true;
					return;
				}
				// Source replaces the picture instead of stacking under it, so the
				// block keeps its size and the reader sees only the diagram code.
				const shown = block.dataset.mermaidShowSource === 'true';
				block.dataset.mermaidShowSource = shown ? 'false' : 'true';
				sourceButton.textContent =
					(shown ? sourceButton.dataset.sourceLabel : sourceButton.dataset.diagramLabel) ??
					sourceButton.textContent;
				return;
			}
			const button = target.closest<HTMLButtonElement>('button[data-code-action]');
			if (!button) return;
			const codeElement = button.closest('[data-code-block]')?.querySelector('code');
			const code = codeElement?.textContent ?? '';
			if (button.dataset.codeAction === 'copy') {
				void navigator.clipboard?.writeText(code);
				const restore = button.dataset.codeLabel ?? button.textContent ?? '';
				button.textContent = button.dataset.copiedLabel ?? 'Copied';
				setTimeout(() => (button.textContent = restore), 1200);
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

	// Diagrams are drawn after the markup lands. Everything waits for the stream
	// to finish, then a short timer absorbs the burst of updates that a history
	// page or a collapsed step group produces.
	$effect(() => {
		const node = root;
		const markup = html;
		const busy = streaming;
		const diagramTheme = diagramMode;
		if (!browser || !node || busy || !markup.includes('data-mermaid')) return;

		const blocks = Array.from(node.querySelectorAll<HTMLElement>('[data-mermaid]'));
		if (blocks.length === 0) return;

		let cancelled = false;
		const timer = window.setTimeout(() => {
			void hydrateDiagrams(blocks, diagramTheme, () => cancelled);
		}, 120);

		return () => {
			cancelled = true;
			window.clearTimeout(timer);
		};
	});

	async function hydrateDiagrams(
		blocks: HTMLElement[],
		diagramTheme: MermaidMode,
		isCancelled: () => boolean
	) {
		for (const block of blocks) {
			if (isCancelled() || !block.isConnected) return;
			// Already drawn in this theme: a re-render of the same markup keeps it.
			if (block.dataset.mermaidState === 'ready' && block.dataset.mermaidTheme === diagramTheme) {
				continue;
			}
			const canvas = block.querySelector<HTMLElement>('[data-mermaid-canvas]');
			const source =
				block.querySelector<HTMLElement>('[data-mermaid-source] code')?.textContent?.trim() ?? '';
			if (!canvas || !source) continue;
			try {
				const svg = await renderMermaid(source, diagramTheme);
				if (!block.isConnected) return;
				// Mermaid's strict security level already ran the SVG through its own
				// DOMPurify pass, so this markup can be trusted the way the block is.
				canvas.innerHTML = svg;
				block.dataset.mermaidState = 'ready';
				block.dataset.mermaidTheme = diagramTheme;
			} catch {
				if (!block.isConnected) return;
				block.dataset.mermaidState = 'error';
				block.dataset.mermaidTheme = diagramTheme;
			}
		}
	}
</script>

<div bind:this={root} class="prose prose-sm dark:prose-invert max-w-none wrap-break-word">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html html}
</div>

{#if viewerOpen}
	<MermaidViewer bind:open={viewerOpen} source={viewerSource} />
{/if}
