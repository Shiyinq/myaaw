<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Boxes,
		Keyboard,
		Languages,
		LayoutDashboard,
		Play,
		MessagesSquare,
		MessageSquarePlus,
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
		Square,
		Activity,
		Archive,
		Bot,
		SquareTerminal,
		RotateCw,
		PanelLeft,
		PanelLeftClose,
		PanelLeftOpen,
		Menu,
		UserRound,
		X
	} from 'lucide-svelte';
	import {
		agentSelection,
		authStore,
		commandPalette,
		conversationHistory,
		shortcutHelp,
		sidebar,
		toastStore,
		topBarSlot,
		topBarTitle
	} from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isOverlayOpen, isTypingTarget, matchesShortcut } from '$lib/utils/keys';
	import type { Command } from '$lib/commands';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import { server as serverApi } from '$lib/apis/server';
	import { getErrorMessage } from '$lib/utils/api';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import ConversationList from '$lib/components/app/ConversationList.svelte';
	import Logo from '$lib/components/common/Logo.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import ShortcutHelp from '$lib/components/app/ShortcutHelp.svelte';
	import CommandPalette from '$lib/components/app/CommandPalette.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { theme, userPrefersMode } from 'mode-watcher';
	import { APPEARANCE, applyAppearance, isAppearanceActive } from '$lib/theme';
	import { preferences } from '$lib/stores/preferences.svelte';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();
	const { t, locale, changeLocale, availableLocales } = useTranslation();

	// On a phone the same sidebar is a drawer: closed until the header opens it,
	// and always showing the labels, because an icon rail that slides in over the
	// page explains nothing. `compact` is the desktop-only "icons and no labels"
	// state, so a phone ignores a collapse that was chosen on a wide screen.
	let drawerOpen = $state(false);
	let wide = $state(false);
	// Simple mode is a chat-first shell: no Fleet shortcut and no page groups,
	// just the agent picker and the chat list. Collapsing it keeps the same
	// minimal rail on desktop: brand, agent, new chat and the profile menu.
	let simpleMode = $derived(preferences.value.simpleMode);
	let compactSidebar = $derived(sidebar.collapsed && wide);
	let activeConversation = $derived(page.params.conversation ?? '');
	let simpleSearchInput = $state<HTMLInputElement | null>(null);

	// The chat list belongs to the shell, so the chat page's history panel and the
	// simple-mode sidebar share one fetch (see stores/conversationHistory).
	conversationHistory.init();

	$effect(() => {
		if (typeof window === 'undefined') return;
		const query = window.matchMedia('(min-width: 768px)');
		const sync = () => {
			wide = query.matches;
			if (wide) drawerOpen = false;
		};
		sync();
		query.addEventListener('change', sync);
		return () => query.removeEventListener('change', sync);
	});

	// A drawer that stays open hides the page it just navigated to.
	afterNavigate(() => {
		drawerOpen = false;
	});

	$effect(() => {
		if (typeof window === 'undefined') return;
		const onKey = (event: KeyboardEvent) => {
			if (event.defaultPrevented) return;
			// The main sidebar belongs to the shell, so this works on every page.
			if (matchesShortcut(event, 'mod+backslash')) {
				event.preventDefault();
				if (wide) sidebar.toggle();
				else drawerOpen = !drawerOpen;
				return;
			}
			if (event.key === 'Escape' && drawerOpen) {
				event.preventDefault();
				drawerOpen = false;
				return;
			}
			// The palette is reachable from every page, including while it is open.
			if (matchesShortcut(event, 'mod+k')) {
				event.preventDefault();
				commandPalette.toggle();
				return;
			}
			// In simple mode the chat list lives in the sidebar, so the shortcut has
			// to reach its search box; the chat page handles its own panel otherwise.
			if (matchesShortcut(event, 'mod+shift+s') && simpleMode) {
				event.preventDefault();
				void focusSimpleSearch();
				return;
			}
			if (matchesShortcut(event, '?') && !isTypingTarget(event.target) && !isOverlayOpen()) {
				event.preventDefault();
				shortcutHelp.show();
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

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
				},
				{
					href: '/app/terminal',
					label: 'myaaw.nav.terminal',
					description: 'myaaw.terminal.subtitle',
					icon: SquareTerminal
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
					href: '/app/backups',
					label: 'myaaw.nav.backups',
					description: 'myaaw.backups.subtitle',
					icon: Archive
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
				}
			]
		}
	];

	const flatNav = nav.flatMap((group) => group.items);

	// Routes that have a title in the top bar but no sidebar entry. Profile and
	// settings are reachable from the account menu instead of the agent-scoped
	// navigation.
	const hiddenNav = [
		{
			href: '/app/new',
			label: 'myaaw.onboard.title',
			description: 'myaaw.onboard.subtitle',
			icon: Sparkles
		},
		{
			href: '/app/profile',
			label: 'myaaw.nav.profile',
			description: 'myaaw.profile.subtitle',
			icon: UserRound
		},
		{
			href: '/app/settings',
			label: 'myaaw.nav.settings',
			description: 'myaaw.settings.subtitle',
			icon: Settings2
		}
	];

	onMount(() => {
		void agentSelection.load();
	});

	// The palette lists each agent's start/stop/restart, so refresh the daemon
	// states as it opens instead of trusting a possibly stale list.
	$effect(() => {
		if (commandPalette.open) void agentSelection.load(true);
	});

	// Everything the palette can reach: every destination of the shell, every
	// agent, and the actions that have no page of their own.
	let commands = $derived.by<Command[]>(() => {
		const list: Command[] = [];
		const navigate = (
			href: string,
			labelKey: string,
			descriptionKey: string,
			icon: Command['icon']
		) => {
			list.push({ id: `nav:${href}`, labelKey, descriptionKey, icon, group: 'navigate', href });
		};

		navigate(fleet.href, fleet.label, fleet.description, fleet.icon);
		for (const group of nav) {
			for (const item of group.items) {
				navigate(item.href, item.label, item.description, item.icon);
			}
		}
		for (const item of hiddenNav) {
			navigate(item.href, item.label, item.description, item.icon);
		}

		for (const agent of agentSelection.agents) {
			list.push({
				id: `agent:${agent.name}`,
				labelKey: 'myaaw.command.switch_to',
				params: { name: agent.name },
				aliases: ['agent', 'agen', agent.name],
				group: 'agent',
				icon: Bot,
				current: agent.name === agentSelection.selected,
				run: () => selectAgent(agent.name)
			});
			// Only the actions that make sense for the current daemon state.
			if (agent.running) {
				list.push({
					id: `agent-restart:${agent.name}`,
					labelKey: 'myaaw.command.restart_agent',
					params: { name: agent.name },
					aliases: ['restart', agent.name],
					group: 'agent',
					icon: RotateCw,
					run: () => runAgentAction('restart', agent.name)
				});
				list.push({
					id: `agent-stop:${agent.name}`,
					labelKey: 'myaaw.command.stop_agent',
					params: { name: agent.name },
					aliases: ['stop', 'hentikan', agent.name],
					group: 'agent',
					icon: Square,
					run: () => runAgentAction('stop', agent.name)
				});
			} else {
				list.push({
					id: `agent-start:${agent.name}`,
					labelKey: 'myaaw.command.start_agent',
					params: { name: agent.name },
					aliases: ['start', 'jalankan', agent.name],
					group: 'agent',
					icon: Play,
					run: () => runAgentAction('start', agent.name)
				});
			}
		}

		// The palette can flip the shell between the chat-first and the default
		// (standard) layout without a trip to Settings.
		list.push({
			id: 'action:simple-mode',
			labelKey: preferences.value.simpleMode
				? 'myaaw.command.simple_mode_disable'
				: 'myaaw.command.simple_mode_enable',
			aliases: ['simple', 'sederhana', 'standard', 'standar', 'mode', 'sidebar', 'layout'],
			group: 'appearance',
			icon: PanelLeft,
			run: () => preferences.update({ simpleMode: !preferences.value.simpleMode })
		});
		for (const option of APPEARANCE) {
			list.push({
				id: `action:theme:${option.id}`,
				labelKey: option.labelKey,
				aliases: ['theme', 'tema', ...option.aliases],
				group: 'appearance',
				icon: option.icon,
				current: isAppearanceActive(option, userPrefersMode.current, theme.current),
				run: () => applyAppearance(option)
			});
		}
		for (const item of availableLocales) {
			list.push({
				id: `action:locale:${item.code}`,
				name: item.nativeName,
				aliases:
					item.code === 'id'
						? ['language', 'bahasa', 'indonesia', 'indonesian', 'id']
						: ['language', 'bahasa', 'english', 'inggris', 'en'],
				group: 'language',
				icon: Languages,
				current: locale.value === item.code,
				run: () => changeLocale(item.code)
			});
		}

		list.push({
			id: 'action:shortcuts',
			labelKey: 'myaaw.shortcuts.title',
			aliases: ['keyboard', 'papan tombol', 'shortcut'],
			group: 'action',
			icon: Keyboard,
			run: () => shortcutHelp.show()
		});
		list.push({
			id: 'action:logout',
			labelKey: 'common.logout',
			aliases: ['sign out', 'keluar'],
			group: 'action',
			icon: LogOut,
			destructive: true,
			run: async () => {
				const confirmed = await askConfirm({
					message: t('myaaw.command.logout_confirm'),
					variant: 'destructive'
				});
				if (confirmed) await logout();
			}
		});

		return list;
	});

	function isActive(href: string) {
		if (href === '/app') return page.url.pathname === '/app';
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}

	let current = $derived(
		isActive(fleet.href)
			? fleet
			: (hiddenNav.find((item) => isActive(item.href)) ??
					flatNav.find((item) => isActive(item.href)) ??
					flatNav[0])
	);
	let initials = $derived((authStore.profile?.username ?? 'MY').slice(0, 2).toUpperCase());
	let isChat = $derived(
		page.url.pathname === '/app/chat' || page.url.pathname.startsWith('/app/chat/')
	);
	// Pages with a split list + editor layout manage their own scrolling, so the
	// shell gives them the full height of the viewport instead of scrolling.
	let isFullHeight = $derived(
		isChat ||
			page.url.pathname.startsWith('/app/dashboard') ||
			page.url.pathname.startsWith('/app/files') ||
			page.url.pathname.startsWith('/app/terminal') ||
			page.url.pathname.startsWith('/app/context') ||
			page.url.pathname.startsWith('/app/skills') ||
			page.url.pathname.startsWith('/app/logs') ||
			page.url.pathname.startsWith('/app/observability') ||
			page.url.pathname.startsWith('/app/tools') ||
			page.url.pathname.startsWith('/app/contacts')
	);

	/** Opens the chat list (from the rail if needed) and focuses its search box. */
	async function focusSimpleSearch() {
		if (!simpleMode) return;
		if (sidebar.collapsed) sidebar.toggle();
		await tick();
		simpleSearchInput?.focus();
		simpleSearchInput?.select();
	}

	/** Starts, stops or restarts one agent from any page, badge flipped first. */
	async function runAgentAction(kind: 'start' | 'stop' | 'restart', name: string) {
		const running = kind !== 'stop';
		agentSelection.setRunning(name, running);
		try {
			if (kind === 'start') await serverApi.start(name);
			else if (kind === 'stop') await serverApi.stop(name);
			else await serverApi.restart(name);
			toastStore.success(
				t(
					kind === 'start'
						? 'myaaw.server.start_done'
						: kind === 'stop'
							? 'myaaw.server.stop_done'
							: 'myaaw.server.restart_done'
				)
			);
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			await agentSelection.load(true);
		}
	}

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
						aria-label={t('myaaw.nav.profile')}
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
			<DropdownMenu.Item
				class="flex-col items-start gap-0 p-2"
				onclick={() => goto('/app/profile')}
			>
				<span class="text-sm font-medium">{authStore.profile?.username ?? '—'}</span>
				<span class="text-xs text-muted-foreground">
					user #{authStore.profile?.user_id ?? '—'}
				</span>
			</DropdownMenu.Item>
			<DropdownMenu.Separator />
			<DropdownMenu.Item onclick={() => goto('/app/settings')}>
				<Settings2 class="mr-2 h-4 w-4" />
				{t('myaaw.nav.settings')}
			</DropdownMenu.Item>
			<DropdownMenu.Item onclick={() => shortcutHelp.show()}>
				<Keyboard class="mr-2 h-4 w-4" />
				{t('myaaw.shortcuts.title')}
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
		'h-dvh bg-background page-clear transition-[padding] duration-200',
		compactSidebar ? 'md:pl-16' : 'md:pl-60'
	]}
