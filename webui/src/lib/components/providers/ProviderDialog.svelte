<script lang="ts">
	import { untrack } from 'svelte';
	import { Cpu, Loader2, Plus, Save, Wand2 } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import ChatGPTConnection from '$lib/components/common/ChatGPTConnection.svelte';
	import { providersStore } from '$lib/stores/providers.svelte';
	import { capabilityFor } from '$lib/stores/modelCatalog';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { providerSchema } from '$lib/schemas';
	import type {
		ModelCapabilitiesView,
		ModelListView,
		ProviderConnection,
		ProviderView
	} from '$lib/types';

	interface Props {
		open: boolean;
		agent: string;
		editing: ProviderView | null;
		modelCapabilities: ModelCapabilitiesView | null;
		onOpenChange: (open: boolean) => void;
	}

	let { open, agent, editing, modelCapabilities, onOpenChange }: Props = $props();
	const { t } = useTranslation();

	let saving = $state(false);
	let discovering = $state(false);
	let formError = $state('');

	let name = $state('');
	let type = $state<'openai' | 'gemini' | 'openai_chatgpt'>('openai');
	let baseUrl = $state('');
	let apiKey = $state('');
	let defaultModel = $state('');
	let thinkingLevel = $state('');
	let discovered = $state<ModelListView | null>(null);
	let errors = $state<FieldErrors>({});
	let credentialId = $state('');
	let connection = $state<ProviderConnection | undefined>();
	let connecting = $state(false);
	let formVersion = $state(0);

	const isChatGPT = $derived(type === 'openai_chatgpt');
	const connected = $derived(
		!!credentialId &&
			!!connection?.configured &&
			['connected', 'refresh_required'].includes(connection.status)
	);

	const typeOptions: { id: 'openai' | 'gemini' | 'openai_chatgpt'; label: string; hint: string }[] =
		$derived([
			{ id: 'openai', label: 'OpenAI', hint: 'OpenAI / compatible' },
			{ id: 'gemini', label: 'Gemini', hint: 'Google AI Studio' },
			{ id: 'openai_chatgpt', label: t('myaaw.chatgpt.title'), hint: 'OAuth' }
		]);

	const capability = $derived(capabilityFor(modelCapabilities, defaultModel));
	const thinkingSupported = $derived(capability?.capabilities?.thinking === true);
	const thinkingChoices = $derived.by(() => {
		if (!thinkingSupported) return [];
		const values = [...(capability?.thinking?.effort_values ?? [])];
		if (
			capability?.thinking?.can_disable === true &&
			!values.some((value) => value === 'none' || value === 'off')
		) {
			values.unshift('off');
		}
		return values;
	});
	const effectiveThinking = $derived(
		thinkingSupported && thinkingChoices.includes(thinkingLevel) ? thinkingLevel : ''
	);

	function thinkingLabel(value: string) {
		if (value === 'none' || value === 'off') return t('myaaw.chat.thinking_off');
		if (value === 'xhigh') return 'XHigh';
		return value.charAt(0).toUpperCase() + value.slice(1);
	}

	// Reset or hydrate form cleanly whenever the dialog opens or switches target.
	$effect(() => {
		if (!open) return;
		const target = editing;
		untrack(() => {
			formVersion += 1;
			credentialId = target?.credential_id ?? '';
			connection = target?.connection;
			connecting = false;
			name = target?.name ?? '';
			type =
				target?.type === 'openai_chatgpt'
					? 'openai_chatgpt'
					: target?.type === 'gemini'
						? 'gemini'
						: 'openai';
			baseUrl = target?.base_url ?? '';
			defaultModel = target?.default_model ?? '';
			thinkingLevel = target?.thinking_level ?? '';
			apiKey = '';
			discovered = null;
			errors = {};
			formError = '';
			saving = false;
		});
	});

	function selectType(next: 'openai' | 'gemini' | 'openai_chatgpt') {
		if (type === next) return;
		type = next;
		formVersion += 1;
		defaultModel = '';
		thinkingLevel = '';
		credentialId = '';
		connection = undefined;
		connecting = false;
		discovered = null;
		errors = {};
		formError = '';
	}

	async function discover() {
		if (!agent || connecting || (isChatGPT && !connected)) return;
		const version = formVersion;
		const currentAgent = agent;
		discovering = true;
		discovered = null;
		try {
			const view = isChatGPT
				? await providersStore.modelsForCredentials({
						type: 'openai_chatgpt',
						credential_id: credentialId
					})
				: editing && !apiKey
					? await providersStore.models(agent, editing.name)
					: await providersStore.modelsForCredentials({
							type,
							base_url: baseUrl || undefined,
							api_key: apiKey || undefined
						});
			if (version !== formVersion || currentAgent !== agent) return;
			discovered = view;
			if (!defaultModel && discovered.models.length > 0) {
				defaultModel = discovered.models[0];
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			discovering = false;
		}
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!agent || connecting || saving) return;

		const saveAgent = agent;
		const version = formVersion;
		if (isChatGPT && !connected) {
			formError = t('myaaw.chatgpt.connect_required');
			return;
		}

		const result = validate(providerSchema, {
			name,
			type,
			base_url: isChatGPT ? undefined : baseUrl,
			api_key: isChatGPT ? undefined : apiKey,
			auth_mode: isChatGPT ? 'oauth' : undefined,
			credential_id: isChatGPT ? credentialId : undefined,
			default_model: defaultModel,
			thinking_level: effectiveThinking
		});
		if (!result.ok) {
			errors = result.errors;
			return;
		}

		errors = {};
		formError = '';
		saving = true;
		try {
			if (isChatGPT) {
				const payload = {
					type: 'openai_chatgpt',
					auth_mode: 'oauth' as const,
					credential_id: credentialId,
					default_model: result.data.default_model ?? '',
					thinking_level: effectiveThinking
				};
				if (editing) await providersStore.update(agent, editing.name, payload);
				else await providersStore.create(agent, { ...payload, name: result.data.name });
			} else if (editing) {
				await providersStore.update(agent, editing.name, {
					type: result.data.type,
					base_url: result.data.base_url ?? '',
					api_key: result.data.api_key?.trim() || undefined,
					default_model: result.data.default_model ?? '',
					thinking_level: effectiveThinking
				});
			} else {
				await providersStore.create(agent, {
					name: result.data.name,
					type: result.data.type,
					base_url: result.data.base_url || undefined,
					api_key: result.data.api_key || undefined,
					default_model: result.data.default_model || undefined,
					thinking_level: effectiveThinking || undefined
				});
			}
			toastStore.success(t('myaaw.providers.saved'));
			if (saveAgent !== agent || version !== formVersion) return;
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
			if (editing) event.preventDefault();
		}}
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<Cpu class="h-4 w-4 text-primary" />
				{editing ? t('myaaw.providers.edit') : t('myaaw.providers.create')}
			</Dialog.Title>
			<Dialog.Description>
				{isChatGPT
					? t('myaaw.chatgpt.description')
					: editing
						? t('myaaw.providers.edit_desc')
						: t('myaaw.providers.create_desc')}
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
					<Label for={editing ? undefined : 'provider-name'}>{t('myaaw.providers.name')}</Label>
					{#if editing}
						<div
							aria-disabled="true"
							class="flex h-9 w-full cursor-not-allowed items-center rounded-md border border-input bg-muted/50 px-3 text-sm text-muted-foreground select-none"
						>
							{name}
						</div>
					{:else}
						<Input id="provider-name" bind:value={name} placeholder="deepseek" />
					{/if}
					<FieldError code={errors.name} />
				</div>

				<!-- Provider Type card picker -->
				<div class="space-y-2">
					<Label>{t('myaaw.providers.type')}</Label>
					<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-3">
						{#each typeOptions as opt (opt.id)}
							{@const active = type === opt.id}
							<button
								type="button"
								onclick={() => selectType(opt.id)}
								aria-pressed={active}
								class={[
									'flex flex-col rounded-xl border p-2.5 text-left transition-colors',
									active
										? 'border-primary bg-primary/8 text-foreground'
										: 'border-border hover:bg-muted/60'
								]}
							>
								<span class="text-xs font-semibold">{opt.label}</span>
								<span class="mt-0.5 text-[10px] text-muted-foreground">{opt.hint}</span>
							</button>
						{/each}
					</div>
				</div>

				<!-- Connection / Credentials -->
				<div class="space-y-3 rounded-xl border p-3">
					{#if isChatGPT}
						{#key `${agent}:${formVersion}`}
							<ChatGPTConnection
								{agent}
								providerId={editing?.name || undefined}
								bind:credentialId
								bind:connection
								bind:busy={connecting}
								ondisconnect={() => void providersStore.refresh()}
								onchange={() => {
									discovered = null;
								}}
							/>
						{/key}
					{:else}
						<div class="space-y-1.5">
							<Label for="provider-base-url" class="text-xs">{t('myaaw.providers.base_url')}</Label>
							<Input
								id="provider-base-url"
								bind:value={baseUrl}
								placeholder="https://api.deepseek.com"
								class="font-mono text-xs"
							/>
							<FieldError code={errors.base_url} />
						</div>
						<div class="space-y-1.5">
							<Label for="provider-api-key" class="text-xs">{t('myaaw.providers.api_key')}</Label>
							<Input
								id="provider-api-key"
								type="password"
								bind:value={apiKey}
								placeholder={editing ? t('myaaw.providers.api_key_keep') : 'sk-...'}
							/>
						</div>
					{/if}
				</div>

				<!-- Model discovery & selection -->
				<div class="space-y-3 rounded-xl border p-3">
					<div class="flex items-center justify-between gap-2">
						<Label
							for={discovered && discovered.models.length > 0
								? 'provider-model'
								: 'provider-model-manual'}
							class="text-xs"
						>
							{t('myaaw.providers.model')}
						</Label>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onclick={discover}
							loading={discovering}
							disabled={connecting || (isChatGPT && !connected)}
							class="h-7 gap-1.5 px-2.5 text-xs"
						>
							<Wand2 class="h-3.5 w-3.5" />
							{t('myaaw.providers.discover')}
						</Button>
					</div>

					{#if discovered?.warning}
						<p class="text-[11px] text-amber-600 dark:text-amber-400">{discovered.warning}</p>
					{/if}

					{#if discovered && discovered.models.length > 0}
						<select
							id="provider-model"
							bind:value={defaultModel}
							class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							{#each discovered.models as model (model)}
								<option value={model}>{model}</option>
							{/each}
						</select>
					{:else}
						<Input
							id="provider-model-manual"
							bind:value={defaultModel}
							placeholder={isChatGPT ? t('myaaw.chatgpt.model_hint') : 'deepseek-chat'}
							class="font-mono text-xs"
						/>
					{/if}
					<FieldError code={errors.default_model} />

					{#if thinkingSupported}
						<div class="space-y-1.5 pt-1">
							<Label for="provider-thinking" class="text-xs">{t('myaaw.providers.thinking')}</Label>
							<select
								id="provider-thinking"
								bind:value={thinkingLevel}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
							>
								<option value="">{t('myaaw.chat.thinking_default')}</option>
								{#each thinkingChoices as value (value)}
									<option {value}>{thinkingLabel(value)}</option>
								{/each}
							</select>
							<p class="text-[11px] text-muted-foreground">
								{t('myaaw.providers.thinking_hint')}
							</p>
						</div>
					{/if}
				</div>

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
				<Button type="submit" disabled={saving || connecting || (isChatGPT && !connected)}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else if editing}
						<Save class="mr-1.5 h-3.5 w-3.5" />
					{:else}
						<Plus class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{editing ? t('common.save') : t('myaaw.providers.create')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
