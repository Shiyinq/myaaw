<script lang="ts">
	import { Brain, Loader2, Pencil, Sparkles, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { contextStore } from '$lib/stores/context.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { colorOf } from '$lib/bloub';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';

	interface Props {
		agent: string;
		/** Home-relative path of the context file, e.g. `SOUL.md`. */
		path: string;
		onClose: () => void;
	}

	let { agent, path, onClose }: Props = $props();

	const { t } = useTranslation();

	/** The placeholder is this agent's, so it wears its colour. */
	const agentColor = $derived(
		colorOf(agentSelection.agents.find((entry) => entry.name === agent)?.agent_color)
	);

	let content = $state('');
	let loading = $state(false);
	let saving = $state(false);
	let editing = $state(false);
	let missing = $state(false);
	let error = $state('');
	let loadKey = '';
	/** Space kept free at the right so the actions never slide under the top bar. */
	let gutter = $state(8);
	let root = $state<HTMLElement | null>(null);

	/**
	 * The chat shell floats its own actions over the top-right of the page, and the
	 * editor column can reach the viewport edge (small screens, or the panel hidden).
	 * Keep the same distance from those buttons at every width instead of guessing a
	 * padding that only works in one layout.
	 */
	function measureGutter() {
		if (!root) return;
		const bar = document.querySelector('header');
		const columnRight = root.getBoundingClientRect().right;
		if (!bar) {
			gutter = 8;
			return;
		}
		const edges = [...bar.querySelectorAll('button')]
			.map((button) => button.getBoundingClientRect())
			.filter((rect) => rect.width > 0 && rect.left > window.innerWidth / 2)
			.map((rect) => rect.left);
		const actionLeft = edges.length > 0 ? Math.min(...edges) : window.innerWidth;
		gutter = Math.max(8, Math.round(columnRight - actionLeft + 8));
	}

	$effect(() => {
		const element = root;
		if (!element) return;
		measureGutter();
		const observer = new ResizeObserver(() => measureGutter());
		observer.observe(element);
		window.addEventListener('resize', measureGutter);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measureGutter);
		};
	});

	const isSoul = $derived(path === 'SOUL.md');
	const title = $derived(isSoul ? t('myaaw.focus.context_soul') : t('myaaw.focus.context_memory'));

	$effect(() => {
		const key = `${agent}:${path}`;
		if (key === loadKey) return;
		loadKey = key;
		void load();
	});

	async function load() {
		loading = true;
		error = '';
		missing = false;
		editing = false;
		try {
			const file = await contextStore.fetchFile(agent, path);
			content = file.content ?? '';
		} catch (err) {
			const status = (err as { status?: number }).status;
			content = '';
			if (status === 404) missing = true;
			else error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	function cancel() {
		editing = false;
		void load();
	}

	async function save() {
		saving = true;
		try {
			const file = await contextStore.write(agent, path, content);
			content = file.content ?? '';
			editing = false;
			missing = false;
			toastStore.success(t('myaaw.context.saved'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}
</script>

<!--
	The context editor takes over the chat column in focus mode, so the panel's
	file list stays visible while the file is read and edited. The chat shell floats
	its own actions over the right edge, so this row keeps its controls next to the
	title: they never sit under those buttons, at any width.
-->
<div class="flex h-full min-h-0 flex-col" data-focus-editor bind:this={root}>
	<div class="flex shrink-0 items-center gap-2 border-b p-2" style:padding-right="{gutter}px">
		<span
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"
		>
			{#if isSoul}
				<Sparkles class="h-4 w-4" />
			{:else}
				<Brain class="h-4 w-4" />
			{/if}
		</span>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium">{title}</p>
			<p class="truncate text-[11px] text-muted-foreground">
				{path}{#if missing}
					· {t('myaaw.focus.context_empty')}{/if}
			</p>
		</div>

		{#if editing}
			<Button variant="ghost" size="sm" disabled={saving} onclick={cancel}>
				{t('common.cancel')}
			</Button>
			<Button size="sm" disabled={saving} onclick={save}>
				{#if saving}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{/if}
				{t('common.save')}
			</Button>
		{:else}
			<Button variant="outline" size="sm" disabled={loading} onclick={() => (editing = true)}>
				<Pencil class="h-3.5 w-3.5" />
				{t('common.edit')}
			</Button>
		{/if}

		<Button
			variant="ghost"
			size="icon-sm"
			onclick={onClose}
			aria-label={t('myaaw.focus.panel_close')}
			title={t('myaaw.focus.panel_close')}
		>
			<X class="h-4 w-4" />
		</Button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto">
		{#if loading}
			<p class="p-4 text-sm text-muted-foreground">{t('common.loading')}</p>
		{:else if error}
			<p class="p-4 text-sm text-destructive">{error}</p>
		{:else if editing}
			<textarea
				bind:value={content}
				class="block min-h-full w-full resize-none bg-transparent p-4 font-mono text-sm leading-relaxed outline-none"
				spellcheck="false"
				aria-label={path}
			></textarea>
		{:else if content.trim()}
			<div class="mx-auto w-full max-w-3xl p-4">
				<Markdown {content} />
			</div>
		{:else}
			<div class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
				<Bloub mood="curieux" size={56} tight color={agentColor} />
				<p class="text-sm text-muted-foreground">{t('myaaw.focus.context_empty')}</p>
				<Button variant="outline" size="sm" onclick={() => (editing = true)}>
					<Pencil class="h-3.5 w-3.5" />
					{t('common.edit')}
				</Button>
			</div>
		{/if}
	</div>
</div>
