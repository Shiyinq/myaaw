<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { chat as chatApi } from '$lib/apis/chat';
	import { config as configApi } from '$lib/apis/config';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { agentSelection, toastStore } from '$lib/stores';
	import { cn } from '$lib/utils';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { askConfirm } from '$lib/stores/confirm.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import ChatStep from '$lib/components/common/ChatStep.svelte';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import {
		AlertTriangle,
		ArrowDownToLine,
		Brain,
		Check,
		ChevronDown,
		Clock,
		Code,
		Cpu,
		Database,
		FolderOpen,
		Gauge,
		Globe,
		ImagePlus,
		Loader2,
		MessageSquarePlus,
		Minimize2,
		Monitor,
		PanelRightClose,
		PanelRightOpen,
		Pencil,
		RotateCcw,
		Search,
		Send,
		ShieldAlert,
		Square,
		SquareTerminal,
		Trash2,
		Users,
		Wrench,
		X
	} from 'lucide-svelte';
	import type {
		ApprovalDecision,
		ApprovalPrompt,
		Attachment,
		ChatStreamEvent,
		ConversationSummary,
		Message,
		TimelineItem,
		Usage
	} from '$lib/types';

	const { t } = useTranslation();

	interface Item extends TimelineItem {
		localId?: string;
		resolved?: string;
		images?: string[];
		running?: boolean;
		expanded?: boolean;
		group_duration_ms?: number;
	}

	interface StepGroup {
		key: string;
		steps: Item[];
		running: boolean;
		durationMs?: number;
		/** The whole run as plain text, for the group's Copy button. */
		transcript?: string;
	}

	type TimelineRow = { kind: 'item'; item: Item } | { kind: 'group'; group: StepGroup };

	const STEP_KINDS = new Set(['thought', 'tool', 'retry', 'compaction']);

	interface PendingImage {
		id: string;
		name: string;
		dataUrl: string;
		base64: string;
	}

	const MAX_IMAGES = 6;
	const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
	const MAX_IMAGE_EDGE = 1600;
	// Raster formats only: the browser re-encodes them to JPEG, and SVG/GIF
	// animations would not survive that, so they are rejected up front.
	const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

	let agent = $derived(agentSelection.selected);
	let conversationId = $state('');
	let items = $state<Item[]>([]);
	let input = $state('');
	let streaming = $state(false);
	let loading = $state(false);
	let pendingApproval = $state<ApprovalPrompt | null>(null);
	let usage = $state<Usage | null>(null);
	// Totals reported by the observability session, the source of truth for
	// turns, tool steps and tokens.
	let sessionStats = $state<{
		turns: number;
		steps: number;
		tokens: number;
		model?: string;
		provider?: string;
		tokPerSec?: number;
		cacheRate?: number;
	} | null>(null);
	// Configured provider and its default model, as the terminal status line shows.
	let providerInfo = $state<{
		default: string;
		providers: { name: string; type: string; model: string }[];
	} | null>(null);
	let attachments = $state<Attachment[]>([]);
	let pending = $state<PendingImage[]>([]);
	let dragOver = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);
	let groupOpen = $state<Record<string, boolean>>({});
	let error = $state('');
	let scroller = $state<HTMLDivElement | null>(null);
	let composer = $state<HTMLTextAreaElement | null>(null);

	let controller: AbortController | null = null;
	let counter = 0;
	// Wall clock for the step group that is currently streaming, so its header
	// can report the duration once the group closes.
	let groupStart = 0;
	let groupLastStep = '';
	// Key of the group that is still collecting steps, so it stays open and
	// spinning between tool calls instead of folding away mid-run.
	let openGroup = $state('');
	let loadKey = '';

	// Conversation history sidebar
	const HISTORY_LIMIT = 20;
	const HISTORY_KEY = 'myaaw_chat_history_open';
	const LAST_CONVERSATION_KEY = 'myaaw_last_conversation';

	let historyOpen = $state(
		browser
			? (localStorage.getItem(HISTORY_KEY) ?? String(window.innerWidth >= 1024)) === 'true'
			: false
	);
	let history = $state<ConversationSummary[]>([]);
	let historyOffset = $state(0);
	let historyHasMore = $state(false);
	let historyLoading = $state(false);
	let historySearch = $state('');
	let historyQuery = $state('');
	let historyKey = '';
	let chatAgentKey = '';
	let editingId = $state('');
	let editingTitle = $state('');

	function scrollToBottom() {
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	// Follow only new timeline rows. Expanding a step reassigns `items` with the
	// same length, and that must not yank the view to the bottom.
	let followedCount = 0;
	$effect(() => {
		if (items.length === followedCount) return;
		followedCount = items.length;
		scrollToBottom();
	});

	async function loadConversation(currentAgent: string, id: string) {
		if (!id) {
			items = [];
			usage = null;
			sessionStats = null;
			attachments = [];
			return;
		}
		loading = true;
		error = '';
		sessionStats = null;
		groupOpen = {};
		groupStart = 0;
		groupLastStep = '';
		openGroup = '';
		void refreshStats(currentAgent, id);
		try {
			const detail = await conversationsApi.get(currentAgent, id, { limit: 0 });
			items = historyTimeline(detail.messages);
			usage = null;
			attachments = [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
			scrollToBottom();
		}
	}

	/**
	 * Session totals from observability: the same turns, tool steps and tokens
	 * the trace view reports. Conversations recorded before tracing was enabled
	 * have no runs, and keep the locally counted numbers.
	 */
	async function refreshStats(currentAgent: string, id: string) {
		if (!currentAgent || !id) return;
		try {
			const detail = await observabilityApi.sessionStats(currentAgent, id);
			if (conversationId !== id) return; // Another conversation won.
			// Cache hits are reported inside the prompt tokens, never above them.
			const cacheRate =
				detail.input_tokens > 0
					? Math.min(100, Math.round((detail.cache_hit_tokens / detail.input_tokens) * 100))
					: 0;
			sessionStats = {
				turns: detail.turns,
				steps: detail.steps,
				tokens: detail.input_tokens + detail.output_tokens,
				model: detail.model ?? '',
				provider: detail.provider ?? '',
				tokPerSec: detail.tok_per_sec ?? 0,
				cacheRate
			};
		} catch {
			// No runs recorded for this conversation; keep the local counts.
		}
	}

	/** The agent's configured provider and its default model. */
	async function loadProviderInfo(currentAgent: string) {
		try {
			const config = await configApi.get(currentAgent);
			providerInfo = {
				default: config.default_provider ?? '',
				providers: (config.providers ?? []).map((provider) => ({
					name: provider.name,
					type: provider.type ?? '',
					model: provider.default_model ?? ''
				}))
			};
		} catch {
			providerInfo = null;
		}
	}

	let providerKey = '';
	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent || currentAgent === providerKey) return;
		providerKey = currentAgent;
		providerInfo = null;
		void loadProviderInfo(currentAgent);
	});

	function readLastConversations(): Record<string, string> {
		if (!browser) return {};
		try {
			const raw = localStorage.getItem(LAST_CONVERSATION_KEY);
			if (!raw) return {};
			const parsed: unknown = JSON.parse(raw);
			return parsed && typeof parsed === 'object' ? (parsed as Record<string, string>) : {};
		} catch {
			return {};
		}
	}

	function rememberConversation(id: string) {
		if (!browser || !agent) return;
		const map = readLastConversations();
		if (id) map[agent] = id;
		else delete map[agent];
		localStorage.setItem(LAST_CONVERSATION_KEY, JSON.stringify(map));
	}

	function lastConversationFor(agentName: string) {
		return readLastConversations()[agentName] ?? '';
	}

	// Switching the agent must not keep showing the previous agent's transcript.
	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent) return;
		const previous = chatAgentKey;
		chatAgentKey = currentAgent;
		if (!previous || previous === currentAgent) return;

		const openConversation = page.params.conversation ?? '';
		// Do not load a conversation that belongs to the previous agent.
		if (openConversation) loadKey = `${currentAgent}:${openConversation}`;
		conversationId = '';
		items = [];
		usage = null;
		sessionStats = null;
		attachments = [];
		error = '';
		pendingApproval = null;
		if (page.url.pathname !== '/app/chat') {
			void goto('/app/chat', { replaceState: true, noScroll: true });
		}
	});

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const currentConversation = page.params.conversation ?? '';
		const key = `${currentAgent}:${currentConversation}`;
		if (!currentAgent || key === loadKey) return;

		// The dashboard's "new chat" action opts out of restoring history.
		if (!currentConversation && page.url.searchParams.has('new')) {
			resetChat();
			// Drop the flag so a refresh keeps the empty composer instead of
			// clearing it again.
			void goto('/app/chat', { replaceState: true, noScroll: true });
			return;
		}

		// The sidebar links to /app/chat, so restore the last opened conversation
		// instead of dropping the user into a brand new chat.
		if (!currentConversation) {
			const last = lastConversationFor(currentAgent);
			if (last) {
				void goto(`/app/chat/${last}`, { replaceState: true, noScroll: true });
				return;
			}
		}

		loadKey = key;
		conversationId = currentConversation;
		rememberConversation(currentConversation);
		void loadConversation(currentAgent, currentConversation);
	});

	// Everything between the user's message and the final answer forms one
	// outer collapsible run. Assistant text in between stays plain text, only
	// the last answer of each run is shown outside the group.
	let rendered: TimelineRow[] = $derived.by(() => {
		const answers = new Set<number>();
		let pending = -1;
		items.forEach((item, index) => {
			if (item.kind === 'user') {
				if (pending >= 0) answers.add(pending);
				pending = -1;
			} else if (item.kind === 'text') {
				pending = index;
			}
		});
		if (pending >= 0) answers.add(pending);

		const rows: TimelineRow[] = [];
		let steps: Item[] = [];
		let lines: string[] = [];
		const flush = () => {
			if (steps.length > 0) {
				const first = steps[0];
				rows.push({
					kind: 'group',
					group: {
						key: `run-${first.localId ?? rows.length}`,
						steps: [...steps],
						running:
							steps.some((step) => step.running) ||
							steps.some((step) => step.localId === openGroup),
						durationMs: steps.find((step) => step.group_duration_ms !== undefined)
							?.group_duration_ms,
						transcript: lines.join('\n\n')
					}
				});
			}
			steps = [];
			lines = [];
		};
		items.forEach((item, index) => {
			const processText = item.kind === 'text' && !answers.has(index);
			if (STEP_KINDS.has(item.kind) || processText) {
				steps.push(item);
				const line = itemTranscript(item);
				if (line) lines.push(line);
			} else {
				if (item.kind === 'text' && answers.has(index)) {
					// The run's answer belongs to its transcript, even though it is
					// shown outside the collapsible group.
					const answer = (item.text ?? '').trim();
					if (answer) lines.push(answer);
				}
				flush();
				rows.push({ kind: 'item', item });
			}
		});
		flush();
		return rows;
	});

	function isGroupOpen(group: StepGroup) {
		return groupOpen[group.key] ?? group.running;
	}

	function groupLabel(group: StepGroup) {
		const kinds = new Set(group.steps.map((step) => step.kind));
		const thought = kinds.has('thought');
		const tool = kinds.has('tool');
		if (thought && tool) return t('myaaw.chat.tools_and_thinking');
		if (tool) return t('myaaw.chat.tools');
		if (thought) return t('myaaw.chat.thinking');
		return t('myaaw.chat.process');
	}

	type PhaseRow = { kind: 'text'; item: Item } | { kind: 'group'; group: StepGroup };

	/** Splits a run into phases: thinking/tool groups with the remarks in between. */
	function phaseRows(group: StepGroup): PhaseRow[] {
		const rows: PhaseRow[] = [];
		let steps: Item[] = [];
		const flush = () => {
			if (steps.length === 0) return;
			const first = steps[0];
			rows.push({
				kind: 'group',
				group: {
					key: `phase-${first.localId ?? rows.length}`,
					steps: [...steps],
					running:
						steps.some((step) => step.running) || steps.some((step) => step.localId === openGroup),
					durationMs: steps.find((step) => step.group_duration_ms !== undefined)?.group_duration_ms
				}
			});
			steps = [];
		};
		for (const item of group.steps) {
			if (item.kind === 'text') {
				flush();
				rows.push({ kind: 'text', item });
			} else {
				steps.push(item);
			}
		}
		flush();
		return rows;
	}

	function toggleGroup(group: StepGroup) {
		groupOpen = { ...groupOpen, [group.key]: !isGroupOpen(group) };
	}

	/** Consecutive thinking/tool rows are rendered as one collapsible group. */
	function stepRow(item: TimelineItem) {
		return STEP_KINDS.has(item.kind);
	}

	function formatDuration(ms: number) {
		const seconds = ms / 1000;
		if (seconds < 60) return `${Math.max(1, Math.round(seconds))}s`;
		const minutes = Math.floor(seconds / 60);
		return `${minutes}m ${Math.round(seconds % 60)}s`;
	}

	let turnCount = $derived(items.filter((item) => item.kind === 'user').length);
	let stepCount = $derived(items.filter((item) => item.kind === 'tool').length);
	let statsTurns = $derived(sessionStats?.turns ?? turnCount);
	let statsSteps = $derived(sessionStats?.steps ?? stepCount);
	let statsTokens = $derived(sessionStats?.tokens ?? usage?.total_tokens ?? 0);
	let statsTokPerSec = $derived(sessionStats?.tokPerSec ?? 0);
	let statsCacheRate = $derived(sessionStats?.cacheRate ?? 0);
	let statsProvider = $derived.by(() => {
		const model = sessionStats?.model ?? '';
		const providers = providerInfo?.providers ?? [];
		if (providers.length === 0) return sessionStats?.provider ?? '';
		// The newest run names the model, so a mid-chat switch follows it.
		const byModel = model ? providers.find((provider) => provider.model === model) : undefined;
		if (byModel) return byModel.name;
		// A model that is not a provider default still points at one provider type.
		const type = sessionStats?.provider ?? '';
		const byType = type ? providers.filter((provider) => provider.type === type) : [];
		if (byType.length === 1) return byType[0].name;
		return providerInfo?.default || byType[0]?.name || '';
	});
	let statsModel = $derived(
		sessionStats?.model ||
			providerInfo?.providers.find((provider) => provider.name === providerInfo?.default)?.model ||
			''
	);
	let statsModelLine = $derived(
		[
			statsTokens > 0 ? t('myaaw.chat.stats_tokens', { count: formatTokens(statsTokens) }) : '',
			statsCacheRate > 0 ? t('myaaw.chat.stats_cache_hit', { percent: statsCacheRate }) : '',
			statsProvider,
			statsModel
		]
			.filter(Boolean)
			.join(' • ')
	);

	function formatTokens(count: number) {
		if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
		if (count >= 1000) return `${Math.round(count / 1000)}K`;
		return String(count);
	}

	function turnLabel(count: number) {
		return count === 1 ? t('myaaw.chat.turn_one') : t('myaaw.chat.turns', { count });
	}

	function stepsLabel(count: number) {
		return count === 1 ? t('myaaw.chat.step_one') : t('myaaw.chat.steps', { count });
	}

	/** Records how long the open step group took on its last step. */
	function closeGroup() {
		if (!groupStart || !groupLastStep) return;
		const duration = Date.now() - groupStart;
		const last = groupLastStep;
		groupStart = 0;
		groupLastStep = '';
		openGroup = '';
		items = items.map((item) =>
			item.localId === last ? { ...item, group_duration_ms: duration } : item
		);
	}

	function push(item: Item) {
		const localId = item.localId ?? `item-${++counter}`;
		if (stepRow(item)) {
			if (!groupStart) {
				groupStart = Date.now();
				openGroup = localId;
			}
			groupLastStep = localId;
		} else if (item.kind !== 'text') {
			// Text belongs to the run: the model often narrates between tools.
			closeGroup();
		}
		items = [...items, { ...item, localId }];
	}

	/** A thinking block closes as soon as the model stops thinking. */
	function finishThoughts() {
		items = items.map((item) =>
			item.kind === 'thought' && item.running ? { ...item, running: false, expanded: false } : item
		);
	}

	function mergeLast(kind: string, text: string) {
		const last = items[items.length - 1];
		if (last && last.kind === kind && (kind !== 'thought' || last.running)) {
			items = [...items.slice(0, -1), { ...last, text: (last.text ?? '') + text }];
		} else if (kind === 'thought') {
			// Streamed thinking starts open and folds away when it ends.
			push({ kind, text, running: true, expanded: true });
		} else {
			push({ kind, text });
		}
	}

	function setToolResult(id: string, result: string, isError?: boolean) {
		const index = items.findIndex((item) => item.kind === 'tool' && item.id === id);
		if (index < 0) {
			// A result without its call still deserves a block.
			push({ kind: 'tool', id, result, is_error: isError, running: false, expanded: false });
			return;
		}
		items = items.map((item, position) =>
			position === index
				? { ...item, result, is_error: isError, running: false, expanded: false }
				: item
		);
	}

	function toggleItem(localId?: string) {
		if (!localId) return;
		items = items.map((item) =>
			item.localId === localId ? { ...item, expanded: !item.expanded } : item
		);
	}

	/** Folds an interrupted or finished run so no step keeps spinning. */
	function endRun() {
		finishThoughts();
		items = items.map((item) => (item.running ? { ...item, running: false } : item));
		closeGroup();
	}

	async function send() {
		const text = input.trim();
		const images = pending.map((item) => item.base64);
		if ((!text && images.length === 0) || !agent || streaming) return;

		push({ kind: 'user', text, images: pending.map((item) => item.dataUrl) });
		input = '';
		clearPending();
		groupStart = 0;
		groupLastStep = '';
		openGroup = '';
		if (composer) composer.style.height = '';
		streaming = true;
		error = '';
		pendingApproval = null;
		usage = null;
		attachments = [];
		controller = new AbortController();

		try {
			await chatApi.stream(
				agent,
				{
					text,
					conversation_id: conversationId || undefined,
					images: images.length > 0 ? images : undefined
				},
				(event) => {
					const data = event.data as Record<string, any>;
					switch (event.event as ChatStreamEvent) {
						case 'thought':
							mergeLast('thought', data?.text ?? '');
							break;
						case 'chunk':
							finishThoughts();
							mergeLast('text', data?.text ?? '');
							break;
						case 'tool':
							finishThoughts();
							push({
								kind: 'tool',
								id: data?.id,
								name: data?.name,
								arguments: data?.arguments,
								running: true,
								expanded: true
							});
							break;
						case 'tool_result':
							setToolResult(data?.id, data?.result ?? '', data?.is_error);
							break;
						case 'retry':
							push({ kind: 'retry', attempt: data?.attempt });
							break;
						case 'compaction':
							push({
								kind: 'compaction',
								tokens_before: data?.tokens_before,
								tokens_after: data?.tokens_after,
								dropped_messages: data?.dropped_messages
							});
							break;
						case 'approval_required': {
							const prompt: ApprovalPrompt = {
								approval_id: data?.approval_id,
								tool_name: data?.tool_name,
								action: data?.action,
								args_summary: data?.args_summary,
								subagent: data?.subagent
							};
							pendingApproval = prompt;
							push({
								kind: 'approval',
								id: prompt.approval_id,
								name: prompt.tool_name,
								arguments: prompt.args_summary,
								subagent: prompt.subagent
							});
							break;
						}
						case 'approval_resolved': {
							pendingApproval = null;
							items = items.map((item) =>
								item.kind === 'approval' && item.id === data?.approval_id
									? { ...item, resolved: data?.decision ?? data?.reason ?? 'resolved' }
									: item
							);
							break;
						}
						case 'done': {
							endRun();
							const nextConversation = data?.conversation_id;
							if (nextConversation && nextConversation !== conversationId) {
								conversationId = nextConversation;
								loadKey = `${agent}:${nextConversation}`;
								rememberConversation(nextConversation);
								void loadHistory(true);
								void goto(`/app/chat/${nextConversation}`, {
									replaceState: true,
									noScroll: true
								});
							}
							if (data?.text) {
								const hasText = [...items].reverse().some((item) => item.kind === 'text');
								if (!hasText) push({ kind: 'text', text: data.text });
							}
							usage = data?.usage ?? null;
							attachments = data?.attachments ?? [];
							const statsId = conversationId;
							void refreshStats(agent, statsId);
							// The run may land in the trace store just after `done`.
							setTimeout(() => void refreshStats(agent, statsId), 1500);
							break;
						}
						case 'error': {
							endRun();
							error = data?.error ?? 'Stream error';
							push({ kind: 'error', text: error });
							break;
						}
					}
				},
				controller.signal
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				error = getErrorMessage(err);
				toastStore.error(error);
			}
		} finally {
			endRun();
			streaming = false;
			controller = null;
		}
	}

	async function decide(decision: ApprovalDecision) {
		const prompt = pendingApproval;
		if (!prompt || !agent) return;
		pendingApproval = null;
		try {
			await chatApi.decide(agent, prompt.approval_id, decision);
			items = items.map((item) =>
				item.kind === 'approval' && item.id === prompt.approval_id
					? { ...item, resolved: decision }
					: item
			);
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		}
	}

	function stop() {
		controller?.abort();
		controller = null;
		streaming = false;
	}

	function newChat() {
		resetChat();
		if (page.url.pathname !== '/app/chat') {
			void goto('/app/chat');
		}
	}

	function resetChat() {
		controller?.abort();
		controller = null;
		streaming = false;
		conversationId = '';
		items = [];
		input = '';
		if (composer) composer.style.height = '';
		usage = null;
		sessionStats = null;
		attachments = [];
		error = '';
		pendingApproval = null;
		loadKey = `${agent}:`;
		rememberConversation('');
		groupOpen = {};
		groupStart = 0;
		groupLastStep = '';
		openGroup = '';
	}

	function toggleHistory() {
		historyOpen = !historyOpen;
		if (browser) localStorage.setItem(HISTORY_KEY, String(historyOpen));
	}

	async function loadHistory(reset = false) {
		if (!agent || historyLoading) return;
		historyLoading = true;
		try {
			const offset = reset ? 0 : historyOffset;
			const result = await conversationsApi.list(agent, {
				search: historyQuery || undefined,
				limit: HISTORY_LIMIT,
				offset
			});
			const pageItems = result.conversations ?? [];
			if (reset) {
				history = pageItems;
			} else {
				const known = new Set(history.map((item) => item.id));
				history = [...history, ...pageItems.filter((item) => !known.has(item.id))];
			}
			historyOffset = offset + pageItems.length;
			historyHasMore = pageItems.length > 0 && result.has_more;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			historyLoading = false;
		}
	}

	function loadMoreHistory() {
		if (!historyHasMore || historyLoading) return;
		void loadHistory(false);
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent) return;
		const key = `${currentAgent}:${historyQuery}`;
		if (historyKey === key) return;
		historyKey = key;
		history = [];
		historyOffset = 0;
		historyHasMore = false;
		void loadHistory(true);
	});

	// Debounce the search box so typing does not hit the API on every keystroke.
	$effect(() => {
		const value = historySearch.trim();
		if (value === historyQuery) return;
		const timer = setTimeout(() => (historyQuery = value), 300);
		return () => clearTimeout(timer);
	});

	function infinite(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) loadMoreHistory();
			},
			{ rootMargin: '160px' }
		);
		observer.observe(node);
		return { destroy: () => observer.disconnect() };
	}

	function startRename(item: ConversationSummary) {
		editingId = item.id;
		editingTitle = item.title;
	}

	async function saveRename() {
		if (!agent || !editingId) return;
		const title = editingTitle.trim();
		if (!title) return;
		try {
			await conversationsApi.rename(agent, editingId, title);
			history = history.map((item) => (item.id === editingId ? { ...item, title } : item));
			editingId = '';
			toastStore.success(t('myaaw.conversations.renamed'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	async function removeHistory(item: ConversationSummary) {
		if (!agent) return;
		if (
			!(await askConfirm({
				message: t('myaaw.conversations.delete_confirm'),
				variant: 'destructive'
			}))
		)
			return;
		try {
			await conversationsApi.remove(agent, item.id);
			history = history.filter((entry) => entry.id !== item.id);
			historyOffset = Math.max(0, historyOffset - 1);
			toastStore.success(t('myaaw.conversations.deleted'));
			if (item.id === conversationId) {
				resetChat();
				await goto('/app/chat');
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	function openConversation(id: string) {
		if (browser && window.innerWidth < 1024) toggleHistory();
		if (id !== conversationId) void goto(`/app/chat/${id}`);
	}

	function formatWhen(value?: string) {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function messageText(message: Message): string {
		if (typeof message.content === 'string') return message.content;
		if (Array.isArray(message.content)) {
			return message.content
				.filter((part) => part.type === 'text')
				.map((part) => part.text ?? '')
				.filter(Boolean)
				.join('\n');
		}
		return '';
	}

	function messageImages(message: Message): string[] {
		const urls: string[] = [];
		if (Array.isArray(message.content)) {
			for (const part of message.content) {
				if (part.type === 'image_url' && part.image_url?.url) urls.push(part.image_url.url);
			}
		}
		for (const image of message.images ?? []) {
			urls.push(`data:image/jpeg;base64,${image}`);
		}
		return urls;
	}

	const TOOL_LABELS: Record<string, string> = {
		bash: 'Bash',
		execute_python: 'Python',
		filesystem: 'Files',
		computer: 'Computer',
		subagent: 'Agent',
		cron: 'Cron',
		provider: 'Provider',
		contact: 'Contact'
	};

	// Fields that read well as a one-line summary, most specific first.
	const SUMMARY_KEYS = [
		'command',
		'code',
		'instruction',
		'action',
		'path',
		'pattern',
		'query',
		'url',
		'text',
		'name'
	];

	function toolArgs(item: Item): Record<string, unknown> {
		if (!item.arguments) return {};
		try {
			const parsed: unknown = JSON.parse(item.arguments);
			return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
		} catch {
			return {};
		}
	}

	function toolLabel(name?: string) {
		const key = (name ?? '').toLowerCase();
		if (TOOL_LABELS[key]) return TOOL_LABELS[key];
		return name ? name.charAt(0).toUpperCase() + name.slice(1) : t('myaaw.chat.tool');
	}

	function toolIcon(name?: string) {
		const key = (name ?? '').toLowerCase();
		if (key === 'bash') return SquareTerminal;
		if (key === 'execute_python') return Code;
		if (key === 'filesystem') return FolderOpen;
		if (key === 'computer') return Monitor;
		if (key === 'subagent') return Users;
		if (key === 'cron') return Clock;
		if (key === 'provider') return Cpu;
		if (/scrap|search|crawl|web|fetch|a2a/.test(key)) return Globe;
		return Wrench;
	}

	function oneLine(text: string) {
		return text.replace(/\s+/g, ' ').trim();
	}

	/** "list_directory /path", the shape the filesystem tool arguments take. */
	function filesystemAction(args: Record<string, unknown>) {
		if (typeof args.tool_name !== 'string') return '';
		const target = ['path', 'old_path', 'new_path', 'pattern']
			.map((key) => args[key])
			.find((value) => typeof value === 'string' && value.trim());
		return oneLine([args.tool_name, target].filter((value) => typeof value === 'string').join(' '));
	}

	/** The invocation shown in the dark panel, without the prompt prefix. */
	function toolInvocation(item: Item) {
		const args = toolArgs(item);
		if (typeof args.command === 'string') return args.command;
		if (typeof args.code === 'string') return args.code;
		if (item.name === 'filesystem') {
			const action = filesystemAction(args);
			if (action) return action;
		}
		if (item.arguments) {
			try {
				return JSON.stringify(JSON.parse(item.arguments), null, 2);
			} catch {
				return item.arguments;
			}
		}
		return item.name ?? '';
	}

	function toolSummary(item: Item) {
		const args = toolArgs(item);
		const keys = Object.keys(args);
		if (keys.length === 0) {
			if (item.result) return oneLine(item.result);
			return item.running ? t('myaaw.chat.tool_running') : '';
		}
		if (item.name === 'filesystem') {
			const action = filesystemAction(args);
			if (action) return action;
		}
		for (const key of SUMMARY_KEYS) {
			const value = args[key];
			if (typeof value === 'string' && value.trim()) return oneLine(value);
		}
		return oneLine(item.arguments ?? '');
	}

	/** One step of a run rendered as plain text, for the run's Copy button. */
	function itemTranscript(item: Item): string {
		if (item.kind === 'thought') {
			const text = (item.text ?? '').trim();
			return text ? `${t('myaaw.chat.think')}:\n${text}` : '';
		}
		if (item.kind === 'text') return (item.text ?? '').trim();
		if (item.kind === 'tool') {
			const invocation = toolInvocation(item).trim();
			const head = invocation ? `${toolLabel(item.name)}: ${invocation}` : toolLabel(item.name);
			return item.result ? `${head}\n${item.result.trim()}` : head;
		}
		if (item.kind === 'retry') return t('myaaw.chat.retry', { attempt: item.attempt ?? 2 });
		if (item.kind === 'compaction') {
			return t('myaaw.chat.compaction', {
				before: item.tokens_before ?? 0,
				after: item.tokens_after ?? 0
			});
		}
		return '';
	}

	/**
	 * Rebuilds the timeline from stored provider messages so reopening a chat
	 * still shows thoughts, tool calls and their results, retries and
	 * compactions. Mirrors the TUI's convertProviderMessages.
	 */
	function historyTimeline(messages: Message[]): Item[] {
		const timeline: Item[] = [];
		const tools = new Map<string, number>();
		let index = 0;
		const nextId = () => `history-${index++}`;

		for (const message of messages) {
			if (message.thought) {
				timeline.push({
					kind: 'thought',
					text: message.thought,
					running: false,
					expanded: false,
					localId: nextId()
				});
			}

			if (message.role === 'tool') {
				const result = messageText(message);
				const position = message.tool_call_id ? tools.get(message.tool_call_id) : undefined;
				if (position !== undefined) {
					timeline[position] = {
						...timeline[position],
						result,
						is_error: message.is_error,
						running: false,
						expanded: false
					};
				} else {
					timeline.push({
						kind: 'tool',
						id: message.tool_call_id,
						name: message.name,
						result,
						is_error: message.is_error,
						running: false,
						expanded: false,
						localId: nextId()
					});
				}
				continue;
			}

			if (message.role === 'user') {
				const text = messageText(message);
				const images = messageImages(message);
				if (text || images.length > 0) {
					timeline.push({
						kind: 'user',
						text,
						images: images.length > 0 ? images : undefined,
						localId: nextId()
					});
				}
				continue;
			}

			if (message.role !== 'assistant') continue;

			const text = messageText(message);
			if (text) timeline.push({ kind: 'text', text, localId: nextId() });

			for (const call of message.tool_calls ?? []) {
				const id = call.id ?? '';
				const args = call.function?.arguments;
				timeline.push({
					kind: 'tool',
					id,
					name: call.function?.name,
					arguments: typeof args === 'string' ? args : args ? JSON.stringify(args) : undefined,
					running: false,
					expanded: false,
					localId: nextId()
				});
				if (id) tools.set(id, timeline.length - 1);
			}
		}

		return timeline;
	}

	function readAsDataUrl(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(String(reader.result));
			reader.onerror = () => reject(reader.error ?? new Error('read failed'));
			reader.readAsDataURL(file);
		});
	}

	/** Downscale and re-encode to JPEG so four screenshots still fit the request limit. */
	async function prepareImage(file: File): Promise<string> {
		const dataUrl = await readAsDataUrl(file);
		const image = new Image();
		await new Promise<void>((resolve, reject) => {
			image.onload = () => resolve();
			image.onerror = () => reject(new Error('decode failed'));
			image.src = dataUrl;
		});
		if (image.naturalWidth === 0 || image.naturalHeight === 0) {
			throw new Error('empty image');
		}

		const scale = Math.min(1, MAX_IMAGE_EDGE / Math.max(image.width, image.height));
		const canvas = document.createElement('canvas');
		canvas.width = Math.max(1, Math.round(image.width * scale));
		canvas.height = Math.max(1, Math.round(image.height * scale));
		const context = canvas.getContext('2d');
		if (!context) return dataUrl;
		// JPEG has no alpha channel; a white backdrop keeps transparent PNGs readable.
		context.fillStyle = '#ffffff';
		context.fillRect(0, 0, canvas.width, canvas.height);
		context.drawImage(image, 0, 0, canvas.width, canvas.height);
		return canvas.toDataURL('image/jpeg', 0.85);
	}

	async function addFiles(files: FileList | File[] | null | undefined) {
		if (!files) return;
		const candidates = Array.from(files);
		const images = candidates.filter((file) => IMAGE_TYPES.has(file.type));
		if (images.length !== candidates.length) {
			toastStore.error(t('myaaw.chat.image_only'));
		}
		if (images.length === 0) return;

		const room = MAX_IMAGES - pending.length;
		if (room <= 0) {
			toastStore.error(t('myaaw.chat.image_limit', { count: MAX_IMAGES }));
			return;
		}
		if (images.length > room) {
			toastStore.error(t('myaaw.chat.image_limit', { count: MAX_IMAGES }));
		}

		const additions: PendingImage[] = [];
		for (const file of images.slice(0, room)) {
			if (file.size > MAX_IMAGE_BYTES) {
				toastStore.error(t('myaaw.chat.image_too_large', { size: 10 }));
				continue;
			}
			try {
				const dataUrl = await prepareImage(file);
				const base64 = dataUrl.split(',')[1] ?? '';
				if (!base64) continue;
				additions.push({
					id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
					name: file.name,
					dataUrl,
					base64
				});
			} catch {
				toastStore.error(t('myaaw.chat.image_failed'));
			}
		}
		if (additions.length > 0) pending = [...pending, ...additions];
	}

	function clearPending() {
		pending = [];
		if (fileInput) fileInput.value = '';
	}

	function removePending(id: string) {
		pending = pending.filter((item) => item.id !== id);
	}

	function onFileInput(event: Event) {
		const element = event.currentTarget as HTMLInputElement;
		void addFiles(element.files);
		element.value = '';
	}

	function onPaste(event: ClipboardEvent) {
		const files = Array.from(event.clipboardData?.files ?? []);
		if (files.length === 0) return;
		if (files.some((file) => IMAGE_TYPES.has(file.type))) {
			event.preventDefault();
		}
		void addFiles(files);
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
			event.preventDefault();
			void send();
		}
	}

	function autosize(node: HTMLTextAreaElement) {
		const resize = () => {
			node.style.height = 'auto';
			node.style.height = `${Math.min(node.scrollHeight, 160)}px`;
		};
		resize();
		node.addEventListener('input', resize);
		return {
			destroy: () => node.removeEventListener('input', resize)
		};
	}

	function attachmentUrl(attachment: Attachment) {
		return `data:${attachment.mime_type || 'application/octet-stream'};base64,${attachment.data_base64}`;
	}

	function isImage(attachment: Attachment) {
		return (attachment.mime_type ?? '').startsWith('image/');
	}
</script>

<TopBarActions id="chat-actions">
	{#snippet children()}
		<TopBarButton label={t('myaaw.chat.new')} icon={MessageSquarePlus} onclick={newChat} />
		<Button
			variant="outline"
			size="icon-sm"
			onclick={toggleHistory}
			title={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
			aria-label={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
		>
			{#if historyOpen}
				<PanelRightClose class="h-4 w-4" />
			{:else}
				<PanelRightOpen class="h-4 w-4" />
			{/if}
		</Button>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0">
	<div class="flex min-w-0 flex-1 flex-col">
		{#if !agent}
			<div class="flex flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</div>
		{:else}
			<div bind:this={scroller} class="min-h-0 flex-1 overflow-y-auto">
				<div class="mx-auto w-full max-w-5xl space-y-5 px-4 py-6">
					{#if loading}
						<p class="text-sm text-muted-foreground">{t('myaaw.chat.loading')}</p>
					{:else if items.length === 0}
						<p class="py-20 text-center text-sm text-muted-foreground">
							{t('myaaw.chat.empty')}
						</p>
					{/if}

					{#each rendered as row (row.kind === 'group' ? row.group.key : (row.item.localId ?? row.item.kind))}
						{#if row.kind === 'group'}
							{@const group = row.group}
							{@const open = isGroupOpen(group)}
							<div class="space-y-0.5">
								<div class="group/run flex items-center gap-0.5">
									<button
										type="button"
										class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
										onclick={() => toggleGroup(group)}
										aria-expanded={open}
									>
										{#if group.running}
											<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
										{:else}
											<Clock class="h-3.5 w-3.5 shrink-0" />
										{/if}
										<span class="font-medium">
											{group.running ? t('myaaw.chat.tool_running') : t('myaaw.chat.completed')}
										</span>
										{#if !group.running && group.durationMs !== undefined}
											<span class="shrink-0 text-muted-foreground">
												· {formatDuration(group.durationMs)}
											</span>
										{/if}
										<ChevronDown
											class={cn('h-3.5 w-3.5 shrink-0 transition-transform', open && 'rotate-180')}
										/>
									</button>
									{#if group.transcript}
										<CopyButton
											text={group.transcript}
											class="opacity-0 transition-opacity group-hover/run:opacity-100 group-focus-within/run:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100"
										/>
									{/if}
								</div>
								{#if open}
									<div class="space-y-1 pl-2">
										{#each phaseRows(group) as phase, index (phase.kind === 'group' ? phase.group.key : (phase.item.localId ?? index))}
											{#if phase.kind === 'text'}
												<div class="px-1 text-sm">
													<Markdown content={phase.item.text ?? ''} />
												</div>
											{:else}
												{@const phaseGroup = phase.group}
												{@const phaseOpen = isGroupOpen(phaseGroup)}
												<div class="space-y-0.5">
													<button
														type="button"
														class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
														onclick={() => toggleGroup(phaseGroup)}
														aria-expanded={phaseOpen}
													>
														{#if phaseGroup.running}
															<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
														{/if}
														<span class="font-medium">{groupLabel(phaseGroup)}</span>
														<ChevronDown
															class={cn(
																'h-3.5 w-3.5 shrink-0 transition-transform',
																phaseOpen && 'rotate-180'
															)}
														/>
													</button>
													{#if phaseOpen}
														<div class="space-y-0.5 pl-2">
															{#each phaseGroup.steps as step (step.localId)}
																{#if step.kind === 'thought'}
																	<ChatStep
																		title={t('myaaw.chat.think')}
																		summary={oneLine(step.text ?? '')}
																		icon={Brain}
																		expanded={!!step.expanded}
																		ontoggle={() => toggleItem(step.localId)}
																		running={step.running}
																	>
																		<p class="whitespace-pre-wrap text-muted-foreground">
																			{step.text}
																		</p>
																	</ChatStep>
																{:else if step.kind === 'tool'}
																	<ChatStep
																		title={toolLabel(step.name)}
																		summary={toolSummary(step)}
																		icon={toolIcon(step.name)}
																		expanded={!!step.expanded}
																		ontoggle={() => toggleItem(step.localId)}
																		running={step.running}
																		error={step.is_error}
																		copyText={toolInvocation(step)}
																	>
																		<div
																			class="overflow-hidden rounded-lg bg-zinc-900 text-zinc-100 ring-1 ring-inset ring-white/10"
																		>
																			<pre
																				class="overflow-x-auto p-3 font-mono text-[11px] leading-relaxed"><span
																					class="select-none text-zinc-500"
																					>$ </span>{toolInvocation(step)}</pre>
																			{#if step.result}
																				<pre
																					class={cn(
																						'max-h-72 overflow-auto border-t border-white/10 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all',
																						step.is_error ? 'text-red-400' : 'text-zinc-300'
																					)}>{step.result}</pre>
																			{:else if step.running}
																				<pre
																					class="border-t border-white/10 p-3 font-mono text-[11px] text-zinc-500">{t(
																						'myaaw.chat.tool_running'
																					)}</pre>
																			{/if}
																		</div>
																	</ChatStep>
																{:else if step.kind === 'retry'}
																	<div
																		class="flex items-center gap-2 px-1 py-1 text-[11px] text-muted-foreground"
																	>
																		<RotateCcw class="h-3.5 w-3.5 shrink-0" />
																		{t('myaaw.chat.retry', { attempt: step.attempt ?? 2 })}
																	</div>
																{:else if step.kind === 'compaction'}
																	<div
																		class="flex items-center gap-2 px-1 py-1 text-[11px] text-muted-foreground"
																	>
																		<Minimize2 class="h-3.5 w-3.5 shrink-0" />
																		{t('myaaw.chat.compaction', {
																			before: step.tokens_before ?? 0,
																			after: step.tokens_after ?? 0
																		})}
																	</div>
																{/if}
															{/each}
														</div>
													{/if}
												</div>
											{/if}
										{/each}
									</div>
								{/if}
							</div>
						{:else}
							{@const item = row.item}
							{#if item.kind === 'user'}
								<div class="flex justify-end">
									<div class="flex max-w-[80%] flex-col items-end gap-2">
										{#if item.images?.length}
											<div class="flex flex-wrap justify-end gap-2">
												{#each item.images as image, imageIndex (imageIndex)}
													<img
														src={image}
														alt=""
														class="max-h-48 rounded-2xl border object-contain"
													/>
												{/each}
											</div>
										{/if}
										{#if item.text}
											<div class="flex flex-col items-end">
												<div
													class="whitespace-pre-wrap rounded-3xl bg-secondary px-4 py-2.5 text-sm text-secondary-foreground"
												>
													{item.text}
												</div>
												<CopyButton text={item.text} class="mt-0.5" />
											</div>
										{/if}
									</div>
								</div>
							{:else if item.kind === 'text'}
								<div class="text-sm">
									<Markdown content={item.text ?? ''} />
									<CopyButton text={item.text ?? ''} class="mt-1" />
								</div>
							{:else if item.kind === 'approval'}
								<div class="rounded-xl border border-amber-500/40 bg-amber-500/10 p-3 text-xs">
									<div
										class="flex items-center gap-2 font-medium text-amber-700 dark:text-amber-300"
									>
										<ShieldAlert class="h-4 w-4" />
										{t('myaaw.chat.waiting')}
									</div>
									<div class="mt-1 font-mono text-foreground">{item.name}</div>
									{#if item.arguments}
										<pre
											class="mt-1 overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{item.arguments}</pre>
									{/if}
									{#if item.resolved}
										<Badge variant="outline" class="mt-2">{item.resolved}</Badge>
									{:else if pendingApproval && pendingApproval.approval_id === item.id}
										<div class="mt-2 flex flex-wrap gap-2">
											<Button size="xs" onclick={() => decide('allow_once')}>
												{t('myaaw.chat.allow_once')}
											</Button>
											<Button size="xs" variant="secondary" onclick={() => decide('allow_session')}>
												{t('myaaw.chat.allow_session')}
											</Button>
											<Button size="xs" variant="secondary" onclick={() => decide('allow_always')}>
												{t('myaaw.chat.allow_always')}
											</Button>
											<Button size="xs" variant="destructive" onclick={() => decide('deny')}>
												{t('myaaw.chat.deny')}
											</Button>
										</div>
									{/if}
								</div>
							{:else if item.kind === 'error'}
								<div class="flex items-start gap-2 text-xs text-destructive">
									<AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
									<span class="whitespace-pre-wrap">{item.text}</span>
								</div>
							{/if}
						{/if}
					{/each}

					{#if attachments.length > 0}
						<div class="space-y-2">
							{#each attachments as attachment (attachment.file_name)}
								{#if isImage(attachment)}
									<img
										src={attachmentUrl(attachment)}
										alt={attachment.file_name}
										class="max-h-72 rounded-xl border"
									/>
								{:else}
									<a
										href={attachmentUrl(attachment)}
										download={attachment.file_name}
										class="inline-flex items-center gap-2 text-xs text-primary underline-offset-4 hover:underline"
									>
										<ArrowDownToLine class="h-3.5 w-3.5" />
										{attachment.file_name}
									</a>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			</div>

			{#if error}
				<div class="shrink-0 px-4">
					<div
						class="mx-auto max-w-5xl rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive"
					>
						{error}
					</div>
				</div>
			{/if}

			<div class="shrink-0 border-t bg-background/80 px-4 py-3 backdrop-blur">
				<div class="mx-auto w-full max-w-5xl">
					<div
						role="group"
						class={[
							'rounded-2xl border bg-card shadow-sm transition-colors focus-within:border-ring/60',
							dragOver ? 'border-ring ring-2 ring-ring/30' : ''
						]}
						ondragover={(event) => {
							event.preventDefault();
							dragOver = true;
						}}
						ondragleave={(event) => {
							event.preventDefault();
							dragOver = false;
						}}
						ondrop={(event) => {
							event.preventDefault();
							dragOver = false;
							void addFiles(event.dataTransfer?.files);
						}}
					>
						{#if pending.length > 0}
							<div class="flex flex-wrap gap-2 px-3 pt-3">
								{#each pending as image (image.id)}
									<div class="relative">
										<img
											src={image.dataUrl}
											alt={image.name}
											class="h-16 w-16 rounded-lg border object-cover"
										/>
										<button
											type="button"
											class="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full border bg-background text-muted-foreground shadow-sm hover:text-foreground"
											onclick={() => removePending(image.id)}
											aria-label={t('myaaw.chat.remove_image')}
											title={t('myaaw.chat.remove_image')}
										>
											<X class="h-3 w-3" />
										</button>
									</div>
								{/each}
							</div>
						{/if}
						{#if dragOver}
							<p class="px-4 pt-2 text-[11px] font-medium text-foreground">
								{t('myaaw.chat.drop_images')}
							</p>
						{/if}
						<textarea
							bind:this={composer}
							bind:value={input}
							use:autosize
							onkeydown={onKeydown}
							onpaste={onPaste}
							rows="1"
							placeholder={t('myaaw.chat.placeholder')}
							class="max-h-40 min-h-[3.25rem] w-full resize-none rounded-t-2xl bg-transparent px-4 py-3 text-sm outline-none placeholder:text-muted-foreground"
						></textarea>
						<div class="flex items-center justify-between px-3 pb-2">
							<div class="flex min-w-0 items-center gap-1">
								<input
									bind:this={fileInput}
									type="file"
									accept="image/*"
									multiple
									class="hidden"
									onchange={onFileInput}
								/>
								<Button
									type="button"
									size="icon-sm"
									variant="ghost"
									class="shrink-0 rounded-full"
									disabled={pending.length >= MAX_IMAGES}
									onclick={() => fileInput?.click()}
									aria-label={t('myaaw.chat.attach_image')}
									title={t('myaaw.chat.attach_image')}
								>
									<ImagePlus class="h-4 w-4" />
								</Button>
								<span class="truncate text-[10px] text-muted-foreground">
									{t('myaaw.chat.hint')}
								</span>
							</div>
							{#if streaming}
								<Button
									size="icon"
									variant="destructive"
									class="rounded-full"
									onclick={stop}
									aria-label={t('myaaw.chat.stop')}
								>
									<Square class="h-4 w-4" />
								</Button>
							{:else}
								<Button
									size="icon"
									class="rounded-full"
									onclick={send}
									disabled={!input.trim() && pending.length === 0}
									aria-label={t('myaaw.chat.send')}
								>
									<Send class="h-4 w-4" />
								</Button>
							{/if}
						</div>
						<div
							class="flex flex-wrap items-center justify-center gap-x-2 gap-y-1 border-t px-4 py-1.5 text-[10px] text-muted-foreground"
						>
							<span class="flex items-center gap-1">
								<Gauge class="h-3 w-3 shrink-0" />
								{turnLabel(statsTurns)} · {stepsLabel(statsSteps)}
								{#if statsTokPerSec > 0}
									· {t('myaaw.chat.stats_tok_per_sec', { count: statsTokPerSec })}
								{/if}
							</span>
							{#if statsModelLine}
								<span class="flex items-center gap-1 font-mono italic">
									<Database class="h-3 w-3 shrink-0" />
									{statsModelLine}
								</span>
							{/if}
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if agent && historyOpen}
		<!-- Drawer on small screens, a static third column from lg up. -->
		<button
			type="button"
			class="fixed inset-0 z-30 bg-black/40 lg:hidden"
			aria-label={t('myaaw.chat.history_hide')}
			onclick={toggleHistory}
		></button>
		<aside
			class="fixed inset-y-0 right-0 z-40 flex w-72 max-w-[85vw] flex-col border-l bg-card lg:static lg:z-auto lg:h-full lg:max-w-none"
		>
			<div class="shrink-0 border-b p-2">
				<div class="relative">
					<Search
						class="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						bind:value={historySearch}
						placeholder={t('myaaw.conversations.search')}
						class="h-8 pl-7 text-xs"
						aria-label={t('myaaw.conversations.search')}
					/>
				</div>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto p-2">
				{#if historyLoading && history.length === 0}
					<p class="px-2 py-3 text-xs text-muted-foreground">{t('common.loading')}</p>
				{:else if history.length === 0}
					<p class="px-2 py-3 text-xs text-muted-foreground">
						{historyQuery ? t('myaaw.chat.history_no_results') : t('myaaw.chat.history_empty')}
					</p>
				{/if}

				{#each history as entry (entry.id)}
					<div
						class={[
							'rounded-md transition-colors',
							entry.id === conversationId ? 'bg-accent' : 'hover:bg-muted'
						]}
					>
						{#if editingId === entry.id}
							<div class="flex items-center gap-1 p-1">
								<Input
									bind:value={editingTitle}
									class="h-7 text-xs"
									onkeydown={(event) => {
										if (event.key === 'Enter') void saveRename();
										if (event.key === 'Escape') editingId = '';
									}}
								/>
								<Button
									size="icon-xs"
									variant="secondary"
									onclick={saveRename}
									aria-label={t('common.save')}
								>
									<Check class="h-3 w-3" />
								</Button>
								<Button
									size="icon-xs"
									variant="ghost"
									onclick={() => (editingId = '')}
									aria-label={t('common.cancel')}
								>
									<X class="h-3 w-3" />
								</Button>
							</div>
						{:else}
							<div class="flex items-start gap-1 p-1">
								<button
									class="min-w-0 flex-1 rounded px-1 py-0.5 text-left"
									onclick={() => openConversation(entry.id)}
								>
									<span class="block truncate text-xs font-medium">
										{entry.title || entry.id.slice(0, 12)}
									</span>
									<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
										{formatWhen(entry.updated_at)} · {t('myaaw.conversations.messages', {
											count: entry.message_count
										})}
									</span>
								</button>
								<div class="flex shrink-0">
									<Button
										variant="ghost"
										size="icon-xs"
										onclick={() => startRename(entry)}
										aria-label={t('myaaw.chat.rename')}
									>
										<Pencil class="h-3 w-3" />
									</Button>
									<Button
										variant="ghost"
										size="icon-xs"
										onclick={() => removeHistory(entry)}
										aria-label={t('common.delete')}
									>
										<Trash2 class="h-3 w-3 text-destructive" />
									</Button>
								</div>
							</div>
						{/if}
					</div>
				{/each}

				{#if historyHasMore}
					<div use:infinite class="flex justify-center py-3">
						{#if historyLoading}
							<Loader2 class="h-4 w-4 animate-spin text-muted-foreground" />
						{/if}
					</div>
				{/if}
			</div>
		</aside>
	{/if}
</div>
