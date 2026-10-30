<script lang="ts">
	import { goto } from '$app/navigation';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import EmptyState from '$lib/components/common/EmptyState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Check, ChevronLeft, ChevronRight, MessagesSquare, Pencil, Trash2, X } from 'lucide-svelte';
	import type { ConversationSummary } from '$lib/types';

	const { t } = useTranslation();
	const limit = 20;

	let items = $state<ConversationSummary[]>([]);
	let total = $state(0);
	let hasMore = $state(false);
	let offset = $state(0);
	let search = $state('');
	let searchInput = $state('');
	let loading = $state(false);
	let error = $state('');
	let editingId = $state('');
	let editingTitle = $state('');

	let agent = $derived(agentSelection.selected);
	let loadKey = '';

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			const page = await conversationsApi.list(currentAgent, {
				search: search || undefined,
				limit,
				offset
			});
			items = page.conversations ?? [];
			total = page.total;
			hasMore = page.has_more;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const key = `${currentAgent}:${search}:${offset}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		void load(currentAgent);
	});

	function applySearch() {
		search = searchInput.trim();
		offset = 0;
	}

	function nextPage() {
		if (hasMore) offset += limit;
	}

	function previousPage() {
		offset = Math.max(0, offset - limit);
	}

	function startRename(conversation: ConversationSummary) {
		editingId = conversation.id;
		editingTitle = conversation.title;
	}

	async function saveRename() {
		if (!agent || !editingId) return;
		const title = editingTitle.trim();
		if (!title) return;
		try {
			await conversationsApi.rename(agent, editingId, title);
			items = items.map((item) => (item.id === editingId ? { ...item, title } : item));
			editingId = '';
			toastStore.success(t('myaaw.conversations.renamed'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeConversation(conversation: ConversationSummary) {
		if (!agent) return;
		if (!window.confirm(t('myaaw.conversations.delete_confirm'))) return;
		try {
			await conversationsApi.remove(agent, conversation.id);
			items = items.filter((item) => item.id !== conversation.id);
			total = Math.max(0, total - 1);
			toastStore.success(t('myaaw.conversations.deleted'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function formatDate(value: string) {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '—';
		return date.toLocaleString();
	}
</script>

<div class="space-y-4">
	<PageHeader title={t('myaaw.conversations.title')} description={t('myaaw.conversations.subtitle')}>
		<div class="flex w-full max-w-xs items-center gap-2">
			<Input
				bind:value={searchInput}
				placeholder={t('myaaw.conversations.search')}
				onkeydown={(event) => {
					if (event.key === 'Enter') applySearch();
				}}
			/>
			<Button variant="outline" size="sm" onclick={applySearch}>
				{t('common.search')}
			</Button>
		</div>
	</PageHeader>

	{#if error}
		<div class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
			{error}
		</div>
	{/if}

	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else if !loading && items.length === 0}
		<EmptyState
			title={t('myaaw.conversations.empty')}
			description={t('myaaw.conversations.empty_desc')}
			icon={MessagesSquare}
		>
			<Button onclick={() => goto('/app/chat')}>{t('myaaw.chat.new')}</Button>
		</EmptyState>
	{:else}
		<div class="space-y-2">
			{#each items as conversation (conversation.id)}
				<Card>
					<CardContent class="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="min-w-0 space-y-1">
							{#if editingId === conversation.id}
								<div class="flex items-center gap-2">
									<Input bind:value={editingTitle} class="h-8 max-w-sm" />
									<Button size="icon-sm" variant="secondary" onclick={saveRename}>
										<Check class="h-3.5 w-3.5" />
									</Button>
									<Button size="icon-sm" variant="ghost" onclick={() => (editingId = '')}>
										<X class="h-3.5 w-3.5" />
									</Button>
								</div>
							{:else}
								<div class="truncate text-sm font-medium">
									{conversation.title || conversation.id}
								</div>
								<div class="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
									<span class="font-mono">{conversation.id.slice(0, 12)}…</span>
									<span>
										{t('myaaw.conversations.messages', { count: conversation.message_count })}
									</span>
									<span>{formatDate(conversation.updated_at)}</span>
									{#if conversation.active}
										<Badge variant="outline">{t('myaaw.conversations.active')}</Badge>
									{/if}
								</div>
							{/if}
						</div>

						<div class="flex shrink-0 items-center gap-1">
							<Button
								size="sm"
								variant="outline"
								onclick={() => goto(`/app/chat/${conversation.id}`)}
							>
								{t('myaaw.conversations.open')}
							</Button>
							<Button size="icon-sm" variant="ghost" onclick={() => startRename(conversation)}>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
							<Button
								size="icon-sm"
								variant="ghost"
								onclick={() => removeConversation(conversation)}
							>
								<Trash2 class="h-3.5 w-3.5 text-destructive" />
							</Button>
						</div>
					</CardContent>
				</Card>
			{/each}
		</div>

		<div class="flex items-center justify-between pt-2">
			<span class="text-xs text-muted-foreground">
				{t('myaaw.conversations.total', { total, offset: offset + 1, end: offset + items.length })}
			</span>
			<div class="flex items-center gap-2">
				<Button variant="outline" size="sm" disabled={offset === 0} onclick={previousPage}>
					<ChevronLeft class="mr-1 h-4 w-4" />
					{t('common.previous')}
				</Button>
				<Button variant="outline" size="sm" disabled={!hasMore} onclick={nextPage}>
					{t('common.next')}
					<ChevronRight class="ml-1 h-4 w-4" />
				</Button>
			</div>
		</div>
	{/if}
</div>
