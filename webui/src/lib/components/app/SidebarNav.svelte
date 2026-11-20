<script lang="ts">
	import { untrack } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { ChevronRight } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isActiveHref } from '$lib/utils/nav';
	import type { NavGroup } from '$lib/utils/nav';

	const { t } = useTranslation();

	const STORAGE_KEY = 'myaaw_nav_groups';

	interface Props {
		/** Icon-only rail: hide the labels and the group headings. */
		compact?: boolean;
		groups: NavGroup[];
	}

	let { compact = false, groups }: Props = $props();

	function loadOpenState(): Record<string, boolean> {
		if (!browser) return {};
		try {
			const raw = localStorage.getItem(STORAGE_KEY);
			if (!raw) return {};
			const parsed = JSON.parse(raw);
			return parsed && typeof parsed === 'object' ? (parsed as Record<string, boolean>) : {};
		} catch {
			return {};
		}
	}

	function saveOpenState(next: Record<string, boolean>) {
		if (!browser) return;
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
		} catch {
			// Ignore storage quota / privacy mode failures.
		}
	}

	let openMap = $state<Record<string, boolean>>(loadOpenState());
	let lastPathname = '';

	function isGroupOpen(group: NavGroup): boolean {
		if (!group.collapsible) return true;
		return openMap[group.label] ?? group.defaultOpen ?? true;
	}

	function toggleGroup(group: NavGroup) {
		if (!group.collapsible) return;
		const next = { ...openMap, [group.label]: !isGroupOpen(group) };
		openMap = next;
		saveOpenState(next);
	}

	// Navigating into a route inside a folded group unfolds it automatically so
	// the active item is visible, while still letting the reader fold it manually.
	$effect(() => {
		const pathname = page.url.pathname;
		if (pathname === lastPathname) return;
		lastPathname = pathname;
		untrack(() => {
			for (const group of groups) {
				if (!group.collapsible) continue;
				if (group.items.some((item) => isActiveHref(pathname, item.href))) {
					if (!isGroupOpen(group)) {
						const next = { ...openMap, [group.label]: true };
						openMap = next;
						saveOpenState(next);
					}
				}
			}
		});
	});
</script>

<nav class={['flex-1 overflow-y-auto', compact ? 'space-y-2 p-2' : 'space-y-4 p-3']}>
	{#each groups as group, groupIndex (group.label)}
		{@const open = compact || isGroupOpen(group)}
		<div class="space-y-1">
			{#if group.collapsible}
				<button
					type="button"
					onclick={() => toggleGroup(group)}
					aria-expanded={open}
					inert={compact}
					class="flex w-full items-center justify-between gap-2 overflow-hidden rounded-md px-2 py-0.5 text-left text-[10px] font-semibold tracking-widest text-muted-foreground uppercase transition-all duration-200 hover:text-foreground {compact
						? 'pointer-events-none max-h-0 py-0 opacity-0 delay-0'
						: 'max-h-6 opacity-100 delay-200'}"
				>
					<span class="truncate">{t(group.label)}</span>
					<ChevronRight
						class="h-3 w-3 shrink-0 transition-transform duration-150 {open ? 'rotate-90' : ''}"
					/>
				</button>
			{:else}
				<p
					class="truncate px-2 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase transition-all duration-200 {compact
						? 'max-h-0 opacity-0 delay-0'
						: 'max-h-5 opacity-100 delay-200'}"
				>
					{t(group.label)}
				</p>
			{/if}
			{#if compact}
				<div class={['mx-auto h-px w-6 bg-border', groupIndex === 0 ? 'mt-0 mb-2' : 'my-2']}></div>
			{/if}
			<div
				class={[
					'grid transition-[grid-template-rows,opacity] duration-200',
					open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
				]}
				inert={!open}
				aria-hidden={!open}
			>
				<div class="space-y-1 overflow-hidden">
					{#each group.items as item (item.href)}
						{@const Icon = item.icon}
						{@const active = isActiveHref(page.url.pathname, item.href)}
						<a
							href={item.href}
							title={compact ? t(item.label) : undefined}
							aria-current={active ? 'page' : undefined}
							class={[
								'relative flex items-center rounded-md text-sm transition-colors list-row',
								compact ? 'gap-0 py-2 pr-0 pl-3' : 'gap-2 px-2 py-1.5',
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
			</div>
		</div>
	{/each}
</nav>
