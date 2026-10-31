<script lang="ts">
	import { approvals as approvalsApi } from '$lib/apis/approvals';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Separator } from '$lib/components/ui/separator';
	import { RefreshCw, ShieldCheck, Trash2 } from 'lucide-svelte';
	import type { ApprovalsView } from '$lib/types';

	const { t } = useTranslation();

	let view = $state<ApprovalsView | null>(null);
	let loading = $state(false);
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			view = await approvalsApi.get(currentAgent);
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

	async function revoke(key: string) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.approvals.revoke_confirm', { key }),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await approvalsApi.revokeKey(agent, key);
			await load(agent);
			toastStore.success(t('myaaw.approvals.revoked'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function revokeSession(key: string, conversation: string) {
		if (!agent) return;
		try {
			await approvalsApi.revokeSession(agent, key, conversation);
			await load(agent);
			toastStore.success(t('myaaw.approvals.revoked'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function clearAll() {
		if (!agent) return;
		if (
			!(await askConfirm({ message: t('myaaw.approvals.clear_confirm'), variant: 'destructive' }))
		)
			return;
		try {
			await approvalsApi.clear(agent);
			await load(agent);
			toastStore.success(t('myaaw.approvals.revoked'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}
</script>

<TopBarActions id="approvals-refresh">
	{#snippet children()}
		<TopBarButton
			label={t('common.refresh')}
			icon={RefreshCw}
			onclick={() => agent && load(agent)}
			{loading}
		/>
	{/snippet}
</TopBarActions>

<div class="space-y-4">
	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		{#if error}
			<div
				class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
			>
				{error}
			</div>
		{/if}

		<div class="grid gap-4 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<CardTitle class="flex items-center gap-2">
						<ShieldCheck class="h-4 w-4" />
						{t('myaaw.approvals.policy')}
					</CardTitle>
					<CardDescription>{t('myaaw.approvals.policy_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="space-y-2 text-sm">
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground">{t('myaaw.approvals.mode')}</span>
						<Badge variant="outline">{view?.config.mode ?? '—'}</Badge>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground">{t('myaaw.approvals.timeout')}</span>
						<span class="font-mono">{view?.config.timeout_seconds ?? '—'}s</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground">{t('myaaw.approvals.unattended')}</span>
						<span class="font-mono">{view?.config.unattended ?? '—'}</span>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-muted-foreground">{t('myaaw.approvals.subagents')}</span>
						<span class="font-mono">{view?.config.subagents ?? '—'}</span>
					</div>
					<Separator />
					<div>
						<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
							{t('myaaw.approvals.allow')}
						</p>
						<div class="mt-1 flex flex-wrap gap-1">
							{#each view?.config.allow ?? [] as entry (entry)}
								<Badge variant="secondary" class="font-mono text-[10px]">{entry}</Badge>
							{/each}
						</div>
					</div>
					{#if (view?.config.deny.length ?? 0) > 0}
						<div>
							<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
								{t('myaaw.approvals.deny')}
							</p>
							<div class="mt-1 flex flex-wrap gap-1">
								{#each view?.config.deny ?? [] as entry (entry)}
									<Badge variant="destructive" class="font-mono text-[10px]">{entry}</Badge>
								{/each}
							</div>
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card>
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle>{t('myaaw.approvals.grants')}</CardTitle>
						<CardDescription>{t('myaaw.approvals.grants_desc')}</CardDescription>
					</div>
					<Button
						size="sm"
						variant="destructive"
						onclick={clearAll}
						disabled={(view?.grants?.length ?? 0) === 0}
					>
						<Trash2 class="mr-2 h-4 w-4" />
						{t('myaaw.approvals.clear')}
					</Button>
				</CardHeader>
				<CardContent class="space-y-2">
					{#if (view?.grants?.length ?? 0) === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.approvals.no_grants')}</p>
					{/if}
					{#each view?.grants ?? [] as grant (grant.key)}
						<div class="space-y-1 rounded-md border p-2">
							<div class="flex items-center justify-between gap-2">
								<span class="font-mono text-sm">{grant.key}</span>
								<Button size="xs" variant="outline" onclick={() => revoke(grant.key)}>
									{t('myaaw.approvals.revoke')}
								</Button>
							</div>
							{#if grant.always && grant.always.length > 0}
								<p class="text-[10px] text-muted-foreground">
									always:
									{#each grant.always as scope, index (index)}
										{scope.channel ?? 'any'}/{scope.user_id ?? 'any'}{index <
										(grant.always?.length ?? 0) - 1
											? ', '
											: ''}
									{/each}
								</p>
							{/if}
							{#each grant.sessions ?? [] as session (session.conversation)}
								<div class="flex items-center justify-between gap-2 text-[11px]">
									<span class="truncate font-mono text-muted-foreground">
										session {session.conversation.slice(0, 12)}…
									</span>
									<Button
										size="xs"
										variant="ghost"
										onclick={() => revokeSession(grant.key, session.conversation)}
									>
										{t('myaaw.approvals.revoke')}
									</Button>
								</div>
							{/each}
						</div>
					{/each}
				</CardContent>
			</Card>
		</div>
	{/if}
</div>
