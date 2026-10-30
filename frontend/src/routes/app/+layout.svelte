<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Boxes,
		LayoutDashboard,
		MessagesSquare,
		FolderOpen,
		Brain,
		Sparkles,
		Cpu,
		Radio,
		Server,
		Settings2,
		LogOut,
		Wrench,
		SlidersHorizontal,
		Clock,
		ScrollText,
		ShieldCheck,
		Activity,
		Bot,
		PanelLeftClose,
		PanelLeftOpen
	} from 'lucide-svelte';
	import { agentSelection, authStore, sidebar, topBarSlot } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();
	const { t } = useTranslation();

	const fleet = {
		href: '/app',
		label: 'myaaw.nav.fleet',
		description: 'myaaw.fleet.subtitle',
		icon: Boxes
	};

	const nav = [
		{
			label: 'myaaw.nav.group_workspace',
			items: [
				{
					href: '/app/dashboard',
					label: 'myaaw.nav.dashboard',
					description: 'myaaw.home.subtitle',
					icon: LayoutDashboard
				},
				{
					href: '/app/chat',
					label: 'myaaw.nav.chat',
					description: 'myaaw.chat.subtitle',
					icon: MessagesSquare
				},
				{
					href: '/app/context',
					label: 'myaaw.nav.context',
					description: 'myaaw.context.subtitle',
					icon: Brain
				},
				{
					href: '/app/skills',
					label: 'myaaw.nav.skills',
					description: 'myaaw.skills.subtitle',
					icon: Sparkles
				},
				{
					href: '/app/tools',
					label: 'myaaw.nav.tools',
					description: 'myaaw.tools.subtitle',
					icon: Wrench
				},
				{
					href: '/app/contacts',
					label: 'myaaw.nav.contacts',
					description: 'myaaw.contacts.subtitle',
					icon: Bot
				},
				{
					href: '/app/files',
					label: 'myaaw.nav.files',
					description: 'myaaw.files.subtitle',
					icon: FolderOpen
				}
			]
		},
		{
			label: 'myaaw.nav.group_configure',
			items: [
				{
					href: '/app/providers',
					label: 'myaaw.nav.providers',
					description: 'myaaw.providers.subtitle',
					icon: Cpu
				},
				{
					href: '/app/channels',
					label: 'myaaw.nav.channels',
					description: 'myaaw.channels.subtitle',
					icon: Radio
				},
				{
					href: '/app/config',
					label: 'myaaw.nav.config',
					description: 'myaaw.config.subtitle',
					icon: SlidersHorizontal
				},
				{
					href: '/app/cron',
					label: 'myaaw.nav.cron',
					description: 'myaaw.cron.subtitle',
					icon: Clock
				}
			]
		},
		{
			label: 'myaaw.nav.group_operations',
			items: [
				{
					href: '/app/server',
					label: 'myaaw.nav.server',
					description: 'myaaw.server.subtitle',
					icon: Server
				},
				{
					href: '/app/logs',
					label: 'myaaw.nav.logs',
					description: 'myaaw.logs.subtitle',
					icon: ScrollText
				},
				{
					href: '/app/approvals',
					label: 'myaaw.nav.approvals',
					description: 'myaaw.approvals.subtitle',
					icon: ShieldCheck
				},
				{
					href: '/app/observability',
					label: 'myaaw.nav.observability',
					description: 'myaaw.observability.subtitle',
					icon: Activity
				},
				{
					href: '/app/settings',
					label: 'myaaw.nav.settings',
					description: 'myaaw.settings.subtitle',
					icon: Settings2
				}
			]
		}
	];

	const flatNav = nav.flatMap((group) => group.items);

	onMount(() => {
		void agentSelection.load();
	});

	function isActive(href: string) {
		if (href === '/app') return page.url.pathname === '/app';
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}

	let current = $derived(
		isActive(fleet.href) ? fleet : (flatNav.find((item) => isActive(item.href)) ?? flatNav[0])
	);
	let initials = $derived((authStore.profile?.username ?? 'MY').slice(0, 2).toUpperCase());
	let isChat = $derived(
		page.url.pathname === '/app/chat' || page.url.pathname.startsWith('/app/chat/')
	);
	// Pages with a split list + editor layout manage their own scrolling, so the
	// shell gives them the full height of the viewport instead of scrolling.
	let isFullHeight = $derived(
		isChat ||
			page.url.pathname.startsWith('/app/files') ||
			page.url.pathname.startsWith('/app/context') ||
			page.url.pathname.startsWith('/app/skills') ||
			page.url.pathname.startsWith('/app/logs') ||
			page.url.pathname.startsWith('/app/observability') ||
			page.url.pathname.startsWith('/app/tools') ||
			page.url.pathname.startsWith('/app/contacts')
	);

	async function logout() {
		await authStore.logout();
		await goto('/login');
	}

	function selectAgent(name: string) {
		agentSelection.select(name);
		// The fleet overview is not agent-scoped, so picking an agent there
		// moves to that agent's dashboard.
		if (page.url.pathname === '/app') {
			void goto('/app/dashboard');
		}
	}
