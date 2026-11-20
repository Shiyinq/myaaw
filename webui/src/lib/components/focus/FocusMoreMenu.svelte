<script lang="ts">
	import { page } from '$app/state';
	import { MoreHorizontal } from 'lucide-svelte';
	import AgentMoreMenuItems, {
		isAgentMoreRoute
	} from '$lib/components/app/AgentMoreMenuItems.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		/** Render as a mobile bottom-tab item instead of a rail icon button. */
		tab?: boolean;
		side?: 'right' | 'top' | 'bottom' | 'left';
		align?: 'start' | 'center' | 'end';
	}

	let { tab = false, side = 'right', align = 'center' }: Props = $props();

	const { t } = useTranslation();

	const agent = $derived(agentSelection.selected);
	const isMoreRouteActive = $derived(isAgentMoreRoute(page.url.pathname));
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			{#if tab}
				<button
					{...props}
					type="button"
					aria-label={t('myaaw.bots.actions')}
					class="flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md px-1 py-2"
				>
					<span
						class={[
							'flex h-7 w-9 items-center justify-center rounded-full transition-colors',
							isMoreRouteActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground'
						]}
					>
						<MoreHorizontal class="h-4.5 w-4.5" />
					</span>
					<span
						class={[
							'text-[10px] font-medium transition-colors',
							isMoreRouteActive ? 'text-primary' : 'text-muted-foreground'
						]}
					>
						{t('common.more')}
					</span>
				</button>
			{:else}
				<button
					{...props}
					type="button"
					title={t('myaaw.bots.actions')}
					aria-label={t('myaaw.bots.actions')}
					class={[
						'flex h-10 w-10 items-center justify-center rounded-md transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
						isMoreRouteActive
							? 'bg-accent text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					<MoreHorizontal class="h-4.5 w-4.5" />
				</button>
			{/if}
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content {side} {align} class="w-48">
		<AgentMoreMenuItems {agent} />
	</DropdownMenu.Content>
</DropdownMenu.Root>