>
	{#if drawerOpen}
		<!-- The drawer covers the page it came from, so the tap that dismisses it must
		     land somewhere that is not a link. -->
		<button
			type="button"
			class="fixed inset-0 z-40 bg-black/40 md:hidden"
			aria-label={t('myaaw.sidebar.close')}
			onclick={() => (drawerOpen = false)}
		></button>
	{/if}
	<aside
		inert={!wide && !drawerOpen}
		class={[
			'fixed inset-y-0 left-0 z-50 flex w-full flex-col border-r bg-card shadow-xl frost-drawer transition-transform duration-200 md:z-40 md:translate-x-0 md:shadow-none md:transition-[width]',
			drawerOpen ? 'translate-x-0' : '-translate-x-full',
			compactSidebar ? 'md:w-16' : 'md:w-60'
		]}
	>
		<!-- One header for both states: the brand block keeps its position while the
		     labels fade and the width animates, so collapsing does not swap the
		     whole header in one frame. -->
		<div class="relative flex h-14 shrink-0 items-center gap-2 overflow-hidden border-b pl-4">
			<div
				class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground"
			>
				<Logo class="h-4.5 w-4.5" />
			</div>
			<div
				class={[
					'flex min-w-0 flex-col overflow-hidden leading-tight whitespace-nowrap transition-all duration-200',
					compactSidebar ? 'max-w-0 opacity-0' : 'max-w-44 opacity-100'
				]}
			>
				<span class="text-sm font-bold tracking-tight">Myaaw</span>
				<span class="text-[10px] uppercase tracking-widest text-muted-foreground">
					Control plane
				</span>
			</div>
			<button
				class={[
					'ml-auto hidden w-10 shrink-0 items-center justify-end self-stretch rounded-l-md pr-1 text-muted-foreground transition-[color,opacity] hover:bg-muted hover:text-foreground md:flex',
					compactSidebar ? 'pointer-events-none opacity-0' : 'opacity-100'
				]}
				onclick={() => sidebar.toggle()}
				title={t('myaaw.sidebar.collapse')}
				aria-label={t('myaaw.sidebar.collapse')}
				aria-keyshortcuts="Control+\\ Meta+\\"
			>
				<PanelLeftClose class="h-4 w-4" />
			</button>
			<button
				class="ml-auto flex w-10 shrink-0 items-center justify-end self-stretch rounded-l-md pr-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground md:hidden"
				onclick={() => (drawerOpen = false)}
				title={t('myaaw.sidebar.close')}
				aria-label={t('myaaw.sidebar.close')}
			>
				<X class="h-4 w-4" />
			</button>
		</div>
		{#if compactSidebar}
			<button
				class="absolute top-4 -right-3 z-50 flex h-6 w-6 items-center justify-center rounded-full border bg-card text-muted-foreground shadow-sm frost-card transition-colors hover:bg-muted hover:text-foreground"
				onclick={() => sidebar.toggle()}
				title={t('myaaw.sidebar.expand')}
				aria-label={t('myaaw.sidebar.expand')}
				aria-keyshortcuts="Control+\\ Meta+\\"
			>
				<PanelLeftOpen class="h-3.5 w-3.5" />
			</button>
		{/if}

		<!-- Fleet overview lives outside the agent scope, above the picker. -->
		{#if !simpleMode}
			<div class={compactSidebar ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
				<a
					href={fleet.href}
					title={compactSidebar ? t(fleet.label) : undefined}
					aria-current={isActive(fleet.href) ? 'page' : undefined}
					class={[
						'relative flex items-center rounded-md text-sm transition-colors list-row',
						compactSidebar ? 'justify-center px-0 py-2' : 'gap-2 px-2 py-1.5',
						isActive(fleet.href)
							? 'bg-accent font-medium text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{#if compactSidebar && isActive(fleet.href)}
						<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
					{/if}
					<Boxes class="h-4 w-4 shrink-0" />
					{#if !compactSidebar}
						<span class="truncate">{t(fleet.label)}</span>
					{/if}
				</a>
			</div>
		{/if}

		<div class={compactSidebar ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
			{#if compactSidebar}
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

		{#if simpleMode}
			{#if compactSidebar}
				<div class="shrink-0 border-b p-2">
					<Button
						variant="ghost"
						size="icon-sm"
						class="mx-auto flex"
						onclick={() => void goto('/app/chat?new=1')}
						title={t('myaaw.chat.new')}
						aria-label={t('myaaw.chat.new')}
					>
						<MessageSquarePlus class="h-4 w-4" />
					</Button>
				</div>
				<div class="flex-1"></div>
			{:else}
				<div class="flex min-h-0 flex-1 flex-col">
					<ConversationList
						activeId={activeConversation}
						onOpen={(id) => void goto(`/app/chat/${id}`)}
						onNew={() => void goto('/app/chat?new=1')}
						onNewInFolder={(folderId) =>
							void goto(`/app/chat?new=1&folder=${encodeURIComponent(folderId)}`)}
						onDeleted={(id) => {
							if (id === activeConversation) void goto('/app/chat?new=1');
						}}
						bind:searchInput={simpleSearchInput}
					/>
				</div>
			{/if}
		{:else}
			<nav class={['flex-1 overflow-y-auto', compactSidebar ? 'space-y-2 p-2' : 'space-y-5 p-3']}>
				{#each nav as group, groupIndex (group.label)}
					<div class="space-y-1">
						{#if !compactSidebar}
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
								title={compactSidebar ? t(item.label) : undefined}
								aria-current={isActive(item.href) ? 'page' : undefined}
								class={[
									'relative flex items-center rounded-md text-sm transition-colors list-row',
									compactSidebar ? 'justify-center px-0 py-2' : 'gap-2 px-2 py-1.5',
									isActive(item.href)
										? 'bg-accent font-medium text-accent-foreground'
										: 'text-muted-foreground hover:bg-muted hover:text-foreground'
								]}
							>
								{#if compactSidebar && isActive(item.href)}
									<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
								{/if}
								<Icon class="h-4 w-4 shrink-0" />
								{#if !compactSidebar}
									<span class="truncate">{t(item.label)}</span>
								{/if}
							</a>
						{/each}
					</div>
				{/each}
			</nav>
		{/if}

		<div class={compactSidebar ? 'shrink-0 border-t p-2' : 'shrink-0 border-t p-3'}>
			{#if compactSidebar}
				<div class="flex justify-center">{@render account(true, 'start')}</div>
			{:else}
				{@render account(false)}
			{/if}
		</div>
	</aside>

	<div class="flex h-dvh flex-col">
		<header class="h-14 shrink-0 border-b bg-background/80 backdrop-blur frost-chrome">
			<div class="flex h-full items-center gap-2 px-2 sm:gap-3 sm:px-4">
				<!-- Phones reach the whole navigation through the drawer; from md up the
				     sidebar is always on screen, so the button is not rendered there. -->
				<Button
					variant="ghost"
					size="icon-sm"
					class="shrink-0 md:hidden"
					onclick={() => (drawerOpen = true)}
					title={t('myaaw.sidebar.open')}
					aria-label={t('myaaw.sidebar.open')}
					aria-keyshortcuts="Control+\\ Meta+\\"
				>
					<Menu class="h-4 w-4" />
				</Button>
				<div class="min-w-0 flex-1">
					{#if isChat && topBarTitle.value}
						<h1 class="truncate text-sm font-semibold tracking-tight">
							{topBarTitle.value}
						</h1>
					{:else}
						<div class="flex items-center gap-2">
							<h1 class="truncate text-sm font-semibold tracking-tight">{t(current.label)}</h1>
							{#if isChat}
								<Badge variant="outline" class="hidden shrink-0 sm:inline-flex">
									{agentSelection.current?.name ?? agentSelection.selected ?? '—'}
								</Badge>
							{/if}
						</div>
						<p class="hidden truncate text-[11px] text-muted-foreground sm:block">
							{t(current.description)}
						</p>
					{/if}
				</div>

				<div class="flex shrink-0 items-center gap-2">
					{#if topBarSlot.current}
						{@render topBarSlot.current.content()}
					{/if}
				</div>
			</div>
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

	<!-- One confirmation dialog for every page; askConfirm() drives it. -->
	<ConfirmDialog />

	<!-- The shortcut list opens from `?` or from any button that calls shortcutHelp.show(). -->
	<ShortcutHelp />

	<!-- Mod+K walks every page, and every agent, without touching the sidebar. -->
	<CommandPalette {commands} />
</div>