</script>

{#snippet account(compact: boolean, align: 'start' | 'end' = 'end')}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				{#if compact}
					<Button
						{...props}
						variant="outline"
						size="icon-sm"
						class="rounded-full"
						aria-label={t('myaaw.nav.settings')}
					>
						<span class="text-[10px] font-bold uppercase">{initials}</span>
					</Button>
				{:else}
					<Button {...props} variant="outline" class="h-auto w-full justify-start gap-2 py-2">
						<span
							class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-bold uppercase"
						>
							{initials}
						</span>
						<span class="flex min-w-0 flex-col items-start leading-tight">
							<span class="w-full truncate text-sm font-medium">
								{authStore.profile?.username ?? '—'}
							</span>
							<span class="text-[10px] text-muted-foreground">
								user #{authStore.profile?.user_id ?? '—'}
							</span>
						</span>
					</Button>
				{/if}
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align={compact ? align : 'start'} class="w-56">
			<DropdownMenu.Label>
				<div class="flex flex-col">
					<span class="text-sm font-medium">{authStore.profile?.username ?? '—'}</span>
					<span class="text-xs text-muted-foreground">
						user #{authStore.profile?.user_id ?? '—'}
					</span>
				</div>
			</DropdownMenu.Label>
			<DropdownMenu.Separator />
			<DropdownMenu.Item onclick={() => goto('/app/settings')}>
				<Settings2 class="mr-2 h-4 w-4" />
				{t('myaaw.nav.settings')}
			</DropdownMenu.Item>
			<DropdownMenu.Separator />
			<DropdownMenu.Item variant="destructive" onclick={logout}>
				<LogOut class="mr-2 h-4 w-4" />
				{t('common.logout')}
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/snippet}

<div
	class={[
		'h-dvh bg-background transition-[padding] duration-200',
		sidebar.collapsed ? 'md:pl-16' : 'md:pl-60'
	]}
