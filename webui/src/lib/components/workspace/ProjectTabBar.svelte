<script lang="ts">
	import { FileCode2, FileDiff, GitCompare, MessageSquare, SquareTerminal, X } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ProjectWorkspaceTab } from '$lib/types';

	interface Props {
		tabs: ProjectWorkspaceTab[];
		terminalOpen: boolean;
		activeTabId: string;
		onSelectChat: () => void;
		onSelectTerminal: () => void;
		onCloseTerminal: () => void;
		onSelectTab: (id: string) => void;
		onCloseTab: (id: string) => void;
	}

	let {
		tabs,
		terminalOpen,
		activeTabId,
		onSelectChat,
		onSelectTerminal,
		onCloseTerminal,
		onSelectTab,
		onCloseTab
	}: Props = $props();

	const { t } = useTranslation();

	let root = $state<HTMLElement | null>(null);
	let tabStrip = $state<HTMLElement | null>(null);
	let gutter = $state(12);

	function measureGutter() {
		if (!root) return;
		const bar = document.querySelector('header');
		const rootRect = root.getBoundingClientRect();
		if (!bar) {
			gutter = 12;
			return;
		}
		const barRect = bar.getBoundingClientRect();
		if (barRect.bottom <= rootRect.top + 1) {
			gutter = 12;
			return;
		}
		const edges = [...bar.querySelectorAll('button, a')]
			.map((el) => el.getBoundingClientRect())
			.filter((rect) => rect.width > 0 && rect.left > window.innerWidth / 2)
			.map((rect) => rect.left);
		const actionLeft = edges.length > 0 ? Math.min(...edges) : window.innerWidth;
		gutter = Math.max(12, Math.round(rootRect.right - actionLeft + 8));
	}

	$effect(() => {
		const el = root;
		if (!el) return;
		measureGutter();
		const observer = new ResizeObserver(() => measureGutter());
		observer.observe(el);
		const bar = document.querySelector('header');
		if (bar) observer.observe(bar);
		window.addEventListener('resize', measureGutter);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measureGutter);
		};
	});

	$effect(() => {
		const currentId = activeTabId;
		const strip = tabStrip;
		if (!currentId || !strip) return;
		const activeEl = strip.querySelector<HTMLElement>(`[data-tab-id="${CSS.escape(currentId)}"]`);
		activeEl?.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' });
	});

	function onTabStripWheel(event: WheelEvent) {
		const strip = tabStrip;
		if (!strip) return;
		if (Math.abs(event.deltaY) > Math.abs(event.deltaX) && strip.scrollWidth > strip.clientWidth) {
			event.preventDefault();
			strip.scrollLeft += event.deltaY;
		}
	}
</script>

<!-- Unified workspace tab bar: pinned [Chat | Terminal] on the left + scrollable open files/diffs -->
<div
	bind:this={root}
	class="flex h-10 shrink-0 items-center gap-1.5 pl-2"
	style:padding-right="{gutter}px"
>
	<!-- Pinned left group: Chat and Terminal always stay visible regardless of file tab scroll -->
	<div
		class={[
			'flex shrink-0 items-center gap-1',
			tabs.length > 0 && 'border-r border-border/50 pr-1.5'
		]}
	>
		<button
			type="button"
			data-tab-id="chat"
			class={[
				'flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
				activeTabId === 'chat'
					? 'border-border/80 bg-muted/70 text-foreground'
					: 'border-transparent bg-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground'
			]}
			onclick={onSelectChat}
			title={t('myaaw.nav.chat')}
		>
			<MessageSquare class="h-3.5 w-3.5 shrink-0" />
			<span>{t('myaaw.nav.chat')}</span>
		</button>

		{#if terminalOpen}
			{@const termSelected = activeTabId === 'terminal'}
			<div
				data-tab-id="terminal"
				class={[
					'flex shrink-0 items-center gap-0.5 rounded-md border text-xs transition-colors',
					termSelected
						? 'border-border/80 bg-muted/70 text-foreground'
						: 'border-transparent bg-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground'
				]}
			>
				<button
					type="button"
					class="flex items-center gap-1.5 py-1 pr-1 pl-2 text-left font-medium"
					onclick={onSelectTerminal}
					title={t('myaaw.nav.terminal')}
				>
					<SquareTerminal class="h-3.5 w-3.5 shrink-0 text-primary" />
					<span>{t('myaaw.nav.terminal')}</span>
				</button>
				<button
					type="button"
					class="mr-0.5 grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={onCloseTerminal}
					aria-label={t('common.close')}
					title={t('common.close')}
				>
					<X class="h-3 w-3" />
				</button>
			</div>
		{/if}
	</div>

	<!-- Scrollable / swipeable strip for open files, single diffs, and multi-diffs -->
	<div
		bind:this={tabStrip}
		onwheel={onTabStripWheel}
		class="flex min-w-0 flex-1 touch-pan-x items-center gap-1 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
	>
		{#each tabs as tab (tab.id)}
			{@const selected = tab.id === activeTabId}
			<div
				data-tab-id={tab.id}
				class={[
					'group/tab flex max-w-52 shrink-0 items-center gap-0.5 rounded-md border text-xs transition-colors',
					selected
						? 'border-border/80 bg-muted/70 text-foreground'
						: 'border-transparent bg-transparent text-muted-foreground hover:bg-muted/50 hover:text-foreground'
				]}
			>
				<button
					type="button"
					class="flex min-w-0 flex-1 items-center gap-1.5 py-1 pr-1 pl-2 text-left"
					onclick={() => onSelectTab(tab.id)}
					title={tab.kind === 'multi-diff'
						? tab.label
						: tab.path.replace(/^\/project\/?/, '') || tab.label}
				>
					{#if tab.kind === 'multi-diff'}
						<FileDiff class="h-3.5 w-3.5 shrink-0 text-primary" />
					{:else if tab.kind === 'diff'}
						<GitCompare class="h-3.5 w-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
					{:else}
						<FileCode2 class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{/if}
					<span class="truncate font-medium">{tab.label}</span>
					{#if tab.kind === 'diff' && tab.commit}
						<span class="shrink-0 font-mono text-[10px] text-muted-foreground">
							@{tab.commit.slice(0, 6)}
						</span>
					{:else if tab.kind === 'diff' && tab.staged}
						<span class="shrink-0 text-[10px] text-emerald-600 dark:text-emerald-400">S</span>
					{/if}
				</button>
				<button
					type="button"
					class="mr-0.5 grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={() => onCloseTab(tab.id)}
					aria-label={t('common.close')}
					title={t('common.close')}
				>
					<X class="h-3 w-3" />
				</button>
			</div>
		{/each}
	</div>
</div>
