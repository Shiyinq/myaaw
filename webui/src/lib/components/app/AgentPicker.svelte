<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ChevronDown } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { colorOf } from '$lib/bloub';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: hide the label, name, and chevron but keep the trigger. */
		compact?: boolean;
	}

	let { compact = false }: Props = $props();

	const current = $derived(agentSelection.current);
	const currentColor = $derived(colorOf(current?.agent_color));

	function select(name: string) {
		agentSelection.select(name);
		// The fleet overview is not agent-scoped, so picking an agent there opens
		// that agent's dashboard.
		if (page.url.pathname === '/app') {
			void goto('/app/dashboard');
		}
	}
</script>

<!--
	A native select cannot show the mascot, so the picker is split: phones keep
	the OS list (fast, familiar, no custom focus handling) with the current face
	beside it, and from md up the list becomes a dropdown where every row carries
	its agent's own face.

	The compact trigger is left-aligned at the same padding as the expanded one
	(8px row + 10px vs 12px row + 1px border + 5px), so the face holds its exact x
	while the label and chevron fold away instead of being shoved to the edge by
	`justify-center`.
-->
<div class="flex w-full flex-col gap-1">
	<span
		class="overflow-hidden text-[10px] font-semibold uppercase tracking-widest whitespace-nowrap text-muted-foreground transition-all duration-200 {compact
			? 'max-h-0 opacity-0 delay-0'
			: 'max-h-4 opacity-100 delay-200'}"
	>
		{t('myaaw.agent.label')}
	</span>

	<div class="flex items-center gap-2 md:hidden">
		<Bloub mood="attentif" size={26} color={currentColor} class="shrink-0" />
		<select
			value={agentSelection.selected}
			onchange={(event) => select(event.currentTarget.value)}
			class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm text-foreground outline-none transition-colors focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
			aria-label={t('myaaw.agent.label')}
		>
			{#if agentSelection.agents.length === 0}
				<option value="">{t('myaaw.agent.none')}</option>
			{/if}
			{#each agentSelection.agents as agent (agent.name)}
				<option value={agent.name}>
					{agent.name}{agent.default ? ` · ${t('myaaw.agent.default')}` : ''}
				</option>
			{/each}
		</select>
	</div>

	<DropdownMenu.Root>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				<button
					{...props}
					type="button"
					class={[
						'hidden items-center rounded-md text-left text-sm text-foreground transition-colors hover:bg-muted focus-visible:ring-1 focus-visible:ring-ring/50 md:flex',
						compact
							? 'h-8 w-10 justify-start pl-2.5 pr-0'
							: 'h-9 w-full gap-3 border border-input bg-transparent pl-1.25 pr-3 focus-visible:border-ring'
					]}
					aria-label={t('myaaw.agent.label')}
				>
					<Bloub mood="attentif" size={22} color={currentColor} class="shrink-0" />
					<span
						class="min-w-0 flex-1 truncate transition-all duration-200 {compact
							? 'max-w-0 opacity-0 delay-0'
							: 'max-w-full opacity-100 delay-200'}"
					>
						{current?.name ?? t('myaaw.agent.none')}
					</span>
					<ChevronDown
						class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-all duration-200 {compact
							? 'max-w-0 opacity-0 delay-0'
							: 'max-w-4 opacity-100 delay-200'}"
					/>
				</button>
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="start" class="w-56">
			{#each agentSelection.agents as agent (agent.name)}
				<DropdownMenu.Item onclick={() => select(agent.name)} class="gap-2">
					<Bloub mood="attentif" size={22} color={colorOf(agent.agent_color)} class="shrink-0" />
					<span class="truncate">{agent.name}</span>
					{#if agent.default}
						<span class="ml-auto shrink-0 text-[10px] text-muted-foreground">
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
</div>
