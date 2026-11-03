<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { colorOf } from '$lib/bloub';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref, type NavItem } from '$lib/utils/nav';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: swap the picker for a dropdown with a bot trigger. */
		compact?: boolean;
		/**
		 * Fleet overview for the expanded sidebar, where it rides in the picker's
		 * row as a bare icon. The compact rail keeps its own fleet row instead, so
		 * it does not pass this in.
		 */
		fleet?: NavItem;
	}

	let { compact = false, fleet }: Props = $props();

	const fleetActive = $derived(fleet ? isActiveHref(page.url.pathname, fleet.href) : false);

	function selectAgent(name: string) {
		agentSelection.select(name);
		// The fleet overview is not agent-scoped, so picking an agent there moves
		// to that agent's dashboard.
		if (page.url.pathname === '/app') void goto('/app/dashboard');
	}
</script>

<div class={compact ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
	{#if compact}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button
						{...props}
						variant="outline"
						size="icon-sm"
						class="mx-auto flex border-0"
						title={agentSelection.current?.name ?? t('myaaw.agent.label')}
						aria-label={t('myaaw.agent.label')}
					>
						<Bloub mood="attentif" size={26} color={colorOf(agentSelection.current?.agent_color)} />
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content side="right" align="start" class="w-48">
				{#each agentSelection.agents as item (item.name)}
					<DropdownMenu.Item onclick={() => selectAgent(item.name)} class="gap-2">
						<Bloub mood="attentif" size={20} color={colorOf(item.agent_color)} class="shrink-0" />
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
		<div class="flex items-end gap-2">
			<div class="min-w-0 flex-1">
				<AgentPicker />
			</div>
			{#if fleet}
				{@const FleetIcon = fleet.icon}
				<a
					href={fleet.href}
					class={[
						'flex h-9 w-9 shrink-0 items-center justify-center rounded-md border transition-colors',
						fleetActive
							? 'border-transparent bg-accent text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
					title={t(fleet.label)}
					aria-label={t(fleet.label)}
					aria-current={fleetActive ? 'page' : undefined}
				>
					<FleetIcon class="h-4 w-4" />
				</a>
			{/if}
		</div>
	{/if}
</div>
