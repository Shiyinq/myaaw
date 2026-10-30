export type FilePreviewKind = 'markdown' | 'code' | 'text' | 'binary';

export interface FilePreview {
	kind: FilePreviewKind;
	/** highlight.js language id, also used for plain text files. */
	language: string;
	/** Short label shown in the header badge. */
	label: string;
}

const CODE_EXTENSIONS: Record<string, { language: string; label: string }> = {
	js: { language: 'javascript', label: 'JavaScript' },
	mjs: { language: 'javascript', label: 'JavaScript' },
	cjs: { language: 'javascript', label: 'JavaScript' },
	jsx: { language: 'javascript', label: 'JSX' },
	ts: { language: 'typescript', label: 'TypeScript' },
	mts: { language: 'typescript', label: 'TypeScript' },
	cts: { language: 'typescript', label: 'TypeScript' },
	tsx: { language: 'typescript', label: 'TSX' },
	py: { language: 'python', label: 'Python' },
	sh: { language: 'bash', label: 'Shell' },
	bash: { language: 'bash', label: 'Shell' },
	zsh: { language: 'bash', label: 'Shell' },
	json: { language: 'json', label: 'JSON' },
	yml: { language: 'yaml', label: 'YAML' },
	yaml: { language: 'yaml', label: 'YAML' },
	toml: { language: 'ini', label: 'TOML' },
	ini: { language: 'ini', label: 'INI' },
	cfg: { language: 'ini', label: 'Config' },
	conf: { language: 'ini', label: 'Config' },
	css: { language: 'css', label: 'CSS' },
	html: { language: 'xml', label: 'HTML' },
	htm: { language: 'xml', label: 'HTML' },
	xml: { language: 'xml', label: 'XML' },
	svg: { language: 'xml', label: 'SVG' },
	go: { language: 'go', label: 'Go' },
	rs: { language: 'rust', label: 'Rust' },
	sql: { language: 'sql', label: 'SQL' },
	md: { language: 'markdown', label: 'Markdown' },
	markdown: { language: 'markdown', label: 'Markdown' },
	mdx: { language: 'markdown', label: 'MDX' }
};

const MARKDOWN_EXTENSIONS: Record<string, string> = {
	md: 'Markdown',
	markdown: 'Markdown',
	mdx: 'MDX'
};

const BINARY_EXTENSIONS = new Set([
	'png',
	'jpg',
	'jpeg',
	'gif',
	'webp',
	'ico',
	'bmp',
	'pdf',
	'zip',
	'gz',
	'tar',
	'7z',
	'mp3',
	'mp4',
	'mov',
	'wav',
	'woff',
	'woff2',
	'ttf',
	'otf',
	'db',
	'sqlite',
	'pyc',
	'so',
	'dylib',
	'exe',
	'bin',
	'dmg',
	'ds_store'
]);

function extensionOf(path: string): string {
	const name = (path.split('/').pop() ?? '').toLowerCase();
	const dot = name.lastIndexOf('.');
	if (dot > 0) return name.slice(dot + 1);
	// Dotfiles such as `.env` or `.gitignore` have no extension of their own.
	if (name.startsWith('.')) return '';
	return name;
}

/**
 * Picks the preview mode for a file from its extension so the editor can show
 * rendered markdown, highlighted source code, plain text, or a "no preview"
 * notice for binary assets.
 */
export function detectPreview(path: string): FilePreview {
	const ext = extensionOf(path);
	const markdownLabel = MARKDOWN_EXTENSIONS[ext];
	const code = CODE_EXTENSIONS[ext];

	if (markdownLabel) {
		return { kind: 'markdown', language: 'markdown', label: markdownLabel };
	}
	if (BINARY_EXTENSIONS.has(ext)) {
		return { kind: 'binary', language: 'plaintext', label: ext.toUpperCase() };
	}
	if (code) {
		return { kind: 'code', language: code.language, label: code.label };
	}
	return { kind: 'text', language: 'plaintext', label: ext ? ext.toUpperCase() : 'Text' };
}

/**
 * Drops a leading YAML frontmatter block so SKILL.md metadata is not rendered
 * as body text in the markdown preview.
 */
export function stripFrontmatter(text: string): string {
	if (!text.startsWith('---')) return text;
	const end = text.indexOf('\n---', 3);
	if (end === -1) return text;
	return text.slice(end + 4).replace(/^\s*\n/, '');
}
