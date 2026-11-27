<script lang="ts">
	import {
		Bot,
		ChevronDown,
		Cpu,
		Download,
		ExternalLink,
		KeyRound,
		LogOut,
		Plus,
		RefreshCw,
		Settings2,
		Terminal
	} from 'lucide-svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import RuntimeInstallDialog from './RuntimeInstallDialog.svelte';
	import RuntimeAuthDialog from './RuntimeAuthDialog.svelte';
	import RuntimeDefaultsDialog from './RuntimeDefaultsDialog.svelte';
	import { runtimesManageStore } from '$lib/stores/runtimes.svelte';
	import { createAgentDialog } from '$lib/stores/createAgentDialog.svelte';
	import { useQuery } from '$lib/stores/useQuery.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { formatRuntimeModelValue } from '$lib/utils/runtimeTools';
	import type { ManagedRuntimeView } from '$lib/types';

	const { t } = useTranslation();

	const compactBtn = 'h-7 gap-1.5 px-2.5 text-xs';

	// Serves cached runtime data immediately across tab switches and page navigations,
	// revalidating silently in the background when stale without showing skeletons again.
	useQuery(runtimesManageStore.query, () => ({}));

	const cached = $derived(runtimesManageStore.data);
	const loading = $derived(runtimesManageStore.loading && !cached);
	const refreshing = $derived(runtimesManageStore.refreshing);
	const probingAuth = $derived(runtimesManageStore.probingAuth);
	const loadError = $derived(runtimesManageStore.error);
	const managementEnabled = $derived(cached?.management_enabled ?? true);
	const runtimes = $derived(cached?.runtimes ?? []);

	// Install / Update modal state
	let installOpen = $state(false);
	let installMode = $state<'install' | 'update'>('install');
	let installTarget = $state<ManagedRuntimeView | null>(null);

	// Auth PTY modal state
	let authOpen = $state(false);
	let authMode = $state<'login' | 'logout'>('login');
	let authTarget = $state<ManagedRuntimeView | null>(null);

	// Global Defaults modal state
	let defaultsOpen = $state(false);
	let defaultsTarget = $state<ManagedRuntimeView | null>(null);

	const countableRuntimes = $derived(runtimes.filter((r) => Boolean(r.default_command)));
	const installedCount = $derived(countableRuntimes.filter((r) => r.installed).length);

	function tOr(key: string, fallback: string): string {
		const translated = t(key);
		return translated === key ? fallback : translated;
	}

	function runtimeLabel(rt: ManagedRuntimeView): string {
		if (rt.id === 'custom_acp') return t('myaaw.onboard.runtime_custom_acp');
		return rt.label;
	}

	function runtimeDescription(rt: ManagedRuntimeView): string {
		return tOr(`myaaw.onboard.runtime_preset_${rt.id}_desc`, rt.description ?? '');
	}

	function shortenHomePath(fullPath: string): string {
		return fullPath
			.replace(/^\/Users\/[^/]+\//, '~/')
			.replace(/^\/home\/[^/]+\//, '~/')
			.replace(/^[A-Za-z]:\\Users\\[^\\]+\\/, '~\\');
	}

	function recommendedInstallCommand(rt: ManagedRuntimeView): string {
		const preferred =
			rt.install_methods.find((m) => m.available && !m.manual_download && m.recommended) ??
			rt.install_methods.find((m) => m.available && m.recommended) ??
			rt.install_methods.find((m) => !m.manual_download) ??
			rt.install_methods[0];
		return preferred?.command_preview ?? '';
	}

	function replaceRuntime(updated: ManagedRuntimeView) {
		runtimesManageStore.replaceOne(updated);
		if (defaultsTarget?.id === updated.id) {
			defaultsTarget = updated;
		}
		if (authTarget?.id === updated.id) {
			authTarget = updated;
		}
		if (installTarget?.id === updated.id) {
			installTarget = updated;
		}
	}

	function openInstaller(rt: ManagedRuntimeView, mode: 'install' | 'update') {
		installTarget = rt;
		installMode = mode;
		installOpen = true;
	}

	function openAuth(rt: ManagedRuntimeView, mode: 'login' | 'logout') {
		authTarget = rt;
		authMode = mode;
		authOpen = true;
	}

	function openDefaults(rt: ManagedRuntimeView) {
		defaultsTarget = rt;
		defaultsOpen = true;
	}
</script>

<div class="space-y-4">
	<Card>
		<CardHeader class="flex flex-row flex-wrap items-start justify-between gap-3 space-y-0">
			<div class="space-y-1">
				<CardTitle class="flex items-center gap-2">
					<Cpu class="h-4 w-4" />
					{t('myaaw.settings.runtimes')}
				</CardTitle>
				<CardDescription>{t('myaaw.settings.runtimes_desc')}</CardDescription>
			</div>
			<div class="flex items-center gap-2">
				{#if !loading && countableRuntimes.length > 0}
					<Badge variant="secondary" class="tabular-nums">
						{t('myaaw.settings.runtimes_summary', {
							installed: installedCount,
							total: countableRuntimes.length
						})}
					</Badge>
				{/if}
				<Button
					variant="outline"
					size="sm"
					onclick={() => void runtimesManageStore.refresh()}
					disabled={loading || refreshing}
					class="gap-1.5"
				>
					<RefreshCw class={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />
					{t('common.refresh')}
				</Button>
			</div>
		</CardHeader>

		{#if !managementEnabled}
			<CardContent class="pt-0">
				<div
					class="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300"
				>
					{t('myaaw.settings.runtimes_disabled_notice')}
				</div>
			</CardContent>
		{/if}
	</Card>

	{#if loadError}
		<div
			class="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{loadError}
		</div>
	{/if}

	{#if loading}
		{#each Array.from({ length: 4 }) as _, idx (idx)}
			<Skeleton class="h-28 w-full rounded-xl" />
		{/each}
	{:else}
		<div class="space-y-2.5">
			{#each runtimes as rt (rt.id)}
				{@const installPreview = !rt.installed ? recommendedInstallCommand(rt) : ''}
				<div class="overflow-hidden rounded-xl border bg-card shadow-xs transition-colors">
					<!-- Top Zone: Full-width identity, status pills, and single-measure description -->
					<div class="flex items-start gap-3 px-4 py-3">
						<RuntimeBadge
							runtimeType={rt.type}
							runtimePreset={rt.id}
							size="lg"
							inline
							class="mt-0.5"
						/>

						<div class="min-w-0 flex-1 space-y-1">
							<div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
								<div class="flex min-w-0 flex-wrap items-center gap-2">
									<h3 class="text-sm font-semibold text-foreground">{runtimeLabel(rt)}</h3>
									{#if rt.docs_url}
										<a
											href={rt.docs_url}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-0.5 text-[11px] text-muted-foreground transition-colors hover:text-primary hover:underline"
										>
											{t('myaaw.settings.runtimes_docs')}
											<ExternalLink class="h-3 w-3" />
										</a>
									{/if}
								</div>

								<div class="flex flex-wrap items-center gap-1.5">
									{#if rt.id !== 'custom_acp' || rt.installed}
										{#if rt.installed}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:text-emerald-300"
											>
												{t('myaaw.settings.runtimes_installed')}{rt.version
													? ` · ${rt.version}`
													: ''}
											</span>
										{:else}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
											>
												{t('myaaw.settings.runtimes_not_installed')}
											</span>
										{/if}
									{/if}

									{#if rt.installed || rt.defaults.api_key.configured}
										{#if rt.auth.status === 'authenticated'}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[11px] font-medium text-primary"
											>
												{t('myaaw.settings.runtimes_auth_authenticated')}{rt.auth.detail
													? ` · ${rt.auth.detail}`
													: ''}
											</span>
										{:else if rt.auth.status === 'free_tier'}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-teal-500/15 px-2 py-0.5 text-[11px] font-medium text-teal-700 dark:text-teal-300"
											>
												{t('myaaw.settings.runtimes_auth_free_tier')}{rt.auth.detail
													? ` · ${rt.auth.detail}`
													: ''}
											</span>
										{:else if rt.auth.status === 'api_key_configured'}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-sky-500/15 px-2 py-0.5 text-[11px] font-medium text-sky-700 dark:text-sky-300"
											>
												{t('myaaw.settings.runtimes_auth_api_key')}{rt.auth.detail
													? ` · ${rt.auth.detail}`
													: ''}
											</span>
										{:else if rt.auth.status === 'unauthenticated'}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300"
											>
												{t('myaaw.settings.runtimes_auth_unauthenticated')}
											</span>
										{:else if probingAuth && rt.installed && rt.id !== 'custom_acp'}
											<span
												class="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
											>
												<RefreshCw class="h-2.5 w-2.5 animate-spin" />
												{t('myaaw.settings.runtimes_auth_checking')}
											</span>
										{/if}
									{/if}
								</div>
							</div>

							<p class="text-xs leading-relaxed text-muted-foreground">
								{runtimeDescription(rt)}
							</p>
						</div>
					</div>

					<!-- Bottom Action & Metadata Bar: Zero wasted space -->
					<div
						class="flex flex-col gap-2 border-t border-border/60 bg-muted/25 px-4 py-2 sm:flex-row sm:items-center sm:justify-between sm:gap-x-3"
					>
						<div class="flex min-w-0 flex-1 items-center gap-2 text-[11px] text-muted-foreground">
							{#if rt.binary_path}
								<span
									title={rt.binary_path}
									class="inline-flex min-w-0 items-center gap-1 truncate font-mono text-[11px] text-muted-foreground"
								>
									<Terminal class="h-3 w-3 shrink-0 opacity-70" />
									<span class="truncate">{shortenHomePath(rt.binary_path)}</span>
								</span>
							{:else if installPreview}
								<span
									title={installPreview}
									class="inline-flex min-w-0 items-center gap-1 truncate font-mono text-[11px] text-muted-foreground/85"
								>
									<Terminal class="h-3 w-3 shrink-0 opacity-65" />
									<span class="truncate">{installPreview}</span>
								</span>
							{:else}
								<span class="truncate font-mono text-[11px] text-muted-foreground/80">
									ACP · JSON-RPC stdio
								</span>
							{/if}

							{#if rt.defaults.model}
								{@const displayModel = formatRuntimeModelValue(rt.defaults.model)}
								<span class="shrink-0 text-muted-foreground/40">·</span>
								<span
									class="max-w-36 shrink-0 truncate rounded border bg-background/70 px-1.5 py-0.5 font-mono text-[10px] text-foreground/85"
									title={displayModel}
								>
									{displayModel}
								</span>
							{/if}

							{#if rt.used_by_agents.length > 0}
								<span class="shrink-0 text-muted-foreground/40">·</span>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}
											<button
												{...props}
												type="button"
												title={rt.used_by_agents.join(', ')}
												class="inline-flex shrink-0 items-center gap-1 rounded-md border bg-background px-1.5 py-0.5 text-[11px] font-medium text-foreground transition-colors hover:border-primary/50 hover:text-primary focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
											>
												<Bot class="h-3 w-3 text-muted-foreground" />
												<span class="tabular-nums">
													{rt.used_by_agents.length === 1
														? t('myaaw.settings.runtimes_agents_one')
														: t('myaaw.settings.runtimes_agents_many', {
																count: rt.used_by_agents.length
															})}
												</span>
												<ChevronDown class="h-3 w-3 text-muted-foreground" />
											</button>
										{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="start" class="max-h-56 w-52 overflow-y-auto">
										<DropdownMenu.Label
											class="text-[10px] tracking-wide text-muted-foreground uppercase"
										>
											{t('myaaw.settings.runtimes_used_by')} ({rt.used_by_agents.length})
										</DropdownMenu.Label>
										<DropdownMenu.Separator />
										{#each rt.used_by_agents as agentName (agentName)}
											<DropdownMenu.Item class="p-0">
												<a
													href={`/app/new?agent=${encodeURIComponent(agentName)}`}
													class="flex w-full items-center gap-2 px-2 py-1.5 font-mono text-xs text-foreground"
												>
													<Bot class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
													<span class="truncate">{agentName}</span>
												</a>
											</DropdownMenu.Item>
										{/each}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						</div>

						<div class="flex shrink-0 flex-wrap items-center gap-1.5 sm:flex-nowrap">
							{#if !rt.installed && rt.install_methods.length > 0}
								<Button
									size="sm"
									disabled={!managementEnabled}
									onclick={() => openInstaller(rt, 'install')}
									class={compactBtn}
								>
									<Download class="h-3 w-3" />
									{t('myaaw.settings.runtimes_install')}
								</Button>
							{/if}

							{#if rt.installed && rt.auth.supports_login}
								{#if rt.auth.status === 'authenticated' && rt.auth.supports_logout}
									<DropdownMenu.Root>
										<DropdownMenu.Trigger>
											{#snippet child({ props })}
												<Button
													{...props}
													size="sm"
													variant="outline"
													disabled={!managementEnabled}
													class={compactBtn}
												>
													<KeyRound class="h-3 w-3" />
													{t('myaaw.settings.runtimes_relogin')}
													<ChevronDown class="h-3 w-3 opacity-60" />
												</Button>
											{/snippet}
										</DropdownMenu.Trigger>
										<DropdownMenu.Content align="end" class="w-52">
											<DropdownMenu.Item
												onclick={() => openAuth(rt, 'login')}
												class="gap-2 text-xs"
											>
												<KeyRound class="h-3.5 w-3.5 text-muted-foreground" />
												{t('myaaw.settings.runtimes_relogin_action')}
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
											<DropdownMenu.Item
												onclick={() => openAuth(rt, 'logout')}
												class="gap-2 text-xs text-destructive focus:text-destructive"
											>
												<LogOut class="h-3.5 w-3.5" />
												{t('myaaw.settings.runtimes_logout')}
											</DropdownMenu.Item>
										</DropdownMenu.Content>
									</DropdownMenu.Root>
								{:else}
									<Button
										size="sm"
										variant={rt.auth.status === 'unauthenticated' ? 'default' : 'outline'}
										disabled={!managementEnabled}
										onclick={() => openAuth(rt, 'login')}
										class={compactBtn}
									>
										<KeyRound class="h-3 w-3" />
										{rt.auth.status === 'authenticated'
											? t('myaaw.settings.runtimes_relogin')
											: rt.auth.status === 'free_tier'
												? t('myaaw.settings.runtimes_connect_provider')
												: t('myaaw.settings.runtimes_login')}
									</Button>
								{/if}
							{/if}

							{#if rt.installed && rt.update_method}
								<Button
									size="sm"
									variant="outline"
									disabled={!managementEnabled}
									onclick={() => openInstaller(rt, 'update')}
									class={compactBtn}
								>
									<RefreshCw class="h-3 w-3" />
									{t('myaaw.settings.runtimes_update')}
								</Button>
							{/if}

							<Button
								size="sm"
								variant="outline"
								onclick={() => openDefaults(rt)}
								class={compactBtn}
							>
								<Settings2 class="h-3 w-3" />
								{t('myaaw.settings.runtimes_configure')}
							</Button>

							{#if rt.installed || rt.id === 'custom_acp'}
								<Button
									size="sm"
									variant="outline"
									onclick={() =>
										createAgentDialog.openCreate({
											runtime: rt.id,
											onSuccess: () => {
												void runtimesManageStore.list(true);
											}
										})}
									class={compactBtn}
								>
									<Plus class="h-3 w-3" />
									{t('myaaw.settings.runtimes_create_agent')}
								</Button>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<RuntimeInstallDialog
	bind:open={installOpen}
	runtime={installTarget}
	mode={installMode}
	onCompleted={replaceRuntime}
/>

<RuntimeAuthDialog
	bind:open={authOpen}
	runtime={authTarget}
	mode={authMode}
	onFinished={replaceRuntime}
/>

<RuntimeDefaultsDialog bind:open={defaultsOpen} runtime={defaultsTarget} onSaved={replaceRuntime} />
