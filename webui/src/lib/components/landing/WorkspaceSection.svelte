<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Archive,
		Check,
		CheckCircle2,
		ChevronDown,
		FileCode2,
		Folder,
		FolderLock,
		FolderTree,
		GitBranch,
		Image,
		Lock,
		RotateCcw,
		Sparkles,
		SquareTerminal
	} from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import {
		isReducedMotion,
		loadGsap,
		motionButton,
		motionPop,
		motionPulse
	} from '$lib/landing/motion';
	import { WORKSPACE_TABS, type WorkspaceTabId } from '$lib/landing/sections';

	const { t } = useTranslation();

	let rootEl: HTMLElement | null = $state(null);
	let activeTab = $state<WorkspaceTabId>('files');

	// --- Files tab interactive fixture ---
	const SAMPLE_FILES = [
		{
			path: 'internal/runtime/bridge.go',
			status: 'M',
			lines: [
				'package runtime',
				'',
				'func NewMCPBridge(cfg BridgeConfig) *Bridge {',
				'    return &Bridge{',
				'        Transport: StdioTransport(),',
				'        Tools:     []string{"cron", "a2a_contact", "extra_messages"},',
				'    }',
				'}'
			]
		},
		{
			path: 'internal/group/room.go',
			status: 'M',
			lines: [
				'package group',
				'',
				'func (r *Room) SyncDelta(agentID string) []Message {',
				'    cursor := r.cursors[agentID]',
				'    return r.store.MessagesAfter(r.ID, cursor)',
				'}'
			]
		},
		{
			path: 'skills/release/SKILL.md',
			status: 'A',
			lines: [
				'---',
				'name: release-checklist',
				'description: Verify Go + WebUI checks before tagging',
				'---',
				'',
				'1. Run `go test ./...`',
				'2. Verify checksums and post approval card'
			]
		}
	] as const;

	let selectedFileIdx = $state(0);
	const currentFile = $derived(SAMPLE_FILES[selectedFileIdx] ?? SAMPLE_FILES[0]);

	// --- Git tab interactive fixture ---
	const GIT_STAGED = [
		{ path: 'webui/src/lib/stores/groups.svelte.ts', badge: 'M' },
		{ path: 'internal/runtime/bridge.go', badge: 'M' }
	] as const;

	const GIT_CHANGES = [{ path: 'docs/CHANGELOG.md', badge: 'A' }] as const;

	const GIT_DIFF_LINES = [
		{
			kind: 'ctx',
			num: '41',
			text: '  export function attachWorkspace(roomId: string, dir: string) {'
		},
		{ kind: 'del', num: '42', text: '-   return api.post(`/groups/${roomId}/dir`, { dir });' },
		{ kind: 'add', num: '42', text: '+   return api.patch(`/groups/${roomId}/workspace`, {' },
		{ kind: 'add', num: '43', text: '+     path: dir,' },
		{ kind: 'add', num: '44', text: '+     shared: true' },
		{ kind: 'add', num: '45', text: '+   });' },
		{ kind: 'ctx', num: '46', text: '  }' }
	] as const;

	// --- Terminal tab interactive fixture ---
	type TerminalLayout = 'grid' | 'split-v' | 'split-h' | 'single';
	const TERMINAL_LAYOUTS: readonly TerminalLayout[] = ['grid', 'split-v', 'split-h', 'single'];
	let terminalLayout = $state<TerminalLayout>('grid');

	const TERMINAL_PANES = [
		{
			id: 'p1',
			titleKey: 'landing.workspace.terminal.pane_1_title',
			cmdKey: 'landing.workspace.terminal.pane_1_cmd',
			outKey: 'landing.workspace.terminal.pane_1_out'
		},
		{
			id: 'p2',
			titleKey: 'landing.workspace.terminal.pane_2_title',
			cmdKey: 'landing.workspace.terminal.pane_2_cmd',
			outKey: 'landing.workspace.terminal.pane_2_out'
		},
		{
			id: 'p3',
			titleKey: 'landing.workspace.terminal.pane_3_title',
			cmdKey: 'landing.workspace.terminal.pane_3_cmd',
			outKey: 'landing.workspace.terminal.pane_3_out'
		},
		{
			id: 'p4',
			titleKey: 'landing.workspace.terminal.pane_4_title',
			cmdKey: 'landing.workspace.terminal.pane_4_cmd',
			outKey: 'landing.workspace.terminal.pane_4_out'
		}
	] as const;

	const visiblePanes = $derived(
		terminalLayout === 'single'
			? TERMINAL_PANES.slice(0, 1)
			: terminalLayout === 'grid'
				? TERMINAL_PANES
				: TERMINAL_PANES.slice(0, 2)
	);

	// --- Rich chat interactive mini-playground ---
	let checks = $state<[boolean, boolean, boolean]>([true, true, false]);
	let lockedChoice = $state<'approve' | 'revise' | null>(null);
	let lockBannerEl: HTMLDivElement | null = $state(null);

	function toggleCheck(index: 0 | 1 | 2) {
		if (lockedChoice) return;
		checks[index] = !checks[index];
	}

	function triggerAction(choice: 'approve' | 'revise') {
		if (lockedChoice) return;
		lockedChoice = choice;
		queueMicrotask(() => {
			motionPulse(lockBannerEl, { scale: 1.03, duration: 0.28 });
		});
	}

	function resetPlayground() {
		lockedChoice = null;
		checks = [true, true, false];
	}

	onMount(() => {
		if (!rootEl || isReducedMotion()) return;

		let cancelled = false;
		let ctx: { revert: () => void } | null = null;

		void loadGsap().then((bundle) => {
			if (cancelled || !bundle || !rootEl || isReducedMotion()) return;
			const { gsap } = bundle;

			ctx = gsap.context(() => {
				gsap.from('[data-ws-hairline]', {
					scaleX: 0,
					transformOrigin: 'left center',
					duration: 0.75,
					ease: 'power3.out',
					clearProps: 'transform',
					scrollTrigger: {
						trigger: rootEl,
						start: 'top 84%',
						once: true
					}
				});

				gsap.from('[data-ws-head]', {
					y: 18,
					opacity: 0,
					duration: 0.58,
					stagger: 0.07,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: rootEl,
						start: 'top 82%',
						once: true
					}
				});

				gsap.from('[data-ws-stage]', {
					y: 22,
					opacity: 0,
					duration: 0.65,
					ease: 'power3.out',
					clearProps: 'transform,opacity',
					scrollTrigger: {
						trigger: '[data-ws-stage]',
						start: 'top 84%',
						once: true
					}
				});
			}, rootEl);
		});

		return () => {
			cancelled = true;
			ctx?.revert();
		};
	});
