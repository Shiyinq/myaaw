<script lang="ts">
	import { approvals as approvalsApi } from '$lib/apis/approvals';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
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
	import * as Tabs from '$lib/components/ui/tabs';
	import { RefreshCw, ShieldCheck, Trash2 } from 'lucide-svelte';
	import type { ApprovalGrant, ApprovalsView, GrantScope } from '$lib/types';

	const { t } = useTranslation();

	let view = $state<ApprovalsView | null>(null);
	let loading = $state(false);
	let error = $state('');
	let loadKey = '';
	/** Always and session grants are separate surfaces; always opens first. */
	let grantTab = $state('always');

	let agent = $derived(agentSelection.selected);

	let alwaysGrants = $derived(
		(view?.grants ?? []).filter((grant) => (grant.always?.length ?? 0) > 0)
	);
	let sessionGrants = $derived(
		(view?.grants ?? []).filter((grant) => (grant.sessions?.length ?? 0) > 0)
	);
	let alwaysTotal = $derived(
		(view?.grants ?? []).reduce((sum, grant) => sum + (grant.always?.length ?? 0), 0)
	);
	let sessionTotal = $derived(
		(view?.grants ?? []).reduce((sum, grant) => sum + (grant.sessions?.length ?? 0), 0)
	);

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

	/** Revokes one always-scope, or the whole key when the scope is a wildcard. */
	async function revokeScope(key: string, scope: GrantScope) {
		if (!agent) return;
		const channel = (scope.channel ?? '').trim();
		const user = (scope.user_id ?? '').trim();
		if (!channel && !user) {
			await revoke(key);
			return;
		}
		try {
			await approvalsApi.revokeKey(agent, key, { channel, user });
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

	/** Newest session first; entries without a timestamp keep their stored order. */
	function sessionsOf(grant: ApprovalGrant) {
		return [...(grant.sessions ?? [])].sort((a, b) => {
			const left = a.created_at ? Date.parse(a.created_at) : NaN;
			const right = b.created_at ? Date.parse(b.created_at) : NaN;
			if (Number.isNaN(left) || Number.isNaN(right)) return 0;
			return right - left;
		});
	}

	function scopeLabel(scope: GrantScope) {
		return `${scope.channel ?? 'any'}/${scope.user_id ?? 'any'}`;
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
			<CardContent
				class="flex flex-col items-center gap-3 py-10 text-center text-sm text-muted-foreground"
			>
				<Bloub mood="confus" size={56} tight color={agentSelection.color} />
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

		<Card>
			<CardHeader>
				<CardTitle class="flex items-center gap-2">
					<ShieldCheck class="h-4 w-4" />
					{t('myaaw.approvals.policy')}
				</CardTitle>
				<CardDescription>{t('myaaw.approvals.policy_desc')}</CardDescription>
			</CardHeader>
			<CardContent class="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
				<div class="flex items-center justify-between gap-2 sm:block">
					<span class="text-muted-foreground">{t('myaaw.approvals.mode')}</span>
					<Badge variant="outline">{view?.config.mode ?? '—'}</Badge>
				</div>
				<div class="flex items-center justify-between gap-2 sm:block">
					<span class="text-muted-foreground">{t('myaaw.approvals.timeout')}</span>
					<span class="font-mono">{view?.config.timeout_seconds ?? '—'}s</span>
				</div>
				<div class="flex items-center justify-between gap-2 sm:block">
					<span class="text-muted-foreground">{t('myaaw.approvals.unattended')}</span>
					<span class="font-mono">{view?.config.unattended ?? '—'}</span>
				</div>
				<div class="flex items-center justify-between gap-2 sm:block">
					<span class="text-muted-foreground">{t('myaaw.approvals.subagents')}</span>
					<span class="font-mono">{view?.config.subagents ?? '—'}</span>
				</div>
				<Separator class="sm:col-span-2 lg:col-span-4" />
				<div class="sm:col-span-2 lg:col-span-4">
					<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t('myaaw.approvals.allow')}
					</p>
					<div class="mt-1 flex flex-wrap gap-1">
						{#each view?.config.allow ?? [] as entry (entry)}
							<Badge variant="secondary" class="font-mono text-[10px]">{entry}</Badge>
						{/each}
					</div>
				</div>
				{#if (view?.config.deny?.length ?? 0) > 0}
					<div class="sm:col-span-2 lg:col-span-4">
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
			<CardHeader class="flex flex-row items-center justify-between gap-2">
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
			<CardContent>
				{#if (view?.grants?.length ?? 0) === 0}
					<div class="flex flex-col items-center gap-2 py-2 text-center">
						<Bloub mood="heureux" size={48} tight color={agentSelection.color} />
						<p class="text-sm text-muted-foreground">{t('myaaw.approvals.no_grants')}</p>
					</div>
				{:else}
					<Tabs.Root bind:value={grantTab} class="space-y-3">
						<Tabs.List variant="line" class="justify-start">
							<Tabs.Trigger value="always">
								{t('myaaw.approvals.always_grants')}
								<span class="text-[10px] text-muted-foreground">{alwaysTotal}</span>
							</Tabs.Trigger>
							<Tabs.Trigger value="sessions">
								{t('myaaw.approvals.session_grants')}
								<span class="text-[10px] text-muted-foreground">{sessionTotal}</span>
							</Tabs.Trigger>
						</Tabs.List>

						<Tabs.Content value="always">
							{#if alwaysGrants.length === 0}
								<p class="py-4 text-center text-xs text-muted-foreground">
									{t('myaaw.approvals.no_always_grants')}
								</p>
							{:else}
								<div class="grid gap-2 lg:grid-cols-2 2xl:grid-cols-3">
									{#each alwaysGrants as grant (grant.key)}
										<div class="space-y-1 rounded-md border p-2">
											<div class="flex items-center justify-between gap-2">
												<span class="truncate font-mono text-sm">{grant.key}</span>
												<Button size="xs" variant="outline" onclick={() => revoke(grant.key)}>
													{t('myaaw.approvals.revoke')}
												</Button>
											</div>
											{#each grant.always ?? [] as scope, index (index)}
												<div class="flex items-center justify-between gap-2 text-[11px]">
													<span class="min-w-0 flex-1 truncate font-mono text-muted-foreground">
														{scopeLabel(scope)}
													</span>
													<Button
														size="xs"
														variant="ghost"
														onclick={() => revokeScope(grant.key, scope)}
													>
														{t('myaaw.approvals.revoke')}
													</Button>
												</div>
											{/each}
										</div>
									{/each}
								</div>
							{/if}
						</Tabs.Content>

						<Tabs.Content value="sessions">
							{#if sessionGrants.length === 0}
								<p class="py-4 text-center text-xs text-muted-foreground">
									{t('myaaw.approvals.no_session_grants')}
								</p>
							{:else}
								<div class="gap-2 lg:columns-2 2xl:columns-3">
									{#each sessionGrants as grant (grant.key)}
										<div class="mb-2 break-inside-avoid space-y-1 rounded-md border p-2">
											<div class="flex items-center justify-between gap-2">
												<span class="min-w-0 flex-1 truncate font-mono text-sm">
													{grant.key}
												</span>
												<Button size="xs" variant="outline" onclick={() => revoke(grant.key)}>
													{t('myaaw.approvals.revoke')}
												</Button>
											</div>
											{#each sessionsOf(grant) as session (session.id)}
												<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px]">
													<span
														class="min-w-0 flex-1 break-all font-mono text-[10px] text-muted-foreground"
													>
														{session.id}
													</span>
													{#if session.channel || session.user_id}
														<span
															class="hidden shrink-0 text-[10px] text-muted-foreground sm:inline"
														>
															{scopeLabel(session)}
														</span>
													{/if}
													<Button
														size="xs"
														variant="ghost"
														onclick={() => revokeSession(grant.key, session.id)}
													>
														{t('myaaw.approvals.revoke')}
													</Button>
												</div>
											{/each}
										</div>
									{/each}
								</div>
							{/if}
						</Tabs.Content>
					</Tabs.Root>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>