>
	<aside
		class={[
			'fixed inset-y-0 left-0 z-40 hidden flex-col border-r bg-card transition-[width] duration-200 md:flex',
			sidebar.collapsed ? 'w-16' : 'w-60'
		]}
	>
		{#if sidebar.collapsed}
			<div class="flex h-14 shrink-0 items-center justify-center border-b">
				<div
					class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"
				>
					<svg
						viewBox="0 0 1024 1024"
						class="h-4 w-4"
						fill="none"
						stroke="currentColor"
						stroke-width="104"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M262 724V330l250 266 250-266v394" />
					</svg>
				</div>
			</div>
			<button
				class="absolute top-4 -right-3 z-50 flex h-6 w-6 items-center justify-center rounded-full border bg-card text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground"
				onclick={() => sidebar.toggle()}
				title={t('myaaw.sidebar.expand')}
				aria-label={t('myaaw.sidebar.expand')}
			>
				<PanelLeftOpen class="h-3.5 w-3.5" />
			</button>
		{:else}
			<div class="flex h-14 shrink-0 items-center gap-2 border-b pl-4">
				<div
					class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"
				>
					<svg
						viewBox="0 0 1024 1024"
						class="h-4 w-4"
						fill="none"
						stroke="currentColor"
						stroke-width="104"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M262 724V330l250 266 250-266v394" />
					</svg>
				</div>
				<div class="flex min-w-0 flex-col leading-tight">
					<span class="text-sm font-bold tracking-tight">Myaaw</span>
					<span class="text-[10px] uppercase tracking-widest text-muted-foreground">
						Control plane
					</span>
				</div>
				<button
					class="ml-auto flex w-10 shrink-0 items-center justify-end self-stretch rounded-l-md pr-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={() => sidebar.toggle()}
					title={t('myaaw.sidebar.collapse')}
					aria-label={t('myaaw.sidebar.collapse')}
				>
					<PanelLeftClose class="h-4 w-4" />
				</button>
			</div>
		{/if}

		<!-- Fleet overview lives outside the agent scope, above the picker. -->
		<div class={sidebar.collapsed ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
			<a
				href={fleet.href}
				title={sidebar.collapsed ? t(fleet.label) : undefined}
				aria-current={isActive(fleet.href) ? 'page' : undefined}
				class={[
					'relative flex items-center rounded-md text-sm transition-colors',
					sidebar.collapsed ? 'justify-center px-0 py-2' : 'gap-2 px-2 py-1.5',
					isActive(fleet.href)
						? 'bg-accent font-medium text-accent-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				{#if sidebar.collapsed && isActive(fleet.href)}
					<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
				{/if}
				<Boxes class="h-4 w-4 shrink-0" />
				{#if !sidebar.collapsed}
					<span class="truncate">{t(fleet.label)}</span>
				{/if}
			</a>
		</div>

		<div class={sidebar.collapsed ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
			{#if sidebar.collapsed}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button
								{...props}
								variant="outline"
								size="icon-sm"
								class="mx-auto flex"
								title={agentSelection.current?.name ?? t('myaaw.agent.label')}
								aria-label={t('myaaw.agent.label')}
							>
								<Bot class="h-4 w-4" />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content side="right" align="start" class="w-48">
						{#each agentSelection.agents as item (item.name)}
							<DropdownMenu.Item onclick={() => selectAgent(item.name)}>
								<span class="truncate">{item.name}</span>
								{#if item.default}
									<span class="ml-auto text-[10px] text-muted-foreground">
										{t('myaaw.agent.default')}
									</span>
								{/if}
							</DropdownMenu.Item>
						{/each}
						{#if agentSelection.agents.length === 0}
							<p class="px-2 py-1 text-xs text-muted-foreground">{t('myaaw.agent.none')}</p>
						{/if}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{:else}
				<AgentPicker />
			{/if}
		</div>

		<nav class={['flex-1 overflow-y-auto', sidebar.collapsed ? 'space-y-2 p-2' : 'space-y-5 p-3']}>
			{#each nav as group, groupIndex (group.label)}
				<div class="space-y-1">
					{#if !sidebar.collapsed}
						<p
							class="px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground"
						>
							{t(group.label)}
						</p>
					{:else if groupIndex > 0}
						<div class="mx-auto my-2 h-px w-6 bg-border"></div>
					{/if}
					{#each group.items as item (item.href)}
						{@const Icon = item.icon}
						<a
							href={item.href}
							title={sidebar.collapsed ? t(item.label) : undefined}
							aria-current={isActive(item.href) ? 'page' : undefined}
							class={[
								'relative flex items-center rounded-md text-sm transition-colors',
								sidebar.collapsed ? 'justify-center px-0 py-2' : 'gap-2 px-2 py-1.5',
								isActive(item.href)
									? 'bg-accent font-medium text-accent-foreground'
									: 'text-muted-foreground hover:bg-muted hover:text-foreground'
							]}
						>
							{#if sidebar.collapsed && isActive(item.href)}
								<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
							{/if}
							<Icon class="h-4 w-4 shrink-0" />
							{#if !sidebar.collapsed}
								<span class="truncate">{t(item.label)}</span>
							{/if}
						</a>
					{/each}
				</div>
			{/each}
		</nav>

		<div class={sidebar.collapsed ? 'shrink-0 border-t p-2' : 'shrink-0 border-t p-3'}>
			{#if sidebar.collapsed}
				<div class="flex justify-center">{@render account(true, 'start')}</div>
			{:else}
				{@render account(false)}
			{/if}
		</div>
	</aside>

	<div class="flex h-dvh flex-col">
		<header class="shrink-0 border-b bg-background/80 backdrop-blur">
			<div class="flex h-14 items-center gap-3 px-4">
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2">
						<h1 class="truncate text-sm font-semibold tracking-tight">
							{t(current.label)}
						</h1>
						{#if isChat}
							<Badge variant="outline" class="hidden shrink-0 sm:inline-flex">
								{agentSelection.current?.name ?? agentSelection.selected ?? '—'}
							</Badge>
						{/if}
					</div>
					<p class="hidden truncate text-[11px] text-muted-foreground sm:block">
						{t(current.description)}
					</p>
				</div>

				<div class="flex shrink-0 items-center gap-2">
					{#if topBarSlot.current}
						{@render topBarSlot.current.content()}
					{/if}

					<div class="flex items-center gap-2 md:hidden">
						<div class="w-24">
							<AgentPicker compact />
						</div>
						{@render account(true)}
					</div>
				</div>
			</div>

			<nav class="flex gap-1 overflow-x-auto border-t px-3 py-2 md:hidden">
				<a
					href={fleet.href}
					aria-current={isActive(fleet.href) ? 'page' : undefined}
					class={[
						'whitespace-nowrap rounded-md px-2 py-1 text-xs transition-colors',
						isActive(fleet.href)
							? 'bg-accent font-medium text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{t(fleet.label)}
				</a>
				{#each flatNav as item (item.href)}
					<a
						href={item.href}
						aria-current={isActive(item.href) ? 'page' : undefined}
						class={[
							'whitespace-nowrap rounded-md px-2 py-1 text-xs transition-colors',
							isActive(item.href)
								? 'bg-accent font-medium text-accent-foreground'
								: 'text-muted-foreground hover:bg-muted hover:text-foreground'
						]}
					>
						{t(item.label)}
					</a>
				{/each}
			</nav>
		</header>

		<main
			class={isFullHeight ? 'min-h-0 flex-1 overflow-hidden' : 'min-h-0 flex-1 overflow-y-auto'}
		>
			{#if isFullHeight}
				<div class="h-full">{@render children()}</div>
			{:else}
				<div class="w-full p-4 lg:p-6">
					{@render children()}
				</div>
			{/if}
		</main>
	</div>
</div>
