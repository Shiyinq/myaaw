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
	agent picker below glides into place instead of jumping. The fold runs on the
	same 200ms clock as the picker row's own height change, with no delay: the row
	grows while the picker shrinks, which keeps the face on one smooth path down
	rather than rising first and dropping again when the row finally opens.

	The icon is centred in the row while it opens, so it slides out from under the
	brand's border at full size. Anchoring it to the top edge instead let the
	growing clip slice through it, which read as a flicker before it settled. The
	fade starts only once the row is tall enough to hold the whole icon, so no
	frame shows a half-drawn cluster.

	The row's link carries no label, so it needs `w-full` and the nav rows' height
	to give the active pill and its rail the same box as every other item; left to
	its own content it shrank to a small pill hugging the icon.
-->
<div
	class={[
		'shrink-0 overflow-hidden transition-[height] duration-200',
		compact ? 'h-12 border-b' : 'h-0 border-b-0'
	]}
>
	<div
		class="flex h-full items-center px-2 transition-opacity duration-100 {compact
			? 'opacity-100 delay-100'
			: 'opacity-0 delay-0'}"
		inert={!compact}
		aria-hidden={!compact}
	>
		<a
			href={fleet.href}
			title={t(fleet.label)}
			aria-current={active ? 'page' : undefined}
			class={[
				'relative flex w-full min-h-9 items-center rounded-md py-2 pl-3 pr-0 text-sm transition-colors list-row',
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
