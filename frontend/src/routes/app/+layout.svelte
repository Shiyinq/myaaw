<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		LayoutDashboard,
		MessagesSquare,
		ListTree,
		FolderOpen,
		Brain,
		Sparkles,
		Cpu,
		Radio,
		Server,
		Settings2,
		LogOut,
		Wrench,
		Users,
		SlidersHorizontal,
		Clock,
		ScrollText,
		ShieldCheck,
		Activity
	} from 'lucide-svelte';
	import { agentSelection, authStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import ThemeToggle from '$lib/components/common/ThemeToggle.svelte';
	import LanguageSwitcher from '$lib/components/common/LanguageSwitcher.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();
	const { t } = useTranslation();

	const nav = [
		{
			label: 'myaaw.nav.group_workspace',
			items: [
				{ href: '/app', label: 'myaaw.nav.dashboard', icon: LayoutDashboard },
				{ href: '/app/chat', label: 'myaaw.nav.chat', icon: MessagesSquare },
				{ href: '/app/conversations', label: 'myaaw.nav.conversations', icon: ListTree },
				{ href: '/app/files', label: 'myaaw.nav.files', icon: FolderOpen },
				{ href: '/app/context', label: 'myaaw.nav.context', icon: Brain },
				{ href: '/app/skills', label: 'myaaw.nav.skills', icon: Sparkles },
				{ href: '/app/tools', label: 'myaaw.nav.tools', icon: Wrench },
				{ href: '/app/contacts', label: 'myaaw.nav.contacts', icon: Users }
			]
		},
		{
			label: 'myaaw.nav.group_configure',
			items: [
				{ href: '/app/providers', label: 'myaaw.nav.providers', icon: Cpu },
				{ href: '/app/channels', label: 'myaaw.nav.channels', icon: Radio },
				{ href: '/app/config', label: 'myaaw.nav.config', icon: SlidersHorizontal },
				{ href: '/app/cron', label: 'myaaw.nav.cron', icon: Clock }
			]
		},
		{
			label: 'myaaw.nav.group_operations',
			items: [
				{ href: '/app/server', label: 'myaaw.nav.server', icon: Server },
				{ href: '/app/logs', label: 'myaaw.nav.logs', icon: ScrollText },
				{ href: '/app/approvals', label: 'myaaw.nav.approvals', icon: ShieldCheck },
				{ href: '/app/observability', label: 'myaaw.nav.observability', icon: Activity },
				{ href: '/app/settings', label: 'myaaw.nav.settings', icon: Settings2 }
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

	async function logout() {
		await authStore.logout();
		await goto('/login');
	}
</script>

<div class="min-h-screen bg-background">
	<aside class="fixed inset-y-0 left-0 z-40 hidden w-60 flex-col border-r bg-card md:flex">
		<div class="flex h-14 items-center gap-2 border-b px-4">
			<div
				class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-sm font-bold text-primary-foreground"
			>
				M
			</div>
			<div class="flex flex-col leading-tight">
				<span class="text-sm font-bold tracking-tight">Myaaw</span>
				<span class="text-[10px] uppercase tracking-widest text-muted-foreground">
					Control plane
				</span>
			</div>
		</div>

		<div class="border-b p-3">
			<AgentPicker />
		</div>

		<nav class="flex-1 space-y-5 overflow-y-auto p-3">
			{#each nav as group}
				<div class="space-y-1">
					<p class="px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t(group.label)}
					</p>
					{#each group.items as item (item.href)}
						{@const Icon = item.icon}
						<a
							href={item.href}
							class={[
								'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
								isActive(item.href)
									? 'bg-accent text-accent-foreground font-medium'
									: 'text-muted-foreground hover:bg-muted hover:text-foreground'
							]}
						>
							<Icon class="h-4 w-4" />
							<span>{t(item.label)}</span>
						</a>
					{/each}
				</div>
			{/each}
		</nav>

		<div class="flex items-center justify-between gap-2 border-t p-3">
			<LanguageSwitcher />
			<ThemeToggle />
		</div>
	</aside>

	<div class="md:pl-60">
		<header class="sticky top-0 z-30 border-b bg-background/80 backdrop-blur">
			<div class="flex h-14 items-center gap-3 px-4">
				<a href="/app" class="flex items-center gap-2 md:hidden">
					<span
						class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-sm font-bold text-primary-foreground"
					>
						M
					</span>
					<span class="text-sm font-bold">Myaaw</span>
				</a>

				<div class="ml-auto flex items-center gap-2">
					<div class="w-40 md:hidden">
						<AgentPicker compact />
					</div>

					<DropdownMenu.Root>
						<DropdownMenu.Trigger>
							{#snippet child({ props })}
								<Button {...props} variant="outline" size="sm" class="gap-2">
									<span
										class="flex h-6 w-6 items-center justify-center rounded-full bg-muted text-[10px] font-bold uppercase"
									>
										{authStore.profile?.username?.slice(0, 2) ?? 'MY'}
									</span>
									<span class="hidden max-w-28 truncate sm:inline">
										{authStore.profile?.username ?? t('myaaw.settings.title')}
									</span>
								</Button>
							{/snippet}
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="end" class="w-52">
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
				</div>
			</div>

			<nav class="flex gap-1 overflow-x-auto border-t px-3 py-2 md:hidden">
				{#each flatNav as item (item.href)}
					<a
						href={item.href}
						class={[
							'whitespace-nowrap rounded-md px-2 py-1 text-xs transition-colors',
							isActive(item.href)
								? 'bg-accent text-accent-foreground font-medium'
								: 'text-muted-foreground hover:bg-muted hover:text-foreground'
						]}
					>
						{t(item.label)}
					</a>
				{/each}
			</nav>
		</header>

		<main class="mx-auto w-full max-w-6xl px-4 py-6">
			{@render children()}
		</main>
	</div>
</div>
