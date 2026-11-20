<script lang="ts" module>
	const MORE_ROUTE_PREFIXES = [
		'/app/dashboard',
		'/app/contacts',
		'/app/terminal',
		'/app/providers',
		'/app/channels',
		'/app/config',
		'/app/server',
		'/app/logs',
		'/app/backups',
		'/app/approvals',
		'/app/observability'
	];

	/** True when `pathname` is one of the shared Configure / Operations / utility routes. */
	export function isAgentMoreRoute(pathname: string): boolean {
		return MORE_ROUTE_PREFIXES.some((prefix) => pathname.startsWith(prefix));
	}
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Activity,
		Archive,
		Bot,
		Brain,
		Clock,
		Cpu,
		FolderOpen,
		Layers,
		LayoutDashboard,
		MessageSquarePlus,
		MessagesSquare,
		Play,
		Puzzle,
		Radio,
		RotateCw,
		ScrollText,
		Server,
		Settings2,
		ShieldCheck,
		SlidersHorizontal,
		Square,
		SquareTerminal,
		Wrench
	} from 'lucide-svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { agentSelection, toastStore } from '$lib/stores';
	import { serverStore } from '$lib/stores/server.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';

	interface Props {
		/** Target agent this menu acts on. */
		agent: string;
		/** Whether the target agent's daemon is currently running. */
		running?: boolean;
		/** Whether a start/stop/restart request is currently in flight. */
		busy?: boolean;
		/** Notifies the parent when a daemon request starts or finishes. */
		onBusyChange?: (busy: boolean) => void;
		/** Show `Chat` and/or `New chat` at the top of the menu. */
		includeChatActions?: boolean;
		/** Show `Chat` above `New chat` (used in Classic mode). */
		includeChatLink?: boolean;
		/** Show `Start` / `Restart` / `Stop` daemon controls. */
		includeServerControls?: boolean;
		/**
		 * Show workspace links (`Context`, `Artifacts`, `Schedule`, `Plugins`, `Files`)
		 * that Classic mode does not display in its sidebar.
		 */
		includeWorkspaceLinks?: boolean;
		/** Append `?agent=<name>` and select the agent on navigation (used in Bots mode). */
		passAgentParam?: boolean;
		/** Optional custom handler for `New chat`. */
		onNewChat?: () => void;
		/** Optional callback after a daemon start/restart completes. */
		onStarted?: (agent: string) => void;
		/** Optional callback after navigating to any destination. */
		onNavigate?: () => void;
	}

	let {
		agent,
		running = false,
		busy = false,
		onBusyChange,
		includeChatActions = false,
		includeChatLink = false,
		includeServerControls = false,
		includeWorkspaceLinks = false,
		passAgentParam = false,
		onNewChat,
		onStarted,
		onNavigate
	}: Props = $props();

	const { t } = useTranslation();

	let internalBusy = $state(false);
	const isBusy = $derived(busy || internalBusy);

	const path = $derived(page.url.pathname);

	const workspaceItems = $derived(
		includeWorkspaceLinks
			? [
					{ href: '/app/dashboard', label: t('myaaw.nav.dashboard'), icon: LayoutDashboard },
					{ href: '/app/context', label: t('myaaw.nav.context'), icon: Brain },
					{ href: '/app/artifacts', label: t('myaaw.nav.artifacts'), icon: Layers },
					{ href: '/app/schedule', label: t('myaaw.nav.schedule'), icon: Clock },
					{ href: '/app/plugins', label: t('myaaw.plugins.title'), icon: Puzzle },
					{ href: '/app/files', label: t('myaaw.nav.files'), icon: FolderOpen },
					{ href: '/app/contacts', label: t('myaaw.nav.contacts'), icon: Bot },
					{ href: '/app/terminal', label: t('myaaw.nav.terminal'), icon: SquareTerminal }
				]
			: [
					{ href: '/app/dashboard', label: t('myaaw.nav.dashboard'), icon: LayoutDashboard },
					{ href: '/app/contacts', label: t('myaaw.nav.contacts'), icon: Bot },
					{ href: '/app/terminal', label: t('myaaw.nav.terminal'), icon: SquareTerminal }
				]
	);

	const configureItems = $derived([
		{ href: '/app/providers', label: t('myaaw.nav.providers'), icon: Cpu },
		{ href: '/app/channels', label: t('myaaw.nav.channels'), icon: Radio },
		{ href: '/app/config', label: t('myaaw.nav.config'), icon: SlidersHorizontal }
	]);

	const operationItems = $derived([
		{ href: '/app/server', label: t('myaaw.nav.server'), icon: Server },
		{ href: '/app/logs', label: t('myaaw.nav.logs'), icon: ScrollText },
		{ href: '/app/backups', label: t('myaaw.nav.backups'), icon: Archive },
		{ href: '/app/approvals', label: t('myaaw.nav.approvals'), icon: ShieldCheck },
		{ href: '/app/observability', label: t('myaaw.nav.observability'), icon: Activity }
	]);

	function navigateTo(href: string) {
		if (passAgentParam && agent) {
			agentSelection.select(agent);
			const sep = href.includes('?') ? '&' : '?';
			void goto(`${href}${sep}agent=${encodeURIComponent(agent)}`);
		} else {
			void goto(href);
		}
		onNavigate?.();
	}

	function handleNewChat() {
		if (onNewChat) {
			onNewChat();
			return;
		}
		void goto('/app/chat?new=1');
		onNavigate?.();
	}

	async function runDaemonAction(kind: 'start' | 'stop' | 'restart') {
		if (!agent || isBusy) return;
		const target = agent;
		internalBusy = true;
		onBusyChange?.(true);
		const nextRunning = kind !== 'stop';
		agentSelection.setRunning(target, nextRunning);
		try {
			if (kind === 'start') await serverStore.start(target);
			else if (kind === 'stop') await serverStore.stop(target);
			else await serverStore.restart(target);
			agentSelection.setRunning(target, nextRunning);
			toastStore.success(
				t(
					kind === 'start'
						? 'myaaw.server.start_done'
						: kind === 'stop'
							? 'myaaw.server.stop_done'
							: 'myaaw.server.restart_done'
				)
			);
			if (nextRunning) onStarted?.(target);
		} catch (err) {
			agentSelection.clearOptimisticRunning(target);
			await agentSelection.load(true);
			toastStore.error(getErrorMessage(err));
		} finally {
			internalBusy = false;
			onBusyChange?.(false);
		}
	}
