<script lang="ts">
	import { page } from '$app/state';
	import { Boxes } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref } from '$lib/utils/nav';
	import type { NavItem } from '$lib/utils/nav';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: the fleet row unfolds above the agent picker. */
		compact?: boolean;
		/** Fleet overview, which sits above the agent and outside its scope. */
		fleet: NavItem;
	}

	let { compact = false, fleet }: Props = $props();

	const active = $derived(isActiveHref(page.url.pathname, fleet.href));
</script>

<!--
	Compact rail fleet row. It is always mounted and folds its own height, so the
	agent picker below glides into place instead of jumping. Expanded, the same
	control rides beside the picker instead, and this row stays at zero height.
-->
<div
	class={[
		'shrink-0 overflow-hidden transition-all duration-200',
		compact ? 'max-h-14 border-b opacity-100 delay-200' : 'max-h-0 border-b-0 opacity-0 delay-0'
	]}
>
	<div class="p-2">
		<a
			href={fleet.href}
			title={t(fleet.label)}
			aria-current={active ? 'page' : undefined}
			class={[
				'relative flex items-center rounded-md py-2 pl-3 pr-0 text-sm transition-colors list-row',
				active
					? 'bg-accent font-medium text-accent-foreground'
					: 'text-muted-foreground hover:bg-muted hover:text-foreground'
			]}
		>
			{#if active}
				<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
			{/if}
			<Boxes class="h-4 w-4 shrink-0" />
		</a>
	</div>
</div>
