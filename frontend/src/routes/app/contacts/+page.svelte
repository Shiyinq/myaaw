<script lang="ts">
	import { contacts as contactsApi } from '$lib/apis/contacts';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Plus, RefreshCw, Trash2, Users } from 'lucide-svelte';
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
		if (!agent || !name.trim() || !url.trim()) return;
		saving = true;
		try {
			await contactsApi.create(agent, {
				name: name.trim(),
				url: url.trim(),
				token: token || undefined,
				description: description.trim() || undefined
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

<div class="space-y-4">
	<PageHeader title={t('myaaw.contacts.title')} description={t('myaaw.contacts.subtitle')}>
		<Button variant="outline" size="sm" onclick={() => agent && load(agent)} loading={loading}>
			<RefreshCw class="mr-2 h-4 w-4" />
			{t('common.refresh')}
		</Button>
	</PageHeader>

	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
				{error}
			</div>
		{/if}

		<div class="grid gap-4 lg:grid-cols-5">
			<Card class="lg:col-span-3">
				<CardHeader>
					<CardTitle class="flex items-center gap-2">
						<Users class="h-4 w-4" />
						{t('myaaw.contacts.list')}
					</CardTitle>
					<CardDescription>{t('myaaw.contacts.list_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-2">
					{#if !loading && contacts.length === 0}
						<EmptyState
							title={t('myaaw.contacts.empty')}
							description={t('myaaw.contacts.empty_desc')}
							icon={Users}
						/>
					{/if}
					{#each contacts as contact (contact.name)}
						<div class="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between">
							<div class="min-w-0 space-y-1">
								<div class="flex items-center gap-2">
									<span class="font-medium">{contact.name}</span>
									<Badge variant={contact.token.configured ? 'secondary' : 'outline'}>
										{contact.token.configured ? contact.token.masked : t('myaaw.contacts.open')}
									</Badge>
								</div>
								<p class="truncate text-[11px] text-muted-foreground">{contact.url}</p>
								{#if contact.description}
									<p class="text-xs text-muted-foreground">{contact.description}</p>
								{/if}
							</div>
							<Button size="icon-sm" variant="ghost" onclick={() => remove(contact.name)}>
								<Trash2 class="h-3.5 w-3.5 text-destructive" />
							</Button>
						</div>
					{/each}
				</CardContent>
			</Card>

			<Card class="lg:col-span-2">
				<CardHeader>
					<CardTitle>{t('myaaw.contacts.create')}</CardTitle>
					<CardDescription>{t('myaaw.contacts.create_desc')}</CardDescription>
				</CardHeader>
				<CardContent>
					<form
						class="space-y-2"
						onsubmit={(event) => {
							event.preventDefault();
							void create();
						}}
					>
						<div class="grid gap-1">
							<Label for="contact-name">{t('myaaw.contacts.name')}</Label>
							<Input id="contact-name" bind:value={name} placeholder="research" />
						</div>
						<div class="grid gap-1">
							<Label for="contact-url">{t('myaaw.contacts.url')}</Label>
							<Input id="contact-url" bind:value={url} placeholder="https://host/a2a" />
						</div>
						<div class="grid gap-1">
							<Label for="contact-token">{t('myaaw.contacts.token')}</Label>
							<Input id="contact-token" type="password" bind:value={token} />
							<p class="text-[10px] text-muted-foreground">{t('myaaw.contacts.token_hint')}</p>
						</div>
						<div class="grid gap-1">
							<Label for="contact-description">{t('myaaw.contacts.description')}</Label>
							<Input id="contact-description" bind:value={description} />
						</div>
						<Button type="submit" size="sm" loading={saving}>
							<Plus class="mr-2 h-4 w-4" />
							{t('myaaw.contacts.create')}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
