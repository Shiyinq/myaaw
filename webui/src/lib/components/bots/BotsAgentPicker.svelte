<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { Check, ChevronDown } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { colorOf } from '$lib/bloub';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	const current = $derived(agentSelection.current);
	const currentColor = $derived(colorOf(current?.agent_color));

	// Sync to an explicit `?agent=...` link (e.g. from the right-side bot panel).
	// Otherwise preserve whichever agent is already active (such as the bot last
	// opened in chat), falling back to the default agent only when none is chosen.
	let appliedForQuery = '';
	$effect(() => {
		const requested = page.url.searchParams.get('agent') ?? '';
		const agents = agentSelection.agents;
		if (agents.length === 0) return;

		if (requested) {
			const key = `${page.url.pathname}?${requested}`;
			if (appliedForQuery !== key) {
				appliedForQuery = key;
				if (
					agents.some((entry) => entry.name === requested) &&
					agentSelection.selected !== requested
				) {
					untrack(() => agentSelection.select(requested));
				}
			}
			return;
		}

		appliedForQuery = '';
		const currentValid = agents.some((entry) => entry.name === agentSelection.selected);
		if (!currentValid) {
			const fallback = agents.find((entry) => entry.default)?.name ?? agents[0]?.name ?? '';
			if (fallback) untrack(() => agentSelection.select(fallback));
		}
	});
</script>

{#if agentSelection.agents.length > 0}
	<span aria-hidden="true" class="shrink-0 text-base text-muted-foreground/45 select-none">/</span>
	<DropdownMenu.Root>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				<button
					{...props}
					type="button"
					aria-label={t('myaaw.bots.select_agent')}
					class="pointer-events-auto inline-flex min-w-0 items-center gap-1.5 rounded-lg border border-border/70 bg-muted/40 px-2 py-1 text-xs font-medium text-foreground transition-colors outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50 sm:px-2.5 sm:text-sm"
				>
					<span class="relative flex h-5 w-5 shrink-0 items-center justify-center">
						<Bloub mood="attentif" size={18} tight color={currentColor} />
						<span
							aria-hidden="true"
							class={[
								'absolute -right-0.5 -bottom-0.5 h-1.5 w-1.5 rounded-full ring-1 ring-card',
								current?.running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
							]}
						></span>
					</span>
					<span class="max-w-24 truncate sm:max-w-44">
						{current?.name ?? agentSelection.selected ?? '—'}
					</span>
					<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
				</button>
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="start" class="w-56">
			<DropdownMenu.Label class="text-[11px] font-medium text-muted-foreground">
				{t('myaaw.bots.select_agent')}
			</DropdownMenu.Label>
			<DropdownMenu.Separator />
			{#each agentSelection.agents as bot (bot.name)}
				{@const active = agentSelection.selected === bot.name}
				<DropdownMenu.Item onclick={() => agentSelection.select(bot.name)} class="gap-2">
					<span class="relative flex h-5 w-5 shrink-0 items-center justify-center">
						<Bloub mood="attentif" size={20} tight color={colorOf(bot.agent_color)} />
						<span
							aria-hidden="true"
							class={[
								'absolute -right-0.5 -bottom-0.5 h-1.5 w-1.5 rounded-full ring-1 ring-popover',
								bot.running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
							]}
						></span>
					</span>
					<span class="min-w-0 flex-1 truncate">{bot.name}</span>
					{#if bot.default}
						<span
							class="shrink-0 rounded-full bg-muted px-1.5 py-0.25 text-[10px] font-medium text-muted-foreground"
						>
							{t('myaaw.agent.default')}
						</span>
					{/if}
					{#if active}
						<Check class="h-3.5 w-3.5 shrink-0 text-foreground" />
					{/if}
				</DropdownMenu.Item>
			{/each}
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/if}
