<script lang="ts">
	import { untrack } from 'svelte';
	import { toolsStore } from '$lib/stores/tools.svelte';
	import { toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { customToolSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Plus } from 'lucide-svelte';
	import type { CustomTool } from '$lib/types';

	interface Props {
		agent: string;
		/** An existing tool to change; mount a fresh instance per tool via `{#key}`. */
		editing?: CustomTool | null;
		onSaved: () => void;
		onCancel?: () => void;
	}

	let { agent, editing = null, onSaved, onCancel }: Props = $props();

	const { t } = useTranslation();
	const uid = $props.id();

	// The parent remounts this form per tool, so the initial values are all it needs.
	let name = $state(untrack(() => editing?.name ?? ''));
	let command = $state(untrack(() => editing?.command ?? ''));
	let args = $state(untrack(() => (editing?.args ?? []).join(', ')));
	let schema = $state(
		untrack(() => (editing?.schema ? JSON.stringify(editing.schema, null, 2) : ''))
	);
	let errors = $state<FieldErrors>({});
	let saving = $state(false);

	function splitArgs(value: string) {
		return value
			.split(',')
			.map((part) => part.trim())
			.filter(Boolean);
	}

	async function save() {
		const result = validate(customToolSchema, { name, command, args, schema });
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		saving = true;
		try {
			const parsed = result.data.schema.trim() ? JSON.parse(result.data.schema) : undefined;
			if (editing) {
				await toolsStore.patchCustom(agent, editing.name, {
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
			onSaved();
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}
</script>

<form
	class="space-y-3"
	onsubmit={(event) => {
		event.preventDefault();
		void save();
	}}
>
	<div class="grid gap-1">
		<Label for="{uid}-name">{t('myaaw.tools.name')}</Label>
		<Input id="{uid}-name" bind:value={name} placeholder="hello_helper" disabled={!!editing} />
		<FieldError code={errors.name} />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-command">{t('myaaw.tools.command')}</Label>
		<Input id="{uid}-command" bind:value={command} placeholder="node" />
		<FieldError code={errors.command} />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-args">{t('myaaw.tools.args')}</Label>
		<Input id="{uid}-args" bind:value={args} placeholder="/path/script.js, --flag" />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-schema">{t('myaaw.tools.schema')}</Label>
		<textarea
			id="{uid}-schema"
			bind:value={schema}
			rows="4"
			placeholder={'{"type":"function","function":{"name":"hello_helper","parameters":{}}}'}
			class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
		></textarea>
		<FieldError code={errors.schema} />
	</div>
	<div class="flex gap-2">
		<Button type="submit" size="sm" loading={saving}>
			{#if editing}
				{t('common.save')}
			{:else}
				<Plus class="mr-2 h-4 w-4" />
				{t('myaaw.tools.create')}
			{/if}
		</Button>
		{#if editing && onCancel}
			<Button type="button" size="sm" variant="ghost" onclick={onCancel}>
				{t('common.cancel')}
			</Button>
		{/if}
	</div>
</form>
