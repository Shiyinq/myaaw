<script lang="ts">
	import { untrack } from 'svelte';
	import { FileCode2, Loader2, Plug, Plus, Save } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { toolsStore } from '$lib/stores/tools.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { customToolSchema, mcpServerSchema } from '$lib/schemas';
	import type { CustomTool, MCPServer } from '$lib/types';

	interface Props {
		open: boolean;
		agent: string;
		kind: 'custom' | 'mcp';
		editingCustom?: CustomTool | null;
		editingMCP?: MCPServer | null;
		onOpenChange: (open: boolean) => void;
	}

	let {
		open,
		agent,
		kind,
		editingCustom = null,
		editingMCP = null,
		onOpenChange
	}: Props = $props();
	const { t } = useTranslation();

	let saving = $state(false);
	let formError = $state('');
	let errors = $state<FieldErrors>({});

	let name = $state('');
	let command = $state('');
	let args = $state('');
	let schema = $state('');
	let env = $state('');

	const isEditing = $derived(kind === 'custom' ? editingCustom !== null : editingMCP !== null);

	// Reset or hydrate fields whenever the modal opens.
	$effect(() => {
		if (!open) return;
		const currentKind = kind;
		const customTarget = editingCustom;
		const mcpTarget = editingMCP;
		untrack(() => {
			if (currentKind === 'custom') {
				name = customTarget?.name ?? '';
				command = customTarget?.command ?? '';
				args = (customTarget?.args ?? []).join(', ');
				schema = customTarget?.schema ? JSON.stringify(customTarget.schema, null, 2) : '';
				env = '';
			} else {
				name = mcpTarget?.name ?? '';
				command = mcpTarget?.command ?? '';
				args = (mcpTarget?.args ?? []).join(', ');
				schema = '';
				env = '';
			}
			errors = {};
			formError = '';
			saving = false;
		});
	});

	function splitArgs(value: string) {
		return value
			.split(',')
			.map((part) => part.trim())
			.filter(Boolean);
	}

	function parseEnv(value: string) {
		const parsed: Record<string, string> = {};
		for (const line of value.split('\n')) {
			const index = line.indexOf('=');
			if (index > 0) parsed[line.slice(0, index).trim()] = line.slice(index + 1).trim();
		}
		return parsed;
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!agent || saving) return;

		if (kind === 'custom') {
			const result = validate(customToolSchema, { name, command, args, schema });
			if (!result.ok) {
				errors = result.errors;
				return;
			}
			errors = {};
			formError = '';
			saving = true;
			try {
				const parsed = result.data.schema.trim() ? JSON.parse(result.data.schema) : undefined;
				if (editingCustom) {
					await toolsStore.patchCustom(agent, editingCustom.name, {
						command: result.data.command,
						args: splitArgs(result.data.args ?? ''),
						schema: parsed
					});
				} else {
					await toolsStore.createCustom(agent, {
						name: result.data.name,
						command: result.data.command,
						args: splitArgs(result.data.args ?? ''),
						schema: parsed,
						enabled: true
					});
				}
				toastStore.success(t('myaaw.tools.saved'));
				onOpenChange(false);
			} catch (err) {
				formError = getErrorMessage(err);
			} finally {
				saving = false;
			}
			return;
		}

		const result = validate(mcpServerSchema, { name, command, args, env });
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		formError = '';
		saving = true;
		try {
			const parsed = parseEnv(result.data.env);
			if (editingMCP) {
				await toolsStore.patchMCP(agent, editingMCP.name, {
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					...(Object.keys(parsed).length > 0 ? { env: parsed } : {})
				});
			} else {
				await toolsStore.createMCP(agent, {
					name: result.data.name,
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					env: parsed,
					enabled: true
				});
			}
			toastStore.success(t('myaaw.tools.saved'));
			onOpenChange(false);
		} catch (err) {
			formError = getErrorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		onOpenAutoFocus={(event) => {
			if (isEditing) event.preventDefault();
		}}
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				{#if kind === 'custom'}
					<FileCode2 class="h-4 w-4 text-primary" />
					{t('myaaw.tools.custom')}
				{:else}
					<Plug class="h-4 w-4 text-primary" />
					{t('myaaw.tools.mcp')}
				{/if}
			</Dialog.Title>
			<Dialog.Description>
				{kind === 'custom' ? t('myaaw.tools.custom_desc') : t('myaaw.tools.mcp_desc')}
			</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={handleSubmit}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<!-- Name -->
				<div class="space-y-1.5">
					<Label for={isEditing ? undefined : 'tool-name'}>{t('myaaw.tools.name')}</Label>
					{#if isEditing}
						<div
							aria-disabled="true"
							class="flex h-9 w-full cursor-not-allowed items-center rounded-md border border-input bg-muted/50 px-3 font-mono text-xs text-muted-foreground select-none"
						>
							{name}
						</div>
					{:else}
						<Input
							id="tool-name"
							bind:value={name}
							placeholder={kind === 'custom' ? 'hello_helper' : 'playwright'}
							class="font-mono text-xs"
						/>
					{/if}
					<FieldError code={errors.name} />
				</div>

				<!-- Command & Arguments -->
				<div class="space-y-3 rounded-xl border p-3">
					<div class="space-y-1.5">
						<Label for="tool-command" class="text-xs">{t('myaaw.tools.command')}</Label>
						<Input
							id="tool-command"
							bind:value={command}
							placeholder={kind === 'custom' ? 'node' : 'npx'}
							class="font-mono text-xs"
						/>
						<FieldError code={errors.command} />
					</div>
					<div class="space-y-1.5">
						<Label for="tool-args" class="text-xs">{t('myaaw.tools.args')}</Label>
						<Input
							id="tool-args"
							bind:value={args}
							placeholder={kind === 'custom' ? '/path/script.js, --flag' : '-y, @playwright/mcp'}
							class="font-mono text-xs"
						/>
					</div>
				</div>

				{#if kind === 'custom'}
					<div class="space-y-1.5">
						<Label for="tool-schema">{t('myaaw.tools.schema')}</Label>
						<textarea
							id="tool-schema"
							bind:value={schema}
							rows={5}
							placeholder={'{"type":"function","function":{"name":"hello_helper","parameters":{}}}'}
							class="w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
						></textarea>
						<FieldError code={errors.schema} />
					</div>
				{:else}
					<div class="space-y-1.5">
						<Label for="tool-env">{t('myaaw.tools.env')}</Label>
						<textarea
							id="tool-env"
							bind:value={env}
							rows={4}
							placeholder="TOKEN=secret"
							class="w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
						></textarea>
						<FieldError code={errors.env} />
					</div>
				{/if}

				{#if formError}
					<p
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{formError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => onOpenChange(false)}
					disabled={saving}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={saving}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else if isEditing}
						<Save class="mr-1.5 h-3.5 w-3.5" />
					{:else}
						<Plus class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{isEditing ? t('common.save') : t('myaaw.tools.create')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
