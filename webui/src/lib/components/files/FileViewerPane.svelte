<script lang="ts">
	import {
		Download,
		FileCode2,
		FileText,
		Image as ImageIcon,
		Loader2,
		Pencil,
		Save,
		X
	} from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { filesStore } from '$lib/stores/files.svelte';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { detectPreview } from '$lib/utils/filePreview';
	import { getErrorMessage } from '$lib/utils/api';

	interface Props {
		agent: string;
		/** Virtual path such as `/project/src/main.ts`. */
		path: string;
		/** Conversation ID that owns the `/project` root. */
		conversationId?: string;
		/** When hosted inside ProjectWorkspacePane, the tab bar owns the top header & close control. */
		embedded?: boolean;
		onClose: () => void;
		onSaved?: () => void;
	}

	let { agent, path, conversationId = '', embedded = false, onClose, onSaved }: Props = $props();

	const { t } = useTranslation();

	let content = $state('');
	let draft = $state('');
	let size = $state(0);
	let loading = $state(false);
	let saving = $state(false);
	let editing = $state(false);
	let error = $state('');
	let loadKey = '';
	let gutter = $state(12);
	let root = $state<HTMLElement | null>(null);
	let editGutter = $state<HTMLElement | null>(null);
	let editArea = $state<HTMLTextAreaElement | null>(null);

	const draftLineCount = $derived(draft.length === 0 ? 1 : draft.split('\n').length);

	function syncEditScroll() {
		if (editGutter && editArea) {
			editGutter.scrollTop = editArea.scrollTop;
		}
	}

	const preview = $derived(detectPreview(path));
	const fileName = $derived(path.split('/').pop() || path);
	const displayPath = $derived(path.replace(/^\/project\/?/, '') || '/');
	const mediaUrl = $derived(filesStore.rawUrl(agent, path, { conversationId }));

	function measureGutter() {
		if (embedded || !root) {
			gutter = 12;
			return;
		}
		const bar = document.querySelector('header');
		const rootRect = root.getBoundingClientRect();
		if (!bar) {
			gutter = 12;
			return;
		}
		const barRect = bar.getBoundingClientRect();
		if (barRect.bottom <= rootRect.top + 1) {
			gutter = 12;
			return;
		}
		const edges = [...bar.querySelectorAll('button, a')]
			.map((el) => el.getBoundingClientRect())
			.filter((rect) => rect.width > 0 && rect.left > window.innerWidth / 2)
			.map((rect) => rect.left);
		const actionLeft = edges.length > 0 ? Math.min(...edges) : window.innerWidth;
		gutter = Math.max(12, Math.round(rootRect.right - actionLeft + 8));
	}

	$effect(() => {
		const el = root;
		if (!el || embedded) {
			gutter = 12;
			return;
		}
		measureGutter();
		const observer = new ResizeObserver(() => measureGutter());
		observer.observe(el);
		const bar = document.querySelector('header');
		if (bar) observer.observe(bar);
		window.addEventListener('resize', measureGutter);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measureGutter);
		};
	});

	$effect(() => {
		const key = `${agent}:${conversationId}:${path}`;
		if (!agent || !path || key === loadKey) return;
		loadKey = key;
		void load();
	});

	async function load() {
		loading = true;
		error = '';
		editing = false;
		content = '';
		draft = '';
		size = 0;

		const kind = detectPreview(path).kind;
		if (
			kind === 'image' ||
			kind === 'video' ||
			kind === 'audio' ||
			kind === 'pdf' ||
			kind === 'binary'
		) {
			await ensureSigned(agent, [path], conversationId);
			loading = false;
			return;
		}

		try {
			const file = await filesStore.readFile(agent, path, conversationId);
			content = file.content ?? '';
			draft = content;
			size = file.size ?? 0;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	async function save() {
		saving = true;
		try {
			await filesStore.write(agent, path, draft, undefined, conversationId);
			content = draft;
			editing = false;
			toastStore.success(t('myaaw.workspace.file_saved'));
			onSaved?.();
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && !event.defaultPrevented) {
			if (editing) {
				event.preventDefault();
				editing = false;
				draft = content;
			} else if (!embedded) {
				event.preventDefault();
				onClose();
			}
		} else if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's' && editing) {
			event.preventDefault();
			void save();
		}
	}

	function formatSize(bytes: number): string {
		if (!bytes) return '';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div bind:this={root} class="flex h-full min-h-0 flex-col bg-background">
	<div
		class="flex min-h-14 shrink-0 items-center gap-2 border-b py-2 pl-3"
		style:padding-right="{gutter}px"
	>
		<span
			class="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground"
		>
			{#if preview.kind === 'image'}
				<ImageIcon class="h-4 w-4" />
			{:else if preview.kind === 'code'}
				<FileCode2 class="h-4 w-4" />
			{:else}
				<FileText class="h-4 w-4" />
			{/if}
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<p class="truncate text-sm font-medium">{fileName}</p>
				<Badge variant="outline" class="shrink-0 text-[10px]">{preview.label}</Badge>
			</div>
			<p class="truncate font-mono text-[11px] text-muted-foreground" title={displayPath}>
				{displayPath}{#if size}
					· {formatSize(size)}{/if}
			</p>
		</div>

		{#if preview.editable && !error && !loading}
			{#if editing}
				<Button
					variant="ghost"
					size="sm"
					disabled={saving}
					onclick={() => {
						editing = false;
						draft = content;
					}}
				>
					{t('common.cancel')}
				</Button>
				<Button size="sm" disabled={saving} onclick={() => void save()}>
					{#if saving}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<Save class="h-3.5 w-3.5" />
					{/if}
					{t('common.save')}
				</Button>
			{:else}
				<Button variant="outline" size="sm" onclick={() => (editing = true)}>
					<Pencil class="h-3.5 w-3.5" />
					{t('common.edit')}
				</Button>
			{/if}
		{/if}

		<Button
			variant="outline"
			size="icon-sm"
			onclick={() => void filesStore.download(agent, path, conversationId)}
			title={t('myaaw.chat.download_file')}
			aria-label={t('myaaw.chat.download_file')}
		>
			<Download class="h-4 w-4" />
		</Button>
		{#if !embedded}
			<Button
				variant="ghost"
				size="icon-sm"
				onclick={onClose}
				title={t('common.close')}
				aria-label={t('common.close')}
			>
				<X class="h-4 w-4" />
			</Button>
		{/if}
	</div>

	<div class="min-h-0 flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center gap-2 p-6 text-sm text-muted-foreground">
				<Loader2 class="h-4 w-4 animate-spin" />
				{t('common.loading')}
			</div>
		{:else if error}
			<div class="p-6 text-sm text-destructive">{error}</div>
		{:else if preview.kind === 'image'}
			<div class="flex h-full min-h-48 items-center justify-center bg-muted/20 p-4">
				<img
					src={mediaUrl}
					alt={fileName}
					class="max-h-full max-w-full rounded-lg border object-contain"
				/>
			</div>
		{:else if preview.kind === 'pdf'}
			<iframe src={mediaUrl} title={fileName} class="h-full w-full border-0"></iframe>
		{:else if preview.kind === 'video'}
			<div class="flex h-full items-center justify-center bg-muted/20 p-4">
				<!-- svelte-ignore a11y_media_has_caption -->
				<video src={mediaUrl} controls class="max-h-full max-w-full rounded-lg"></video>
			</div>
		{:else if preview.kind === 'audio'}
			<div class="flex h-full items-center justify-center p-6">
				<audio src={mediaUrl} controls class="w-full max-w-md"></audio>
			</div>
		{:else if preview.kind === 'binary'}
			<div
				class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground"
			>
				<p>{t('myaaw.workspace.binary_preview_unavailable')}</p>
				<Button
					variant="outline"
					size="sm"
					onclick={() => void filesStore.download(agent, path, conversationId)}
				>
					<Download class="h-4 w-4" />
					{t('myaaw.chat.download_file')}
				</Button>
			</div>
		{:else if editing}
			<div class="flex h-full min-h-0 bg-background font-mono text-xs leading-6">
				<div
					bind:this={editGutter}
					aria-hidden="true"
					class="flex shrink-0 flex-col overflow-hidden border-r border-border/50 bg-muted/30 py-3 pr-2.5 pl-3 text-right text-[11px] text-muted-foreground/70 select-none tabular-nums"
				>
					{#each { length: draftLineCount } as _, idx (idx)}
						<span class="leading-6">{idx + 1}</span>
					{/each}
				</div>
				<textarea
					bind:this={editArea}
					bind:value={draft}
					onscroll={syncEditScroll}
					spellcheck="false"
					wrap="off"
					class="h-full min-w-0 flex-1 resize-none overflow-auto bg-background py-3 pr-4 pl-3 font-mono text-xs leading-6 whitespace-pre outline-none"
				></textarea>
			</div>
		{:else if preview.kind === 'markdown'}
			<div class="mx-auto max-w-3xl p-6">
				<Markdown {content} />
			</div>
		{:else}
			<div class="h-full p-2">
				<CodePreview code={content} language={preview.language} />
			</div>
		{/if}
	</div>
</div>
