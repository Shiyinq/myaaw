<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Bot } from 'lucide-svelte';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: swap the picker for a dropdown with a bot trigger. */
		compact?: boolean;
	}

	let { compact = false }: Props = $props();

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
