<script lang="ts">
	import { page } from '$app/state';
	import { Clock, MessagesSquare, Puzzle } from 'lucide-svelte';
	import AccountMenu from '$lib/components/app/AccountMenu.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	const path = $derived(page.url.pathname);
	const items = $derived([
		{
			href: '/app/chat',
			label: t('myaaw.focus.rail_chat'),
			icon: MessagesSquare,
			active: path.startsWith('/app/chat')
		},
		{
			href: '/app/cron',
			label: t('myaaw.focus.rail_cron'),
			icon: Clock,
			active: path.startsWith('/app/cron')
		},
		{
			href: '/app/plugins',
			label: t('myaaw.focus.rail_plugins'),
			icon: Puzzle,
			active: path.startsWith('/app/plugins')
		}
	]);
</script>

<!--
	Phones cannot afford the 56px rail, so focus mode swaps it for a bottom tab bar
	the way phone apps do: big tap targets, an icon and a short label per tab.
	The layout reserves the matching space under the page content.
-->
<nav
	class="fixed inset-x-0 bottom-0 z-40 flex min-h-16 border-t bg-card/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
	aria-label={t('myaaw.focus.tabbar_label')}
>
	{#each items as item (item.href)}
		{@const Icon = item.icon}
		<a
			href={item.href}
			class="flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md px-1 py-2"
			aria-current={item.active ? 'page' : undefined}
			aria-label={item.label}
		>
			<span
				class={[
					'flex h-7 w-9 items-center justify-center rounded-full transition-colors',
					item.active ? 'bg-primary/10 text-primary' : 'text-muted-foreground'
				]}
			>
				<Icon class="h-4.5 w-4.5" />
			</span>
			<span
				class={[
					'text-[10px] font-medium transition-colors',
					item.active ? 'text-primary' : 'text-muted-foreground'
				]}
			>
				{item.label}
			</span>
		</a>
	{/each}

	<AccountMenu tab side="top" align="end" />
</nav>
