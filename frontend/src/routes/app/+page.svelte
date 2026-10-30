<script lang="ts">
	import { goto } from '$app/navigation';
	import { agents as agentsApi } from '$lib/apis/agents';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { onboardSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		Bot,
		CircleCheck,
		CircleSlash,
		Ellipsis,
		Plus,
		RefreshCw,
		Star,
		Trash2
	} from 'lucide-svelte';

	const { t } = useTranslation();

	let loading = $state(false);
	let onboardOpen = $state(false);
	let onboarding = $state(false);
	let onboardErrors = $state<FieldErrors>({});
	let newName = $state('');
	let newType = $state('openai');
	let newBaseUrl = $state('');
	let newApiKey = $state('');
	let newModel = $state('');

	let agents = $derived(agentSelection.agents);
	let runningCount = $derived(agents.filter((agent) => agent.running).length);
	let defaultAgent = $derived(agents.find((agent) => agent.default)?.name ?? '—');

	async function refresh() {
		loading = true;
		try {
			await agentSelection.load(true);
		} finally {
			loading = false;
		}
	}

	// Opening an agent also selects it, so the agent dashboard shows its data.
	function openChat(name: string) {
		agentSelection.select(name);
		void goto('/app/chat');
	}

	async function setDefaultAgent(name: string) {
		try {
			await agentsApi.setDefault(name);
			await agentSelection.load(true);
			toastStore.success(t('myaaw.home.agent_default_set', { name }));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeAgent(name: string) {
		if (!window.confirm(t('myaaw.home.agent_delete_confirm', { name }))) return;
		try {
			await agentsApi.remove(name);
			await agentSelection.load(true);
			toastStore.success(t('myaaw.home.agent_deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function openOnboard() {
		onboardErrors = {};
		onboardOpen = true;
	}

	async function createAgent() {
		const result = validate(onboardSchema, {
			name: newName,
			provider_type: newType,
			base_url: newBaseUrl,
			api_key: newApiKey,
			model: newModel
		});
		if (!result.ok) {
			onboardErrors = result.errors;
			return;
		}
		onboardErrors = {};
		onboarding = true;
		try {
			const response = await agentsApi.onboard(result.data.name, {
				provider: {
					type: result.data.provider_type,
					base_url: result.data.base_url || undefined,
					api_key: result.data.api_key || undefined,
					model: result.data.model || undefined
				}
			});
			onboardOpen = false;
			newName = '';
			newBaseUrl = '';
			newApiKey = '';
			newModel = '';
			toastStore.success(t('myaaw.home.agent_created', { name: result.data.name }));
			for (const warning of response.warnings ?? []) {
				toastStore.warning(warning);
			}
			await agentSelection.load(true);
			agentSelection.select(result.data.name);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			onboarding = false;
		}
	}
</script>

<TopBarActions id="fleet-actions">
	{#snippet children()}
		<TopBarButton
			label={t('myaaw.home.new_agent')}
			icon={Plus}
			onclick={openOnboard}
			variant="default"
		/>
		<TopBarButton label={t('common.refresh')} icon={RefreshCw} onclick={refresh} {loading} />
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col gap-4 p-4 lg:p-6">
	<div class="grid shrink-0 gap-3 sm:grid-cols-3">
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.agents')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="text-2xl font-bold">{agents.length}</div>
			</CardContent>
		</Card>
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.home.running')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="text-2xl font-bold">{runningCount}</div>
			</CardContent>
		</Card>
		<Card>
			<CardHeader class="pb-2">
				<CardTitle class="text-sm font-medium">{t('myaaw.fleet.default_agent')}</CardTitle>
			</CardHeader>
			<CardContent>
				<div class="truncate font-mono text-lg font-semibold">{defaultAgent}</div>
			</CardContent>
		</Card>
	</div>

	<Card class="flex min-h-0 flex-1 flex-col">
		<CardHeader class="shrink-0">
			<CardTitle class="flex items-center gap-2">
				<Bot class="h-4 w-4 shrink-0" />
				{t('myaaw.home.agents')}
			</CardTitle>
			<CardDescription>{t('myaaw.home.agents_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="min-h-0 flex-1 overflow-y-auto">
			{#if agents.length === 0}
				<EmptyState
					title={t('myaaw.home.no_agents')}
					description={t('myaaw.home.no_agents_desc')}
					icon={Bot}
				/>
			{:else}
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
					{#each agents as agent (agent.name)}
						<div
							class="relative rounded-xl border bg-card transition-colors hover:border-ring/40 hover:bg-muted/40"
						>
							<button
								class="flex w-full flex-col items-center gap-1.5 p-5 text-center"
								onclick={() => openChat(agent.name)}
								title={t('myaaw.home.open_chat')}
							>
								<Bot class="h-9 w-9 text-muted-foreground" />
								<span class="mt-1 w-full truncate text-sm font-semibold">{agent.name}</span>
								<span class="flex flex-wrap items-center justify-center gap-1">
									{#if agent.default}
										<Badge variant="secondary">{t('myaaw.agent.default')}</Badge>
									{/if}
									{#if agent.running}
										<Badge variant="outline" class="text-emerald-600 dark:text-emerald-400">
											<CircleCheck class="mr-1 h-3 w-3" />
											{t('myaaw.agent.running')}
										</Badge>
									{:else}
										<Badge variant="outline" class="text-muted-foreground">
											<CircleSlash class="mr-1 h-3 w-3" />
											{t('myaaw.agent.stopped')}
										</Badge>
									{/if}
								</span>
								{#if agent.port}
									<span class="font-mono text-[10px] text-muted-foreground">
										:{agent.port}
									</span>
								{/if}
							</button>
							<div class="absolute top-1 right-1">
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}
											<Button
												{...props}
												variant="ghost"
												size="icon-sm"
												aria-label={t('myaaw.home.agent_actions')}
											>
												<Ellipsis class="h-4 w-4" />
											</Button>
										{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										{#if !agent.default}
											<DropdownMenu.Item onclick={() => setDefaultAgent(agent.name)}>
												<Star class="mr-2 h-4 w-4" />
												{t('myaaw.home.set_default')}
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
										{/if}
										<DropdownMenu.Item
											variant="destructive"
											onclick={() => removeAgent(agent.name)}
										>
											<Trash2 class="mr-2 h-4 w-4" />
											{t('myaaw.home.delete_agent')}
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</CardContent>
	</Card>
</div>

<Dialog.Root bind:open={onboardOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{t('myaaw.home.new_agent')}</Dialog.Title>
			<Dialog.Description>{t('myaaw.home.new_agent_desc')}</Dialog.Description>
		</Dialog.Header>
		<form
			class="space-y-3"
			onsubmit={(event) => {
				event.preventDefault();
				void createAgent();
			}}
		>
			<div class="grid gap-1">
				<Label for="agent-name">{t('myaaw.home.agent_name')}</Label>
				<Input id="agent-name" bind:value={newName} placeholder="research" />
				<FieldError code={onboardErrors.name} />
			</div>
			<div class="grid gap-1">
				<Label for="agent-provider-type">{t('myaaw.providers.type')}</Label>
				<select
					id="agent-provider-type"
					bind:value={newType}
					class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
				>
					<option value="openai">openai</option>
					<option value="gemini">gemini</option>
				</select>
			</div>
			<div class="grid gap-1">
				<Label for="agent-base-url">{t('myaaw.providers.base_url')}</Label>
				<Input id="agent-base-url" bind:value={newBaseUrl} placeholder="https://api.deepseek.com" />
				<FieldError code={onboardErrors.base_url} />
			</div>
			<div class="grid gap-1">
				<Label for="agent-api-key">{t('myaaw.providers.api_key')}</Label>
				<Input id="agent-api-key" type="password" bind:value={newApiKey} placeholder="sk-..." />
			</div>
			<div class="grid gap-1">
				<Label for="agent-model">{t('myaaw.providers.model')}</Label>
				<Input id="agent-model" bind:value={newModel} placeholder="deepseek-chat" />
			</div>
			<Dialog.Footer class="gap-2">
				<Button type="button" variant="ghost" onclick={() => (onboardOpen = false)}>
					{t('common.cancel')}
				</Button>
				<Button type="submit" loading={onboarding}>
					<Plus class="mr-2 h-4 w-4" />
					{t('myaaw.home.create_agent')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
