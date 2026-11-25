<script lang="ts">
	import { Save, Settings2 } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { runtimesManageStore } from '$lib/stores/runtimes.svelte';
	import { probeRuntimeOptions } from '$lib/stores/runtimeOptions';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { formatRuntimeModelLabel } from '$lib/utils/runtimeTools';
	import type { ManagedRuntimeView, RuntimeOptionItem, RuntimeOptions } from '$lib/types';

	interface Props {
		open?: boolean;
		runtime: ManagedRuntimeView | null;
		onSaved?: (updated: ManagedRuntimeView) => void;
	}

	let { open = $bindable(false), runtime, onSaved }: Props = $props();

	const { t } = useTranslation();

	const selectClass =
		'h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 disabled:opacity-60';

	let defCommand = $state('');
	let defArgs = $state('');
	let defModel = $state('');
	let defThinking = $state('');
	let defSessionMode = $state('');
	let defAuthSource = $state<'cli_login' | 'api_key'>('cli_login');
	let defApiKeyEnv = $state('');
	let defApiKey = $state('');
	let defClearApiKey = $state(false);
	let defPermissionMode = $state<'bridge' | 'auto' | 'read_only'>('bridge');
	let defMcpBridge = $state(true);
	let defApplyExisting = $state(false);
	let saving = $state(false);

	let probedOptions = $state<RuntimeOptions | null>(null);
	let probingOptions = $state(false);
	let probeSeq = 0;
	let wasOpen = false;

	function tOr(key: string, fallback: string): string {
		const translated = t(key);
		return translated === key ? fallback : translated;
	}

	function runtimeLabel(rt: ManagedRuntimeView | null): string {
		if (!rt) return '';
		if (rt.id === 'custom_acp') return t('myaaw.onboard.runtime_custom_acp');
		return rt.label;
	}

	function formatThinkingLabel(level: string): string {
		const normalized = level.trim().toLowerCase();
		if (normalized === 'off' || normalized === 'none') return t('myaaw.chat.thinking_off');
		if (normalized === 'xhigh') return 'XHigh';
		return level
			.trim()
			.replace(/[_-]+/g, ' ')
			.replace(/\b\w/g, (ch) => ch.toUpperCase());
	}

	function approvalLabel(item: RuntimeOptionItem): string {
		return tOr(`myaaw.chat.approval_runtime_${item.value}`, item.label);
	}

	function approvalDescription(item: RuntimeOptionItem | undefined): string {
		if (!item?.description) return '';
		return tOr(`myaaw.chat.approval_runtime_${item.value}_desc`, item.description);
	}

	function formatAgentSummary(names: string[]): string {
		if (names.length <= 3) return names.join(', ');
		return `${names.slice(0, 3).join(', ')} +${names.length - 3}`;
	}

	const parseArgs = (raw: string) =>
		raw
			.split(',')
			.map((v) => v.trim())
			.filter(Boolean);

	let defModelGroups = $derived.by(() => {
		const grouped = new Map<string, RuntimeOptionItem[]>();
		for (const item of probedOptions?.models.options ?? []) {
			const key = item.group ?? '';
			const bucket = grouped.get(key);
			if (bucket) bucket.push(item);
			else grouped.set(key, [item]);
		}
		return Array.from(grouped.entries()).map(([group, items]) => ({ group, items }));
	});

	let defModelKnown = $derived(
		(probedOptions?.models.options ?? []).some((item) => item.value === defModel)
	);

	let defThinkingOptions = $derived.by((): RuntimeOptionItem[] => {
		if (!probedOptions) return [];
		const models = probedOptions.models.options ?? [];
		if (models.some((m) => (m.thinking_levels?.length ?? 0) > 0)) {
			const selected = defModel || probedOptions.models.current || '';
			const match = models.find((m) => m.value === selected);
			if (match) {
				return (match.thinking_levels ?? []).map((lvl) => ({
					value: lvl,
					label: formatThinkingLabel(lvl)
				}));
			}
		}
		return probedOptions.thinking.options ?? [];
	});

	let defSessionModeOptions = $derived(probedOptions?.session_modes.options ?? []);
	let defApprovalOptions = $derived(probedOptions?.approval_modes.options ?? []);

	$effect(() => {
		if (open && !wasOpen && runtime) {
			wasOpen = true;
			populateFromRuntime(runtime);
			void triggerOptionsProbe(false);
		} else if (!open && wasOpen) {
			wasOpen = false;
			probeSeq++;
			probedOptions = null;
			probingOptions = false;
		}
	});

	// Clear stale thinking level if the selected model does not support it.
	$effect(() => {
		if (!defThinking || !probedOptions) return;
		const opts = defThinkingOptions;
		if (opts.length === 0 || !opts.some((item) => item.value === defThinking)) {
			defThinking = '';
		}
	});

	function populateFromRuntime(rt: ManagedRuntimeView) {
		const d = rt.defaults;
		defCommand = d.command || rt.default_command || '';
		defArgs = (d.args && d.args.length > 0 ? d.args : (rt.default_args ?? [])).join(', ');
		defModel = d.model ?? '';
		defThinking = d.thinking_level ?? '';
		defSessionMode = d.session_mode ?? '';
		defAuthSource =
			d.auth_source === 'api_key' || !rt.capabilities.supports_cli_auth ? 'api_key' : 'cli_login';
		defApiKeyEnv = d.api_key_env || rt.default_api_key_env || '';
		defApiKey = '';
		defClearApiKey = false;
		defPermissionMode =
			d.permission_mode === 'auto'
				? 'auto'
				: d.permission_mode === 'read_only'
					? 'read_only'
					: 'bridge';
		defMcpBridge = d.mcp_bridge ?? true;
		defApplyExisting = false;
	}

	async function triggerOptionsProbe(forceRefresh: boolean) {
		if (!runtime) return;
		const cmd = defCommand.trim();
		if (!cmd) {
			probedOptions = null;
			probingOptions = false;
			return;
		}
		const seq = ++probeSeq;
		probingOptions = true;
		try {
			const args = parseArgs(defArgs);
			const opts = await probeRuntimeOptions(
				{
					preset: runtime.id,
					type: runtime.type,
					command: cmd,
					args: args.length > 0 ? args : undefined,
					model: defModel.trim() || undefined,
					auth_source: defAuthSource,
					api_key_env: defApiKeyEnv.trim() || undefined,
					api_key: defApiKey.trim() || undefined
				},
				forceRefresh
			);
			if (seq !== probeSeq) return;
			probedOptions = opts;
		} catch (err) {
			if (seq !== probeSeq) return;
			probedOptions = null;
			if (forceRefresh) toastStore.error(getErrorMessage(err));
		} finally {
			if (seq === probeSeq) probingOptions = false;
		}
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!runtime || saving) return;
		saving = true;
		try {
			const updated = await runtimesManageStore.saveDefaults(runtime.id, {
				command: defCommand.trim(),
				args: parseArgs(defArgs),
				model: defModel.trim(),
				thinking_level: defThinking,
				session_mode: defSessionMode,
				auth_source: defAuthSource,
				api_key_env: defApiKeyEnv.trim(),
				api_key: defClearApiKey ? '' : defApiKey.trim() || undefined,
				clear_api_key: defClearApiKey || undefined,
				permission_mode: defPermissionMode,
				mcp_bridge: defMcpBridge,
				apply_to_agents: defApplyExisting ? runtime.used_by_agents : undefined
			});
			onSaved?.(updated);
			toastStore.success(
				t('myaaw.settings.runtimes_defaults_saved', { name: runtimeLabel(runtime) })
			);
			open = false;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-[calc(100vw-2rem)] sm:max-w-2xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<Settings2 class="h-4 w-4 shrink-0 text-primary" />
				{t('myaaw.settings.runtimes_defaults_title')} — {runtimeLabel(runtime)}
			</Dialog.Title>
			<Dialog.Description>
				{t('myaaw.settings.runtimes_defaults_desc', { name: runtimeLabel(runtime) })}
			</Dialog.Description>
		</Dialog.Header>

		{#if runtime}
			<form
				onsubmit={handleSubmit}
				autocomplete="off"
				class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
			>
				<div
					class="min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-3"
				>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid content-start gap-1.5">
							<Label for="rt-def-cmd">{t('myaaw.onboard.runtime_binary')}</Label>
							<Input
								id="rt-def-cmd"
								bind:value={defCommand}
								placeholder={runtime.default_command || 'agy'}
								onchange={() => void triggerOptionsProbe(false)}
							/>
						</div>
						<div class="grid content-start gap-1.5">
							<Label for="rt-def-args">{t('myaaw.onboard.runtime_args')}</Label>
							<Input
								id="rt-def-args"
								bind:value={defArgs}
								placeholder={(runtime.default_args ?? []).join(', ') || 'acp'}
								onchange={() => void triggerOptionsProbe(false)}
							/>
						</div>
					</div>

					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid content-start gap-1.5">
							<Label for="rt-def-model">{t('myaaw.onboard.model')}</Label>
							<select
								id="rt-def-model"
								class={selectClass}
								value={defModel}
								disabled={probingOptions && !probedOptions}
								onchange={(e) => (defModel = e.currentTarget.value)}
							>
								<option value="">
									{t('myaaw.onboard.runtime_cli_default')}{probedOptions?.models.current
										? ` (${formatRuntimeModelLabel(probedOptions.models.current, probedOptions.models.options)})`
										: ''}
								</option>
								{#if defModel && !defModelKnown}
									<option value={defModel}
										>{formatRuntimeModelLabel(defModel, probedOptions?.models.options)}</option
									>
								{/if}
								{#each defModelGroups as grp (grp.group)}
									{#if grp.group}
										<optgroup label={grp.group}>
											{#each grp.items as item (item.value)}
												<option value={item.value}>{item.label}</option>
											{/each}
										</optgroup>
									{:else}
										{#each grp.items as item (item.value)}
											<option value={item.value}>{item.label}</option>
										{/each}
									{/if}
								{/each}
							</select>
							<p class="text-[11px] text-muted-foreground">
								{#if probingOptions}
									{t('myaaw.onboard.runtime_models_loading')}
								{:else if probedOptions?.warning}
									{t('myaaw.onboard.runtime_models_error', { error: probedOptions.warning })}
								{:else if probedOptions && probedOptions.models.options.length === 0}
									{t('myaaw.onboard.runtime_models_empty')}
								{:else}
									{t('myaaw.onboard.runtime_models_live')}
								{/if}
								{#if defCommand.trim()}
									<button
										type="button"
										class="underline"
										onclick={() => void triggerOptionsProbe(true)}
									>
										{t('common.refresh')}
									</button>
								{/if}
							</p>
						</div>

						<div class="grid content-start gap-1.5">
							<Label for="rt-def-perm">{t('myaaw.onboard.runtime_approvals')}</Label>
							<select
								id="rt-def-perm"
								class={selectClass}
								value={defPermissionMode}
								disabled={defApprovalOptions.length === 0}
								onchange={(e) =>
									(defPermissionMode = e.currentTarget.value as 'bridge' | 'auto' | 'read_only')}
							>
								{#each defApprovalOptions as item (item.value)}
									<option value={item.value}>{approvalLabel(item)}</option>
								{:else}
									<option value={defPermissionMode}>
										{probingOptions
											? t('common.loading')
											: probedOptions
												? t('myaaw.onboard.runtime_approvals_managed_by_cli')
												: t('myaaw.onboard.runtime_set_binary_first')}
									</option>
								{/each}
							</select>
							{#if approvalDescription(defApprovalOptions.find((o) => o.value === defPermissionMode))}
								<p class="text-[11px] text-muted-foreground">
									{approvalDescription(
										defApprovalOptions.find((o) => o.value === defPermissionMode)
									)}
								</p>
							{/if}
						</div>
					</div>

					{#if defThinkingOptions.length > 0 || defSessionModeOptions.length > 0}
						<div class="grid gap-4 sm:grid-cols-2">
							{#if defThinkingOptions.length > 0}
								<div class="grid content-start gap-1.5">
									<Label for="rt-def-think">{t('myaaw.providers.thinking')}</Label>
									<select
										id="rt-def-think"
										class={selectClass}
										value={defThinking}
										onchange={(e) => (defThinking = e.currentTarget.value)}
									>
										<option value="">
											{t('myaaw.onboard.runtime_cli_default')}{probedOptions?.thinking.current
												? ` (${probedOptions.thinking.current})`
												: ''}
										</option>
										{#each defThinkingOptions as item (item.value)}
											<option value={item.value}>{item.label}</option>
										{/each}
									</select>
								</div>
							{/if}
							{#if defSessionModeOptions.length > 0}
								<div class="grid content-start gap-1.5">
									<Label for="rt-def-mode">{t('myaaw.onboard.runtime_mode')}</Label>
									<select
										id="rt-def-mode"
										class={selectClass}
										value={defSessionMode}
										onchange={(e) => (defSessionMode = e.currentTarget.value)}
									>
										<option value="">
											{t('myaaw.onboard.runtime_cli_default')}{probedOptions?.session_modes.current
												? ` (${probedOptions.session_modes.current})`
												: ''}
										</option>
										{#each defSessionModeOptions as item (item.value)}
											<option value={item.value}>{item.label}</option>
										{/each}
									</select>
								</div>
							{/if}
						</div>
					{/if}

					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid content-start gap-1.5">
							<Label for="rt-def-auth">{t('myaaw.onboard.runtime_auth_mode')}</Label>
							<select
								id="rt-def-auth"
								class={selectClass}
								value={defAuthSource}
								onchange={(e) => (defAuthSource = e.currentTarget.value as 'cli_login' | 'api_key')}
							>
								{#if runtime.capabilities.supports_cli_auth}
									<option value="cli_login">{t('myaaw.onboard.runtime_auth_cli_login')}</option>
								{/if}
								<option value="api_key">{t('myaaw.onboard.runtime_auth_api_key')}</option>
							</select>
						</div>

						{#if defAuthSource === 'api_key'}
							<div class="grid content-start gap-1.5">
								<Label for="rt-def-keyenv">{t('myaaw.onboard.runtime_api_key_env')}</Label>
								<Input
									id="rt-def-keyenv"
									bind:value={defApiKeyEnv}
									placeholder={runtime.default_api_key_env || 'OPENAI_API_KEY'}
								/>
							</div>
						{/if}
					</div>

					{#if defAuthSource === 'api_key'}
						<div class="grid content-start gap-1.5">
							<div class="flex items-center justify-between gap-2">
								<Label for="rt-def-key">{t('myaaw.onboard.runtime_api_key')}</Label>
								{#if runtime.defaults.api_key.configured && !defClearApiKey}
									<button
										type="button"
										class="text-[11px] text-destructive hover:underline"
										onclick={() => {
											defClearApiKey = true;
											defApiKey = '';
										}}
									>
										{t('myaaw.settings.runtimes_clear_api_key')}
									</button>
								{/if}
							</div>
							<Input
								id="rt-def-key"
								type="password"
								bind:value={defApiKey}
								oninput={() => {
									if (defApiKey.trim()) defClearApiKey = false;
								}}
								placeholder={runtime.defaults.api_key.configured && !defClearApiKey
									? t('myaaw.onboard.api_key_saved')
									: t('myaaw.onboard.runtime_api_key_placeholder')}
							/>
							{#if defClearApiKey}
								<p class="text-[11px] text-amber-600 dark:text-amber-400">
									{t('myaaw.settings.runtimes_api_key_cleared_hint')}
								</p>
							{/if}
						</div>
					{/if}

					<div class="space-y-2.5 pt-1">
						<label class="flex cursor-pointer items-center gap-2 text-xs text-foreground">
							<Checkbox bind:checked={defMcpBridge} />
							<span>
								{t('myaaw.onboard.runtime_mcp_bridge')} (<code class="font-mono">cron</code>,
								<code class="font-mono">a2a_contact</code>,
								<code class="font-mono">extra_messages</code>,
								{t('myaaw.onboard.runtime_mcp_bridge_custom_tools')})
							</span>
						</label>

						{#if runtime.used_by_agents.length > 0}
							<label class="flex cursor-pointer items-center gap-2 text-xs text-foreground">
								<Checkbox bind:checked={defApplyExisting} />
								<span title={runtime.used_by_agents.join(', ')}>
									{t('myaaw.settings.runtimes_apply_existing', {
										name: runtimeLabel(runtime),
										agents: formatAgentSummary(runtime.used_by_agents)
									})}
								</span>
							</label>
						{/if}
					</div>
				</div>

				<Dialog.Footer
					class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
				>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onclick={() => (open = false)}
						disabled={saving}
					>
						{t('common.cancel')}
					</Button>
					<Button type="submit" size="sm" loading={saving} class="gap-1.5">
						<Save class="h-3.5 w-3.5" />
						{t('myaaw.settings.runtimes_save_defaults')}
					</Button>
				</Dialog.Footer>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>
