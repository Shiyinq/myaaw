<script lang="ts">
	import { Brain, Loader2, Pencil, Sparkles, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { context as contextApi } from '$lib/apis/context';
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
			const file = await contextApi.read(agent, path);
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
			const file = await contextApi.write(agent, path, content);
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
	file list stays visible while the file is read and edited.
-->
<div class="flex h-full min-h-0 flex-col" data-focus-editor>
	<div class="flex shrink-0 items-center gap-2 border-b p-2">
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
