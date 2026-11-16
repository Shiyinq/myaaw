<script lang="ts">
	import { page } from '$app/state';
	import AgentPicker from '$lib/components/app/AgentPicker.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref, type NavItem } from '$lib/utils/nav';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: the picker keeps its trigger and hides the text. */
		compact?: boolean;
		/**
		 * Fleet overview for the expanded sidebar, where it rides in the picker's
		 * row as a bare icon. The compact rail keeps its own fleet row instead.
		 */
		fleet?: NavItem;
	}

	let { compact = false, fleet }: Props = $props();

	const fleetActive = $derived(fleet ? isActiveHref(page.url.pathname, fleet.href) : false);
</script>

<!--
	One AgentPicker instance for both widths. The picker column keeps a fixed
	216px width and the block clips it, so the trigger never shrinks to a sliver
	while the rail narrows.
-->
<div
	class={[
		'relative flex w-full min-w-0 shrink-0 items-end overflow-hidden border-b pb-2 transition-[height] duration-200',
		compact ? 'h-12.5 px-2' : 'h-19 px-3'
	]}
>
	<div class="flex w-54 shrink-0 items-end gap-2">
		<div class="min-w-0 flex-1">
			<AgentPicker {compact} />
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
</div>
