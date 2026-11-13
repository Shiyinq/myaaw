<script lang="ts">
	import { page } from '$app/state';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref } from '$lib/utils/nav';
	import type { NavGroup } from '$lib/utils/nav';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: hide the labels and the group headings. */
		compact?: boolean;
		groups: NavGroup[];
	}

	let { compact = false, groups }: Props = $props();
</script>

<nav class={['flex-1 overflow-y-auto', compact ? 'space-y-2 p-2' : 'space-y-5 p-3']}>
	{#each groups as group, groupIndex (group.label)}
		<div class="space-y-1">
			<p
				class="truncate px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground transition-all duration-200 {compact
					? 'max-h-0 opacity-0 delay-0'
					: 'max-h-5 opacity-100 delay-200'}"
			>
				{t(group.label)}
			</p>
			{#if compact && groupIndex > 0}
				<div class="mx-auto my-2 h-px w-6 bg-border"></div>
			{/if}
			{#each group.items as item (item.href)}
				{@const Icon = item.icon}
				{@const active = isActiveHref(page.url.pathname, item.href)}
				<a
					href={item.href}
					title={compact ? t(item.label) : undefined}
					aria-current={active ? 'page' : undefined}
					class={[
						'relative flex items-center rounded-md text-sm transition-colors list-row',
						compact ? 'gap-0 pl-3 pr-0 py-2' : 'gap-2 px-2 py-1.5',
						active
							? 'bg-accent font-medium text-accent-foreground'
							: 'text-muted-foreground hover:bg-muted hover:text-foreground'
					]}
				>
					{#if compact && active}
						<span class="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"></span>
					{/if}
					<Icon class="h-4 w-4 shrink-0" />
					<span
						class="truncate transition-all duration-200 {compact
							? 'max-w-0 opacity-0 delay-0'
							: 'max-w-full opacity-100 delay-200'}"
					>
						{t(item.label)}
					</span>
				</a>
			{/each}
		</div>
	{/each}
</nav>
