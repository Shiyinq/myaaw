<script lang="ts">
	import { contacts as contactsApi } from '$lib/apis/contacts';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { contactSchema } from '$lib/schemas';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
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
	import { Bot, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { ContactView } from '$lib/types';

	const { t } = useTranslation();

	let contacts = $state<ContactView[]>([]);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadKey = '';

	let name = $state('');
	let url = $state('');
	let token = $state('');
	let description = $state('');
	let errors = $state<FieldErrors>({});

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const result = await contactsApi.list(currentAgent);
			contacts = result.contacts ?? [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	async function create() {
		if (!agent) return;
		const result = validate(contactSchema, { name, url, token, description });
		if (!result.ok) {
			errors = result.errors;
			return;
		}
		errors = {};
		saving = true;
		try {
			await contactsApi.create(agent, {
				name: result.data.name,
				url: result.data.url,
				token: result.data.token || undefined,
				description: result.data.description || undefined
			});
			name = '';
			url = '';
			token = '';
			description = '';
			toastStore.success(t('myaaw.contacts.saved'));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			saving = false;
		}
	}

	async function remove(contact: string) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.contacts.delete_confirm', { name: contact }))) return;
		try {
			await contactsApi.remove(agent, contact);
			await load(agent);
			toastStore.success(t('myaaw.contacts.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="contacts-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col lg:flex-row">
	{#if !agent}
		<div class="w-full p-4 lg:p-6">
			<Card>
				<CardContent class="py-10 text-center text-sm text-muted-foreground">
					{t('myaaw.agent.none')}
				</CardContent>
			</Card>
		</div>
	{:else}
		<!-- Contacts are remote A2A agents: a second sidebar nested inside the main one. -->
		<aside
			class="flex h-[45vh] shrink-0 flex-col border-b bg-card lg:h-auto lg:w-72 lg:border-r lg:border-b-0 xl:w-80"
		>
			<div class="shrink-0 border-b px-3 py-2.5">
				<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
					{t('myaaw.contacts.list')}
				</p>
			</div>

			<div class="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
				{#if loading}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('common.loading')}</p>
				{:else if contacts.length === 0}
					<p class="px-1 py-2 text-sm text-muted-foreground">{t('myaaw.contacts.empty')}</p>
				{/if}

				{#each contacts as contact (contact.name)}
					<div class="flex items-start gap-2 rounded-md px-1 py-1.5 hover:bg-muted">
						<Bot class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-1.5">
								<span class="min-w-0 truncate text-sm font-medium">{contact.name}</span>
								<Badge
									variant={contact.token.configured ? 'secondary' : 'outline'}
									class="shrink-0 text-[10px]"
								>
									{contact.token.configured ? contact.token.masked : t('myaaw.contacts.open')}
								</Badge>
							</div>
							<p class="truncate text-[10px] text-muted-foreground" title={contact.url}>
								{contact.url}
							</p>
							{#if contact.description}
								<p class="truncate text-[10px] text-muted-foreground" title={contact.description}>
									{contact.description}
								</p>
							{/if}
						</div>
						<Button
							size="icon-xs"
							variant="ghost"
							class="mt-0.5 shrink-0"
							onclick={() => remove(contact.name)}
							aria-label={t('common.delete')}
						>
							<Trash2 class="h-3 w-3 text-destructive" />
						</Button>
					</div>
				{/each}
			</div>
		</aside>

		<section class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 lg:p-6">
			{#if error}
				<div
					class="shrink-0 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
				>
					{error}
				</div>
			{/if}

			<Card class="mx-auto w-full max-w-3xl shrink-0">
				<CardHeader>
					<CardTitle class="flex items-center gap-2">
						<Bot class="h-4 w-4" />
						{t('myaaw.contacts.create')}
					</CardTitle>
					<CardDescription>{t('myaaw.contacts.create_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-3"
						onsubmit={(event) => {
							event.preventDefault();
							void create();
						}}
					>
						<div class="grid gap-1">
							<Label for="contact-name">{t('myaaw.contacts.name')}</Label>
							<Input id="contact-name" bind:value={name} placeholder="research" />
							<FieldError code={errors.name} />
						</div>
						<div class="grid gap-1">
							<Label for="contact-url">{t('myaaw.contacts.url')}</Label>
							<Input id="contact-url" bind:value={url} placeholder="https://host/a2a" />
							<FieldError code={errors.url} />
						</div>
						<div class="grid gap-1">
							<Label for="contact-token">{t('myaaw.contacts.token')}</Label>
							<Input id="contact-token" type="password" bind:value={token} />
							<p class="text-[10px] text-muted-foreground">{t('myaaw.contacts.token_hint')}</p>
						</div>
						<div class="grid gap-1">
							<Label for="contact-description">{t('myaaw.contacts.description')}</Label>
							<Input id="contact-description" bind:value={description} />
							<FieldError code={errors.description} />
						</div>
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.contacts.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</section>
	{/if}
</div>