</script>

<section
	id="workspace"
	data-landing-section
	bind:this={rootEl}
	class="border-b border-border/60 py-16 sm:py-24"
>
	<div class="mx-auto max-w-6xl px-4 sm:px-6">
		<div data-ws-hairline class="mb-6 h-px w-full bg-border/70"></div>

		<div class="grid gap-6 lg:grid-cols-[1.2fr_0.8fr] lg:items-end">
			<div class="min-w-0">
				<p data-ws-head class="ws-label flex items-center gap-2 text-muted-foreground">
					<span class="text-primary">02</span>
					<span class="text-border">/</span>
					<span>{t('landing.workspace.eyebrow')}</span>
				</p>
				<h2
					data-ws-head
					class="mt-3 text-2xl leading-[1.12] font-bold tracking-tight text-foreground sm:text-4xl"
				>
					{t('landing.workspace.title')}
				</h2>
			</div>

			<p data-ws-head class="text-sm leading-relaxed text-muted-foreground sm:text-[15px]">
				{t('landing.workspace.lead')}
			</p>
		</div>

		<!-- Tab Bar + Sample Badge -->
		<div
			data-ws-head
			class="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
		>
			<div
				role="tablist"
				aria-label={t('landing.workspace.eyebrow')}
				class="flex flex-wrap items-center gap-1.5 rounded-xl border border-border/70 bg-muted/35 p-1"
			>
				{#each WORKSPACE_TABS as tab (tab)}
					{@const active = activeTab === tab}
					<button
						type="button"
						role="tab"
						aria-selected={active}
						onclick={() => (activeTab = tab)}
						use:motionButton={{ scale: 0.96, hoverScale: 1.02 }}
						class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {active
							? 'bg-primary text-primary-foreground shadow-xs'
							: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}"
					>
						{#if tab === 'files'}
							<FolderTree class="size-3.5 shrink-0" />
						{:else if tab === 'git'}
							<GitBranch class="size-3.5 shrink-0" />
						{:else if tab === 'terminal'}
							<SquareTerminal class="size-3.5 shrink-0" />
						{:else}
							<Sparkles class="size-3.5 shrink-0" />
						{/if}
						<span>{t(`landing.workspace.tabs.${tab}`)}</span>
					</button>
				{/each}
			</div>

			<span
				class="ws-mono inline-flex w-fit items-center rounded-md border border-border/70 bg-muted/35 px-2.5 py-1 text-[10px] tracking-wider text-muted-foreground uppercase"
			>
				{t('landing.workspace.sample_badge')}
			</span>
		</div>

		<!-- Active Tab Panel -->
		<div data-ws-stage class="mt-4">
			{#key activeTab}
				<div
					role="tabpanel"
					use:motionPop={{ y: 12, duration: 0.32 }}
					data-slot="card"
					class="frost-card rounded-2xl bg-card p-5 text-card-foreground ring-1 ring-(--surface-ring) sm:p-6"
				>
					{#if activeTab === 'files'}
						<div class="grid gap-6 lg:grid-cols-[0.82fr_1.18fr] lg:items-start">
							<div class="min-w-0 space-y-4">
								<span
									class="ws-mono inline-flex rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px] text-primary"
								>
									{t('landing.workspace.files.badge')}
								</span>
								<h3 class="text-lg font-semibold tracking-tight text-foreground sm:text-xl">
									{t('landing.workspace.files.title')}
								</h3>
								<p class="text-sm leading-relaxed text-muted-foreground">
									{t('landing.workspace.files.body')}
								</p>
								<p class="text-xs leading-relaxed text-muted-foreground">
									{t('landing.workspace.files.shared_note')}
								</p>

								<div class="rounded-xl border border-border/60 bg-muted/30 p-3.5">
									<div class="flex items-center gap-2 text-xs font-semibold text-foreground">
										<Image class="size-3.5 shrink-0 text-primary" />
										<span>{t('landing.workspace.rich.artifacts_title')}</span>
									</div>
									<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
										{t('landing.workspace.rich.artifacts_body')}
									</p>
								</div>
							</div>

							<!-- Mock IDE Split -->
							<div
								class="min-w-0 overflow-hidden rounded-xl border border-border/70 bg-background/60"
							>
								<div
									class="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-muted/40 px-3.5 py-2"
								>
									<div class="ws-mono flex min-w-0 items-center gap-2 text-xs text-foreground">
										<FolderLock class="size-3.5 shrink-0 text-primary" />
										<span class="truncate">{t('landing.workspace.files.preview_path')}</span>
									</div>
									<span class="ws-mono text-[10px] text-muted-foreground">
										{t('landing.workspace.files.preview_status')}
									</span>
								</div>

								<div
									class="grid divide-y divide-border/60 sm:grid-cols-[0.85fr_1.15fr] sm:divide-x sm:divide-y-0"
								>
									<div class="min-w-0 p-2.5">
										<div
											class="ws-mono mb-2 px-1.5 text-[10px] tracking-wider text-muted-foreground uppercase"
										>
											{t('landing.workspace.files.tree_title')}
										</div>
										<div class="space-y-1">
											{#each SAMPLE_FILES as file, idx (file.path)}
												{@const active = idx === selectedFileIdx}
												<button
													type="button"
													onclick={() => (selectedFileIdx = idx)}
													class="flex w-full min-w-0 items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {active
														? 'bg-accent text-accent-foreground'
														: 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'}"
												>
													<span class="flex min-w-0 items-center gap-1.5">
														<FileCode2 class="size-3.5 shrink-0 opacity-75" />
														<span class="ws-mono truncate text-[11px]">{file.path}</span>
													</span>
													<span class="ws-mono shrink-0 text-[10px] text-primary">
														{file.status}
													</span>
												</button>
											{/each}
										</div>
									</div>

									<div class="min-w-0 bg-muted/20 p-3">
										<div
											class="mb-2 flex items-center justify-between gap-2 border-b border-border/50 pb-1.5"
										>
											<span class="ws-mono truncate text-[11px] font-medium text-foreground">
												{currentFile.path}
											</span>
											<Folder class="size-3.5 shrink-0 text-muted-foreground" />
										</div>
										<div data-lenis-prevent class="max-h-52 overflow-auto">
											<pre
												class="ws-mono text-[11px] leading-relaxed text-foreground/90">{#each currentFile.lines as line, i (i)}<span
														class="mr-3 inline-block w-4 text-right text-muted-foreground/50 select-none"
														>{i + 1}</span
													>{line}
												{/each}</pre>
										</div>
									</div>
								</div>

								<div
									class="border-t border-border/60 bg-muted/30 px-3.5 py-2 text-[11px] text-muted-foreground"
								>
									{t('landing.workspace.files.preview_caption')}
								</div>
							</div>
						</div>
					{:else if activeTab === 'git'}
						<div class="grid gap-6 lg:grid-cols-[0.82fr_1.18fr] lg:items-start">
							<div class="min-w-0 space-y-4">
								<span
									class="ws-mono inline-flex rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px] text-primary"
								>
									{t('landing.workspace.git.badge')}
								</span>
								<h3 class="text-lg font-semibold tracking-tight text-foreground sm:text-xl">
									{t('landing.workspace.git.title')}
								</h3>
								<p class="text-sm leading-relaxed text-muted-foreground">
									{t('landing.workspace.git.body')}
								</p>
								<p class="text-xs leading-relaxed text-muted-foreground">
									{t('landing.workspace.git.diff_caption')}
								</p>
							</div>

							<!-- VS Code-style Source Control Mockup -->
							<div
								class="min-w-0 overflow-hidden rounded-xl border border-border/70 bg-background/60"
							>
								<div
									class="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-muted/40 px-3.5 py-2.5"
								>
									<div class="flex flex-wrap items-center gap-2">
										<span
											class="ws-mono inline-flex items-center gap-1.5 rounded-md border border-border/70 bg-card px-2 py-0.5 text-[11px] text-foreground"
										>
											<GitBranch class="size-3 text-primary" />
											{t('landing.workspace.git.branch')}
										</span>
										<span
											class="ws-mono inline-flex items-center gap-1 rounded-md border border-border/60 bg-muted/50 px-2 py-0.5 text-[10px] text-muted-foreground"
										>
											<Archive class="size-3" />
											{t('landing.workspace.git.stash_label')}
										</span>
									</div>

									<div
										class="inline-flex items-center overflow-hidden rounded-md bg-primary text-primary-foreground"
									>
										<span class="px-2.5 py-1 text-xs font-medium">
											{t('landing.workspace.git.commit_cta')}
										</span>
										<span class="border-l border-primary-foreground/25 px-1.5 py-1">
											<ChevronDown class="size-3.5" />
										</span>
									</div>
								</div>

								<div
									class="grid divide-y divide-border/60 sm:grid-cols-[0.82fr_1.18fr] sm:divide-x sm:divide-y-0"
								>
									<div class="min-w-0 space-y-3 p-3">
										<div>
											<div
												class="ws-mono mb-1.5 text-[10px] tracking-wider text-muted-foreground uppercase"
											>
												{t('landing.workspace.git.staged_label')}
											</div>
											<div class="space-y-1">
												{#each GIT_STAGED as file (file.path)}
													<div
														class="flex min-w-0 items-center justify-between gap-2 rounded px-2 py-1 text-xs bg-muted/35"
													>
														<span class="ws-mono truncate text-[11px] text-foreground"
															>{file.path}</span
														>
														<span
															class="ws-mono shrink-0 rounded bg-primary/15 px-1 text-[10px] font-semibold text-primary"
														>
															{file.badge}
														</span>
													</div>
												{/each}
											</div>
										</div>

										<div>
											<div
												class="ws-mono mb-1.5 text-[10px] tracking-wider text-muted-foreground uppercase"
											>
												{t('landing.workspace.git.changes_label')}
											</div>
											<div class="space-y-1">
												{#each GIT_CHANGES as file (file.path)}
													<div
														class="flex min-w-0 items-center justify-between gap-2 rounded px-2 py-1 text-xs"
													>
														<span class="ws-mono truncate text-[11px] text-muted-foreground">
															{file.path}
														</span>
														<span
															class="ws-mono shrink-0 rounded bg-accent px-1 text-[10px] font-semibold text-accent-foreground"
														>
															{file.badge}
														</span>
													</div>
												{/each}
											</div>
										</div>
									</div>

									<div class="min-w-0 bg-muted/20 p-3">
										<div
											class="ws-mono mb-2 truncate border-b border-border/50 pb-1.5 text-[11px] text-foreground"
										>
											{t('landing.workspace.git.diff_file')}
										</div>
										<div data-lenis-prevent class="max-h-52 overflow-auto">
											<div class="ws-mono space-y-0.5 text-[11px] leading-relaxed">
												{#each GIT_DIFF_LINES as line, i (i)}
													<div
														class="flex gap-2 rounded px-1.5 whitespace-pre {line.kind === 'add'
															? 'bg-primary/12 text-foreground'
															: line.kind === 'del'
																? 'bg-destructive/12 text-muted-foreground line-through'
																: 'text-muted-foreground'}"
													>
														<span class="w-5 shrink-0 text-right opacity-55 select-none"
															>{line.num}</span
														>
														<span>{line.text}</span>
													</div>
												{/each}
											</div>
										</div>
									</div>
								</div>
							</div>
						</div>
					{:else if activeTab === 'terminal'}
						<div class="grid gap-6 lg:grid-cols-[0.78fr_1.22fr] lg:items-start">
							<div class="min-w-0 space-y-4">
								<span
									class="ws-mono inline-flex rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px] text-primary"
								>
									{t('landing.workspace.terminal.badge')}
								</span>
								<h3 class="text-lg font-semibold tracking-tight text-foreground sm:text-xl">
									{t('landing.workspace.terminal.title')}
								</h3>
								<p class="text-sm leading-relaxed text-muted-foreground">
									{t('landing.workspace.terminal.body')}
								</p>
								<p class="text-xs leading-relaxed text-muted-foreground">
									{t('landing.workspace.terminal.persisted_note')}
								</p>
							</div>

							<!-- Multi-Tab Split Terminal Mockup -->
							<div
								class="min-w-0 overflow-hidden rounded-xl border border-border/70 bg-background/60"
							>
								<div
									class="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-muted/40 px-3.5 py-2"
								>
									<div class="ws-mono flex items-center gap-2 text-[11px] text-foreground">
										<SquareTerminal class="size-3.5 text-primary" />
										<span>{t('landing.workspace.terminal.layout_label')}:</span>
										<span class="text-muted-foreground">
											{t('landing.workspace.terminal.layout_value')}
										</span>
									</div>

									<div class="flex flex-wrap items-center gap-1">
										{#each TERMINAL_LAYOUTS as mode (mode)}
											<button
												type="button"
												aria-pressed={terminalLayout === mode}
												onclick={() => (terminalLayout = mode)}
												class="ws-mono rounded px-1.5 py-0.5 text-[10px] transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {terminalLayout ===
												mode
													? 'bg-primary text-primary-foreground'
													: 'bg-muted/60 text-muted-foreground hover:text-foreground'}"
											>
												{mode}
											</button>
										{/each}
									</div>
								</div>

								<div
									class="grid gap-2 p-2.5 {terminalLayout === 'single'
										? 'grid-cols-1'
										: terminalLayout === 'split-h'
											? 'grid-cols-1'
											: 'grid-cols-1 sm:grid-cols-2'}"
								>
									{#each visiblePanes as pane (pane.id)}
										<div
											class="min-w-0 rounded-lg border border-border/60 bg-card/80 p-3 text-card-foreground"
										>
											<div
												class="mb-2 flex items-center justify-between gap-2 border-b border-border/50 pb-1.5"
											>
												<span class="ws-mono truncate text-[10px] font-semibold text-foreground">
													{t(pane.titleKey)}
												</span>
												<span class="size-1.5 shrink-0 rounded-full bg-primary"></span>
											</div>
											<div data-lenis-prevent class="overflow-x-auto">
												<div class="ws-mono text-[11px] leading-relaxed text-foreground">
													<span class="text-primary">$</span>
													{t(pane.cmdKey)}
												</div>
												<div class="ws-mono mt-1 text-[11px] leading-relaxed text-muted-foreground">
													{t(pane.outKey)}
												</div>
											</div>
										</div>
									{/each}
								</div>
							</div>
						</div>
					{:else}
						<!-- Rich Chat & Interactive extra_messages Mini-Playground -->
						<div class="grid gap-6 lg:grid-cols-[0.88fr_1.12fr] lg:items-start">
							<div class="min-w-0 space-y-4">
								<span
									class="ws-mono inline-flex rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px] text-primary"
								>
									{t('landing.workspace.rich.badge')}
								</span>
								<h3 class="text-lg font-semibold tracking-tight text-foreground sm:text-xl">
									{t('landing.workspace.rich.title')}
								</h3>
								<p class="text-sm leading-relaxed text-muted-foreground">
									{t('landing.workspace.rich.body')}
								</p>

								<div class="rounded-xl border border-border/60 bg-muted/30 p-3.5 space-y-1.5">
									<div class="ws-mono text-[10px] tracking-wider text-primary uppercase">
										{t('landing.workspace.rich.blocks_label')}
									</div>
									<p class="ws-mono text-[11px] leading-relaxed text-foreground/85">
										{t('landing.workspace.rich.blocks_list')}
									</p>
								</div>

								<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
									<span
										class="ws-mono rounded border border-border/70 bg-muted/40 px-2 py-0.5 text-[10px]"
									>
										{t('landing.workspace.rich.markdown_note')}
									</span>
								</div>
							</div>

							<!-- Interactive extra_messages Card -->
							<div
								class="min-w-0 space-y-3 rounded-xl border border-border/70 bg-background/60 p-4"
							>
								<!-- Callout Block -->
								<div class="rounded-lg border-l-2 border-primary bg-muted/40 px-3.5 py-2.5">
									<div class="text-xs font-semibold text-foreground">
										{t('landing.workspace.rich.demo_callout_title')}
									</div>
									<p class="mt-0.5 text-xs text-muted-foreground">
										{t('landing.workspace.rich.demo_callout_body')}
									</p>
								</div>

								<!-- Interactive Checklist Block -->
								<div class="rounded-lg border border-border/60 bg-card/80 p-3.5">
									<div
										class="ws-mono mb-2 text-[10px] tracking-wider text-muted-foreground uppercase"
									>
										{t('landing.workspace.rich.demo_checklist_title')}
									</div>
									<div class="space-y-1.5">
										{#each [0, 1, 2] as idx (idx)}
											{@const checked = checks[idx as 0 | 1 | 2]}
											<button
												type="button"
												disabled={Boolean(lockedChoice)}
												aria-pressed={checked}
												onclick={() => toggleCheck(idx as 0 | 1 | 2)}
												class="flex w-full items-start gap-2.5 rounded-md px-2 py-1.5 text-left text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 {lockedChoice
													? 'cursor-not-allowed opacity-75'
													: 'hover:bg-muted/50'}"
											>
												<span
													class="mt-0.5 inline-flex size-4 shrink-0 items-center justify-center rounded border transition-colors {checked
														? 'border-primary bg-primary text-primary-foreground'
														: 'border-border bg-background'}"
												>
													{#if checked}
														<Check class="size-2.5" />
													{/if}
												</span>
												<span
													class="min-w-0 leading-snug {checked
														? 'text-foreground'
														: 'text-muted-foreground'}"
												>
													{t(`landing.workspace.rich.demo_check_${idx + 1}`)}
												</span>
											</button>
										{/each}
									</div>
								</div>

								<!-- Single-Use Action Buttons / Locked State -->
								{#if lockedChoice}
									<div
										bind:this={lockBannerEl}
										class="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3.5 py-2.5"
									>
										<div class="flex min-w-0 items-center gap-2 text-xs text-foreground">
											<CheckCircle2 class="size-4 shrink-0 text-primary" />
											<span class="font-medium">
												{lockedChoice === 'approve'
													? t('landing.workspace.rich.demo_btn_approve')
													: t('landing.workspace.rich.demo_btn_revise')}
											</span>
											<span class="text-border">·</span>
											<span class="inline-flex items-center gap-1 text-muted-foreground">
												<Lock class="size-3 shrink-0" />
												{t('landing.workspace.rich.demo_locked')}
											</span>
										</div>

										<button
											type="button"
											onclick={resetPlayground}
											aria-label={t('landing.stream.replay')}
											use:motionButton={{ scale: 0.94, hoverScale: 1.04 }}
											class="inline-flex items-center gap-1 rounded border border-border/70 bg-card px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
										>
											<RotateCcw class="size-3" />
										</button>
									</div>
								{:else}
									<div class="flex flex-wrap items-center gap-2">
										<button
											type="button"
											onclick={() => triggerAction('approve')}
											use:motionButton={{ scale: 0.96, hoverScale: 1.02 }}
											class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
										>
											{t('landing.workspace.rich.demo_btn_approve')}
										</button>
										<button
											type="button"
											onclick={() => triggerAction('revise')}
											use:motionButton={{ scale: 0.96, hoverScale: 1.02 }}
											class="inline-flex items-center gap-1.5 rounded-lg border border-border/70 bg-muted/40 px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
										>
											{t('landing.workspace.rich.demo_btn_revise')}
										</button>
									</div>
								{/if}

								<p class="flex items-center gap-1.5 text-[11px] text-muted-foreground">
									<Lock class="size-3 shrink-0 text-primary/70" />
									<span>{t('landing.workspace.rich.lock_note')}</span>
								</p>
							</div>
						</div>
					{/if}
				</div>
			{/key}
		</div>
	</div>
</section>

<style>
	.ws-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.ws-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}
</style>
