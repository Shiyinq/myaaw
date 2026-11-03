<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import Switch from '$lib/components/common/Switch.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import CodePreview from '$lib/components/common/CodePreview.svelte';
	import { detectPreview, stripFrontmatter } from '$lib/utils/filePreview';
	import { getErrorMessage } from '$lib/utils/api';
	import { skills as skillsApi } from '$lib/apis/skills';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Loader2, Pencil, Plug, Sparkles, SquareTerminal, Trash2, Wrench } from 'lucide-svelte';
	import type { SkillDetail, SkillFileContent } from '$lib/types';
	import type { PluginTarget } from '$lib/types/plugins';

	interface Props {
		agent: string;
		target: PluginTarget;
		/** The skill body, fetched by the page; other kinds already carry everything. */
		detail?: SkillDetail | null;
		loading?: boolean;
		onToggle: (enabled: boolean) => void;
		onEdit?: () => void;
		onDelete?: () => void;
		onClose: () => void;
	}

	let {
		agent,
		target,
		detail = null,
		loading = false,
		onToggle,
		onEdit,
		onDelete,
		onClose
	}: Props = $props();

	const { t } = useTranslation();

	let filePath = $state('');
	let fileContent = $state<SkillFileContent | null>(null);
	let fileLoading = $state(false);
	let fileError = $state('');

	const name = $derived(
		target.kind === 'skill'
			? target.skill.display_name || target.skill.name
			: target.kind === 'builtin'
				? target.tool.name
				: target.kind === 'custom'
					? target.tool.name
					: target.server.name
	);

	const enabled = $derived(
		target.kind === 'skill'
			? target.skill.enabled
			: target.kind === 'builtin'
				? target.tool.enabled
				: target.kind === 'custom'
					? target.tool.enabled
					: target.server.enabled
	);

	const description = $derived(
		target.kind === 'skill'
			? target.skill.description
			: target.kind === 'builtin'
				? (target.tool.description ?? '')
				: ''
	);

	const groupLabel = $derived(
		target.kind === 'skill'
			? t('myaaw.plugins.skills')
			: target.kind === 'builtin'
				? t('myaaw.plugins.tools')
				: target.kind === 'custom'
					? t('myaaw.plugins.custom')
					: t('myaaw.plugins.mcp')
	);

	const icon = $derived(
		target.kind === 'skill'
			? Sparkles
			: target.kind === 'builtin'
				? Wrench
				: target.kind === 'custom'
					? SquareTerminal
					: Plug
	);

	// The body shown on the right starts on SKILL.md, like the skills page does.
	$effect(() => {
		const files = detail?.files ?? [];
		if (target.kind !== 'skill' || files.length === 0) return;
		filePath =
			files.find((file) => file.path.toUpperCase() === 'SKILL.MD')?.path ??
			files.find((file) => file.path.toLowerCase().endsWith('.md'))?.path ??
			files[0].path;
		fileContent = null;
		fileError = '';
	});

	const previewPath = $derived(fileContent?.path || filePath || 'SKILL.md');
	const preview = $derived(detectPreview(previewPath));
	const previewBinary = $derived(!!fileContent?.binary || preview.kind === 'binary');
	const previewText = $derived.by(() => {
		const raw = fileContent ? fileContent.content : (detail?.content ?? '');
		return preview.kind === 'markdown' ? stripFrontmatter(raw) : raw;
	});

	async function openFile(path: string) {
		if (target.kind !== 'skill' || fileLoading) return;
		filePath = path;
		fileLoading = true;
		fileError = '';
		try {
			fileContent = await skillsApi.file(agent, target.skill.name, path);
		} catch (err) {
			fileContent = null;
			fileError = getErrorMessage(err);
		} finally {
			fileLoading = false;
		}
	}

	function line(command: string, args?: string[]) {
		return [command, ...(args ?? [])].filter(Boolean).join(' ');
	}
</script>

