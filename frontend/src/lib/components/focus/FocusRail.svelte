<script lang="ts">
	import { page } from '$app/state';
	import { Clock, MessagesSquare, Puzzle } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import AccountMenu from '$lib/components/app/AccountMenu.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	const chatActive = $derived(page.url.pathname.startsWith('/app/chat'));
	const cronActive = $derived(page.url.pathname.startsWith('/app/cron'));
	const pluginsActive = $derived(page.url.pathname.startsWith('/app/plugins'));

	const itemClass = (active: boolean) => [
		'flex h-10 w-10 items-center justify-center rounded-md transition-colors',
		active
			? 'bg-accent text-accent-foreground'
			: 'text-muted-foreground hover:bg-muted hover:text-foreground'
	];
</script>

<!--
	From md up, focus mode has its own rail instead of the main sidebar: a permanent
	56px column with the brand, chat, cron, plugins and the profile menu, so the
	shell never shifts and there is no drawer to discover. Phones use FocusTabBar.
-->
<aside
	class="fixed inset-y-0 left-0 z-40 hidden w-14 flex-col items-center gap-1 border-r bg-card py-2 md:flex"
	aria-label={t('myaaw.focus.rail_label')}
>
	<!-- The chat screen floats its own mark; every other page keeps the rail brand. -->
	{#if !chatActive}
		<a
			href="/app/chat"
			class="mb-1 flex h-10 w-10 items-center justify-center"
			title={t('myaaw.focus.rail_brand')}
			aria-label={t('myaaw.focus.rail_brand')}
		>
			<Bloub mood="attentif" size={40} />
		</a>
	{/if}

	<nav
		class="absolute top-1/2 left-1/2 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1"
	>
		<a
			href="/app/chat"
			class={itemClass(chatActive)}
			aria-current={chatActive ? 'page' : undefined}
			title={t('myaaw.focus.rail_chat')}
			aria-label={t('myaaw.focus.rail_chat')}
		>
			<MessagesSquare class="h-4.5 w-4.5" />
		</a>
		<a
			href="/app/cron"
			class={itemClass(cronActive)}
			aria-current={cronActive ? 'page' : undefined}
			title={t('myaaw.focus.rail_cron')}
			aria-label={t('myaaw.focus.rail_cron')}
		>
			<Clock class="h-4.5 w-4.5" />
		</a>
		<a
			href="/app/plugins"
			class={itemClass(pluginsActive)}
			aria-current={pluginsActive ? 'page' : undefined}
			title={t('myaaw.focus.rail_plugins')}
			aria-label={t('myaaw.focus.rail_plugins')}
		>
			<Puzzle class="h-4.5 w-4.5" />
		</a>
	</nav>

	<div class="flex-1"></div>
	<AccountMenu compact align="start" />
</aside>
