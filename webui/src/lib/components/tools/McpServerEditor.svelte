<script lang="ts">
	import { untrack } from 'svelte';
	import { tools as toolsApi } from '$lib/apis/tools';
	import { toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { mcpServerSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Plus } from 'lucide-svelte';
	import type { MCPServer } from '$lib/types';

	interface Props {
		agent: string;
		/** An existing server to change; mount a fresh instance per server via `{#key}`. */
		editing?: MCPServer | null;
		onSaved: () => void;
		onCancel?: () => void;
	}

	let { agent, editing = null, onSaved, onCancel }: Props = $props();

	const { t } = useTranslation();
	const uid = $props.id();

	// The parent remounts this form per server, so the initial values are all it needs.
	let name = $state(untrack(() => editing?.name ?? ''));
	let command = $state(untrack(() => editing?.command ?? ''));
	let args = $state(untrack(() => (editing?.args ?? []).join(', ')));
	let env = $state('');
	let errors = $state<FieldErrors>({});
	let saving = $state(false);

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

	async function save() {
		const result = validate(mcpServerSchema, { name, command, args, env });
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		saving = true;
		try {
			const parsed = parseEnv(result.data.env);
			if (editing) {
				await toolsApi.patchMCP(agent, editing.name, {
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					...(Object.keys(parsed).length > 0 ? { env: parsed } : {})
				});
			} else {
				await toolsApi.createMCP(agent, {
					name: result.data.name,
					command: result.data.command,
					args: splitArgs(result.data.args ?? ''),
					env: parsed,
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
		<Input id="{uid}-name" bind:value={name} placeholder="playwright" disabled={!!editing} />
		<FieldError code={errors.name} />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-command">{t('myaaw.tools.command')}</Label>
		<Input id="{uid}-command" bind:value={command} placeholder="npx" />
		<FieldError code={errors.command} />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-args">{t('myaaw.tools.args')}</Label>
		<Input id="{uid}-args" bind:value={args} placeholder="-y, @playwright/mcp" />
	</div>
	<div class="grid gap-1">
		<Label for="{uid}-env">{t('myaaw.tools.env')}</Label>
		<textarea
			id="{uid}-env"
			bind:value={env}
			rows="3"
			placeholder="TOKEN=secret"
			class="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
		></textarea>
		<FieldError code={errors.env} />
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
