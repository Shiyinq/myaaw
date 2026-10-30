<script lang="ts">
	import { server as serverApi } from '$lib/apis/server';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
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
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		CircleCheck,
		CircleSlash,
		Play,
		RefreshCw,
		RotateCcw,
		Server,
		Square
	} from 'lucide-svelte';
	import type { ServerStatus } from '$lib/types';

	const { t } = useTranslation();

	let status = $state<ServerStatus | null>(null);
	let loading = $state(false);
	let action = $state<'start' | 'stop' | 'restart' | ''>('');
	let error = $state('');
	let loadKey = '';

	let agent = $derived(agentSelection.selected);

	async function load(currentAgent: string) {
		loading = true;
		error = '';
		try {
			status = await serverApi.status(currentAgent);
		} catch (err) {
			status = null;
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

	async function run(command: 'start' | 'stop' | 'restart') {
		if (!agent) return;
		action = command;
		try {
			await serverApi[command](agent);
			toastStore.success(t(`myaaw.server.${command}_done`));
			await load(agent);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			action = '';
		}
	}
</script>

<TopBarActions id="server-refresh">
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

		<div class="grid gap-4 md:grid-cols-2">
			<Card>
				<CardHeader class="flex flex-row items-center justify-between">
					<div>
						<CardTitle>{t('myaaw.server.status')}</CardTitle>
						<CardDescription>{agent}</CardDescription>
					</div>
					<Server class="h-5 w-5 text-muted-foreground" />
				</CardHeader>
				<CardContent class="space-y-3">
					{#if loading}
						<Skeleton class="h-16 w-full" />
					{:else}
						<div class="flex items-center gap-2">
							{#if status?.running}
								<Badge variant="outline" class="text-emerald-600 dark:text-emerald-400">
									<CircleCheck class="mr-1 h-3 w-3" />
									{t('myaaw.server.running')}
								</Badge>
							{:else}
								<Badge variant="outline" class="text-muted-foreground">
									<CircleSlash class="mr-1 h-3 w-3" />
									{t('myaaw.server.stopped')}
								</Badge>
							{/if}
						</div>
						<dl class="grid grid-cols-2 gap-2 text-sm">
							<dt class="text-muted-foreground">{t('myaaw.server.port')}</dt>
							<dd class="font-mono">{status?.port ?? '—'}</dd>
							<dt class="text-muted-foreground">{t('myaaw.server.pid')}</dt>
							<dd class="font-mono">{status?.pid ?? '—'}</dd>
							<dt class="text-muted-foreground">base_url</dt>
							<dd class="truncate font-mono text-xs">{status?.base_url ?? '—'}</dd>
						</dl>
					{/if}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>{t('myaaw.server.controls')}</CardTitle>
					<CardDescription>{t('myaaw.server.controls_desc')}</CardDescription>
				</CardHeader>
				<CardContent class="flex flex-wrap gap-2">
					<Button
						size="sm"
						onclick={() => run('start')}
						loading={action === 'start'}
						disabled={status?.running}
					>
						<Play class="mr-2 h-4 w-4" />
						{t('myaaw.server.start')}
					</Button>
					<Button
						size="sm"
						variant="destructive"
						onclick={() => run('stop')}
						loading={action === 'stop'}
						disabled={!status?.running}
					>
						<Square class="mr-2 h-4 w-4" />
						{t('myaaw.server.stop')}
					</Button>
					<Button
						size="sm"
						variant="secondary"
						onclick={() => run('restart')}
						loading={action === 'restart'}
					>
						<RotateCcw class="mr-2 h-4 w-4" />
						{t('myaaw.server.restart')}
					</Button>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.server.note_title')}</CardTitle>
				<CardDescription>{t('myaaw.server.note_desc')}</CardDescription>
			</CardHeader>
		</Card>
	{/if}
</div>
