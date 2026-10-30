<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	interface Props {
		compact?: boolean;
	}

	let { compact = false }: Props = $props();

	function onChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		agentSelection.select(value);
		// The fleet overview is not agent-scoped, so picking an agent there opens
		// that agent's dashboard.
		if (page.url.pathname === '/app') {
			void goto('/app/dashboard');
		}
	}
</script>

<label class="flex w-full flex-col gap-1">
	{#if !compact}
		<span class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
			{t('myaaw.agent.label')}
		</span>
	{/if}
	<select
		value={agentSelection.selected}
		onchange={onChange}
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
</label>
