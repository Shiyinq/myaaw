<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import { chat as chatApi } from '$lib/apis/chat';
	import { config as configApi } from '$lib/apis/config';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { files as filesApi } from '$lib/apis/files';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { agentSelection, toastStore, topBarTitle } from '$lib/stores';
	import { chatRuns, type ChatItem } from '$lib/stores/chatRuns.svelte';
	import { cn } from '$lib/utils';
	import { isOverlayOpen, isTypingTarget, matchesShortcut, formatShortcut } from '$lib/utils/keys';
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
		Download,
		FileText,
		FolderOpen,
		Gauge,
		Globe,
		Loader2,
		MessageSquarePlus,
		Mic,
		Minimize2,
		Monitor,
		PanelRightClose,
		PanelRightOpen,
		Paperclip,
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
		Attachment,
		ConversationSummary,
		ConversationTurn,
		Message,
		Upload,
		Usage
	} from '$lib/types';

	const { t, locale } = useTranslation();

	type Item = ChatItem;

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

	interface PendingFile {
		id: string;
		name: string;
		mimeType: string;
		size: number;
		base64: string;
	}

	const MAX_IMAGES = 6;
	const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
	const MAX_IMAGE_EDGE = 1600;
	// Raster formats only: the browser re-encodes them to JPEG, and SVG/GIF
	// animations would not survive that, so they are rejected up front.
	const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

	const MAX_FILES = 8;
	const MAX_FILE_BYTES = 10 * 1024 * 1024;
	// Images and files share one budget because both travel inline as base64 in
	// the same request. The control plane caps a request body at 32 MiB, which
	// leaves room for base64 inflation (~4/3) and the JSON envelope.
	const MAX_TOTAL_UPLOAD_BYTES = 20 * 1024 * 1024;
	const FILE_EXTENSIONS = new Set([
		'.txt',
		'.md',
		'.markdown',
		'.csv',
		'.tsv',
		'.json',
		'.xml',
		'.yaml',
		'.yml',
		'.log',
		'.rtf',
		'.pdf',
		'.doc',
		'.docx',
		'.xls',
		'.xlsx',
		'.ppt',
		'.pptx',
		'.odt',
		'.ods',
		'.odp'
	]);

	let agent = $derived(agentSelection.selected);
	let conversationId = $state('');
	// A run of the open conversation keeps streaming while the user is elsewhere;
	// the page only renders whatever the store currently holds.
	let activeRun = $derived(chatRuns.forConversation(agent ?? '', conversationId));
	let historyItems = $state<Item[]>([]);
	// The transcript is loaded a page at a time so a long conversation does not
	// arrive in one payload; these track where the loaded page starts.
	let loadedOffset = $state(0);
	let loadingOlder = $state(false);
	// Set while older messages are being prepended: the transcript must keep the
	// reading position and must not follow the new rows to the bottom.
	let prepending = false;
	let items = $derived<Item[]>(activeRun ? activeRun.items : historyItems);
	let input = $state('');
	let streaming = $derived(activeRun?.streaming ?? false);
	let loading = $state(false);
	let pendingApproval = $derived(activeRun?.pendingApproval ?? null);
	let storedUsage = $state<Usage | null>(null);
	let usage = $derived(activeRun?.usage ?? storedUsage);
	let runError = $derived(activeRun?.error ?? '');
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
		cacheHitTokens?: number;
		uncachedInputTokens?: number;
		outputTokens?: number;
		llmMs?: number;
		toolMs?: number;
		ttftMs?: number;
	} | null>(null);
	// Configured provider and its default model, as the terminal status line shows.
	let providerInfo = $state<{
		default: string;
		providers: { name: string; type: string; model: string }[];
	} | null>(null);
	let attachments = $derived<Attachment[]>(activeRun?.attachments ?? []);
	let pending = $state<PendingImage[]>([]);
	let pendingFiles = $state<PendingFile[]>([]);
	let dragOver = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);
	let groupOpen = $state<Record<string, boolean>>({});
	let error = $state('');
	// The two status-line popovers open on click and close on an outside click.
	let openStats = $state(false);
	let openUsage = $state(false);
	let scroller = $state<HTMLDivElement | null>(null);
	let composer = $state<HTMLTextAreaElement | null>(null);

	// Turn rail: one mark per user message, so a long answer can be skipped
	// straight back to its question. The rail is a short column centred on the
	// right edge; it unfolds into a list on hover.
	// A tick in the rail points at one of the user's questions.
	interface TurnMark {
		id: string;
		text: string;
		top: number;
		index: number;
	}

	// The gap between ticks, and the height of a row in the unfolded list: they are
	// the same, so nothing shifts when the card appears.
	const RAIL_GAP = 26;
	const RAIL_PAD = 8;
	// Only this many ticks are drawn at once, so a long conversation never
	// squeezes them into an unreadable column.
	const RAIL_VISIBLE = 11;

	// Every question of the conversation, from the server summary, so the rail
	// covers the whole transcript even when only a page of messages is loaded.
	//
	// The summary is not paginated: it rides along with every detail response, so
	// it stays cheap for hundreds of turns but grows with the conversation.
	// If improvement is needed (a transcript of thousands of turns, or a payload
	// that starts to hurt), the options are, cheapest first:
	//   (a) ship the indexes only and fetch the labels for the open window on hover
	//       (the ticks stay complete, the card loads its rows in batches);
	//   (b) cap the list, e.g. 300 turns, and add a `turns_truncated` flag;
	//   (c) lower the per-turn cap (160 characters today) so the list is smaller.
	let turns = $state<ConversationTurn[]>([]);
	// The measured marks of the questions that are currently rendered.
	let turnMarks = $state<TurnMark[]>([]);
	let activeTurnIndex = $state<number | null>(null);
	// The question the pointer is on, either a tick or a row of the unfolded list.
	let hoverTurnIndex = $state<number | null>(null);
	let railOpen = $state(false);
	let cardList = $state<HTMLDivElement | null>(null);
	// The list hides its scrollbar, so the edges tell whether it can be scrolled.
	let cardScrollUp = $state(false);
	let cardScrollDown = $state(false);

	// A question asked in a live run is not in the summary yet; it gets a negative
	// index so the rail can still show and light it.
	let railTurns = $derived.by(() => {
		const stored = new Set(turns.map((turn) => turn.index));
		const live = turnMarks
			.filter((mark) => !stored.has(mark.index))
			.map((mark) => ({ index: mark.index, text: mark.text }));
		// One entry per question: a row can repeat an index the summary already has
		// while a run is settling, and a keyed block cannot repeat a key.
		const unique = new Map<number, { index: number; text: string }>();
		for (const turn of [...turns, ...live]) {
			if (!unique.has(turn.index)) unique.set(turn.index, turn);
		}
		return [...unique.values()];
	});

	// The ticks are a window over the list, centred on the question being read. It
	// rolls like a lens: the marks at both ends of the window are shorter and
	// dimmer than the ones near the middle.
	let railMarks = $derived.by(() => {
		const total = railTurns.length;
		const visible = Math.min(total, RAIL_VISIBLE);
		if (visible === 0) return [];
		const found = railTurns.findIndex((turn) => turn.index === activeTurnIndex);
		const active = found < 0 ? 0 : found;
		const half = (visible - 1) / 2;
		const start = Math.min(Math.max(active - Math.round(half), 0), total - visible);
		// A short conversation shows every mark at the same size; a long one rolls.
		const rolls = total > RAIL_VISIBLE;
		return Array.from({ length: visible }, (_, position) => {
			const distance = rolls && half > 0 ? Math.abs(position - half) / half : 0;
			return {
				turn: railTurns[start + position],
				width: 14 - 7 * distance,
				opacity: 1 - 0.5 * distance
			};
		});
	});

	let railHeight = $derived(
		railMarks.length > 1 ? (railMarks.length - 1) * RAIL_GAP + RAIL_PAD * 2 : 0
	);

	let loadKey = '';

	// Key of the group that is still collecting steps, so its header stays open
	// and spinning between tool calls instead of folding away mid-run. It lives
	// on the run, because the run outlives this page.
	let openGroup = $derived(activeRun?.openGroup ?? '');

	// Conversation history sidebar
	const HISTORY_LIMIT = 20;
	// Messages fetched per transcript page; scrolling to the top fetches the page
	// before the loaded one.
	const PAGE_SIZE = 20;
	const HISTORY_KEY = 'myaaw_chat_history_open';
	const LAST_CONVERSATION_KEY = 'myaaw_last_conversation';

	let historyOpen = $state(
		browser
			? (localStorage.getItem(HISTORY_KEY) ?? String(window.innerWidth >= 1024)) === 'true'
			: false
	);
	let history = $state<ConversationSummary[]>([]);
	// Conversations whose run is still streaming in the background. The open one
	// is skipped: its own timeline already shows the progress.
	let streamingConversations = $derived(
		new Set(
			chatRuns
				.all()
				.filter((run) => run.streaming && run.agent === agent && run.conversationId !== '')
				.map((run) => run.conversationId)
		)
	);
	let historyOffset = $state(0);
	let historyHasMore = $state(false);
	let historyLoading = $state(false);
	let historySearch = $state('');
	let historyQuery = $state('');
	let historyKey = '';
	let chatAgentKey = '';
	let searchBox = $state<HTMLInputElement | null>(null);
	let editingId = $state('');
	let editingTitle = $state('');

	function scrollToBottom() {
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	/** Drops the rail and the paging cursor, for when the transcript is cleared. */
	function clearRail() {
		turns = [];
		turnMarks = [];
		loadedOffset = 0;
		activeTurnIndex = null;
		hoverTurnIndex = null;
		railOpen = false;
	}

	/** Re-reads where each user message sits, after the transcript changes size. */
	function measureTurns() {
		const container = scroller;
		if (!container) return;
		const origin = container.getBoundingClientRect().top;
		const texts = new Map<number, string>();
		for (const item of items) {
			if (item.kind === 'user' && item.messageIndex !== undefined) {
				texts.set(item.messageIndex, item.text ?? '');
			}
		}
		const storedTexts = new Map(turns.map((turn) => [turn.index, turn.text]));

		turnMarks = Array.from(container.querySelectorAll<HTMLElement>('[data-chat-turn]')).map(
			(node, position) => {
				const id = node.dataset.chatTurn ?? '';
				const raw = node.dataset.chatTurnIndex;
				const stored = raw === undefined || raw === '' ? Number.NaN : Number(raw);
				// A question from a live run is not stored yet: it gets a negative
				// index so the rail can still point at it.
				const index = Number.isFinite(stored) ? stored : -(position + 1);
				const top = node.getBoundingClientRect().top - origin + container.scrollTop;
				const text = storedTexts.get(index) ?? texts.get(index) ?? '';
				return { id, index, text, top };
			}
		);
	}

	/**
	 * The mark of the message that is currently in view: the newest question that
	 * started above a probe a quarter down the viewport, or the last one once the
	 * transcript is scrolled to the bottom, so an answer still being read keeps
	 * its own question lit.
	 */
	function syncActiveTurn() {
		if (!scroller || turnMarks.length === 0) return;
		if (scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 8) {
			// At the bottom the newest question is the one being answered.
			const last = railTurns[railTurns.length - 1];
			activeTurnIndex = last ? last.index : null;
			return;
		}
		const probe = scroller.scrollTop + scroller.clientHeight * 0.25;
		let active = turnMarks[0].index;
		for (const mark of turnMarks) {
			if (mark.top <= probe) active = mark.index;
		}
		activeTurnIndex = active;
	}

	// Measuring reads a rect per user message, so a burst of triggers (typing in
	// the composer, resizing, prepending a page) coalesces into one frame.
	let measureFrame = 0;
	function queueTurnMeasure() {
		if (measureFrame) return;
		measureFrame = requestAnimationFrame(() => {
			measureFrame = 0;
			measureTurns();
			syncActiveTurn();
		});
	}

	/**
	 * Scrolls to one of the questions. A question in a page that is not loaded yet
	 * pulls the transcript back to it first, so the whole conversation stays
	 * reachable from the rail even though only a window of it is in memory.
	 */
	async function jumpToTurn(turn: ConversationTurn) {
		if (!scroller) return;
		let mark = turnMarks.find((candidate) => candidate.index === turn.index);
		if (!mark) {
			await loadOlderUntil(turn.index);
			mark = turnMarks.find((candidate) => candidate.index === turn.index);
		}
		if (!mark) return;
		scroller.scrollTo({ top: Math.max(0, mark.top - 16), behavior: 'smooth' });
		activeTurnIndex = turn.index;
	}

	/** Loads pages until the question at index is rendered, or nothing is left. */
	async function loadOlderUntil(index: number) {
		// A jump can cross hundreds of messages. Chunks of 100 keep the walk to a few
		// requests, where page-sized chunks would need one request per twenty
		// messages and one big request would drag in every image on the way.
		//
		// If improvement is needed (an image-heavy stretch makes one chunk too
		// expensive), the options are:
		//   (a) jump by replacing the window: one page around the target, then add a
		//       `loadNewer` so scrolling down fetches the messages after it;
		//   (b) shrink the chunk to 40–60 messages, trading requests for weight;
		//   (c) fall back to the page size and accept one request per 20 messages.
		for (let guard = 0; guard < 6 && loadedOffset > index; guard += 1) {
			const before = loadedOffset;
			await loadOlder(100);
			if (loadedOffset === before) return;
		}
	}

	function onScroll() {
		syncActiveTurn();
		// Near the top the previous page is pulled in, keeping the reader's place.
		if (scroller && scroller.scrollTop < 120) void loadOlder();
	}

	/** Whether the unfolded list has more turns above or below the visible part. */
	function updateCardScroll() {
		const list = cardList;
		if (!list) return;
		cardScrollUp = list.scrollTop > 4;
		cardScrollDown = list.scrollTop + list.clientHeight < list.scrollHeight - 4;
	}

	$effect(() => {
		// Open the list on the question being read, not on the oldest one.
		if (!railOpen || !cardList) return;
		const row = cardList.querySelector<HTMLElement>('[data-turn-active="true"]');
		if (row) {
			const delta = row.getBoundingClientRect().top - cardList.getBoundingClientRect().top;
			cardList.scrollTop = Math.max(
				0,
				cardList.scrollTop + delta - (cardList.clientHeight - row.offsetHeight) / 2
			);
		}
		updateCardScroll();
	});

	$effect(() => {
		// Re-measure when the transcript gains or loses rows, or the viewport changes.
		const count = items.length;
		if (!scroller || count === 0 || !conversationId) {
			turnMarks = [];
			return;
		}
		queueTurnMeasure();
	});

	$effect(() => {
		if (!browser) return;
		const onResize = () => queueTurnMeasure();
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
	});

	// A run that ends while another conversation (or menu) is open must not leave
	// a stale spinner, title or message count in the sidebar. Refresh the list
	// once, when a run of this agent stops running.
	let runningCount = -1;
	$effect(() => {
		const running = chatRuns.all().filter((run) => run.agent === agent && run.streaming).length;
		const previous = runningCount;
		runningCount = running;
		if (previous > running && previous >= 0) {
			void loadHistory(true);
			// The title comes from a background LLM call that runs after the answer,
			// so the first refresh usually still shows "New Chat".
			setTimeout(() => void loadHistory(true), 4000);
		}
	});

	// Follow only new timeline rows. Expanding a step reassigns `items` with the
	// same length, and that must not yank the view to the bottom. Neither must
	// prepending an older page.
	let followedCount = 0;
	$effect(() => {
		if (items.length === followedCount) return;
		followedCount = items.length;
		if (prepending) return;
		scrollToBottom();
	});

	async function loadConversation(currentAgent: string, id: string) {
		// Dictation belongs to the composer the reader is looking at.
		stopSpeech();
		if (!id) {
			historyItems = [];
			clearRail();
			storedUsage = null;
			sessionStats = null;
			stopTitlePolling();
			topBarTitle.clear();
			return;
		}
		loading = true;
		error = '';
		sessionStats = null;
		groupOpen = {};
		void refreshStats(currentAgent, id);
		try {
			// Ask for the last page so the chat opens at the newest message without
			// pulling the whole transcript, which for image-heavy chats is megabytes.
			// A negative offset means "the last page"; the server normalises it to
			// total - limit, and every later page walks back from that point.
			// If improvement is needed (the API stops accepting a negative offset),
			// fetch `limit: 1` first to read total_messages and then ask for
			// `offset: total - PAGE_SIZE` — one extra round trip, no server change.
			const detail = await conversationsApi.get(currentAgent, id, {
				limit: PAGE_SIZE,
				offset: -1
			});
			loadedOffset = detail.offset;
			turns = detail.turns ?? [];
			historyItems = historyTimeline(detail.messages, detail.offset);
			// A finished run is redundant once the stored transcript is loaded; the
			// server's copy includes the final answer. A live run keeps going.
			chatRuns.settle(currentAgent, id);
			const title = detail.title ?? '';
			applyTitle(id, title);
			if (isPlaceholderTitle(title)) pollTitle(currentAgent, id, 0);
			storedUsage = null;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
			scrollToBottom();
		}

		// A page that does not fill the viewport cannot be scrolled, so the reader
		// would never reach the trigger that pulls older messages; fetch until it
		// scrolls (or the transcript is complete).
		await tick();
		for (let guard = 0; guard < 5; guard += 1) {
			if (!scroller || loadedOffset === 0) break;
			if (scroller.scrollHeight > scroller.clientHeight + 8) break;
			const before = loadedOffset;
			await loadOlder();
			if (loadedOffset === before) break;
		}
		scrollToBottom();
	}

	/**
	 * Prepends the page of messages that sit before the loaded one. The scroll
	 * position is restored to the row the reader was on, because the measured
	 * marks of the turn rail are absolute offsets inside this scroller.
	 */
	async function loadOlder(pageSize = PAGE_SIZE) {
		const currentAgent = agent ?? '';
		if (loadingOlder || loadedOffset === 0 || !conversationId || !currentAgent) return;
		// While a run streams, the timeline comes from the run, not from storage.
		if (activeRun) return;

		loadingOlder = true;
		prepending = true;
		const before = scroller?.scrollHeight ?? 0;
		const previousTop = scroller?.scrollTop ?? 0;
		try {
			// The loaded window is [loadedOffset, total), so the page before it ends
			// exactly at loadedOffset and the two never overlap. Offsets therefore
			// walk backwards one page at a time (323 → 303 → 283 …).
			const start = Math.max(0, loadedOffset - pageSize);
			const detail = await conversationsApi.get(currentAgent, conversationId, {
				limit: loadedOffset - start,
				offset: start
			});
			if (conversationId === '' || detail.messages.length === 0 || detail.offset >= loadedOffset) {
				// Nothing older is left: stop asking, but keep what is already shown.
				loadedOffset = 0;
				return;
			}
			const known = new Set(historyItems.map((item) => item.localId));
			historyItems = [
				...historyTimeline(detail.messages, detail.offset).filter(
					(item) => !known.has(item.localId)
				),
				...historyItems
			];
			loadedOffset = detail.offset;
			await tick();
			if (scroller) scroller.scrollTop = previousTop + (scroller.scrollHeight - before);
			measureTurns();
			syncActiveTurn();
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			// The rows really were added above: keep the counter in step so the
			// auto-scroll effect does not fire for them later.
			followedCount = items.length;
			loadingOlder = false;
			prepending = false;
		}
	}

	/**
	 * The shell shows the conversation title instead of the page label. The title
	 * of a new chat is generated in a background LLM call *after* the response,
	 * so the first turn ends with "New Chat" in place; poll until it changes.
	 */
	let titleTimer: ReturnType<typeof setTimeout> | undefined;

	function isPlaceholderTitle(title: string) {
		return !title || title === 'New Chat';
	}

	function applyTitle(id: string, title: string) {
		if (conversationId !== id) return; // Another conversation won.
		topBarTitle.set(title);
		// Keep the history entry in step with the bar without reloading the list.
		history = history.map((item) => (item.id === id ? { ...item, title } : item));
	}

	function stopTitlePolling() {
		clearTimeout(titleTimer);
		titleTimer = undefined;
	}

	function pollTitle(currentAgent: string, id: string, attempt: number) {
		stopTitlePolling();
		if (attempt > 8) return;
		titleTimer = setTimeout(async () => {
			if (conversationId !== id) return;
			try {
				const detail = await conversationsApi.get(currentAgent, id, { limit: 1 });
				if (conversationId !== id) return;
				const title = detail.title ?? '';
				applyTitle(id, title);
				if (isPlaceholderTitle(title)) pollTitle(currentAgent, id, attempt + 1);
			} catch {
				pollTitle(currentAgent, id, attempt + 1);
			}
		}, 2000);
	}

	async function refreshTitle(currentAgent: string, id: string) {
		if (!currentAgent || !id) return;
		try {
			const detail = await conversationsApi.get(currentAgent, id, { limit: 1 });
			if (conversationId !== id) return; // Another conversation won.
			const title = detail.title ?? '';
			applyTitle(id, title);
			if (isPlaceholderTitle(title)) pollTitle(currentAgent, id, 0);
		} catch {
			// Keep the title already on screen.
		}
	}

	// Leaving the chat must not leave its title on every other page.
	$effect(() => {
		return () => {
			stopTitlePolling();
			topBarTitle.clear();
		};
	});

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
			// Every number below is computed by the API; the view only formats it.
			sessionStats = {
				turns: detail.turns,
				steps: detail.steps,
				tokens: detail.total_tokens,
				model: detail.model ?? '',
				provider: detail.provider ?? '',
				tokPerSec: detail.tok_per_sec ?? 0,
				cacheRate: detail.cache_hit_rate ?? 0,
				cacheHitTokens: detail.cache_hit_tokens ?? 0,
				uncachedInputTokens: detail.uncached_input_tokens ?? 0,
				outputTokens: detail.output_tokens ?? 0,
				llmMs: detail.llm_ms ?? 0,
				toolMs: detail.tool_ms ?? 0,
				ttftMs: detail.ttft_ms ?? 0
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
		historyItems = [];
		clearRail();
		storedUsage = null;
		sessionStats = null;
		error = '';
		stopTitlePolling();
		topBarTitle.clear();
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

	// This page is only a viewer of a run: callbacks are attached while it is on
	// screen and dropped on unmount, so a background run never touches a
	// destroyed page.
	$effect(() => {
		const run = activeRun;
		if (!run) return;
		const view = {
			onConversation: (id: string) => {
				if (conversationId === id) return;
				conversationId = id;
				loadKey = `${agent}:${id}`;
				rememberConversation(id);
				void loadHistory(true);
				void goto(`/app/chat/${id}`, { replaceState: true, noScroll: true });
			},
			onDone: () => {
				const currentAgent = agent ?? '';
				const statsId = conversationId;
				void refreshStats(currentAgent, statsId);
				void refreshTitle(currentAgent, statsId);
				// The run may land in the trace store just after `done`.
				setTimeout(() => void refreshStats(currentAgent, statsId), 1500);
			},
			onError: (message: string) => {
				error = message;
			}
		};
		run.view = view;
		return () => {
			if (run.view === view) run.view = null;
		};
	});

	// An answer just ended: the reader is most likely typing next.
	let wasStreaming = false;
	$effect(() => {
		const now = streaming;
		if (wasStreaming && !now) void focusComposer();
		wasStreaming = now;
	});

	// Runs started before this page existed (reload, closed tab, fresh login) are
	// still alive on the agent server; attach to them again.
	$effect(() => {
		const currentAgent = agent ?? '';
		if (!currentAgent) return;
		void chatRuns.restore(currentAgent);
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

	function formatDuration(ms: number) {
		const seconds = ms / 1000;
		if (seconds < 60) return `${Math.max(1, Math.round(seconds))}s`;
		const minutes = Math.floor(seconds / 60);
		return `${minutes}m ${Math.round(seconds % 60)}s`;
	}

	let turnCount = $derived(items.filter((item) => item.kind === 'user').length);
	let stepCount = $derived(items.filter((item) => item.kind === 'tool').length);

	/** The newest row of a kind, which keeps its copy button visible. */
	function lastItemId(kind: string) {
		for (let index = items.length - 1; index >= 0; index--) {
			if (items[index].kind === kind) return items[index].localId ?? '';
		}
		return '';
	}

	let latestUserKey = $derived(lastItemId('user'));
	let latestAnswerKey = $derived(lastItemId('text'));
	let statsTurns = $derived(sessionStats?.turns ?? turnCount);
	let statsSteps = $derived(sessionStats?.steps ?? stepCount);
	let statsTokens = $derived(sessionStats?.tokens ?? usage?.total_tokens ?? 0);
	let statsTokPerSec = $derived(sessionStats?.tokPerSec ?? 0);
	let statsCacheRate = $derived(sessionStats?.cacheRate ?? 0);
	let statsLlmMs = $derived(sessionStats?.llmMs ?? 0);
	let statsToolMs = $derived(sessionStats?.toolMs ?? 0);
	let statsTtftMs = $derived(sessionStats?.ttftMs ?? 0);
	let statsOutputTokens = $derived(sessionStats?.outputTokens ?? 0);
	let statsCachedTokens = $derived(sessionStats?.cacheHitTokens ?? 0);
	let statsUncachedTokens = $derived(sessionStats?.uncachedInputTokens ?? 0);
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

	function formatCount(count: number) {
		return count.toLocaleString();
	}

	function formatBytes(bytes: number) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function formatSeconds(ms: number) {
		if (ms <= 0) return '—';
		const seconds = ms / 1000;
		if (seconds < 60) return `${seconds.toFixed(1)}s`;
		const minutes = Math.floor(seconds / 60);
		return `${minutes}m ${Math.round(seconds % 60)}s`;
	}

	function turnLabel(count: number) {
		return count === 1 ? t('myaaw.chat.turn_one') : t('myaaw.chat.turns', { count });
	}

	function stepsLabel(count: number) {
		return count === 1 ? t('myaaw.chat.step_one') : t('myaaw.chat.steps', { count });
	}

	function toggleItem(localId?: string) {
		if (!localId) return;
		if (activeRun) {
			activeRun.toggleItem(localId);
			return;
		}
		historyItems = historyItems.map((item) =>
			item.localId === localId ? { ...item, expanded: !item.expanded } : item
		);
	}

	function send() {
		const text = input.trim();
		// Sending ends the dictation: the answer must not be typed over.
		stopSpeech();
		const images = pending.map((item) => item.base64);
		const files = pendingFiles.map((item) => ({
			name: item.name,
			mime_type: item.mimeType || undefined,
			data_base64: item.base64
		}));
		if ((!text && images.length === 0 && files.length === 0) || !agent || streaming) return;

		// The run owns the stream and its timeline, so opening another
		// conversation or another menu no longer cuts the answer off.
		chatRuns.start({
			agent,
			conversationId,
			text,
			images,
			imagesDisplay: pending.map((item) => item.dataUrl),
			files,
			uploads: pendingFiles.map((item) => ({
				name: item.name,
				path: '',
				mime_type: item.mimeType,
				size: item.size
			})),
			// Keep the earlier turns of this conversation visible.
			initialItems: items
		});

		input = '';
		clearPending();
		groupOpen = {};
		if (composer) composer.style.height = '';
		error = '';
		storedUsage = null;
	}

	async function decide(decision: ApprovalDecision) {
		const run = activeRun;
		const prompt = run?.pendingApproval;
		if (!run || !prompt || !agent) return;
		try {
			await chatApi.decide(agent, prompt.approval_id, decision);
			run.resolveApproval(prompt.approval_id, decision);
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		}
	}

	/** Stops the open conversation's run; other conversations keep streaming. */
	function stop() {
		void activeRun?.stop();
	}

	async function newChat() {
		resetChat();
		if (page.url.pathname !== '/app/chat') {
			await goto('/app/chat');
		}
		// A fresh chat exists to be typed into; focus once the route has settled.
		await focusComposer();
	}

	async function focusComposer() {
		await tick();
		composer?.focus();
	}

	/** Opens the chat history and puts the cursor in its search box. */
	async function focusHistorySearch() {
		if (!historyOpen) toggleHistory();
		await tick();
		searchBox?.focus();
		searchBox?.select();
	}

	function resetChat() {
		// Runs of other conversations keep streaming: this only clears the view.
		conversationId = '';
		historyItems = [];
		clearRail();
		input = '';
		if (composer) composer.style.height = '';
		storedUsage = null;
		sessionStats = null;
		pending = [];
		pendingFiles = [];
		error = '';
		loadKey = `${agent}:`;
		rememberConversation('');
		stopTitlePolling();
		topBarTitle.clear();
		groupOpen = {};
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
			if (editingId === conversationId) {
				stopTitlePolling();
				topBarTitle.set(title);
			}
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

	/**
	 * Uploads live under <agent>/user/uploads/<conversation>/<message>/ and the
	 * transcript stores their absolute path. The Files API addresses them through
	 * the /user virtual root, so keep the path from that folder on.
	 */
	function uploadVirtualPath(upload: Upload): string {
		const marker = '/user/uploads/';
		const index = upload.path.indexOf(marker);
		return index >= 0 ? upload.path.slice(index) : '';
	}

	/**
	 * The agent prefixes a user turn that carries uploads with an <attached>
	 * block of file paths, so the model can read them with its tools. The UI
	 * lists those files as chips instead of showing the raw paths.
	 */
	function withoutAttached(text: string): string {
		return text.replace(/<attached>[\s\S]*?<\/attached>\s*/g, '').trim();
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
	function historyTimeline(messages: Message[], startIndex = 0): Item[] {
		const timeline: Item[] = [];
		const tools = new Map<string, number>();
		let messageIndex = startIndex - 1;
		let index = 0;
		// Ids carry the position in the whole conversation, not in the loaded page,
		// so prepending an older page keeps every existing row (and any step the
		// reader has open) under the same key.
		const nextId = () => `history-${messageIndex}-${index++}`;

		for (const message of messages) {
			messageIndex += 1;
			index = 0;
			if (message.thought) {
				timeline.push({
					kind: 'thought',
					text: message.thought,
					running: false,
					expanded: false,
					localId: nextId(),
					messageIndex
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
						localId: nextId(),
						messageIndex
					});
				}
				continue;
			}

			if (message.role === 'user') {
				const text = withoutAttached(messageText(message));
				const images = messageImages(message);
				if (text || images.length > 0 || (message.uploads?.length ?? 0) > 0) {
					timeline.push({
						kind: 'user',
						text,
						images: images.length > 0 ? images : undefined,
						uploads: message.uploads,
						localId: nextId(),
						messageIndex
					});
				}
				continue;
			}

			if (message.role !== 'assistant') continue;

			const text = messageText(message);
			if (text) timeline.push({ kind: 'text', text, localId: nextId(), messageIndex });

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
					localId: nextId(),
					messageIndex
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

	/** Reads a file's bytes as raw base64 (no data-URL prefix). */
	function readAsBase64(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => {
				const dataUrl = String(reader.result);
				resolve(dataUrl.includes(',') ? (dataUrl.split(',')[1] ?? '') : dataUrl);
			};
			reader.onerror = () => reject(reader.error ?? new Error('read failed'));
			reader.readAsDataURL(file);
		});
	}

	/** Encoded size of a base64 payload, rounded up. */
	function base64Bytes(value: string): number {
		return Math.ceil((value.length * 3) / 4);
	}

	/** Bytes already queued for upload: inlined images plus document sizes. */
	function uploadBytes(images: PendingImage[], files: PendingFile[]): number {
		const imageBytes = images.reduce((sum, item) => sum + base64Bytes(item.base64), 0);
		const fileBytes = files.reduce((sum, item) => sum + item.size, 0);
		return imageBytes + fileBytes;
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
		const documents = candidates.filter((file) => !IMAGE_TYPES.has(file.type));

		let totalBytes = uploadBytes(pending, pendingFiles);

		const room = MAX_IMAGES - pending.length;
		if (images.length > room) {
			toastStore.error(t('myaaw.chat.image_limit', { count: MAX_IMAGES }));
		}

		const additions: PendingImage[] = [];
		for (const file of images.slice(0, Math.max(0, room))) {
			if (file.size > MAX_IMAGE_BYTES) {
				toastStore.error(t('myaaw.chat.image_too_large', { size: 10 }));
				continue;
			}
			try {
				const dataUrl = await prepareImage(file);
				const base64 = dataUrl.split(',')[1] ?? '';
				if (!base64) continue;
				const bytes = base64Bytes(base64);
				if (totalBytes + bytes > MAX_TOTAL_UPLOAD_BYTES) {
					toastStore.error(t('myaaw.chat.attachment_total_too_large', { size: 20 }));
					continue;
				}
				additions.push({
					id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
					name: file.name,
					dataUrl,
					base64
				});
				totalBytes += bytes;
			} catch {
				toastStore.error(t('myaaw.chat.image_failed'));
			}
		}
		if (additions.length > 0) pending = [...pending, ...additions];

		const fileRoom = MAX_FILES - pendingFiles.length;
		if (documents.length > fileRoom) {
			toastStore.error(t('myaaw.chat.file_limit', { count: MAX_FILES }));
		}

		const fileAdditions: PendingFile[] = [];
		for (const file of documents.slice(0, Math.max(0, fileRoom))) {
			const dot = file.name.lastIndexOf('.');
			const ext = dot >= 0 ? file.name.slice(dot).toLowerCase() : '';
			if (!FILE_EXTENSIONS.has(ext)) {
				toastStore.error(t('myaaw.chat.file_unsupported'));
				continue;
			}
			if (file.size > MAX_FILE_BYTES) {
				toastStore.error(t('myaaw.chat.file_too_large', { size: 10 }));
				continue;
			}
			if (totalBytes + file.size > MAX_TOTAL_UPLOAD_BYTES) {
				toastStore.error(t('myaaw.chat.attachment_total_too_large', { size: 20 }));
				continue;
			}
			try {
				const base64 = await readAsBase64(file);
				if (!base64) continue;
				fileAdditions.push({
					id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
					name: file.name,
					mimeType: file.type,
					size: file.size,
					base64
				});
				totalBytes += file.size;
			} catch {
				toastStore.error(t('myaaw.chat.file_failed'));
			}
		}
		if (fileAdditions.length > 0) pendingFiles = [...pendingFiles, ...fileAdditions];
	}

	function clearPending() {
		pending = [];
		pendingFiles = [];
		if (fileInput) fileInput.value = '';
	}

	function removePending(id: string) {
		pending = pending.filter((item) => item.id !== id);
	}

	function removePendingFile(id: string) {
		pendingFiles = pendingFiles.filter((item) => item.id !== id);
	}

	function onFileInput(event: Event) {
		const element = event.currentTarget as HTMLInputElement;
		void addFiles(element.files);
		element.value = '';
	}

	function onPaste(event: ClipboardEvent) {
		const files = Array.from(event.clipboardData?.files ?? []);
		if (files.length === 0) return;
		if (
			files.some(
				(file) => IMAGE_TYPES.has(file.type) || FILE_EXTENSIONS.has(fileExtension(file.name))
			)
		) {
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

	/** Turns the page shortcuts into actions; the composer keeps its own keys. */
	function onShortcutKey(event: KeyboardEvent) {
		// The composer already consumed Enter (with or without a modifier).
		if (event.defaultPrevented) return;

		if (matchesShortcut(event, 'escape')) {
			// An open dialog or menu owns Esc, so interrupting waits its turn.
			if (isOverlayOpen()) return;
			if (listening) {
				event.preventDefault();
				stopSpeech();
				return;
			}
			openStats = false;
			openUsage = false;
			const run = activeRun;
			if (run?.streaming) {
				event.preventDefault();
				void run.stop();
			}
			return;
		}

		if (matchesShortcut(event, '/')) {
			event.preventDefault();
			void focusComposer();
			return;
		}
		if (matchesShortcut(event, 'mod+enter')) {
			event.preventDefault();
			send();
			return;
		}
		if (matchesShortcut(event, 'mod+shift+space')) {
			event.preventDefault();
			toggleSpeech();
			return;
		}
		if (matchesShortcut(event, 'mod+shift+s')) {
			event.preventDefault();
			void focusHistorySearch();
			return;
		}
		if (matchesShortcut(event, 'mod+shift+f')) {
			event.preventDefault();
			if (!(pending.length >= MAX_IMAGES && pendingFiles.length >= MAX_FILES)) {
				fileInput?.click();
			}
			return;
		}
		if (matchesShortcut(event, 'mod+shift+h')) {
			event.preventDefault();
			toggleHistory();
			return;
		}

		// Arrow and letter shortcuts must not steal keys from a text field.
		if (isTypingTarget(event.target)) return;
		if (matchesShortcut(event, 'alt+arrowup') || matchesShortcut(event, 'alt+arrowdown')) {
			event.preventDefault();
			jumpTurn(matchesShortcut(event, 'alt+arrowup') ? -1 : 1);
			return;
		}
		if (matchesShortcut(event, 'alt+n')) {
			event.preventDefault();
			newChat();
		}
	}

	/** Moves to the question before or after the one being read. */
	function jumpTurn(direction: -1 | 1) {
		if (railTurns.length === 0) return;
		const current = railTurns.findIndex((turn) => turn.index === activeTurnIndex);
		const next = current < 0 ? 0 : Math.min(Math.max(current + direction, 0), railTurns.length - 1);
		void jumpToTurn(railTurns[next]);
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

	// --- Dictation -------------------------------------------------------------
	// Speech to text uses the browser's own recogniser (Chrome, Edge, Safari): no
	// library, and the audio never passes through this app.
	interface SpeechEventLike {
		resultIndex: number;
		results: {
			length: number;
			[index: number]: { isFinal: boolean; [index: number]: { transcript: string } };
		};
	}

	interface RecognitionLike {
		lang: string;
		continuous: boolean;
		interimResults: boolean;
		start: () => void;
		stop: () => void;
		abort: () => void;
		onresult: ((event: SpeechEventLike) => void) | null;
		onerror: ((event: { error: string }) => void) | null;
		onend: (() => void) | null;
	}

	let listening = $state(false);
	let speech: RecognitionLike | null = null;
	// What the composer held when dictation started, and the part the recogniser
	// has settled on. The interim tail is rewritten on every event.
	let speechHead = '';
	let speechSettled = '';

	function speechConstructor() {
		if (!browser) return null;
		const scope = window as unknown as {
			SpeechRecognition?: new () => RecognitionLike;
			webkitSpeechRecognition?: new () => RecognitionLike;
		};
		return scope.SpeechRecognition ?? scope.webkitSpeechRecognition ?? null;
	}

	async function setSpeechText(value: string) {
		input = value;
		// The composer grows from its own input event, which a state change does not
		// dispatch.
		await tick();
		composer?.dispatchEvent(new Event('input', { bubbles: true }));
	}

	function stopSpeech() {
		if (!listening && !speech) return;
		listening = false;
		try {
			speech?.stop();
		} catch {
			// Already stopped.
		}
		speech = null;
	}

	function toggleSpeech() {
		if (listening) {
			stopSpeech();
			return;
		}
		const Constructor = speechConstructor();
		if (!Constructor) {
			toastStore.error(t('myaaw.chat.speech_unsupported'));
			return;
		}
		// One recogniser per dictation: it cannot be started again after an abort.
		const recognition = new Constructor();
		recognition.lang = locale.value === 'id' ? 'id-ID' : 'en-US';
		recognition.continuous = true;
		recognition.interimResults = true;
		speechHead = input ? `${input.trimEnd()} ` : '';
		speechSettled = '';
		recognition.onresult = (event) => {
			let interim = '';
			for (let index = event.resultIndex; index < event.results.length; index += 1) {
				const result = event.results[index];
				const text = result[0]?.transcript ?? '';
				if (result.isFinal) speechSettled += text;
				else interim += text;
			}
			void setSpeechText(`${speechHead}${speechSettled}${interim}`);
		};
		recognition.onerror = (event) => {
			listening = false;
			speech = null;
			const key =
				event.error === 'not-allowed' || event.error === 'service-not-allowed'
					? 'speech_denied'
					: event.error === 'audio-capture'
						? 'speech_no_microphone'
						: 'speech_error';
			toastStore.error(t(`myaaw.chat.${key}`));
		};
		recognition.onend = () => {
			if (!listening) return;
			// Chrome ends a session after a pause; keep the dictation going.
			try {
				recognition.start();
			} catch {
				listening = false;
				speech = null;
			}
		};
		try {
			recognition.start();
		} catch {
			toastStore.error(t('myaaw.chat.speech_error'));
			return;
		}
		speech = recognition;
		listening = true;
	}

	// Leaving the page must not keep the microphone open.
	$effect(() => () => {
		listening = false;
		try {
			speech?.abort();
		} catch {
			// Already stopped.
		}
		speech = null;
	});

	function attachmentUrl(attachment: Attachment) {
		return `data:${attachment.mime_type || 'application/octet-stream'};base64,${attachment.data_base64}`;
	}

	function isImage(attachment: Attachment) {
		return (attachment.mime_type ?? '').startsWith('image/');
	}

	function fileExtension(name: string) {
		const dot = name.lastIndexOf('.');
		return dot >= 0 ? name.slice(dot).toLowerCase() : '';
	}
</script>

<svelte:window
	onclick={() => {
		openStats = false;
		openUsage = false;
	}}
	onkeydown={onShortcutKey}
/>

<TopBarActions id="chat-actions">
	{#snippet children()}
		<TopBarButton label={t('myaaw.chat.new')} icon={MessageSquarePlus} onclick={newChat} />
		<Button
			variant="outline"
			size="icon-sm"
			onclick={toggleHistory}
			title={`${historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')} (${formatShortcut('mod+shift+h')})`}
			aria-label={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
			aria-keyshortcuts="Control+Shift+H Meta+Shift+H"
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
			<div class="relative flex min-h-0 flex-1 flex-col">
				<div bind:this={scroller} class="min-h-0 flex-1 overflow-y-auto" onscroll={onScroll}>
					<div
						class={cn(
							'mx-auto w-full max-w-4xl space-y-5 px-4 py-6',
							railTurns.length > 1 && 'pr-14'
						)}
					>
						{#if loadingOlder}
							<p
								class="flex items-center justify-center gap-2 text-xs text-muted-foreground"
								aria-live="polite"
							>
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
								{t('myaaw.chat.loading_older')}
							</p>
						{/if}
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
												class={cn(
													'h-3.5 w-3.5 shrink-0 transition-transform',
													open && 'rotate-180'
												)}
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
									<div
										class="flex justify-end"
										data-chat-turn={item.localId}
										data-chat-turn-index={item.messageIndex}
									>
										<div class="group/user flex max-w-[80%] flex-col items-end gap-2">
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
											{#if item.uploads?.length}
												<div class="flex flex-wrap justify-end gap-2">
													{#each item.uploads as upload (upload.path || upload.name)}
														{@const downloadPath = uploadVirtualPath(upload)}
														{#if agent && downloadPath}
															<button
																type="button"
																title={t('myaaw.chat.download_file')}
																aria-label={t('myaaw.chat.download_file')}
																class="group/file flex max-w-[16rem] items-center gap-2 rounded-lg border bg-muted/40 px-2 py-1.5 transition-colors hover:border-foreground/30 hover:bg-muted"
																onclick={() => filesApi.download(agent, downloadPath)}
															>
																<FileText class="h-4 w-4 shrink-0 text-muted-foreground" />
																<span class="min-w-0 flex-1 truncate text-xs">{upload.name}</span>
																{#if upload.size}
																	<span class="shrink-0 text-[10px] text-muted-foreground">
																		{formatBytes(upload.size)}
																	</span>
																{/if}
																<Download
																	class="h-3.5 w-3.5 shrink-0 text-muted-foreground group-hover/file:text-foreground"
																/>
															</button>
														{:else}
															<div
																class="flex max-w-[16rem] items-center gap-2 rounded-lg border bg-muted/40 px-2 py-1.5"
															>
																<FileText class="h-4 w-4 shrink-0 text-muted-foreground" />
																<span class="min-w-0 flex-1 truncate text-xs">{upload.name}</span>
																{#if upload.size}
																	<span class="shrink-0 text-[10px] text-muted-foreground">
																		{formatBytes(upload.size)}
																	</span>
																{/if}
															</div>
														{/if}
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
													<CopyButton
														text={item.text}
														class={cn(
															'mt-0.5 opacity-0 transition-opacity group-hover/user:opacity-100 group-focus-within/user:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100',
															item.localId === latestUserKey && 'opacity-100'
														)}
													/>
												</div>
											{/if}
										</div>
									</div>
								{:else if item.kind === 'text'}
									<div class="group/message text-sm">
										<Markdown content={item.text ?? ''} />
										<CopyButton
											text={item.text ?? ''}
											class={cn(
												'mt-1 opacity-0 transition-opacity group-hover/message:opacity-100 group-focus-within/message:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100',
												item.localId === latestAnswerKey && 'opacity-100'
											)}
										/>
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
												<Button
													size="xs"
													variant="secondary"
													onclick={() => decide('allow_session')}
												>
													{t('myaaw.chat.allow_session')}
												</Button>
												<Button
													size="xs"
													variant="secondary"
													onclick={() => decide('allow_always')}
												>
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

						{#if streaming && items[items.length - 1]?.kind === 'user'}
							<!-- The first token can take a while; show that the run started. -->
							<div class="flex items-center gap-2 px-1 py-1 text-xs text-muted-foreground">
								<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
								<span class="font-medium">{t('myaaw.chat.tool_running')}</span>
							</div>
						{/if}

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

				{#if railTurns.length > 1}
					<!-- One tick per user message in the whole conversation, in a short column
					     centred on the right edge. Hovering unfolds the list of questions, each
					     row carrying its own tick, so a tick always shares its line with the
					     question it belongs to. -->
					<div
						class="absolute top-1/2 right-5 z-10 w-6 -translate-y-1/2"
						style="height: {railHeight}px"
						role="group"
						aria-label={t('myaaw.chat.turns', { count: railTurns.length })}
						onmouseenter={() => (railOpen = true)}
						onmouseleave={() => {
							railOpen = false;
							hoverTurnIndex = null;
						}}
					>
						{#if railOpen}
							<div class="absolute top-1/2 right-0 -translate-y-1/2">
								<div
									class="relative w-64 rounded-xl border bg-popover p-1.5 shadow-lg"
									transition:fade={{ duration: 150 }}
								>
									<div
										bind:this={cardList}
										onscroll={updateCardScroll}
										class="max-h-[286px] overflow-y-auto overscroll-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
									>
										{#each railTurns as turn (turn.index)}
											<button
												type="button"
												class={cn(
													'flex h-[26px] w-full cursor-pointer items-center gap-3 rounded-md pr-3 pl-2 text-left text-[13px] transition-colors',
													turn.index === activeTurnIndex
														? 'text-primary'
														: turn.index === hoverTurnIndex
															? 'bg-muted/60 text-foreground'
															: 'text-muted-foreground hover:bg-muted/60'
												)}
												data-turn-active={turn.index === activeTurnIndex ? 'true' : undefined}
												onclick={() => void jumpToTurn(turn)}
												onmouseenter={() => (hoverTurnIndex = turn.index)}
												onmouseleave={() => (hoverTurnIndex = null)}
											>
												<span class="min-w-0 flex-1 truncate">
													{oneLine(turn.text) || t('myaaw.chat.untitled_turn')}
												</span>
												<span
													class={cn(
														'shrink-0 rounded-full transition-all',
														turn.index === activeTurnIndex
															? 'h-1 w-5 bg-primary'
															: turn.index === hoverTurnIndex
																? 'h-1 w-5 bg-muted-foreground'
																: 'h-0.5 w-3.5 bg-muted-foreground/40'
													)}
												></span>
											</button>
										{/each}
									</div>

									{#if cardScrollUp}
										<div
											class="pointer-events-none absolute inset-x-1.5 top-1.5 h-5 rounded-t-lg bg-gradient-to-b from-popover to-transparent"
										></div>
									{/if}
									{#if cardScrollDown}
										<div
											class="pointer-events-none absolute inset-x-1.5 bottom-1.5 h-5 rounded-b-lg bg-gradient-to-t from-popover to-transparent"
										></div>
									{/if}
								</div>
							</div>
						{:else}
							{#each railMarks as entry, position (entry.turn.index)}
								<button
									type="button"
									class="absolute right-0 flex h-3 w-full -translate-y-1/2 cursor-pointer items-center justify-end transition-all duration-200"
									style="top: {RAIL_PAD + position * RAIL_GAP}px; opacity: {entry.turn.index ===
									activeTurnIndex
										? 1
										: entry.opacity}"
									aria-label={oneLine(entry.turn.text) || t('myaaw.chat.jump_to_turn')}
									title={oneLine(entry.turn.text)}
									onclick={() => void jumpToTurn(entry.turn)}
								>
									<span
										class={cn(
											'block rounded-full transition-all duration-200',
											entry.turn.index === activeTurnIndex
												? 'h-1 bg-primary'
												: 'h-0.5 bg-muted-foreground/60'
										)}
										style="width: {entry.turn.index === activeTurnIndex ? 20 : entry.width}px"
									></span>
								</button>
							{/each}
						{/if}
					</div>
				{/if}
			</div>

			{#if runError || error}
				<div class="shrink-0 px-4">
					<div
						class="mx-auto max-w-4xl rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive"
					>
						{runError || error}
					</div>
				</div>
			{/if}

			<div class="shrink-0 border-t bg-background/80 px-4 py-3 backdrop-blur">
				<div class={cn('mx-auto w-full max-w-4xl', railTurns.length > 1 && 'pr-14')}>
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
						{#if pendingFiles.length > 0}
							<div class="flex flex-wrap gap-2 px-3 pt-3">
								{#each pendingFiles as file (file.id)}
									<div
										class="flex max-w-[16rem] items-center gap-2 rounded-lg border bg-muted/40 px-2 py-1.5"
									>
										<FileText class="h-4 w-4 shrink-0 text-muted-foreground" />
										<span class="min-w-0 flex-1 truncate text-xs">{file.name}</span>
										<span class="shrink-0 text-[10px] text-muted-foreground">
											{formatBytes(file.size)}
										</span>
										<button
											type="button"
											class="shrink-0 rounded-full p-0.5 text-muted-foreground transition-colors hover:text-foreground"
											onclick={() => removePendingFile(file.id)}
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
								{t('myaaw.chat.drop_files')}
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
									accept="image/*,.txt,.md,.markdown,.csv,.tsv,.json,.xml,.yaml,.yml,.log,.rtf,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp"
									multiple
									class="hidden"
									onchange={onFileInput}
								/>
								<Button
									type="button"
									size="icon-sm"
									variant="ghost"
									class="shrink-0 rounded-full"
									disabled={pending.length >= MAX_IMAGES && pendingFiles.length >= MAX_FILES}
									onclick={() => fileInput?.click()}
									aria-label={t('myaaw.chat.attach')}
									aria-keyshortcuts="Control+Shift+F Meta+Shift+F"
									title={`${t('myaaw.chat.attach')} (${formatShortcut('mod+shift+f')})`}
								>
									<Paperclip class="h-4 w-4" />
								</Button>
								<Button
									type="button"
									size="icon-sm"
									variant="ghost"
									class={cn('shrink-0 rounded-full', listening && 'text-destructive')}
									onclick={toggleSpeech}
									aria-pressed={listening}
									aria-label={listening
										? t('myaaw.chat.speech_stop')
										: t('myaaw.chat.speech_start')}
									aria-keyshortcuts="Control+Shift+Space Meta+Shift+Space"
									title={`${listening ? t('myaaw.chat.speech_stop') : t('myaaw.chat.speech_start')} (${formatShortcut('mod+shift+space')})`}
								>
									<Mic class={cn('h-4 w-4', listening && 'animate-pulse')} />
								</Button>
								<span class="truncate text-[10px] text-muted-foreground">
									{listening ? t('myaaw.chat.speech_listening') : t('myaaw.chat.hint')}
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
									disabled={!input.trim() && pending.length === 0 && pendingFiles.length === 0}
									aria-label={t('myaaw.chat.send')}
								>
									<Send class="h-4 w-4" />
								</Button>
							{/if}
						</div>
						<div
							class="flex flex-wrap items-center justify-center gap-x-2 gap-y-1 border-t px-4 py-1.5 text-[10px] text-muted-foreground"
						>
							<div class="relative">
								<button
									type="button"
									class={cn(
										'flex items-center gap-1 rounded-md px-1 py-0.5 transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:outline-none',
										openStats && 'text-foreground'
									)}
									onclick={(event) => {
										event.stopPropagation();
										openStats = !openStats;
										openUsage = false;
									}}
									aria-expanded={openStats}
								>
									<Gauge class="h-3 w-3 shrink-0" />
									{turnLabel(statsTurns)} · {stepsLabel(statsSteps)}
									{#if statsTokPerSec > 0}
										· {t('myaaw.chat.stats_tok_per_sec', { count: statsTokPerSec })}
									{/if}
								</button>
								{#if openStats && sessionStats}
									<div
										class="absolute bottom-full left-1/2 z-30 mb-2 w-64 -translate-x-1/2 rounded-lg border bg-popover p-3 text-xs text-popover-foreground shadow-md"
										onclick={(event) => event.stopPropagation()}
										role="presentation"
									>
										<div class="flex items-center gap-2 font-medium">
											<Gauge class="h-3.5 w-3.5" />
											{t('myaaw.chat.stats_session_title')}
										</div>
										<div class="my-2 h-px bg-border"></div>
										<dl class="space-y-1.5">
											<div class="flex justify-between gap-3">
												<dt>{t('myaaw.chat.stats_llm_time')}</dt>
												<dd class="tabular-nums">{formatSeconds(statsLlmMs)}</dd>
											</div>
											<div class="flex justify-between gap-3">
												<dt>{t('myaaw.chat.stats_tool_time')}</dt>
												<dd class="tabular-nums">{formatSeconds(statsToolMs)}</dd>
											</div>
											<div class="flex justify-between gap-3">
												<dt>{t('myaaw.chat.stats_ttft')}</dt>
												<dd class="tabular-nums">{formatSeconds(statsTtftMs)}</dd>
											</div>
											<div class="flex justify-between gap-3">
												<dt>{t('myaaw.chat.stats_tps')}</dt>
												<dd class="tabular-nums">
													{t('myaaw.chat.stats_tok_per_sec', { count: statsTokPerSec })}
												</dd>
											</div>
										</dl>
									</div>
								{/if}
							</div>

							{#if statsModelLine}
								<div class="relative">
									<button
										type="button"
										class={cn(
											'flex items-center gap-1 rounded-md px-1 py-0.5 font-mono italic transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:outline-none',
											openUsage && 'text-foreground'
										)}
										onclick={(event) => {
											event.stopPropagation();
											openUsage = !openUsage;
											openStats = false;
										}}
										aria-expanded={openUsage}
									>
										<Database class="h-3 w-3 shrink-0" />
										{statsModelLine}
									</button>
									{#if openUsage && sessionStats}
										<div
											class="absolute bottom-full left-1/2 z-30 mb-2 w-64 -translate-x-1/2 rounded-lg border bg-popover p-3 text-xs text-popover-foreground shadow-md"
											onclick={(event) => event.stopPropagation()}
											role="presentation"
										>
											<div class="flex items-center justify-between gap-3 font-medium">
												<span class="flex items-center gap-2">
													<Database class="h-3.5 w-3.5" />
													{t('myaaw.chat.stats_usage_title')}
												</span>
												<span class="tabular-nums">
													{t('myaaw.chat.stats_tokens', {
														count: formatCount(statsTokens)
													})}
												</span>
											</div>
											<div class="my-2 h-px bg-border"></div>
											<dl class="space-y-1.5">
												<div class="flex justify-between gap-3">
													<dt>{t('myaaw.chat.stats_cache_hit_label')}</dt>
													<dd class="tabular-nums">{statsCacheRate}%</dd>
												</div>
												<div class="flex justify-between gap-3">
													<dt>{t('myaaw.chat.stats_uncached_input')}</dt>
													<dd class="tabular-nums">
														{t('myaaw.chat.stats_tokens', {
															count: formatCount(statsUncachedTokens)
														})}
													</dd>
												</div>
												<div class="flex justify-between gap-3">
													<dt>{t('myaaw.chat.stats_cached_input')}</dt>
													<dd class="tabular-nums">
														{t('myaaw.chat.stats_tokens', {
															count: formatCount(statsCachedTokens)
														})}
													</dd>
												</div>
												<div class="flex justify-between gap-3">
													<dt>{t('myaaw.chat.stats_output')}</dt>
													<dd class="tabular-nums">
														{t('myaaw.chat.stats_tokens', {
															count: formatCount(statsOutputTokens)
														})}
													</dd>
												</div>
											</dl>
										</div>
									{/if}
								</div>
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
						bind:ref={searchBox}
						bind:value={historySearch}
						placeholder={t('myaaw.conversations.search')}
						class="h-8 pl-7 text-xs"
						aria-label={t('myaaw.conversations.search')}
						aria-keyshortcuts="Control+Shift+S Meta+Shift+S"
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
							'group/entry rounded-md transition-colors',
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
									<span class="flex items-center gap-1.5">
										{#if streamingConversations.has(entry.id) && entry.id !== conversationId}
											<!-- A run left in the background keeps going; flag it until it is
											     opened, where the live stream itself shows the progress. -->
											<Loader2
												class="h-3 w-3 shrink-0 animate-spin text-muted-foreground"
												aria-label={t('myaaw.chat.streaming')}
												data-streaming="true"
											/>
										{/if}
										<span class="min-w-0 truncate text-xs font-medium">
											{entry.title || entry.id.slice(0, 12)}
										</span>
									</span>
									<span class="mt-0.5 block truncate text-[10px] text-muted-foreground">
										{formatWhen(entry.updated_at)} · {t('myaaw.conversations.messages', {
											count: entry.message_count
										})}
									</span>
								</button>
								<div
									class="pointer-events-none flex shrink-0 opacity-0 transition-opacity group-hover/entry:pointer-events-auto group-hover/entry:opacity-100 group-focus-within/entry:pointer-events-auto group-focus-within/entry:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
								>
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
