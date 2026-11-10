<script lang="ts">
	import { page } from '$app/state';
	import { Boxes } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref } from '$lib/utils/nav';
	import type { NavItem } from '$lib/utils/nav';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: the label collapses to a tooltip. */
		compact?: boolean;
		/** Fleet overview, which sits above the agent picker and outside its scope. */
		fleet: NavItem;
	}

	let { compact = false, fleet }: Props = $props();

	const active = $derived(isActiveHref(page.url.pathname, fleet.href));
</script>

<div class={compact ? 'shrink-0 border-b p-2' : 'shrink-0 border-b p-3'}>
	<a
		href={fleet.href}
		title={compact ? t(fleet.label) : undefined}
		aria-current={active ? 'page' : undefined}
		class={[
			'relative flex items-center rounded-md text-sm transition-colors list-row',
			compact ? 'justify-center px-0 py-2' : 'gap-2 px-2 py-1.5',
			active
				? 'bg-accent font-medium text-accent-foreground'
				: 'text-muted-foreground hover:bg-muted hover:text-foreground'
		]}
	>
		{#if compact && active}
			<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
		{/if}
		<Boxes class="h-4 w-4 shrink-0" />
		{#if !compact}
			<span class="truncate">{t(fleet.label)}</span>
		{/if}
	</a>
</div>