</script>

{#if includeChatActions}
	{#if includeChatLink}
		<DropdownMenu.Item
			onclick={() => {
				void goto('/app/chat');
				onNavigate?.();
			}}
			class="gap-2"
		>
			<MessagesSquare class="h-3.5 w-3.5" />
			{t('myaaw.nav.chat')}
		</DropdownMenu.Item>
	{/if}
	<DropdownMenu.Item onclick={handleNewChat} class="gap-2">
		<MessageSquarePlus class="h-3.5 w-3.5" />
		{t('myaaw.chat.new')}
	</DropdownMenu.Item>
{/if}

{#if includeServerControls && agent}
	{#if includeChatActions}
		<DropdownMenu.Separator />
	{/if}
	{#if running}
		<DropdownMenu.Item
			disabled={isBusy}
			onclick={() => void runDaemonAction('restart')}
			class="gap-2"
		>
			<RotateCw class="h-3.5 w-3.5" />
			{t('myaaw.server.restart')}
		</DropdownMenu.Item>
		<DropdownMenu.Item
			disabled={isBusy}
			onclick={() => void runDaemonAction('stop')}
			class="gap-2 text-destructive focus:text-destructive"
		>
			<Square class="h-3.5 w-3.5" />
			{t('myaaw.server.stop')}
		</DropdownMenu.Item>
	{:else}
		<DropdownMenu.Item
			disabled={isBusy}
			onclick={() => void runDaemonAction('start')}
			class="gap-2"
		>
			<Play class="h-3.5 w-3.5" />
			{t('myaaw.server.start')}
		</DropdownMenu.Item>
	{/if}
{/if}

{#if includeChatActions || (includeServerControls && agent)}
	<DropdownMenu.Separator />
{/if}

{#each workspaceItems as item (item.href)}
	{@const Icon = item.icon}
	{@const active = !passAgentParam && path.startsWith(item.href)}
	<DropdownMenu.Item
		onclick={() => navigateTo(item.href)}
		class={active ? 'gap-2 bg-accent font-medium text-accent-foreground' : 'gap-2'}
	>
		<Icon class="h-3.5 w-3.5" />
		{item.label}
	</DropdownMenu.Item>
{/each}

<DropdownMenu.Sub>
	<DropdownMenu.SubTrigger class="gap-2">
		<Settings2 class="h-3.5 w-3.5" />
		{t('myaaw.nav.group_configure')}
	</DropdownMenu.SubTrigger>
	<DropdownMenu.SubContent class="w-44">
		{#each configureItems as item (item.href)}
			{@const Icon = item.icon}
			<DropdownMenu.Item onclick={() => navigateTo(item.href)} class="gap-2">
				<Icon class="h-3.5 w-3.5" />
				{item.label}
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.SubContent>
</DropdownMenu.Sub>

<DropdownMenu.Sub>
	<DropdownMenu.SubTrigger class="gap-2">
		<Wrench class="h-3.5 w-3.5" />
		{t('myaaw.nav.group_operations')}
	</DropdownMenu.SubTrigger>
	<DropdownMenu.SubContent class="w-44">
		{#each operationItems as item (item.href)}
			{@const Icon = item.icon}
			<DropdownMenu.Item onclick={() => navigateTo(item.href)} class="gap-2">
				<Icon class="h-3.5 w-3.5" />
				{item.label}
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.SubContent>
</DropdownMenu.Sub>

{#if agent}
	<DropdownMenu.Separator />
	<DropdownMenu.Item
		onclick={() => {
			void goto(`/app/new?agent=${encodeURIComponent(agent)}`);
			onNavigate?.();
		}}
		class="gap-2"
	>
		<SlidersHorizontal class="h-3.5 w-3.5" />
		{t('myaaw.bots.reconfigure')}
	</DropdownMenu.Item>
{/if}
