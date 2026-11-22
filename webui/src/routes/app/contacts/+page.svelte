<script lang="ts">
	import { contactsStore } from '$lib/stores/contacts.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import ContactDialog from '$lib/components/contacts/ContactDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Bot, Globe, KeyRound, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-svelte';
	import type { ContactView } from '$lib/types';

	const { t } = useTranslation();

	let query = $state('');
	let dialogOpen = $state(false);
	let editing = $state<ContactView | null>(null);

	let agent = $derived(agentSelection.selected);
	useQuery(contactsStore.query, () => (agent ? { agent } : null));

	let contacts = $derived(contactsStore.contacts);
	let loading = $derived(contactsStore.loading);
	let error = $derived(contactsStore.error);

	let filteredContacts = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return contacts;
		return contacts.filter(
			(contact) =>
				contact.name.toLowerCase().includes(needle) ||
				contact.url.toLowerCase().includes(needle) ||
				(contact.description ?? '').toLowerCase().includes(needle)
		);
	});

	function openCreate() {
		editing = null;
		dialogOpen = true;
	}

	function openEdit(contact: ContactView) {
		editing = contact;
		dialogOpen = true;
	}

	async function remove(contact: string) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.contacts.delete_confirm', { name: contact }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await contactsStore.remove(agent, contact);
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
			onclick={() => void contactsStore.refresh()}
			loading={loading || contactsStore.refreshing}
		/>
	{/snippet}
</TopBarActions>

<div class="mx-auto w-full max-w-5xl space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="rounded-xl border bg-card shadow-xs">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
				<div class="flex items-center gap-2">
					<Bot class="h-4 w-4 text-primary" />
					<h2 class="text-sm font-semibold text-foreground">{t('myaaw.contacts.list')}</h2>
					<span
						class="rounded-full bg-muted px-2 py-0.5 text-[11px] leading-none font-medium text-muted-foreground tabular-nums"
					>
						{contacts.length}
					</span>
				</div>

				<div class="flex w-full items-center gap-2 sm:w-auto">
					{#if contacts.length > 0 || query}
						<div class="relative flex-1 sm:w-56 sm:flex-none">
							<Search
								class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
							/>
							<Input bind:value={query} placeholder={t('common.search')} class="h-8 pl-8 text-xs" />
						</div>
					{/if}
					<Button size="sm" onclick={openCreate} class="shrink-0 gap-1.5 rounded-lg">
						<Plus class="h-3.5 w-3.5" />
						{t('myaaw.contacts.create')}
					</Button>
				</div>
			</div>

			<div class="p-4 sm:p-5">
				{#if loading}
					<div class="space-y-2.5">
						<Skeleton class="h-20 w-full rounded-xl" />
						<Skeleton class="h-20 w-full rounded-xl" />
					</div>
				{:else if contacts.length === 0}
					<EmptyState
						title={t('myaaw.contacts.empty')}
						description={t('myaaw.contacts.create_desc')}
						bloub="curieux"
						icon={Bot}
						class="min-h-64"
					/>
				{:else if filteredContacts.length === 0}
					<p class="py-10 text-center text-sm text-muted-foreground">
						{t('myaaw.contacts.no_matches')}
					</p>
				{:else}
					<div class="space-y-2.5">
						{#each filteredContacts as contact (contact.name)}
							<div
								class="flex flex-col gap-3 rounded-xl border bg-background/40 p-3.5 transition-colors hover:border-border/90 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between"
							>
								<div class="flex min-w-0 items-start gap-3">
									<div
										class="mt-0.5 grid h-9 w-9 shrink-0 place-items-center rounded-xl border bg-muted/40 text-muted-foreground"
									>
										<Bot class="h-4 w-4" />
									</div>
									<div class="min-w-0 flex-1 space-y-1">
										<div class="flex flex-wrap items-center gap-2">
											<span class="text-sm font-semibold text-foreground wrap-anywhere">
												{contact.name}
											</span>
											<Badge
												variant={contact.token.configured ? 'secondary' : 'outline'}
												class="gap-1 text-[10px]"
											>
												{#if contact.token.configured}
													<KeyRound class="h-2.5 w-2.5" />
													{contact.token.masked}
												{:else}
													{t('myaaw.contacts.open')}
												{/if}
											</Badge>
										</div>

										<p
											class="flex items-center gap-1.5 truncate font-mono text-[11px] text-muted-foreground"
											title={contact.url}
										>
											<Globe class="h-3 w-3 shrink-0 opacity-70" />
											<span class="truncate">{contact.url}</span>
										</p>

										{#if contact.description}
											<p class="line-clamp-2 text-xs text-muted-foreground">
												{contact.description}
											</p>
										{/if}
									</div>
								</div>

								<div class="flex shrink-0 items-center justify-end gap-1">
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => openEdit(contact)}
										aria-label={t('myaaw.contacts.edit')}
										title={t('myaaw.contacts.edit')}
									>
										<Pencil class="h-3.5 w-3.5" />
									</Button>
									<Button
										size="icon-sm"
										variant="ghost"
										onclick={() => remove(contact.name)}
										aria-label={t('common.delete')}
										title={t('common.delete')}
									>
										<Trash2 class="h-3.5 w-3.5 text-destructive" />
									</Button>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	<ContactDialog
		open={dialogOpen}
		agent={agent ?? ''}
		{editing}
		onOpenChange={(next) => (dialogOpen = next)}
	/>
</div>
