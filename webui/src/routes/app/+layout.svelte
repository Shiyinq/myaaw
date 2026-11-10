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
		FolderOpen,
		Brain,
		Sparkles,
		Cpu,
		Radio,
		Server,
		Globe,
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
		Puzzle,
		UserRound
	} from 'lucide-svelte';
	import {
		agentSelection,
		apiBase,
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
	import { isActiveHref } from '$lib/utils/nav';
	import type { NavGroup } from '$lib/utils/nav';
	import ClassicSidebar from '$lib/components/classic/ClassicSidebar.svelte';
	import AdvancedSidebar from '$lib/components/advanced/AdvancedSidebar.svelte';
	import FocusRail from '$lib/components/focus/FocusRail.svelte';
	import FocusTabBar from '$lib/components/focus/FocusTabBar.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import ShortcutHelp from '$lib/components/app/ShortcutHelp.svelte';
	import CommandPalette from '$lib/components/app/CommandPalette.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { theme, userPrefersMode } from 'mode-watcher';
	import { APPEARANCE, applyAppearance, isAppearanceActive } from '$lib/theme';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { chatUpdates } from '$lib/stores/conversationEvents.svelte';
	import { chatRuns } from '$lib/stores/chatRuns.svelte';
	import { runCompletion } from '$lib/stores/runCompletion.svelte';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import { isAuthenticated } from '$lib/stores/authStatus.svelte';

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
	// Classic mode is the chat-first shell of the first LLM apps: no Fleet
	// shortcut and no page groups, just the agent picker and the chat list.
	// Collapsing it keeps the same minimal rail on desktop.
	// Focus mode is the third shell: one agent, its own fixed rail, and no menus.
	let shellMode = $derived(preferences.value.shellMode);
	let classicMode = $derived(shellMode === 'classic');
	let focusMode = $derived(shellMode === 'focus');
	let compactSidebar = $derived(sidebar.collapsed && wide);
	let activeConversation = $derived(page.params.conversation ?? '');
	let classicSearchInput = $state<HTMLInputElement | null>(null);

	// The header badge says which host this dashboard is driving, so nobody
	// mistakes a remote control plane for the machine in front of them. It can be
	// turned off in Settings; the default is on.
	let showHostBadge = $derived(preferences.value.showHostBadge);
	let connectionHost = $derived.by(() => {
		if (!apiBase.isCustom) return 'localhost';
		try {
			return new URL(apiBase.value).host;
		} catch {
			return apiBase.value;
		}
	});

	// The chat list belongs to the shell, so the chat page's history panel and the
	// classic-mode sidebar share one fetch (see stores/conversationHistory).
	conversationHistory.init();

	// Background turns (cron, heartbeat, sub-agent) are pushed over SSE. The
	// connection is per agent and lives in the shell, so it keeps working on any
	// page and while the reader switches agents.
	$effect(() => {
		if (!isAuthenticated.value) {
			chatUpdates.stop();
			return;
		}
		// Only running agents can emit events; a stopped one would answer 502
		// behind the control plane and retry forever.
		const names = agentSelection.agents.filter((agent) => agent.running).map((agent) => agent.name);
		chatUpdates.sync(names);
		// A run the reader started outlives its chat page, and a reload reattaches it
		// from the agent server. Recover them on any route, so an answer that lands
		// while the reader is elsewhere is still announced.
		for (const name of names) void chatRuns.restore(name);
	});

	// A run that ends while another conversation (or menu) is open must not leave a
	// stale title or message count in either sidebar. This lives in the shell because
	// the run outlives its chat page; the title itself comes from a background call
	// after the answer, so a second refresh follows.
	let runningCount = -1;
	let runningAgent = '';
	$effect(() => {
		if (!isAuthenticated.value) return;
		const agent = agentSelection.selected;
		if (!agent) return;
		const running = chatRuns.all().filter((run) => run.agent === agent && run.streaming).length;
		const previous = runningCount;
		const sameAgent = runningAgent === agent;
		runningAgent = agent;
		runningCount = running;
		if (!sameAgent || previous < 0 || running >= previous) return;
		conversationHistory.refresh();
		setTimeout(() => conversationHistory.refresh(), 4000);
	});

	// Reading the run store here is what makes the watcher re-run on a finish; it
	// turns a normal chat that ends into the same unread dot, tone, and browser
	// notification a triggered turn gets.
	$effect(() => {
		runCompletion.watch();
	});

	// An approval can arrive while its chat is on screen — no alert then, the card
	// is right there — and the reader can move on while it is still waiting. Read
	// the open chat here so switching chats re-runs this and rings once, and listen
	// for a hidden tab, where the answer is out of reach even on the right page.
	$effect(() => {
		if (!isAuthenticated.value) return;
		const agent = agentSelection.selected;
		if (!agent) return;
		approvalQueue.syncAlerts(agent);
		const onVisibility = () => {
			if (document.visibilityState === 'hidden') approvalQueue.syncAlerts(agent);
		};
		document.addEventListener('visibilitychange', onVisibility);
		return () => document.removeEventListener('visibilitychange', onVisibility);
	});

	// Focus mode is single-agent: whichever page is open, the chosen agent is the
	// one that answers, so the chat, the panel, and the APIs stay in step.
	$effect(() => {
		if (!focusMode) return;
		const agents = agentSelection.agents;
		const preferred = preferences.value.focusAgent;
		const known = agents.some((agent) => agent.name === preferred);
		const fallback = agents.find((agent) => agent.default)?.name ?? agents[0]?.name ?? '';
		const wanted = known ? preferred : fallback;
		if (wanted && agentSelection.selected !== wanted) agentSelection.select(wanted);
	});

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
				// Focus mode has a permanent rail instead of a collapsible sidebar.
				if (focusMode) return;
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
			// In classic mode the chat list lives in the sidebar, so the shortcut has
			// to reach its search box; the chat page handles its own panel otherwise.
			if (matchesShortcut(event, 'mod+shift+s') && classicMode) {
				event.preventDefault();
				void focusClassicSearch();
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

	const nav: NavGroup[] = [
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
			href: '/app/plugins',
			label: 'myaaw.plugins.title',
			description: 'myaaw.plugins.subtitle',
			icon: Puzzle
		},
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

		// The palette can flip between the three shells without a trip to Settings.
		const shells = ['advanced', 'classic', 'focus'] as const;
		list.push({
			id: 'action:shell-mode',
			labelKey: 'myaaw.command.shell_mode_cycle',
			aliases: ['mode', 'layout', 'focus', 'classic', 'advanced', 'sidebar', 'shell'],
			group: 'appearance',
			icon: PanelLeft,
			run: () => {
				const next = shells[(shells.indexOf(preferences.value.shellMode) + 1) % shells.length];
				const focusAgent =
					preferences.value.focusAgent ||
					agentSelection.current?.name ||
					agentSelection.selected ||
					'';
				preferences.update({ shellMode: next, focusAgent });
			}
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
		return isActiveHref(page.url.pathname, href);
	}

	let current = $derived(
		isActive(fleet.href)
			? fleet
			: (hiddenNav.find((item) => isActive(item.href)) ??
					flatNav.find((item) => isActive(item.href)) ??
					flatNav[0])
	);
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
	async function focusClassicSearch() {
		if (!classicMode) return;
		if (sidebar.collapsed) sidebar.toggle();
		await tick();
		classicSearchInput?.focus();
		classicSearchInput?.select();
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

<div
	class={[
		'h-dvh bg-background page-clear transition-[padding] duration-200',
		focusMode || compactSidebar ? 'md:pl-14' : 'md:pl-60'
	]}
>
	{#if focusMode}
		<FocusRail />
		<FocusTabBar />
	{:else}
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
				compactSidebar ? 'md:w-14' : 'md:w-60'
			]}
		>
			<!-- Each shell owns its sidebar; the drawer frame above stays shared. -->
			{#if classicMode}
				<ClassicSidebar
					compact={compactSidebar}
					activeId={activeConversation}
					bind:searchInput={classicSearchInput}
					onClose={() => (drawerOpen = false)}
				/>
			{:else}
				<AdvancedSidebar
					compact={compactSidebar}
					{fleet}
					groups={nav}
					onClose={() => (drawerOpen = false)}
				/>
			{/if}
		</aside>
	{/if}

	<div class="relative flex h-dvh flex-col">
		<header
			class={focusMode && isChat
				? 'pointer-events-none absolute inset-x-0 top-0 z-40 h-14 lg:z-60'
				: 'h-14 shrink-0 border-b bg-background/80 backdrop-blur frost-chrome'}
		>
			<div class="flex h-full items-center gap-2 px-2 sm:gap-3 sm:px-4">
				<!-- Phones reach the whole navigation through the drawer; from md up the
				     sidebar is always on screen, so the control is not rendered there.
				     Focus mode has no drawer: its mascot is a static brand mark, and the
				     chat screen already floats its own mark. -->
				{#if focusMode}
					{#if !isChat}
						<Bloub
							mood="attentif"
							size={32}
							eager
							color={agentSelection.color}
							class="-mr-2 shrink-0 md:hidden"
						/>
					{/if}
				{:else}
					<Button
						variant="ghost"
						size="icon-sm"
						class="-mr-2 shrink-0 border-0 md:hidden"
						onclick={() => (drawerOpen = true)}
						title={t('myaaw.sidebar.open')}
						aria-label={t('myaaw.sidebar.open')}
						aria-keyshortcuts="Control+\\ Meta+\\"
					>
						<Bloub mood="attentif" size={32} eager color={agentSelection.color} />
					</Button>
				{/if}
				<div class="min-w-0 flex-1">
					{#if focusMode && isChat}
						<!-- The chat screen has no title; the agent mark floats over it. -->
					{:else if topBarTitle.value}
						<!-- A page can name itself: the open conversation, or the agent being
						     edited, instead of the route's static label. -->
						<h1 class="truncate text-sm font-semibold tracking-tight">{topBarTitle.value}</h1>
						{#if topBarTitle.description}
							<p class="hidden truncate text-[11px] text-muted-foreground sm:block">
								{topBarTitle.description}
							</p>
						{/if}
					{:else if !focusMode}
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
					{:else}
						<!-- Focus mode keeps the page header for every page except chat. -->
						<h1 class="truncate text-sm font-semibold tracking-tight">{t(current.label)}</h1>
						<p class="hidden truncate text-[11px] text-muted-foreground sm:block">
							{t(current.description)}
						</p>
					{/if}
				</div>

				<div class="pointer-events-auto flex min-w-0 shrink-0 items-center gap-2">
					{#if showHostBadge}
						<a
							href="/app/settings"
							class="inline-flex max-w-none items-center gap-1.5 rounded-full border border-border/70 bg-background/60 px-2.5 py-1 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:max-w-36"
							title={apiBase.isCustom
								? t('myaaw.settings.connection_remote', { url: apiBase.value })
								: t('myaaw.settings.connection_local')}
							aria-label={apiBase.isCustom
								? t('myaaw.settings.connection_remote', { url: apiBase.value })
								: t('myaaw.settings.connection_local')}
						>
							{#if apiBase.isCustom}
								<Globe class="size-3 shrink-0" />
							{:else}
								<Server class="size-3 shrink-0" />
							{/if}
							<!-- Phones get the short word, desktop the host: a tunnel host is too
							     long for a 375 px header and would squeeze the page title. -->
							<span class="truncate sm:hidden">
								{apiBase.isCustom
									? t('myaaw.settings.host_badge_remote')
									: t('myaaw.settings.host_badge_local')}
							</span>
							<span class="hidden truncate sm:inline">{connectionHost}</span>
						</a>
					{/if}
					{#if topBarSlot.current}
						{@render topBarSlot.current.content()}
					{/if}
				</div>
			</div>
		</header>

		<!-- Focus mode phones get a bottom tab bar instead of the rail, so the page
		     content keeps clear of it, including the iOS home-indicator inset. -->
		<main
			class={[
				isFullHeight ? 'min-h-0 flex-1 overflow-hidden' : 'min-h-0 flex-1 overflow-y-auto',
				focusMode ? 'pb-[calc(4rem+env(safe-area-inset-bottom))] md:pb-0' : ''
			]}
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
