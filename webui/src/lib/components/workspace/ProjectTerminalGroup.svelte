<script lang="ts">
	import { untrack } from 'svelte';
	import {
		Columns2,
		GripVertical,
		LayoutGrid,
		Plus,
		RefreshCw,
		Rows2,
		Sparkles,
		Square,
		SquareTerminal,
		X
	} from 'lucide-svelte';
	import TerminalSession from '$lib/components/terminal/TerminalSession.svelte';
	import {
		clearTerminalGroupState as clearGroupState,
		readTerminalGroupState as readGroupState,
		writeTerminalGroupState as writeGroupState,
		type PersistedTerminalGroup,
		type TerminalLayoutMode as LayoutMode
	} from '$lib/stores/projectWorkspace.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const MAX_TERMINALS = 8;

	interface TerminalTabItem {
		id: string;
		num: number;
		phase: 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error';
	}

	interface GridGeometry {
		cols: number;
		rows: number;
		capacity: number;
		/** Special 3-pane layout: left pane spans both rows, right column has 2 stacked panes. */
		leftSpan?: boolean;
	}

	interface Props {
		agent: string;
		cwd: string;
		conversationId?: string;
		onCloseAll: () => void;
	}

	let { agent, cwd, conversationId = '', onCloseAll }: Props = $props();

	const { t } = useTranslation();

	const groupKey = $derived(agent && cwd ? `${agent}:${conversationId}:${cwd}` : '');
	const initialSaved = untrack(() => readGroupState(groupKey));

	let tabs = $state<TerminalTabItem[]>(
		initialSaved
			? initialSaved.tabs.map((item) => ({ ...item, phase: 'idle' as const }))
			: [{ id: 'term-1', num: 1, phase: 'idle' }]
	);
	let activeId = $state(initialSaved?.activeId ?? 'term-1');
	let layoutMode = $state<LayoutMode>(initialSaved?.layoutMode ?? 'single');
	let draggingTabId = $state('');
	let dragOverTabId = $state('');

	/** Dynamic column & row percentages for any 1..4 cols × 1..2 rows split grid. */
	let colSizes = $state<number[]>(
		initialSaved?.colSizes?.length ? [...initialSaved.colSizes] : [100]
	);
	let rowSizes = $state<number[]>(
		initialSaved?.rowSizes?.length ? [...initialSaved.rowSizes] : [100]
	);
	let restoredKey = untrack(() => groupKey);

	// Reload persisted tabs & layout when switching to another conversation/agent.
	$effect(() => {
		const key = groupKey;
		if (!key || key === restoredKey) return;
		restoredKey = key;
		untrack(() => {
			const saved = readGroupState(key);
			if (saved) {
				tabs = saved.tabs.map((item) => ({ ...item, phase: 'idle' as const }));
				activeId = saved.activeId;
				layoutMode = saved.layoutMode;
				colSizes = saved.colSizes.length ? [...saved.colSizes] : [100];
				rowSizes = saved.rowSizes.length ? [...saved.rowSizes] : [100];
			} else {
				tabs = [{ id: 'term-1', num: 1, phase: 'idle' }];
				activeId = 'term-1';
				layoutMode = 'single';
				colSizes = [100];
				rowSizes = [100];
			}
		});
	});

	// Persist tab list, focus, layout mode, and manual divider ratios for this conversation.
	$effect(() => {
		const key = groupKey;
		if (!key || key !== restoredKey || tabs.length === 0) return;
		const snapshot: PersistedTerminalGroup = {
			tabs: tabs.map((item) => ({ id: item.id, num: item.num })),
			activeId,
			layoutMode,
			colSizes: [...colSizes],
			rowSizes: [...rowSizes]
		};
		untrack(() => {
			writeGroupState(key, snapshot);
		});
	});

	const sessionRefs: Record<string, TerminalSession | undefined> = {};
	let tabStrip = $state<HTMLElement | null>(null);
	let gridEl = $state<HTMLElement | null>(null);

	function setSessionRef(id: string, instance: TerminalSession | undefined) {
		if (instance) {
			sessionRefs[id] = instance;
		} else {
			delete sessionRefs[id];
		}
	}

	const activeTab = $derived(tabs.find((item) => item.id === activeId) ?? tabs[0]);

	const geometry = $derived.by((): GridGeometry => {
		const count = tabs.length;
		if (count <= 1 || layoutMode === 'single') {
			return { cols: 1, rows: 1, capacity: 1 };
		}
		if (layoutMode === 'split-v') {
			return { cols: 2, rows: 1, capacity: 2 };
		}
		if (layoutMode === 'split-h') {
			return { cols: 1, rows: 2, capacity: 2 };
		}
		// Both 'auto' and 'grid' adapt cleanly to ALL open terminals (up to 8).
		if (count === 2) return { cols: 2, rows: 1, capacity: 2 };
		if (count === 3) return { cols: 2, rows: 2, capacity: 3, leftSpan: true };
		if (count === 4) return { cols: 2, rows: 2, capacity: 4 };
		if (count <= 6) return { cols: 3, rows: 2, capacity: 6 };
		return { cols: 4, rows: 2, capacity: 8 };
	});

	// Keep colSizes / rowSizes aligned with active column & row counts.
	$effect(() => {
		const c = geometry.cols;
		const r = geometry.rows;
		untrack(() => {
			if (colSizes.length !== c) {
				colSizes = Array.from({ length: c }, () => 100 / c);
			}
			if (rowSizes.length !== r) {
				rowSizes = Array.from({ length: r }, () => 100 / r);
			}
		});
	});

	const visibleTabIds = $derived.by((): string[] => {
		if (tabs.length === 0) return [];
		if (geometry.capacity === 1) {
			return [activeTab?.id ?? tabs[0].id];
		}
		const ids = tabs.slice(0, geometry.capacity).map((item) => item.id);
		if (activeId && !ids.includes(activeId) && tabs.some((item) => item.id === activeId)) {
			ids[ids.length - 1] = activeId;
		}
		return ids;
	});

	const isSplit = $derived(visibleTabIds.length > 1);

	const gridStyle = $derived.by((): string => {
		const cols =
			colSizes.length === geometry.cols
				? colSizes
				: Array.from({ length: geometry.cols }, () => 100 / geometry.cols);
		const rows =
			rowSizes.length === geometry.rows
				? rowSizes
				: Array.from({ length: geometry.rows }, () => 100 / geometry.rows);

		const colTracks = cols
			.map((pct, i) =>
				i === cols.length - 1
					? `minmax(0, ${pct.toFixed(2)}fr)`
					: `minmax(0, ${pct.toFixed(2)}fr) 5px`
			)
			.join(' ');
		const rowTracks = rows
			.map((pct, i) =>
				i === rows.length - 1
					? `minmax(0, ${pct.toFixed(2)}fr)`
					: `minmax(0, ${pct.toFixed(2)}fr) 5px`
			)
			.join(' ');
		return `grid-template-columns: ${colTracks}; grid-template-rows: ${rowTracks};`;
	});

	function panePlacementStyle(slotIdx: number): string {
		if (slotIdx < 0) return '';
		const total = visibleTabIds.length;
		if (geometry.leftSpan) {
			if (slotIdx === 0) return 'grid-column: 1; grid-row: 1 / span 3;';
			if (slotIdx === 1) return 'grid-column: 3; grid-row: 1;';
			return 'grid-column: 3; grid-row: 3;';
		}
		const c = geometry.cols;
		const r = geometry.rows;
		if (r === 1) {
			return `grid-column: ${slotIdx * 2 + 1}; grid-row: 1;`;
		}
		if (c === 1) {
			return `grid-column: 1; grid-row: ${slotIdx * 2 + 1};`;
		}
		// 2-row grids (2x2, 3x2, 4x2): if the bottom row has fewer items than cols
		// (e.g. 5 or 7 terminals), let the last pane span the remaining columns so
		// there is never an awkward dead corner.
		const rowIdx = Math.floor(slotIdx / c);
		const colIdx = slotIdx % c;
		const gridRow = rowIdx * 2 + 1;
		const startCol = colIdx * 2 + 1;
		const isLast = slotIdx === total - 1;
		const remainingCols = c - colIdx;
		if (isLast && rowIdx === 1 && remainingCols > 1) {
			const spanTracks = remainingCols * 2 - 1;
			return `grid-column: ${startCol} / span ${spanTracks}; grid-row: ${gridRow};`;
		}
		return `grid-column: ${startCol}; grid-row: ${gridRow};`;
	}

	function colDividerRowStyle(cIdx: number): string {
		if (geometry.rows <= 1) return 'grid-row: 1;';
		if (geometry.leftSpan) return `grid-row: 1 / span ${geometry.rows * 2 - 1};`;
		const bottomCount = visibleTabIds.length - geometry.cols;
		// Stop the vertical divider at row 1 when the last bottom pane spans across
		// this column boundary (e.g. Terminal 5 in a 5-pane grid, Terminal 7 in a 7-pane grid).
		if (cIdx + 1 >= bottomCount) return 'grid-row: 1;';
		return `grid-row: 1 / span ${geometry.rows * 2 - 1};`;
	}

	function nextFreeNumber(): number {
		const used = new Set(tabs.map((item) => item.num));
		let n = 1;
		while (used.has(n)) n += 1;
		return n;
	}

	function selectTab(id: string) {
		activeId = id;
		queueMicrotask(() => sessionRefs[id]?.focus());
	}

	function addTerminal() {
		if (tabs.length >= MAX_TERMINALS) return;
		const num = nextFreeNumber();
		const id = `term-${num}`;
		tabs = [...tabs, { id, num, phase: 'idle' }];
		selectTab(id);
	}

	function chooseLayout(mode: LayoutMode) {
		if (mode === 'auto') {
			layoutMode = layoutMode === 'auto' ? 'single' : 'auto';
			return;
		}
		layoutMode = mode;
		if (mode !== 'single' && tabs.length < 2) {
			addTerminal();
		}
	}

	export function terminateAll() {
		clearGroupState(groupKey);
		for (const id of Object.keys(sessionRefs)) {
			const ref = sessionRefs[id];
			if (ref) {
				void ref.terminate();
				delete sessionRefs[id];
			}
		}
	}

	function closeTerminal(id: string) {
		const ref = sessionRefs[id];
		if (ref) {
			void ref.terminate();
			delete sessionRefs[id];
		}
		const idx = tabs.findIndex((item) => item.id === id);
		if (idx < 0) return;
		const next = tabs.filter((item) => item.id !== id);
		if (next.length === 0) {
			clearGroupState(groupKey);
			onCloseAll();
			return;
		}
		tabs = next;
		if (activeId === id) {
			activeId = next[Math.min(idx, next.length - 1)]?.id ?? next[0].id;
		}
	}

	function updatePhase(id: string, phase: TerminalTabItem['phase']) {
		const existing = tabs.find((item) => item.id === id);
		if (!existing || existing.phase === phase) return;
		tabs = tabs.map((item) => (item.id === id ? { ...item, phase } : item));
	}

	function reconnectTab(id: string) {
		const ref = sessionRefs[id];
		if (ref) void ref.reconnect();
	}

	function dotClass(phase: TerminalTabItem['phase']): string {
		if (phase === 'connected') return 'bg-emerald-500';
		if (phase === 'connecting') return 'bg-amber-500 animate-pulse';
		if (phase === 'error') return 'bg-destructive';
		return 'bg-muted-foreground/50';
	}

	// Drag-and-drop tab reordering (also swaps split pane positions without reconnecting PTYs).
	function onTabDragStart(event: DragEvent, id: string) {
		draggingTabId = id;
		if (event.dataTransfer) {
			event.dataTransfer.effectAllowed = 'move';
			event.dataTransfer.setData('text/plain', id);
		}
	}

	function onTabDragOver(event: DragEvent, targetId: string) {
		if (!draggingTabId || draggingTabId === targetId) return;
		event.preventDefault();
		if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
		dragOverTabId = targetId;
	}

	function onTabDrop(event: DragEvent, targetId: string) {
		event.preventDefault();
		const sourceId = draggingTabId || event.dataTransfer?.getData('text/plain') || '';
		draggingTabId = '';
		dragOverTabId = '';
		if (!sourceId || sourceId === targetId) return;
		const fromIdx = tabs.findIndex((item) => item.id === sourceId);
		const toIdx = tabs.findIndex((item) => item.id === targetId);
		if (fromIdx < 0 || toIdx < 0) return;
		const next = [...tabs];
		const [moved] = next.splice(fromIdx, 1);
		next.splice(toIdx, 0, moved);
		tabs = next;
	}

	function onTabDragEnd() {
		draggingTabId = '';
		dragOverTabId = '';
	}

	// Smooth pointer-captured manual resizing for any divider index between adjacent panes.
	function startResize(axis: 'col' | 'row', dividerIndex: number, event: PointerEvent) {
		const container = gridEl;
		const target = event.currentTarget as HTMLElement | null;
		if (!container || !target) return;
		event.preventDefault();
		target.setPointerCapture(event.pointerId);

		const startSizes = axis === 'col' ? [...colSizes] : [...rowSizes];
		const pairSum = (startSizes[dividerIndex] ?? 50) + (startSizes[dividerIndex + 1] ?? 50);
		const minSize = 15;
		const startPos = axis === 'col' ? event.clientX : event.clientY;
		const rect = container.getBoundingClientRect();
		const totalPx = axis === 'col' ? rect.width : rect.height;

		const onMove = (ev: PointerEvent) => {
			if (totalPx <= 0) return;
			const deltaPct = (((axis === 'col' ? ev.clientX : ev.clientY) - startPos) / totalPx) * 100;
			const nextA = Math.min(
				pairSum - minSize,
				Math.max(minSize, (startSizes[dividerIndex] ?? 50) + deltaPct)
			);
			const nextB = pairSum - nextA;
			const updated = [...startSizes];
			updated[dividerIndex] = nextA;
			updated[dividerIndex + 1] = nextB;
			if (axis === 'col') colSizes = updated;
			else rowSizes = updated;
		};

		const onUp = (ev: PointerEvent) => {
			target.removeEventListener('pointermove', onMove);
			target.removeEventListener('pointerup', onUp);
			target.removeEventListener('pointercancel', onUp);
			if (target.hasPointerCapture(ev.pointerId)) {
				target.releasePointerCapture(ev.pointerId);
			}
		};

		target.addEventListener('pointermove', onMove);
		target.addEventListener('pointerup', onUp);
		target.addEventListener('pointercancel', onUp);
	}

	function resetAxis(axis: 'col' | 'row') {
		if (axis === 'col') {
			colSizes = Array.from({ length: geometry.cols }, () => 100 / geometry.cols);
		} else {
			rowSizes = Array.from({ length: geometry.rows }, () => 100 / geometry.rows);
		}
	}

	function onStripWheel(event: WheelEvent) {
		const strip = tabStrip;
		if (!strip) return;
		if (Math.abs(event.deltaY) > Math.abs(event.deltaX) && strip.scrollWidth > strip.clientWidth) {
			event.preventDefault();
			strip.scrollLeft += event.deltaY;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-1 flex-col bg-background">
	<!-- Multi-terminal sub-tab bar: draggable tabs + segmented layout controls + reconnect -->
	<div class="flex shrink-0 items-center gap-1.5 border-b bg-muted/20 px-2 py-1">
		<div
			bind:this={tabStrip}
			onwheel={onStripWheel}
			class="flex min-w-0 flex-1 touch-pan-x items-center gap-1 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
		>
			{#each tabs as tab (tab.id)}
				{@const selected = tab.id === activeId}
				{@const visible = visibleTabIds.includes(tab.id)}
				<div
					role="listitem"
					draggable="true"
					ondragstart={(e) => onTabDragStart(e, tab.id)}
					ondragover={(e) => onTabDragOver(e, tab.id)}
					ondragleave={() => {
						if (dragOverTabId === tab.id) dragOverTabId = '';
					}}
					ondrop={(e) => onTabDrop(e, tab.id)}
					ondragend={onTabDragEnd}
					title={t('myaaw.workspace.terminal_drag_hint')}
					class={[
						'group/term flex shrink-0 items-center gap-0.5 rounded-md border text-[11px] transition-colors',
						selected
							? 'border-border bg-background text-foreground shadow-2xs'
							: visible
								? 'border-border/50 bg-background/50 text-foreground/80 hover:bg-background/80'
								: 'border-transparent text-muted-foreground hover:bg-muted/70 hover:text-foreground',
						draggingTabId === tab.id && 'opacity-50',
						dragOverTabId === tab.id && 'ring-2 ring-primary/50'
					]}
				>
					<span
						class="cursor-grab pl-1 text-muted-foreground/40 transition-colors group-hover/term:text-muted-foreground active:cursor-grabbing"
						aria-hidden="true"
					>
						<GripVertical class="h-3 w-3" />
					</span>
					<button
						type="button"
						class="flex items-center gap-1.5 py-1 pr-1 pl-0.5 text-left font-medium"
						onclick={() => selectTab(tab.id)}
					>
						<span class={['h-1.5 w-1.5 shrink-0 rounded-full', dotClass(tab.phase)]}></span>
						<SquareTerminal class="h-3 w-3 shrink-0 text-muted-foreground" />
						<span>{t('myaaw.workspace.terminal_tab_label', { n: tab.num })}</span>
					</button>
					<button
						type="button"
						class="mr-1 grid h-4 w-4 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
						onclick={() => closeTerminal(tab.id)}
						aria-label={t('common.close')}
						title={t('common.close')}
					>
						<X class="h-2.5 w-2.5" />
					</button>
				</div>
			{/each}

			<button
				type="button"
				disabled={tabs.length >= MAX_TERMINALS}
				class="grid h-6 w-6 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
				onclick={addTerminal}
				title={t('myaaw.workspace.terminal_new_tab')}
				aria-label={t('myaaw.workspace.terminal_new_tab')}
			>
				<Plus class="h-3.5 w-3.5" />
			</button>
		</div>

		<!-- Sleek segmented layout controls: Auto + Single / Split-V / Split-H / Grid + Reconnect -->
		<div class="flex shrink-0 items-center gap-1">
			<div class="flex items-center gap-0.5 rounded-lg border bg-muted/40 p-0.5">
				<button
					type="button"
					class={[
						'inline-flex h-5.5 items-center gap-1 rounded-md px-2 text-[10px] font-medium transition-all',
						layoutMode === 'auto'
							? 'bg-background text-primary shadow-2xs'
							: 'text-muted-foreground hover:text-foreground'
					]}
					onclick={() => chooseLayout('auto')}
					title={t('myaaw.workspace.terminal_layout_auto')}
					aria-pressed={layoutMode === 'auto'}
				>
					<Sparkles class="h-3 w-3" />
					<span class="hidden sm:inline">Auto</span>
				</button>

				<button
					type="button"
					class={[
						'grid h-5.5 w-5.5 place-items-center rounded-md transition-all',
						layoutMode === 'single'
							? 'bg-background text-foreground shadow-2xs'
							: 'text-muted-foreground hover:text-foreground'
					]}
					onclick={() => chooseLayout('single')}
					title={t('myaaw.workspace.terminal_layout_single')}
					aria-label={t('myaaw.workspace.terminal_layout_single')}
				>
					<Square class="h-3 w-3" />
				</button>
				<button
					type="button"
					class={[
						'grid h-5.5 w-5.5 place-items-center rounded-md transition-all',
						layoutMode === 'split-v'
							? 'bg-background text-foreground shadow-2xs'
							: 'text-muted-foreground hover:text-foreground'
					]}
					onclick={() => chooseLayout('split-v')}
					title={t('myaaw.workspace.terminal_layout_split_v')}
					aria-label={t('myaaw.workspace.terminal_layout_split_v')}
				>
					<Columns2 class="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					class={[
						'grid h-5.5 w-5.5 place-items-center rounded-md transition-all',
						layoutMode === 'split-h'
							? 'bg-background text-foreground shadow-2xs'
							: 'text-muted-foreground hover:text-foreground'
					]}
					onclick={() => chooseLayout('split-h')}
					title={t('myaaw.workspace.terminal_layout_split_h')}
					aria-label={t('myaaw.workspace.terminal_layout_split_h')}
				>
					<Rows2 class="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					class={[
						'grid h-5.5 w-5.5 place-items-center rounded-md transition-all',
						layoutMode === 'grid'
							? 'bg-background text-foreground shadow-2xs'
							: 'text-muted-foreground hover:text-foreground'
					]}
					onclick={() => chooseLayout('grid')}
					title={t('myaaw.workspace.terminal_layout_grid')}
					aria-label={t('myaaw.workspace.terminal_layout_grid')}
				>
					<LayoutGrid class="h-3.5 w-3.5" />
				</button>
			</div>

			<button
				type="button"
				disabled={activeTab?.phase === 'connecting'}
				class="grid h-6 w-6 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
				onclick={() => activeTab && reconnectTab(activeTab.id)}
				title={t('myaaw.terminal.reconnect')}
				aria-label={t('myaaw.terminal.reconnect')}
			>
				<RefreshCw
					class={activeTab?.phase === 'connecting' ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'}
				/>
			</button>
		</div>
	</div>

	<!-- Single CSS Grid host: all TerminalSession instances stay mounted at stable keys
	     so switching layouts, resizing, or reordering tabs NEVER disconnects a PTY. -->
	<div
		bind:this={gridEl}
		class="grid min-h-0 flex-1 overflow-hidden bg-background"
		style={gridStyle}
	>
		{#each tabs as tab (tab.id)}
			{@const slotIdx = visibleTabIds.indexOf(tab.id)}
			{@const isVisible = slotIdx >= 0}
			{@const isFocused = tab.id === activeId}
			<div
				role="presentation"
				onpointerdown={() => {
					if (activeId !== tab.id) activeId = tab.id;
				}}
				style={panePlacementStyle(slotIdx)}
				class={isVisible ? 'flex min-h-0 min-w-0 flex-col overflow-hidden bg-background' : 'hidden'}
			>
				{#if isSplit}
					<!-- Minimal modern pane header: clean 2px top accent bar on focused pane -->
					<div
						class={[
							'flex shrink-0 items-center justify-between gap-1 border-t-2 border-b border-b-border/50 px-2.5 py-1 text-[11px] transition-colors',
							isFocused
								? 'border-t-primary bg-background text-foreground'
								: 'border-t-transparent bg-muted/15 text-muted-foreground/80'
						]}
					>
						<span class="flex min-w-0 items-center gap-1.5 font-medium">
							<span class={['h-1.5 w-1.5 shrink-0 rounded-full', dotClass(tab.phase)]}></span>
							<span class="truncate">
								{t('myaaw.workspace.terminal_tab_label', { n: tab.num })}
							</span>
						</span>
						<span class="flex shrink-0 items-center gap-0.5">
							<button
								type="button"
								class="grid h-4.5 w-4.5 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
								onclick={(e) => {
									e.stopPropagation();
									reconnectTab(tab.id);
								}}
								title={t('myaaw.terminal.reconnect')}
								aria-label={t('myaaw.terminal.reconnect')}
							>
								<RefreshCw
									class={tab.phase === 'connecting' ? 'h-2.5 w-2.5 animate-spin' : 'h-2.5 w-2.5'}
								/>
							</button>
							<button
								type="button"
								class="grid h-4.5 w-4.5 place-items-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
								onclick={(e) => {
									e.stopPropagation();
									closeTerminal(tab.id);
								}}
								title={t('common.close')}
								aria-label={t('common.close')}
							>
								<X class="h-2.5 w-2.5" />
							</button>
						</span>
					</div>
				{/if}

				<div class="flex min-h-0 flex-1 flex-col">
					<TerminalSession
						bind:this={() => sessionRefs[tab.id], (instance) => setSessionRef(tab.id, instance)}
						{agent}
						{cwd}
						{conversationId}
						slot={tab.id}
						compact
						hideHeader
						onStateChange={(st) => updatePhase(tab.id, st.phase)}
					/>
				</div>
			</div>
		{/each}

		<!-- Dynamic vertical resizers between every adjacent column -->
		{#if geometry.cols > 1}
			{#each { length: geometry.cols - 1 } as _, cIdx (cIdx)}
				<div
					role="separator"
					aria-orientation="vertical"
					style={`grid-column: ${cIdx * 2 + 2}; ${colDividerRowStyle(cIdx)}`}
					class="group/sep relative z-10 flex cursor-col-resize items-center justify-center bg-transparent transition-colors hover:bg-primary/15"
					onpointerdown={(e) => startResize('col', cIdx, e)}
					ondblclick={() => resetAxis('col')}
					title={t('myaaw.workspace.terminal_resize_hint')}
				>
					<span
						class="h-full w-px bg-border/60 transition-colors group-hover/sep:w-0.5 group-hover/sep:bg-primary/70"
					></span>
				</div>
			{/each}
		{/if}

		<!-- Horizontal resizer between top & bottom rows -->
		{#if geometry.rows > 1}
			<div
				role="separator"
				aria-orientation="horizontal"
				style={geometry.leftSpan
					? 'grid-column: 3; grid-row: 2;'
					: `grid-column: 1 / span ${geometry.cols * 2 - 1}; grid-row: 2;`}
				class="group/sep relative z-10 flex cursor-row-resize items-center justify-center bg-transparent transition-colors hover:bg-primary/15"
				onpointerdown={(e) => startResize('row', 0, e)}
				ondblclick={() => resetAxis('row')}
				title={t('myaaw.workspace.terminal_resize_hint')}
			>
				<span
					class="h-px w-full bg-border/60 transition-colors group-hover/sep:h-0.5 group-hover/sep:bg-primary/70"
				></span>
			</div>
		{/if}
	</div>
</div>