{#snippet kindIcon()}
	{@const Icon = icon}
	<Icon class="h-5 w-5" />
{/snippet}

<Dialog.Root
	open={true}
	onOpenChange={(open) => {
		if (!open) onClose();
	}}
>
	<Dialog.Content class="max-h-[85dvh] min-w-0 gap-4 overflow-x-hidden overflow-y-auto sm:max-w-xl">
		<Dialog.Header>
			<div class="flex min-w-0 items-start gap-3 pr-8">
				<span
					class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-muted text-muted-foreground"
				>
					{@render kindIcon()}
				</span>
				<div class="min-w-0">
					<Dialog.Title class="truncate">{name}</Dialog.Title>
					<Dialog.Description class="text-xs">{groupLabel}</Dialog.Description>
				</div>
			</div>
		</Dialog.Header>

		<div class="min-w-0 space-y-4">
			{#if target.kind === 'skill' || target.kind === 'builtin'}
				<p class="text-sm break-words whitespace-pre-line text-muted-foreground">
					{description || t('myaaw.plugins.no_description')}
				</p>
			{/if}

			{#if target.kind === 'skill'}
				<div class="flex flex-wrap items-center gap-1.5">
					<Badge variant="secondary" class="text-[10px]">{target.skill.source}</Badge>
					{#if !target.skill.parse_ok}
						<Badge variant="destructive" class="text-[10px]">{t('myaaw.skills.invalid')}</Badge>
					{/if}
				</div>

				{#if loading}
					<p class="flex items-center gap-2 py-4 text-sm text-muted-foreground">
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{t('common.loading')}
					</p>
				{:else}
					<div class="min-w-0 space-y-1.5">
						<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
							{t('myaaw.skills.files')}
						</p>
						<div class="flex flex-wrap gap-1">
							{#each detail?.files ?? [] as file (file.path)}
								<button
									type="button"
									class={[
										'rounded-md border px-2 py-1 font-mono text-[11px] transition-colors',
										filePath === file.path ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
									]}
									onclick={() => openFile(file.path)}
								>
									{file.path}
								</button>
							{/each}
						</div>
					</div>

					<div class="min-w-0 rounded-md border border-input bg-muted/20">
						{#if fileLoading}
							<p class="flex items-center justify-center gap-2 py-8 text-xs text-muted-foreground">
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
								{t('common.loading')}
							</p>
						{:else if fileError}
							<p class="p-3 text-xs text-destructive">{fileError}</p>
						{:else if previewBinary}
							<p class="p-6 text-center text-xs text-muted-foreground">
								{t('myaaw.skills.binary')}
							</p>
						{:else if preview.kind === 'markdown'}
							<div class="max-h-80 min-w-0 overflow-x-hidden overflow-y-auto px-3 py-2">
								<Markdown content={previewText} />
							</div>
						{:else}
							<div class="max-h-80 overflow-auto">
								<CodePreview code={previewText} language={preview.language} />
							</div>
						{/if}
					</div>
				{/if}
			{/if}

			{#if target.kind === 'builtin'}
				<p class="text-xs text-muted-foreground">{t('myaaw.tools.builtin_desc')}</p>
			{/if}

			{#if target.kind === 'custom' || target.kind === 'mcp'}
				{@const entry = target.kind === 'custom' ? target.tool : target.server}
				<dl class="min-w-0 space-y-2 text-sm">
					<div class="grid min-w-0 gap-0.5">
						<dt class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
							{t('myaaw.tools.command')}
						</dt>
						<dd
							class="rounded-md border border-input bg-muted/20 px-2 py-1 font-mono text-xs break-all"
						>
							{line(entry.command, entry.args)}
						</dd>
					</div>
					{#if target.kind === 'custom' && target.tool.schema}
						<div class="grid min-w-0 gap-0.5">
							<dt class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.tools.schema')}
							</dt>
							<dd
								class="max-h-56 overflow-auto rounded-md border border-input bg-muted/20 px-2 py-1 font-mono text-[11px]"
							>
								<pre class="whitespace-pre-wrap break-all">{JSON.stringify(
										target.tool.schema,
										null,
										2
									)}</pre>
							</dd>
						</div>
					{/if}
					{#if target.kind === 'mcp' && target.server.env?.configured}
						<div class="grid min-w-0 gap-0.5">
							<dt class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
								{t('myaaw.plugins.environment')}
							</dt>
							<dd class="rounded-md border border-input bg-muted/20 px-2 py-1 font-mono text-xs">
								{target.server.env.masked || '••••••'}
							</dd>
						</div>
					{/if}
				</dl>
			{/if}

			<div class="flex items-center justify-between gap-3 rounded-lg border p-3">
				<div class="min-w-0">
					<p class="text-sm font-medium">
						{enabled ? t('myaaw.plugins.active') : t('myaaw.plugins.off')}
					</p>
					<p class="text-xs text-muted-foreground">{t('myaaw.plugins.toggle_desc')}</p>
				</div>
				<Switch checked={enabled} label={t('myaaw.plugins.toggle', { name })} onchange={onToggle} />
			</div>

			{#if onEdit || onDelete}
				<div class="flex flex-wrap justify-end gap-2">
					{#if onEdit}
						<Button variant="outline" size="sm" onclick={onEdit}>
							<Pencil class="mr-2 h-3.5 w-3.5" />
							{t('common.edit')}
						</Button>
					{/if}
					{#if onDelete}
						<Button variant="destructive" size="sm" onclick={onDelete}>
							<Trash2 class="mr-2 h-3.5 w-3.5" />
							{t('common.delete')}
						</Button>
					{/if}
				</div>
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>
