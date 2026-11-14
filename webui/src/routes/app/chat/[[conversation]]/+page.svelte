<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { tick, untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { chat as chatApi } from '$lib/apis/chat';
	import { config as configApi } from '$lib/apis/config';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { files as filesApi } from '$lib/apis/files';
	import { ensureSigned } from '$lib/stores/mediaSign.svelte';
	import { providers as providersApi } from '$lib/apis/providers';
	import { observability as observabilityApi } from '$lib/apis/observability';
	import { agentSelection, conversationHistory, toastStore, topBarTitle } from '$lib/stores';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { chatRuns, type ChatItem, type ChatRun } from '$lib/stores/chatRuns.svelte';
	import { chatUpdates } from '$lib/stores/conversationEvents.svelte';
	import { unreadChats } from '$lib/stores/unreadChats.svelte';
	import { approvalQueue } from '$lib/stores/approvalQueue.svelte';
	import {
		capabilityFor,
		loadModelGroups as loadCachedModelGroups,
		loadModelCapabilities,
		type ModelGroup
	} from '$lib/stores/modelCatalog';
	import { tts } from '$lib/speech/tts.svelte';
	import { cn } from '$lib/utils';
	import { isOverlayOpen, isTypingTarget, matchesShortcut, formatShortcut } from '$lib/utils/keys';
	import { getErrorMessage } from '$lib/utils/api';
	import { MAX_ASK_QUOTES, MAX_ASK_QUOTE_CHARS, parseAskPrompt } from '$lib/utils/ask';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import ConversationList from '$lib/components/app/ConversationList.svelte';
	import AttachmentPanel from '$lib/components/app/AttachmentPanel.svelte';
	import AttachmentPreview from '$lib/components/app/AttachmentPreview.svelte';
	import AskQuoteList from '$lib/components/app/AskQuoteList.svelte';
	import PendingApprovalCard from '$lib/components/app/PendingApprovalCard.svelte';
	import RunAttachments from '$lib/components/app/RunAttachments.svelte';
	import ImageLightbox from '$lib/components/app/ImageLightbox.svelte';
	import FocusAgentPanel from '$lib/components/focus/FocusAgentPanel.svelte';
	import FocusContextEditor from '$lib/components/focus/FocusContextEditor.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import ChatStep from '$lib/components/common/ChatStep.svelte';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import SpeakButton from '$lib/components/common/SpeakButton.svelte';
	import ChatTime from '$lib/components/common/ChatTime.svelte';
	import Logo from '$lib/components/common/Logo.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf, cycleSrc, moodSrc, type BloubCycle, type BloubMood } from '$lib/bloub';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { theme } from 'mode-watcher';
	import { isFrosted } from '$lib/theme';
	import {
		AlertTriangle,
		Bot,
		Brain,
		CalendarClock,
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
		HeartPulse,
		History,
		ListEnd,
		Loader2,
		MessageSquarePlus,
		Mic,
		Minimize2,
		Monitor,
		PanelRightClose,
		PanelRightOpen,
		Paperclip,
		Pencil,
		Quote,
		RotateCcw,
		Send,
		ShieldCheck,
		ShieldOff,
		Square,
		SquareTerminal,
		Users,
		Wrench,
		X
	} from 'lucide-svelte';
	import type {
		ApprovalDecision,
		AskQuote,
		AskQuoteSource,
		ContextUsage,
		ConversationAttachment,
		ConversationTurn,
		LightboxImage,
		Message,
		ModelCapabilitiesView,
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

	/** One message of a send: the text, the quoted excerpts and the attachments. */
	interface MessagePayload {
		text: string;
		quotes: AskQuote[];
		images: PendingImage[];
		files: PendingFile[];
	}

	/** A message typed while an answer streams; it waits above the composer. */
	interface QueuedMessage extends MessagePayload {
		id: string;
		createdAt: string;
	}

	/** A live text selection in an agent reply, positioned for the Ask button. */
	interface AskAnchor {
		text: string;
		source: AskQuoteSource;
		label?: string;
		sourceId?: string;
		startOffset?: number;
		endOffset?: number;
		top: number;
		left: number;
	}

	const MAX_IMAGES = 6;
	// A saved queue is bounded so a long-lived draft cannot grow without limit.
	const MAX_QUEUED_MESSAGES = 20;
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
	// The classic shell already carries the chat list and a new-chat action, so the
	// page hides its own copies and never opens the duplicate history panel.
	let shellMode = $derived(preferences.value.shellMode);
	let classicMode = $derived(shellMode === 'classic');
	let focusMode = $derived(shellMode === 'focus');
	// In focus mode a context file replaces the transcript column until it closes.
	let focusContextPath = $state('');
	// The right column shows either the conversation list or the files of the open
	// chat. The choice is remembered per browser; a mode change never forces an
	// empty panel open.
	const PANEL_TAB_KEY = 'myaaw_chat_right_tab';
	let rightTab = $state<'history' | 'attachments'>(
		browser && localStorage.getItem(PANEL_TAB_KEY) === 'attachments' ? 'attachments' : 'history'
	);
	// Focus mode owns a tab set of its own; the header can return it to history.
	let focusPanelTab = $state<'history' | 'context' | 'control'>('history');
	// Classic has no right column, so its attachment surface is an overlay drawer.
	let classicAttachmentsOpen = $state(false);
	// Bumped when a run ends so an open attachment list picks up new files.
	let attachmentsRefresh = $state(0);
	let attachmentCount = $state(0);
	// The file whose preview pane replaces the transcript column (classic overlays it).
	let previewAttachment = $state<ConversationAttachment | null>(null);
	// The turn a "Show in chat" jump landed on, so its row can flash a ring.
	let highlightedMessage = $state<number | null>(null);
	let highlightTimer: ReturnType<typeof setTimeout> | undefined;
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
	// Inline edit of one user message, addressed by its row id.
	let editingMessageId = $state('');
	let messageDraft = $state('');
	let draftArea = $state<HTMLTextAreaElement | null>(null);
	let streaming = $derived(activeRun?.streaming ?? false);
	// Ticks the elapsed time shown on the live process header. It stops with the run.
	let now = $state(Date.now());
	$effect(() => {
		if (!streaming) return;
		now = Date.now();
		const timer = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(timer);
	});
	let loading = $state(false);
	// A pending approval can belong to this run, or to a detached one (a sub-agent)
	// announced on the agent's approval stream, which has no run object here.
	let pendingApproval = $derived.by(() => {
		if (activeRun?.pendingApproval) return activeRun.pendingApproval;
		const entry = approvalQueue.pendingFor(agent ?? '', conversationId);
		return entry
			? {
					approval_id: entry.approvalId,
					tool_name: entry.toolName,
					args_summary: entry.argsSummary,
					args: entry.args,
					subagent: entry.subAgent
				}
			: null;
	});
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
		context?: ContextUsage;
	} | null>(null);
	// Configured provider and its default model, as the terminal status line shows.
	let providerInfo = $state<{
		default: string;
		providers: { name: string; type: string; model: string; thinkingLevel: string }[];
	} | null>(null);
	// The composer's quick switches. The model menu lists every model of every
	// configured provider; the list is fetched the first time the menu opens and
	// cached for the rest of the page load.
	let modelGroups = $state<ModelGroup[] | null>(null);
	let modelGroupsKey = '';
	let modelMenuOpen = $state(false);
	let loadingModels = $state(false);
	let modelCapabilities = $state<ModelCapabilitiesView | null>(null);
	// Thinking level is persisted per provider (the agent only ever runs the
	// active provider). Keyed by provider:model so a pending choice survives a
	// model switch within the same page load.
	let thinkingLevels = $state<Record<string, string>>({});
	let providerInfoLoaded = $state(false);
	let switchingModel = $state(false);
	let switchingThinking = $state(false);
	// Approvals mode for the next message: "off" is Full access, "manual" asks.
	let approvalMode = $state<'manual' | 'off'>('manual');
	let switchingApproval = $state(false);
	let pending = $state<PendingImage[]>([]);
	let pendingFiles = $state<PendingFile[]>([]);
	// Whether the composer holds something worth sending; while a run streams the
	// action button swaps from Stop to queue-send only once this is true.
	let canSend = $derived(input.trim().length > 0 || pending.length > 0 || pendingFiles.length > 0);
	// How a send is handled while the agent is still answering. The two modes are
	// mutually exclusive in preferences; `none` leaves the composer editable but
	// unsendable until the run stops.
	let sendWhileStreaming = $derived<'queue' | 'interrupt' | 'none'>(
		preferences.value.queueMessages
			? 'queue'
			: preferences.value.interruptOnSend
				? 'interrupt'
				: 'none'
	);
	// The key that promotes the queue head; the first row advertises it whenever the
	// composer is empty, because that is when pressing it sends the first message.
	let queueKeyLabel = $derived(
		formatShortcut(preferences.value.sendKey === 'mod-enter' ? 'mod+enter' : 'enter')
	);
	// Excerpts the reader highlighted in an agent reply and attached to the next
	// message; the whole set travels with one send.
	let quotes = $state<AskQuote[]>([]);
	let askCounter = 0;
	// Messages sent while a run streams wait here. When a response finishes the
	// first one goes out on its own, so the conversation keeps moving.
	let queue = $state<QueuedMessage[]>([]);
	let queueOpen = $state(true);
	// The floating Ask button, anchored to the current range in viewport units.
	let askAnchor = $state<AskAnchor | null>(null);
	// Sent-message quotes opened in the preview dialog.
	let quotePreview = $state<AskQuote[] | null>(null);
	// The temporary overlay rectangles that mark exactly the quoted passage.
	let quoteRects = $state<{ left: number; top: number; width: number; height: number }[]>([]);
	let quoteMarkTimer: ReturnType<typeof setTimeout> | undefined;
	// Jumps to a quote bypass pagination until the scroll settles; paging older
	// messages mid-jump would rebuild the rows the reader is being sent to.
	let locatingQuote = false;
	let locateTimer: ReturnType<typeof setTimeout> | undefined;
	// Photos from user turns use this shared viewer; agent attachments mount their
	// own instance inside RunAttachments.
	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let lightboxItems = $state<LightboxImage[]>([]);

	/** Opens the photos of one user turn with the clicked image selected. */
	function openImageLightbox(sources: string[], startIndex: number) {
		lightboxItems = sources.map((src, index) => ({ src, name: imageFileName(src, index) }));
		lightboxIndex = Math.min(Math.max(startIndex, 0), Math.max(0, sources.length - 1));
		lightboxOpen = true;
	}

	/** User uploads arrive as data URLs, so the download name comes from the MIME type. */
	function imageFileName(src: string, index: number): string {
		const mime = /^data:image\/([a-z0-9.+-]+)/i.exec(src)?.[1]?.toLowerCase() ?? 'jpeg';
		const ext = mime === 'jpeg' ? 'jpg' : mime.replace(/[^a-z0-9]/g, '') || 'jpg';
		return `image-${index + 1}.${ext}`;
	}

	/** Data-URL photos have no virtual path, so the browser saves them directly. */
	function downloadImage(src: string, name: string) {
		const link = document.createElement('a');
		link.href = src;
		link.download = name;
		link.rel = 'noopener';
		document.body.appendChild(link);
		link.click();
		link.remove();
	}
	let dragOver = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);
	let groupOpen = $state<Record<string, boolean>>({});
	let error = $state('');
	// The two status-line popovers open on click and close on an outside click.
	let openStats = $state(false);
	let openUsage = $state(false);
	let scroller = $state<HTMLDivElement | null>(null);
	let transcript = $state<HTMLDivElement | null>(null);
	let composer = $state<HTMLTextAreaElement | null>(null);
	// True while the transcript sits at the end, so a growing answer may follow it.
	// Scrolling away clears it until the reader comes back.
	let stickToBottom = $state(true);
	// The composer floats over the transcript, so the transcript is padded by its
	// measured height: the last message can always be scrolled above it.
	let composerHeight = $state(0);
	// The frosted themes float the composer over the transcript; every other theme
	// keeps it in its own row, the way it has always been.
	let frosted = $derived(isFrosted(theme.current));

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
	// A touch screen has no hover, so the same list is also reachable by tapping
	// the rail; it then floats over the transcript until the reader picks a row.
	let railSheetOpen = $state(false);
	let cardList = $state<HTMLDivElement | null>(null);
	// The list hides its scrollbar, so the edges tell whether it can be scrolled.
	let cardScrollUp = $state(false);
	let cardScrollDown = $state(false);

	// A question asked in a live run is not in the summary yet; it gets a negative
	// index so the rail can still show and light it.
	let railTurns = $derived.by(() => {
		// A loaded row knows the question as the transcript parsed it (and hides
		// context tags). Prefer that over the server summary, which may predate the
		// current stripping rules, while keeping the summary for rows not loaded yet.
		const local = new Map<number, string>();
		for (const mark of turnMarks) {
			if (mark.text) local.set(mark.index, mark.text);
		}
		const unique = new Map<number, { index: number; text: string }>();
		for (const turn of turns) {
			unique.set(turn.index, {
				index: turn.index,
				text: turnLabelText(local.get(turn.index) ?? turn.text)
			});
		}
		for (const mark of turnMarks) {
			if (!unique.has(mark.index)) {
				unique.set(mark.index, { index: mark.index, text: turnLabelText(mark.text) });
			}
		}
		return [...unique.values()];
	});

	/**
	 * Strips model-only blocks from a turn label. The server summary is normally
	 * clean, but a row from an older server may carry a whole <selections> block (or
	 * its truncated head) and must never show it in the rail.
	 */
	function turnLabelText(raw: string): string {
		const hadSelections = /<selections\b/i.test(raw);
		const stripped = raw
			.replace(/<selections\b[^>]*>[\s\S]*?<\/selections>\s*/gi, '')
			.replace(/<selections\b[\s\S]*$/i, '')
			.replace(/<userinfo>[\s\S]*?<\/userinfo>\s*/gi, '')
			.replace(/<attached>[\s\S]*?<\/attached>\s*/gi, '')
			.replace(/<trigger\b[^>]*>[\s\S]*?<\/trigger>\s*/gi, '')
			.replace(/<trigger\b[\s\S]*$/i, '')
			.trim();
		return !stripped && hadSelections ? t('myaaw.chat.quoted_turn') : stripped;
	}

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
	// Messages fetched per transcript page; scrolling to the top fetches the page
	// before the loaded one.
	const PAGE_SIZE = 20;
	const HISTORY_KEY = 'myaaw_chat_history_open';
	const LAST_CONVERSATION_KEY = 'myaaw_last_conversation';

	let historyOpen = $state(
		browser ? window.innerWidth >= 1024 && localStorage.getItem(HISTORY_KEY) !== 'false' : false
	);
	let searchBox = $state<HTMLInputElement | null>(null);

	$effect(() => {
		if (browser) localStorage.setItem(PANEL_TAB_KEY, rightTab);
	});

	// A shell change never leaves an overlay from the previous mode on screen.
	let lastShellMode = '';
	$effect(() => {
		const mode = shellMode;
		if (mode === lastShellMode) return;
		lastShellMode = mode;
		classicAttachmentsOpen = false;
		previewAttachment = null;
	});

	function scrollToBottom() {
		stickToBottom = true;
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	/** Whether the transcript is close enough to the end to keep following it. */
	function nearBottom(slack = 80) {
		if (!scroller) return true;
		return scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= slack;
	}

	// Images and videos report their real size after they load, which grows the
	// transcript outside the item-change effect. Follow the bottom again when that
	// happens, otherwise the media lands below the composer.
	$effect(() => {
		const node = transcript;
		if (!node || typeof ResizeObserver === 'undefined') return;
		const observer = new ResizeObserver(() => {
			if (preferences.value.autoScroll && stickToBottom) scrollToBottom();
		});
		observer.observe(node);
		return () => observer.disconnect();
	});

	/** How much text the last row holds; it changes on every streamed token. */
	function tailGrowth(item: Item | undefined): number {
		if (!item) return 0;
		return (item.text?.length ?? 0) + (item.arguments?.length ?? 0) + (item.result?.length ?? 0);
	}

	/** Drops the rail and the paging cursor, for when the transcript is cleared. */
	function clearRail() {
		turns = [];
		turnMarks = [];
		loadedOffset = 0;
		activeTurnIndex = null;
		hoverTurnIndex = null;
		railOpen = false;
		railSheetOpen = false;
	}

	/** True where the pointer cannot hover, which is where the rail is tapped open. */
	function touchOnly() {
		return browser && window.matchMedia('(hover: none)').matches;
	}

	/**
	 * A tap on a tick: with a mouse the tick jumps where it points, but on a touch
	 * screen the same tap unfolds the list first, because a 20px mark says nothing
	 * about the question behind it.
	 */
	function tapTick(turn: ConversationTurn) {
		if (!touchOnly()) {
			void jumpToTurn(turn);
			return;
		}
		railSheetOpen = true;
		hoverTurnIndex = turn.index;
	}

	/** Picking a question from the unfolded list closes it on the way to the answer. */
	function pickTurn(turn: ConversationTurn) {
		railSheetOpen = false;
		void jumpToTurn(turn);
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
				const text = turnLabelText(texts.get(index) || storedTexts.get(index) || '');
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
		stickToBottom = nearBottom();
		syncActiveTurn();
		// The Ask button is placed in viewport coordinates; scrolling moves the
		// selection out from under it, so it closes instead of pointing elsewhere.
		askAnchor = null;
		// Near the top the previous page is pulled in, keeping the reader's place.
		if (scroller && scroller.scrollTop < 120 && !locatingQuote) void loadOlder();
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
		if (!(railOpen || railSheetOpen) || !cardList) return;
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

	// Follow only new timeline rows and the growing tail of a streamed answer.
	// Expanding a step reassigns `items` with the same length, and that must not
	// yank the view to the bottom. Neither must prepending an older page. A sent
	// message always jumps into view; a growing answer is followed only when the
	// reader asked for it and has not scrolled away.
	let followedCount = 0;
	let followedTail = -1;
	$effect(() => {
		const count = items.length;
		const tail = items[count - 1];
		const growth = tailGrowth(tail);
		if (count === followedCount && growth === followedTail) return;
		const isNewRow = count !== followedCount;
		followedCount = count;
		followedTail = growth;
		if (prepending) return;
		if (isNewRow && tail?.kind === 'user') {
			scrollToBottom();
			return;
		}
		if (preferences.value.autoScroll && stickToBottom) scrollToBottom();
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
		attachmentCount = 0;
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
			// Sign every media path on this page once, so photos load without the
			// session token and stay valid for their short lifetime.
			const mediaPaths: string[] = [];
			for (const message of detail.messages) {
				for (const attachment of message.attachments ?? []) {
					if (attachment.path) mediaPaths.push(attachment.path);
				}
				for (const upload of message.uploads ?? []) {
					const virtual = uploadVirtualPath(upload);
					if (virtual) mediaPaths.push(virtual);
				}
			}
			void ensureSigned(currentAgent, mediaPaths);
			// A finished run is redundant once the stored transcript is loaded; the
			// server's copy includes the final answer. A live run keeps going.
			chatRuns.settle(currentAgent, id);
			const title = detail.title ?? '';
			applyTitle(id, title);
			unreadChats.ack(currentAgent, id, detail.updated_at);
			if (isPlaceholderTitle(title)) pollTitle(currentAgent, id, 0);
			storedUsage = null;
			void refreshAttachmentCount();
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
	 * Appends a background turn without blanking the transcript: the tail page is
	 * fetched, older rows the reader already loaded are kept, and the tail is
	 * replaced. A finished run is dropped so its rows give way to the stored
	 * transcript. The reader keeps their place unless they are at the bottom.
	 */
	let appending = false;
	async function appendNew() {
		const currentAgent = agent ?? '';
		if (!currentAgent || !conversationId || appending) return;
		// A streaming run owns the timeline; it picks the new turn up when it ends.
		if (activeRun?.streaming) return;
		appending = true;
		const follow = preferences.value.autoScroll && stickToBottom;
		try {
			const detail = await conversationsApi.get(currentAgent, conversationId, {
				limit: PAGE_SIZE,
				offset: -1
			});
			if (conversationId === '' || detail.messages.length === 0) return;

			// Keep the older pages the reader may have scrolled back to; the server's
			// tail replaces everything from its offset on, so the background turn
			// lands in order and a just-finished run's live rows are reconciled.
			const kept = historyItems.filter(
				(item) => item.messageIndex === undefined || item.messageIndex < detail.offset
			);
			const known = new Set(kept.map((item) => item.localId));
			const tail = historyTimeline(detail.messages, detail.offset).filter(
				(item) => !known.has(item.localId)
			);
			historyItems = [...kept, ...tail];
			loadedOffset = Math.min(loadedOffset, detail.offset);
			turns = detail.turns ?? turns;
			chatRuns.settle(currentAgent, conversationId);
			await tick();
			measureTurns();
			if (follow) scrollToBottom();
		} catch {
			// A failed tail fetch is not worth an error banner; the next event retries.
		} finally {
			appending = false;
		}
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
		conversationHistory.update(id, title);
		approvalQueue.setTitle(agent ?? '', id, title);
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
				ttftMs: detail.ttft_ms ?? 0,
				context: detail.context
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
					model: provider.default_model ?? '',
					thinkingLevel: provider.thinking_level ?? ''
				}))
			};
			// Restore the persisted level for each configured provider/model pair.
			const restored: Record<string, string> = {};
			for (const provider of providerInfo.providers) {
				if (provider.model) {
					restored[thinkingKey(provider.name, provider.model)] = provider.thinkingLevel;
				}
			}
			thinkingLevels = restored;
			approvalMode = config.approvals?.mode === 'off' ? 'off' : 'manual';
		} catch {
			providerInfo = null;
		} finally {
			providerInfoLoaded = true;
		}
	}

	/**
	 * Every model of every configured provider, grouped per provider. The store
	 * caches live discovery for the rest of the page load, so only the first open
	 * after a refresh (or after the provider set changes) reaches the API.
	 */
	async function loadModelGroups() {
		const currentAgent = agent;
		if (!currentAgent) return;
		if (modelGroupsKey === currentAgent && modelGroups) return;
		const providers = providerInfo?.providers ?? [];
		if (providers.length === 0) return;
		loadingModels = true;
		try {
			const [groups, caps] = await Promise.all([
				loadCachedModelGroups(currentAgent, providers),
				loadModelCapabilities().catch(() => null)
			]);
			if (agent !== currentAgent) return;
			modelGroups = groups;
			modelGroupsKey = currentAgent;
			if (caps) modelCapabilities = caps;
		} finally {
			loadingModels = false;
		}
	}

	/** Points the agent at one provider and model; the next message uses it. */
	async function switchModel(provider: string, model: string) {
		const currentAgent = agent;
		if (!currentAgent || !provider) return;
		const isCurrent =
			providerInfo?.default === provider &&
			providerInfo.providers.find((entry) => entry.name === provider)?.model === model;
		if (isCurrent) return;
		switchingModel = true;
		try {
			if (providerInfo?.providers.find((entry) => entry.name === provider)?.model !== model) {
				await providersApi.update(currentAgent, provider, { default_model: model });
			}
			if (providerInfo?.default !== provider) {
				await providersApi.setDefault(currentAgent, provider);
			}
			if (agent !== currentAgent) return;
			if (providerInfo) {
				providerInfo = {
					default: provider,
					providers: providerInfo.providers.map((entry) =>
						entry.name === provider ? { ...entry, model } : entry
					)
				};
				// The new model inherits the provider's persisted level.
				const level =
					providerInfo.providers.find((entry) => entry.name === provider)?.thinkingLevel ?? '';
				thinkingLevels = { ...thinkingLevels, [thinkingKey(provider, model)]: level };
			}
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			switchingModel = false;
		}
	}

	/** Full access turns the approval gate off; approval keeps it manual. */
	async function switchApproval(mode: 'manual' | 'off') {
		const currentAgent = agent;
		if (!currentAgent || approvalMode === mode) return;
		const previous = approvalMode;
		approvalMode = mode;
		switchingApproval = true;
		try {
			const config = await configApi.patch(currentAgent, { approvals: { mode } });
			if (agent !== currentAgent) return;
			approvalMode = config.approvals?.mode === 'off' ? 'off' : 'manual';
		} catch (err) {
			approvalMode = previous;
			toastStore.error(getErrorMessage(err));
		} finally {
			switchingApproval = false;
		}
	}

	// The configured pair the menu and its trigger show, not the last run's pair:
	// this is what the next message will use.
	let activeProvider = $derived(providerInfo?.default ?? '');
	let activeModel = $derived(
		providerInfo?.providers.find((provider) => provider.name === providerInfo?.default)?.model ?? ''
	);

	/** Gemini ids carry a `models/` namespace that the picker hides. */
	function modelDisplayName(model: string) {
		return model.startsWith('models/') ? model.slice('models/'.length) : model;
	}

	const CAPABILITY_CHIPS = [
		['thinking', 'myaaw.chat.model_cap_thinking'],
		['vision', 'myaaw.chat.model_cap_vision'],
		['function_calling', 'myaaw.chat.model_cap_tools'],
		['parallel_function_calling', 'myaaw.chat.model_cap_parallel'],
		['structured_outputs', 'myaaw.chat.model_cap_structured'],
		['json_mode', 'myaaw.chat.model_cap_json'],
		['streaming', 'myaaw.chat.model_cap_streaming'],
		['prompt_caching', 'myaaw.chat.model_cap_cache'],
		['batch_api', 'myaaw.chat.model_cap_batch']
	] as const;

	function capabilityChips(model: string) {
		const flags = capabilityFor(modelCapabilities, model)?.capabilities;
		if (!flags) return [];
		return CAPABILITY_CHIPS.filter(([key]) => flags[key] === true);
	}

	function contextWindowLabel(model: string) {
		const tokens = capabilityFor(modelCapabilities, model)?.context_window_tokens;
		if (!tokens) return '';
		if (tokens >= 1_000_000) return `${Number((tokens / 1_000_000).toFixed(1))}M`;
		if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}K`;
		return String(tokens);
	}

	function modelStatusLabel(status?: string) {
		if (!status || status === 'active') return '';
		if (status === 'preview') return t('myaaw.chat.model_status_preview');
		if (status === 'deprecated') return t('myaaw.chat.model_status_deprecated');
		return t('myaaw.chat.model_status_unknown');
	}

	function thinkingOptions(model: string) {
		const entry = capabilityFor(modelCapabilities, model);
		if (entry?.capabilities?.thinking !== true) return [];
		const values = [...(entry.thinking?.effort_values ?? [])];
		if (
			entry.thinking?.can_disable === true &&
			!values.some((value) => value === 'none' || value === 'off')
		) {
			values.unshift('off');
		}
		return values;
	}

	function thinkingLabel(value: string) {
		if (value === 'none' || value === 'off') return t('myaaw.chat.thinking_off');
		if (value === 'xhigh') return 'XHigh';
		return value.charAt(0).toUpperCase() + value.slice(1);
	}

	function thinkingKey(provider: string, model: string) {
		return `${provider}:${model}`;
	}

	/** Persists the reasoning level for the active provider and updates the picker. */
	async function switchThinkingLevel(value: string) {
		const currentAgent = agent;
		const provider = activeProvider;
		if (!currentAgent || !provider) return;
		const key = activeThinkingKey;
		const previous = activeThinkingValue;
		if (previous === value) return;
		thinkingLevels = { ...thinkingLevels, [key]: value };
		switchingThinking = true;
		try {
			await providersApi.update(currentAgent, provider, { thinking_level: value });
			if (agent !== currentAgent) return;
			if (providerInfo) {
				providerInfo = {
					...providerInfo,
					providers: providerInfo.providers.map((entry) =>
						entry.name === provider ? { ...entry, thinkingLevel: value } : entry
					)
				};
			}
		} catch (err) {
			thinkingLevels = { ...thinkingLevels, [key]: previous };
			toastStore.error(getErrorMessage(err));
		} finally {
			switchingThinking = false;
		}
	}

	let activeThinkingKey = $derived(thinkingKey(activeProvider, activeModel));
	let activeThinkingOptions = $derived(thinkingOptions(activeModel));
	// A persisted value may predate a catalog change (for example a Gemini 2.5
	// level the adapter can no longer send). Treat it as the model default until
	// the user picks again.
	let activeThinkingValue = $derived(
		activeThinkingOptions.includes(thinkingLevels[activeThinkingKey] ?? '')
			? (thinkingLevels[activeThinkingKey] ?? '')
			: ''
	);

	let providerKey = '';
	$effect(() => {
		const currentAgent = agentSelection.selected;
		if (!currentAgent || currentAgent === providerKey) return;
		providerKey = currentAgent;
		providerInfo = null;
		providerInfoLoaded = false;
		modelGroups = null;
		modelGroupsKey = '';
		void loadProviderInfo(currentAgent);
	});

	// Opening the menu before the config has arrived must not leave it empty: the
	// effect retries as soon as the provider list is available.
	$effect(() => {
		const currentAgent = agent;
		const providers = providerInfo?.providers;
		if (!modelMenuOpen || !currentAgent || !providers?.length) return;
		untrack(() => void loadModelGroups());
	});

	// The thinking control lives in the composer footer, so the capability catalog
	// must arrive without the model menu being opened first.
	$effect(() => {
		if (!agent) return;
		untrack(() => {
			loadModelCapabilities()
				.then((caps) => {
					if (caps) modelCapabilities = caps;
				})
				.catch(() => {
					// Without the catalog the footer hides the thinking control.
				});
		});
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
	let chatAgentKey = '';
	// A folder picked in the sidebar before the first message exists; it is
	// applied once the server creates the conversation.
	let pendingFolder = $state('');
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
		previewAttachment = null;
		attachmentCount = 0;
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
		if (!currentAgent) return;

		// The dashboard's (and a folder's) "new chat" action opts out of restoring
		// history. The folder stays in the URL until the first message creates the
		// conversation. This runs before the key guard so a second "new chat" in
		// the same folder still resets the composer.
		if (!currentConversation && page.url.searchParams.has('new')) {
			pendingFolder = page.url.searchParams.get('folder') ?? '';
			resetChat();
			// Drop the flag so a refresh keeps the empty composer instead of
			// clearing it again.
			const suffix = pendingFolder ? `?folder=${encodeURIComponent(pendingFolder)}` : '';
			void goto(`/app/chat${suffix}`, { replaceState: true, noScroll: true });
			return;
		}

		const key = `${currentAgent}:${currentConversation}`;
		if (key === loadKey) return;

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
		focusContextPath = '';
		previewAttachment = null;
		classicAttachmentsOpen = false;
		rememberConversation(currentConversation);
		void loadConversation(currentAgent, currentConversation);
	});

	// Background turns (cron, heartbeat, sub-agent) arrive over SSE from the
	// agent server. Append them to the open transcript instead of reloading it;
	// the unread marker is owned by conversationEvents, which knows this
	// conversation is open and acknowledges instead of marking it.
	$effect(() => {
		if (!browser) return;
		return chatUpdates.subscribe((event) => {
			if (event.agent !== agent || event.conversation_id !== conversationId) return;
			void appendNew();
		});
	});

	// Tells the unread store which conversation is on screen, so a list sync never
	// paints a dot on the chat the reader is already looking at.
	$effect(() => {
		unreadChats.setOpen(agent ?? '', conversationId);
		return () => unreadChats.setOpen('', '');
	});

	// One draft per agent and conversation, so switching chats keeps what was typed
	// and sending removes it. `draftKey` guards against writing the old text under
	// the conversation that was just opened.
	let draftKey = '';

	function draftStorageKey(who: string, id: string) {
		return `myaaw_draft:${who}:${id || 'new'}`;
	}

	function draftQuotesStorageKey(who: string, id: string) {
		return `myaaw_draft_quotes:${who}:${id || 'new'}`;
	}

	/** Queued messages ride with the draft, so a refresh keeps what was not sent yet. */
	function draftQueueStorageKey(who: string, id: string) {
		return `myaaw_draft_queue:${who}:${id || 'new'}`;
	}

	/** Restores excerpts saved with a draft, dropping anything malformed. */
	function readDraftQuotes(key: string): AskQuote[] {
		if (!browser) return [];
		try {
			const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? '[]');
			if (!Array.isArray(parsed)) return [];
			const restored: AskQuote[] = [];
			for (const item of parsed) {
				if (!item || typeof item !== 'object') continue;
				const quote = item as Partial<AskQuote>;
				if (typeof quote.text !== 'string' || !quote.text.trim()) continue;
				if (quote.source !== 'answer' && quote.source !== 'thinking' && quote.source !== 'tool') {
					continue;
				}
				restored.push({
					id: typeof quote.id === 'string' && quote.id ? quote.id : `ask-${restored.length + 1}`,
					text: quote.text.slice(0, MAX_ASK_QUOTE_CHARS),
					source: quote.source,
					label: typeof quote.label === 'string' && quote.label ? quote.label : undefined,
					sourceId:
						typeof quote.sourceId === 'string' && quote.sourceId ? quote.sourceId : undefined,
					startOffset: typeof quote.startOffset === 'number' ? quote.startOffset : undefined,
					endOffset: typeof quote.endOffset === 'number' ? quote.endOffset : undefined
				});
				if (restored.length >= MAX_ASK_QUOTES) break;
			}
			return restored;
		} catch {
			return [];
		}
	}

	/** Keeps new ids from colliding with the ones a restored draft already used. */
	function nextAskCounter(items: AskQuote[]): number {
		return items.reduce((max, quote) => {
			const match = /^ask-(\d+)$/.exec(quote.id);
			return match ? Math.max(max, Number(match[1])) : max;
		}, 0);
	}

	/** Restores the messages queued in a conversation, dropping anything malformed. */
	function readDraftQueue(key: string): QueuedMessage[] {
		if (!browser) return [];
		try {
			const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? '[]');
			if (!Array.isArray(parsed)) return [];
			const restored: QueuedMessage[] = [];
			for (const item of parsed) {
				if (!item || typeof item !== 'object') continue;
				const entry = item as Partial<QueuedMessage>;
				const text = typeof entry.text === 'string' ? entry.text : '';
				const images = Array.isArray(entry.images)
					? entry.images.filter(
							(image) =>
								!!image && typeof image.base64 === 'string' && typeof image.dataUrl === 'string'
						)
					: [];
				const files = Array.isArray(entry.files)
					? entry.files.filter(
							(file) => !!file && typeof file.base64 === 'string' && typeof file.name === 'string'
						)
					: [];
				if (!text.trim() && images.length === 0 && files.length === 0) continue;

				const quotes = Array.isArray(entry.quotes)
					? entry.quotes.filter(
							(quote): quote is AskQuote =>
								!!quote &&
								typeof quote === 'object' &&
								typeof (quote as AskQuote).text === 'string' &&
								['answer', 'thinking', 'tool'].includes((quote as AskQuote).source)
						)
					: [];

				restored.push({
					id: typeof entry.id === 'string' && entry.id ? entry.id : crypto.randomUUID(),
					text,
					quotes,
					images,
					files,
					createdAt:
						typeof entry.createdAt === 'string' && entry.createdAt
							? entry.createdAt
							: new Date().toISOString()
				});
				if (restored.length >= MAX_QUEUED_MESSAGES) break;
			}
			return restored;
		} catch {
			return [];
		}
	}

	$effect(() => {
		if (!browser) return;
		const key = draftStorageKey(agent ?? '', conversationId);
		if (key === draftKey) return;
		// A new chat's first answer binds its conversation id: whatever was queued
		// before that belongs to the chat being created, not to the `new` slot.
		const carried = draftKey === draftStorageKey(agent ?? '', '');
		draftKey = key;
		const keep = preferences.value.keepDrafts;
		input = keep ? (localStorage.getItem(key) ?? '') : '';
		const restored = keep
			? readDraftQuotes(draftQuotesStorageKey(agent ?? '', conversationId))
			: [];
		quotes = restored;
		askCounter = nextAskCounter(restored);
		askAnchor = null;
		clearQuoteFlash();
		if (keep && carried && queue.length > 0) {
			// The queue follows the id; drop the temporary key so the next new chat
			// does not inherit these messages.
			localStorage.removeItem(draftQueueStorageKey(agent ?? '', ''));
		} else {
			queue =
				keep && preferences.value.queueMessages
					? readDraftQueue(draftQueueStorageKey(agent ?? '', conversationId))
					: [];
		}
		queueOpen = true;
	});

	$effect(() => {
		if (!browser || !draftKey) return;
		const key = draftStorageKey(agent ?? '', conversationId);
		if (key !== draftKey) return;
		if (preferences.value.keepDrafts && input) localStorage.setItem(key, input);
		else localStorage.removeItem(key);
	});

	// Quotes are part of the same draft: a refresh must bring back the excerpts
	// even when no question was typed yet.
	$effect(() => {
		if (!browser || !draftKey) return;
		const key = draftStorageKey(agent ?? '', conversationId);
		if (key !== draftKey) return;
		const quotesKey = draftQuotesStorageKey(agent ?? '', conversationId);
		if (preferences.value.keepDrafts && quotes.length > 0) {
			localStorage.setItem(quotesKey, JSON.stringify(quotes));
		} else {
			localStorage.removeItem(quotesKey);
		}
	});

	// The queue is part of the draft too: a refresh, a menu switch, or another chat
	// must bring back what was queued here but not sent yet. When the feature is off
	// the stored queue is left untouched, so re-enabling it restores the messages.
	$effect(() => {
		if (!browser || !draftKey || !preferences.value.queueMessages) return;
		const key = draftStorageKey(agent ?? '', conversationId);
		if (key !== draftKey) return;
		const queueKey = draftQueueStorageKey(agent ?? '', conversationId);
		if (preferences.value.keepDrafts && queue.length > 0) {
			try {
				localStorage.setItem(queueKey, JSON.stringify(queue));
			} catch {
				// Attachments can outgrow the storage quota; keep the queue in memory
				// rather than showing it out of sync with what would be restored.
			}
		} else {
			localStorage.removeItem(queueKey);
		}
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
				if (pendingFolder) {
					const folderId = pendingFolder;
					pendingFolder = '';
					// move() refreshes the sections, including the new title.
					void conversationHistory.move(id, folderId);
				} else {
					conversationHistory.refresh();
				}
				void goto(`/app/chat/${id}`, { replaceState: true, noScroll: true });
			},
			onDone: (run: ChatRun) => {
				const currentAgent = agent ?? '';
				const statsId = conversationId;
				void refreshStats(currentAgent, statsId);
				void refreshTitle(currentAgent, statsId);
				// A run may have delivered files: refresh the panel and the badge.
				attachmentsRefresh += 1;
				void refreshAttachmentCount();
				// The run may land in the trace store just after `done`.
				setTimeout(() => void refreshStats(currentAgent, statsId), 1500);

				// The completion tone and the browser notification live in
				// stores/runCompletion: they must also fire when this page is gone.

				// A background turn may have landed while this run held the timeline;
				// reconcile the stored transcript now that the run is done.
				void appendNew();

				// Only an answer that was produced while the reader was here is read
				// aloud: a restored run or a background tab stays quiet.
				if (
					preferences.value.speakResponses &&
					!run.error &&
					!run.resumed &&
					document.visibilityState === 'visible'
				) {
					const answer = [...run.items].reverse().find((item) => item.kind === 'text');
					if (answer?.text) tts.speak(answer.localId ?? '', answer.text);
				}
			},
			onError: (message: string) => {
				error = message;
			}
		};
		run.view = view;
		// Run completion outside this reactive effect: `onDone` reads and writes
		// page state, and tracking those reads would re-trigger the effect forever.
		if (run.finishedAt > 0) queueMicrotask(() => run.notifyDone());
		return () => {
			if (run.view === view) run.view = null;
		};
	});

	// A reloaded tab restores the run before the transcript page arrives. Seed the
	// run with those rows so the previous turns do not disappear while it streams.
	$effect(() => {
		const run = activeRun;
		if (!run?.resumed || historyItems.length === 0) return;
		if (run.adopt(historyItems) && stickToBottom) scrollToBottom();
	});

	// An answer just ended: the reader is most likely typing next, and the mark
	// plays one notification loop before settling back to idle.
	let wasStreaming = false;
	let completed = $state(false);
	let completedTimer: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const now = streaming;
		const run = activeRun;
		if (wasStreaming && !now) {
			void focusComposer();
			// The response just ended (finished, stopped or failed), so the head of the
			// queue goes out next. A run picked up after a reload does not drain: the
			// reader refreshed, so the queue must stay exactly as it was. Deferred
			// outside this tracking effect: draining starts a run, and that must not
			// look like a dependency.
			if (!run?.resumed) queueMicrotask(() => untrack(drainQueue));
			if (!runError && !error && preferences.value.notifyEnabled) {
				completed = true;
				clearTimeout(completedTimer);
				// Show the completion notification for two seconds, then idle again.
				completedTimer = setTimeout(() => (completed = false), 2000);
			}
		}
		if (now) {
			completed = false;
			clearTimeout(completedTimer);
		}
		wasStreaming = now;
	});
	$effect(() => () => clearTimeout(completedTimer));

	// Speech belongs to the conversation that started it; another chat, another
	// agent, or leaving the page stops the audio instead of talking over it.
	let spokenScope = '';
	$effect(() => {
		const scope = `${agent ?? ''}:${conversationId}`;
		if (scope === spokenScope) return;
		spokenScope = scope;
		editingMessageId = '';
		messageDraft = '';
		// Selections quote this conversation, so they do not follow the reader
		// into another chat. The draft effect restores the new chat's saved
		// excerpts instead of clearing them here.
		askAnchor = null;
		clearQuoteFlash();
		locatingQuote = false;
		clearTimeout(locateTimer);
		untrack(() => tts.stop());
	});

	$effect(() => () => tts.stop());

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
		// A completed turn always shows its final answer, even while the next turn
		// streams. Only the current turn's trailing text is held back until the run
		// ends, because more tool calls may follow it.
		const answers = new Set<number>();
		let lastUser = -1;
		for (let index = items.length - 1; index >= 0; index -= 1) {
			if (items[index].kind === 'user') {
				lastUser = index;
				break;
			}
		}
		const completedEnd = lastUser < 0 ? 0 : lastUser + 1;
		let pending = -1;
		for (let index = 0; index < completedEnd; index += 1) {
			const item = items[index];
			if (item.kind === 'user') {
				if (pending >= 0) answers.add(pending);
				pending = -1;
			} else if (item.kind === 'text') {
				pending = index;
			}
		}
		// The text just before the newest question is the previous turn's answer.
		if (pending >= 0) answers.add(pending);

		if (!streaming) {
			pending = -1;
			for (let index = completedEnd; index < items.length; index += 1) {
				if (items[index].kind === 'text') pending = index;
			}
			if (pending >= 0) answers.add(pending);
		}

		const rows: TimelineRow[] = [];
		let steps: Item[] = [];
		let lines: string[] = [];
		const flush = (live = false) => {
			if (steps.length > 0) {
				const first = steps[0];
				rows.push({
					kind: 'group',
					group: {
						key: `run-${first.localId ?? rows.length}`,
						steps: [...steps],
						running:
							(live && streaming) ||
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
		flush(true);
		return rows;
	});

	type BloubStage = 'idle' | 'waiting' | 'thinking' | 'tools' | 'writing' | 'approval' | 'error';

	const BLOUB_STAGE_MOOD: Record<BloubStage, BloubMood> = {
		idle: 'neutre',
		waiting: 'attentif',
		thinking: 'curieux',
		tools: 'excite',
		writing: 'timide',
		approval: 'surpris',
		error: 'triste'
	};

	// Faces the mascot cycles through while hovered in the idle state.
	const HOVER_MOODS: BloubMood[] = ['confus', 'curieux', 'mefiant', 'effraye', 'colere'];

	// Warm every face the chat can switch to (phase + hover). Without this the
	// first switch swaps to an uncached SVG while the old GIF's 1.27x transform
	// is already gone, which reads as a flash.
	const CHAT_MOODS: BloubMood[] = [
		...new Set([...Object.values(BLOUB_STAGE_MOOD), ...HOVER_MOODS])
	];

	// Cycles the chat can switch to. The notification GIF is otherwise fetched
	// cold on the first finished run, which makes it pop in late.
	const CHAT_CYCLES: BloubCycle[] = ['idle2', 'notification'];

	// What the agent is doing right now, for the mascot. The last transcript row
	// tells the story: a user row means the first token is still pending, a
	// running group is thinking or calling tools, and a text row is being written.
	const bloubStage = $derived.by((): BloubStage => {
		if (runError || error) return 'error';
		if (pendingApproval) return 'approval';
		if (!streaming) return 'idle';
		const last = rendered[rendered.length - 1];
		if (!last) return 'waiting';
		if (last.kind === 'item') {
			if (last.item.kind === 'text') return 'writing';
			if (last.item.kind === 'error') return 'error';
			return 'waiting';
		}
		if (!last.group.running) return 'writing';
		return last.group.steps.some((step) => step.kind === 'tool') ? 'tools' : 'thinking';
	});

	const bloubMood: BloubMood = $derived(BLOUB_STAGE_MOOD[bloubStage]);
	const bloubCycle: BloubCycle | undefined = $derived(bloubStage === 'idle' ? 'idle2' : undefined);

	/** The expression for a running step group: tools wear the excited face. */
	function groupMood(group: StepGroup): BloubMood {
		return group.steps.some((step) => step.kind === 'tool') ? 'excite' : 'curieux';
	}

	// Hovering the mascot is a small, idle-only reaction: it picks a random face
	// and keeps changing while the pointer stays. The phase expression wins
	// whenever the agent is responding, so hover never overrides it.
	let hovering = $state(false);
	let hoverMood = $state<BloubMood>('curieux');
	let hoverTimer: ReturnType<typeof setTimeout> | undefined;

	/** The mark belongs to the agent, so it wears that agent's palette colour. */
	const markColor = $derived(colorOf(agentSelection.current?.agent_color));

	function nextHoverMood(current: BloubMood): BloubMood {
		const pool = HOVER_MOODS.filter((mood) => mood !== current);
		return pool[Math.floor(Math.random() * pool.length)] ?? current;
	}

	function scheduleHoverMood() {
		clearTimeout(hoverTimer);
		hoverTimer = setTimeout(
			() => {
				if (!hovering) return;
				hoverMood = nextHoverMood(hoverMood);
				scheduleHoverMood();
			},
			2400 + Math.random() * 1600
		);
	}

	function startHover() {
		hovering = true;
		hoverMood = nextHoverMood(hoverMood);
		scheduleHoverMood();
	}

	function stopHover() {
		hovering = false;
		clearTimeout(hoverTimer);
	}

	// A hover that began on the empty screen must not survive into a new run or a
	// different transcript.
	$effect(() => {
		void items.length;
		stopHover();
	});

	$effect(() => () => clearTimeout(hoverTimer));

	const markMood: BloubMood = $derived(
		completed ? 'neutre' : bloubStage === 'idle' && hovering ? hoverMood : bloubMood
	);
	const markCycle: BloubCycle | undefined = $derived(
		completed ? 'notification' : bloubStage === 'idle' && hovering ? undefined : bloubCycle
	);

	function isGroupOpen(group: StepGroup) {
		return groupOpen[group.key] ?? (group.running || preferences.value.steps === 'expanded');
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
					// A phase is working only while one of its own steps is. The run's
					// openGroup keeps the outer "Working..." header alive between calls;
					// it must not make a finished Thinking/Tools phase spin again.
					running: steps.some((step) => step.running),
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
		const total = Math.max(1, Math.round(ms / 1000));
		const hours = Math.floor(total / 3600);
		const minutes = Math.floor((total % 3600) / 60);
		const seconds = total % 60;
		if (hours > 0) return t('myaaw.chat.duration_hours', { hours, minutes, seconds });
		if (minutes > 0) return t('myaaw.chat.duration_minutes', { minutes, seconds });
		return t('myaaw.chat.duration_seconds', { seconds });
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
	// The footer packs these into one line on a phone, so they stay apart rather
	// than pre-joined: the cache figure is the first to go when the composer is
	// narrow. The provider and model live in the composer's model dropdown, not
	// here.
	let statsTokenText = $derived(
		statsTokens > 0 ? t('myaaw.chat.stats_tokens', { count: formatTokens(statsTokens) }) : ''
	);
	let statsCacheText = $derived(
		statsCacheRate > 0 ? t('myaaw.chat.stats_cache_hit', { percent: statsCacheRate }) : ''
	);

	// Context view: a live run's newest request wins over the stored session, and
	// the stored session wins over nothing. `tokens` is the exact input count the
	// provider reported; the breakdown rows are the pre-request estimate.
	let onlineContext = $derived(activeRun?.context ?? sessionStats?.context ?? null);
	let contextTokens = $derived(onlineContext?.tokens ?? 0);
	let contextWindow = $derived(onlineContext?.window ?? 0);
	let contextKnown = $derived(contextTokens > 0 && contextWindow > 0);
	let contextPercent = $derived(
		contextWindow > 0 ? Math.min(100, Math.round((contextTokens / contextWindow) * 100)) : 0
	);
	let contextSystem = $derived(onlineContext?.system_tokens ?? 0);
	let contextTool = $derived(onlineContext?.tool_tokens ?? 0);
	let contextMessage = $derived(onlineContext?.message_tokens ?? 0);
	let contextPartsTotal = $derived(contextSystem + contextTool + contextMessage);
	let contextDash = $derived(`${((contextPercent / 100) * 94.25).toFixed(2)} 94.25`);
	let contextTokenText = $derived(
		contextKnown
			? t('myaaw.chat.context_usage_ratio', {
					used: formatTokens(contextTokens),
					window: formatTokens(contextWindow)
				})
			: ''
	);

	function contextStrokeClass(percent: number) {
		if (percent >= 90) return 'stroke-red-500';
		if (percent >= 70) return 'stroke-amber-500';
		return 'stroke-primary';
	}

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

	// --- Quoted selections -----------------------------------------------------
	// Selecting text in an agent answer, a thinking block, or a tool log raises a
	// small Ask button. Clicking it pins that excerpt to the next message; any
	// number of excerpts can be collected before sending.

	function askSourceHost(node: Node | null): HTMLElement | null {
		if (!node) return null;
		const element =
			node.nodeType === Node.ELEMENT_NODE ? (node as HTMLElement) : node.parentElement;
		return element?.closest<HTMLElement>('[data-ask-source]') ?? null;
	}

	/** Character offset of a range boundary measured from `host`'s text. */
	function textOffsetIn(host: HTMLElement, container: Node, offset: number): number {
		const range = document.createRange();
		range.selectNodeContents(host);
		try {
			range.setEnd(container, offset);
		} catch {
			return -1;
		}
		return range.toString().length;
	}

	function updateAskAnchor() {
		if (!browser) return;
		const selection = window.getSelection();
		if (!selection || selection.isCollapsed || selection.rangeCount === 0 || !transcript) {
			askAnchor = null;
			return;
		}
		const text = selection.toString().trim();
		if (!text) {
			askAnchor = null;
			return;
		}
		const range = selection.getRangeAt(0);
		const startHost = askSourceHost(range.startContainer);
		const endHost = askSourceHost(range.endContainer);
		// A selection that spans two messages (or reaches the composer) has no single
		// source to attribute, so it does not become a quote.
		if (!startHost || !endHost || startHost !== endHost || !transcript.contains(startHost)) {
			askAnchor = null;
			return;
		}
		const rects = range.getClientRects();
		const rect = rects[rects.length - 1] ?? range.getBoundingClientRect();
		if (!rect || (rect.width === 0 && rect.height === 0)) {
			askAnchor = null;
			return;
		}
		// Offsets record exactly what was highlighted, so the card can mark that
		// same passage later instead of washing the whole reply.
		const raw = range.toString();
		const startOffset = textOffsetIn(startHost, range.startContainer, range.startOffset);
		const endOffset = textOffsetIn(startHost, range.endContainer, range.endOffset);
		const leading = raw.length - raw.trimStart().length;
		const trailing = raw.length - raw.trimEnd().length;
		const exactRange = startOffset >= 0 && endOffset - trailing > startOffset + leading;
		const width = 104;
		const height = 34;
		const margin = 8;
		// Sit above the end of the selection, flip below near the top edge, and stay
		// inside the viewport on narrow screens.
		let top = rect.top - height - 6;
		if (top < 64) top = rect.bottom + 6;
		let left = rect.right - width;
		left = Math.min(Math.max(margin, left), window.innerWidth - width - margin);
		top = Math.min(Math.max(margin, top), window.innerHeight - height - margin);
		askAnchor = {
			text,
			source: startHost.dataset.askSource as AskQuoteSource,
			label: startHost.dataset.askLabel || undefined,
			sourceId: startHost.dataset.askId || undefined,
			startOffset: exactRange ? startOffset + leading : undefined,
			endOffset: exactRange ? endOffset - trailing : undefined,
			top,
			left
		};
	}

	function addAskQuote() {
		const anchor = askAnchor;
		if (!anchor) return;
		if (quotes.length >= MAX_ASK_QUOTES) {
			toastStore.error(t('myaaw.chat.ask_limit', { count: MAX_ASK_QUOTES }));
			return;
		}
		const trimmed = anchor.text.length > MAX_ASK_QUOTE_CHARS;
		const text = trimmed ? anchor.text.slice(0, MAX_ASK_QUOTE_CHARS).trimEnd() : anchor.text;
		askAnchor = null;
		window.getSelection()?.removeAllRanges();
		if (quotes.some((quote) => quote.text === text && quote.source === anchor.source)) {
			return;
		}
		askCounter += 1;
		quotes = [
			...quotes,
			{
				id: `ask-${askCounter}`,
				text,
				source: anchor.source,
				label: anchor.label,
				sourceId: anchor.sourceId,
				startOffset: anchor.startOffset,
				endOffset: anchor.endOffset
			}
		];
		if (trimmed) {
			toastStore.warning(t('myaaw.chat.ask_trimmed', { count: MAX_ASK_QUOTE_CHARS }));
		}
		void focusComposer();
	}

	function removeAskQuote(id: string) {
		quotes = quotes.filter((quote) => quote.id !== id);
	}

	function clearAskQuotes() {
		quotes = [];
	}

	function openQuotePreview(items: AskQuote[]) {
		if (items.length > 0) quotePreview = items;
	}

	/**
	 * Rebuilds a range from character offsets inside one agent row, so a quote can
	 * mark exactly the passage the reader highlighted earlier.
	 */
	function rangeFromOffsets(host: HTMLElement, start: number, end: number): Range | null {
		if (start < 0 || end <= start) return null;
		const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT);
		const range = document.createRange();
		let position = 0;
		let started = false;
		let node: Node | null;
		while ((node = walker.nextNode())) {
			const length = node.nodeValue?.length ?? 0;
			if (!started && start <= position + length) {
				range.setStart(node, Math.max(0, start - position));
				started = true;
			}
			if (started && end <= position + length) {
				range.setEnd(node, Math.max(0, end - position));
				break;
			}
			position += length;
		}
		return started && !range.collapsed ? range : null;
	}

	/**
	 * Finds a quote again when its row id no longer exists (a finished run is
	 * replaced by the stored transcript). The first pass prefers the same kind of
	 * source and tool, so an identical sentence in another answer is not picked
	 * before the most likely one.
	 */
	function findQuoteRange(
		text: string,
		source: AskQuoteSource,
		label?: string
	): { host: HTMLElement; range: Range } | null {
		if (!transcript || !text) return null;
		const hosts = Array.from(transcript.querySelectorAll<HTMLElement>('[data-ask-source]'));
		const passes: ((host: HTMLElement) => boolean)[] = [
			(host) => host.dataset.askSource === source && (!label || host.dataset.askLabel === label),
			(host) => host.dataset.askSource === source,
			() => true
		];
		for (const accept of passes) {
			for (const host of hosts) {
				if (!accept(host)) continue;
				const index = (host.textContent ?? '').indexOf(text);
				if (index < 0) continue;
				const range = rangeFromOffsets(host, index, index + text.length);
				if (range) return { host, range };
			}
		}
		return null;
	}

	/** Stops the yellow wash and forgets the rectangles behind it. */
	function clearQuoteFlash() {
		clearTimeout(quoteMarkTimer);
		quoteRects = [];
	}

	/**
	 * Jumps to the reply an excerpt came from and paints exactly the quoted text.
	 * A step inside a collapsed process group opens first, so the reader always
	 * lands on the passage rather than on a closed header. The overlay is drawn
	 * inside the transcript, so it follows the text while the view scrolls.
	 */
	async function locateAskQuote(quote: AskQuote) {
		if (!browser || !transcript) return;
		const sourceId = quote.sourceId;
		const selector = sourceId ? `[data-ask-id="${CSS.escape(sourceId)}"]` : '';
		let host = selector ? transcript.querySelector<HTMLElement>(selector) : null;
		if (!host && sourceId) {
			const row = transcript.querySelector<HTMLElement>(`[data-ask-row="${CSS.escape(sourceId)}"]`);
			row?.querySelector<HTMLButtonElement>('button[aria-expanded]')?.click();
			await tick();
			host = transcript.querySelector<HTMLElement>(selector);
		}

		clearQuoteFlash();
		let range =
			host && quote.startOffset !== undefined && quote.endOffset !== undefined
				? rangeFromOffsets(host, quote.startOffset, quote.endOffset)
				: null;
		if (!range) {
			const found = findQuoteRange(quote.text, quote.source, quote.label);
			if (found) {
				host = found.host;
				range = found.range;
			}
		}
		if (!host || !range) return;

		// Rects are measured against the transcript, not the viewport, so the
		// overlay scrolls with the message it belongs to.
		const base = transcript.getBoundingClientRect();
		const rects = Array.from(range.getClientRects())
			.filter((rect) => rect.width > 0 && rect.height > 0)
			.map((rect) => ({
				left: rect.left - base.left,
				top: rect.top - base.top,
				width: rect.width,
				height: rect.height
			}));
		if (rects.length === 0) return;

		quoteRects = rects;
		locatingQuote = true;
		clearTimeout(locateTimer);
		locateTimer = setTimeout(() => (locatingQuote = false), 1200);
		const startElement = range.startContainer.parentElement ?? host;
		startElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
		quoteMarkTimer = setTimeout(clearQuoteFlash, 3400);
	}

	$effect(() => () => clearQuoteFlash());
	$effect(() => () => clearTimeout(locateTimer));

	function askQuoteLabel(quote: AskQuote): string {
		if (quote.source === 'tool') {
			return quote.label
				? `${t('myaaw.chat.ask_source_tool')} · ${quote.label}`
				: t('myaaw.chat.ask_source_tool');
		}
		return quote.source === 'thinking'
			? t('myaaw.chat.ask_source_thinking')
			: t('myaaw.chat.ask_source_answer');
	}

	function askQuoteIcon(quote: AskQuote) {
		return quote.source === 'tool' ? Wrench : quote.source === 'thinking' ? Brain : Quote;
	}

	// Selections can be made with the mouse, a finger, or the keyboard. A frame
	// delay lets the browser settle the range after the pointer is released.
	$effect(() => {
		if (!browser) return;
		let frame = 0;
		const schedule = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(updateAskAnchor);
		};
		const onPointerDown = (event: PointerEvent) => {
			const target = event.target as HTMLElement | null;
			if (!target?.closest('[data-ask-anchor]')) askAnchor = null;
		};
		document.addEventListener('selectionchange', schedule);
		document.addEventListener('mouseup', schedule);
		document.addEventListener('touchend', schedule);
		document.addEventListener('pointerdown', onPointerDown, true);
		return () => {
			cancelAnimationFrame(frame);
			document.removeEventListener('selectionchange', schedule);
			document.removeEventListener('mouseup', schedule);
			document.removeEventListener('touchend', schedule);
			document.removeEventListener('pointerdown', onPointerDown, true);
		};
	});

	/**
	 * Starts the run for one message: the composer hands it a fresh one, a queued
	 * row hands it the message it held. Returns false when there is nothing to send.
	 */
	function submit(message: MessagePayload): boolean {
		if (!agent) return false;
		const text = message.text.trim();
		const images = message.images.map((item) => item.base64);
		const files = message.files.map((item) => ({
			name: item.name,
			mime_type: item.mimeType || undefined,
			data_base64: item.base64
		}));
		if (!text && images.length === 0 && files.length === 0) return false;

		// The run owns the stream and its timeline, so opening another
		// conversation or another menu no longer cuts the answer off.
		chatRuns.start({
			agent,
			conversationId,
			text,
			quotes: message.quotes,
			images,
			imagesDisplay: message.images.map((item) => item.dataUrl),
			files,
			uploads: message.files.map((item) => ({
				name: item.name,
				path: '',
				mime_type: item.mimeType,
				size: item.size
			})),
			// Keep the earlier turns of this conversation visible.
			initialItems: items
		});

		groupOpen = {};
		error = '';
		storedUsage = null;
		return true;
	}

	function send() {
		// Sending ends the dictation, any read-aloud, and an unfinished edit.
		stopSpeech();
		tts.stop();
		cancelEdit();
		const text = input.trim();
		if (!text && pending.length === 0 && pendingFiles.length === 0) {
			// Enter on an empty composer promotes the head of the queue instead of
			// doing nothing, so the reader can keep the agent moving with one key.
			const [next] = queue;
			if (next) sendQueuedNow(next);
			return;
		}
		if (!agent) return;

		// While an answer streams, the composer honours the chosen mode: queue the
		// message, interrupt the run and send now, or (both off) just keep the text.
		if (streaming) {
			if (sendWhileStreaming === 'queue') {
				queue = [
					...queue,
					{
						id: crypto.randomUUID(),
						text,
						quotes: [...quotes],
						images: pending,
						files: pendingFiles,
						createdAt: new Date().toISOString()
					}
				];
				queueOpen = true;
			} else if (sendWhileStreaming === 'interrupt') {
				// The stop request is dispatched before the new stream, so the agent
				// sees the interrupt first; starting the run here keeps the swap instant.
				void activeRun?.stop();
				if (!submit({ text, quotes: [...quotes], images: pending, files: pendingFiles })) return;
			} else {
				return;
			}
		} else if (!submit({ text, quotes: [...quotes], images: pending, files: pendingFiles })) {
			return;
		}

		input = '';
		quotes = [];
		askAnchor = null;
		clearPending();
		if (composer) composer.style.height = '';
	}

	/**
	 * Sends the head of the queue when a response settles, so the reader's next
	 * message continues the conversation without another action.
	 */
	function drainQueue() {
		if (!preferences.value.queueMessages || streaming || queue.length === 0) return;
		const [next, ...rest] = queue;
		queue = rest;
		submit(next);
	}

	/**
	 * Removes one queued message. Send now and the drain above both do the same
	 * thing when it is time for that message to go out.
	 */
	function removeQueued(id: string) {
		queue = queue.filter((item) => item.id !== id);
	}

	function clearQueue() {
		queue = [];
	}

	/** Pulls a queued message back into the composer, with its excerpts and files. */
	function editQueued(item: QueuedMessage) {
		queue = queue.filter((entry) => entry.id !== item.id);
		pending = [...pending, ...item.images];
		pendingFiles = [...pendingFiles, ...item.files];
		quotes = [...quotes, ...item.quotes];
		askCounter = Math.max(askCounter, nextAskCounter(item.quotes));
		input = input.trim() ? `${input.trim()}\n\n${item.text}` : item.text;
		void focusComposer();
	}

	/** Interrupts the streaming answer and sends this queued message right away. */
	function sendQueuedNow(item: QueuedMessage) {
		queue = queue.filter((entry) => entry.id !== item.id);
		// The stop request is dispatched before the new stream, so the agent sees
		// the interrupt first. Starting the run here keeps the swap instant.
		void activeRun?.stop();
		submit(item);
	}

	/** The one-line preview of a queued message. */
	function queuedPreview(item: QueuedMessage): string {
		return item.text || t('myaaw.chat.queue_attachment_only');
	}

	// --- Editing and regenerating ---------------------------------------------
	// Both rewrite the stored transcript on the agent (linear history, the way
	// the chat UI already reads): everything after the target turn is dropped
	// before the new answer streams in.

	/** How many user turns sit after this row; the agent rewinds from the end. */
	function rewindOffsetAfter(index: number) {
		let offset = 0;
		for (let i = index + 1; i < items.length; i += 1) {
			if (items[i].kind === 'user') offset += 1;
		}
		return offset;
	}

	function beginEdit(index: number, item: Item) {
		if (streaming || !item.text) return;
		tts.stop();
		stopSpeech();
		editingMessageId = item.localId ?? '';
		messageDraft = item.text;
		void tick().then(() => {
			draftArea?.focus();
			draftArea?.select();
		});
	}

	function cancelEdit() {
		editingMessageId = '';
		messageDraft = '';
	}

	function startRewind(
		nextItems: Item[],
		text: string,
		offset: number,
		rewindQuotes: AskQuote[] = []
	) {
		if (!agent || streaming) return;
		stopSpeech();
		tts.stop();
		editingMessageId = '';
		messageDraft = '';
		error = '';
		storedUsage = null;
		groupOpen = {};
		chatRuns.start({
			agent,
			conversationId,
			text,
			quotes: rewindQuotes,
			images: [],
			imagesDisplay: [],
			files: [],
			uploads: [],
			initialItems: nextItems,
			rewindUserOffset: offset
		});
	}

	function saveEdit(index: number, item: Item) {
		const text = messageDraft.trim();
		if (!text || !item.text || text === item.text.trim()) {
			cancelEdit();
			return;
		}
		const next = items
			.slice(0, index + 1)
			.map((row, position) => (position === index ? { ...row, text } : row));
		// Editing keeps the excerpt block attached, so the model still sees the
		// quoted context while only the question changes.
		startRewind(next, text, rewindOffsetAfter(index), item.quotes ?? []);
	}

	/**
	 * Regenerates the answer at `index`: rewind to the user turn it answers and
	 * drop everything after it, the same linear history an edit produces.
	 */
	function regenerateFrom(index: number) {
		if (streaming || !conversationId) return;
		let userIndex = -1;
		for (let i = Math.min(index, items.length - 1); i >= 0; i -= 1) {
			if (items[i].kind === 'user') {
				userIndex = i;
				break;
			}
		}
		if (userIndex < 0) return;
		startRewind(items.slice(0, userIndex + 1), '', rewindOffsetAfter(userIndex));
	}

	async function decide(decision: ApprovalDecision) {
		const prompt = pendingApproval;
		if (!prompt || !agent) return;
		try {
			await chatApi.decide(agent, prompt.approval_id, decision);
			// Drop it locally too, so the queue never waits for the resolved event.
			approvalQueue.resolved({ approval_id: prompt.approval_id });
			activeRun?.resolveApproval(prompt.approval_id);
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
		quotes = [];
		askAnchor = null;
		// A fresh chat starts without a queue; the stored `new` slot is dropped so
		// the draft-switch effect does not bring it back.
		queue = [];
		queueOpen = true;
		if (browser) localStorage.removeItem(draftQueueStorageKey(agent ?? '', ''));
		focusContextPath = '';
		previewAttachment = null;
		attachmentCount = 0;
		classicAttachmentsOpen = false;
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
		if (classicMode) return;
		// The history button shows history; a second press on it closes the panel.
		if (historyOpen && rightTab === 'history') {
			historyOpen = false;
		} else {
			historyOpen = true;
			rightTab = 'history';
			focusPanelTab = 'history';
		}
		if (browser) localStorage.setItem(HISTORY_KEY, String(historyOpen));
	}

	/** Closes whichever right-panel surface the current shell uses. */
	function closeSidePanel() {
		if (classicMode) {
			classicAttachmentsOpen = false;
			return;
		}
		historyOpen = false;
		if (browser) localStorage.setItem(HISTORY_KEY, 'false');
	}

	function openConversation(id: string) {
		focusContextPath = '';
		previewAttachment = null;
		if (browser && window.innerWidth < 1024) closeSidePanel();
		if (id !== conversationId) void goto(`/app/chat/${id}`);
	}

	/**
	 * The header paperclip owns the attachment surface and the history button owns
	 * the conversation list: each toggles its own view in the shared right column.
	 */
	function openAttachments() {
		if (!browser || !agent) return;
		if (classicMode) {
			classicAttachmentsOpen = !classicAttachmentsOpen;
			return;
		}
		if (historyOpen && rightTab === 'attachments') {
			historyOpen = false;
			if (browser) localStorage.setItem(HISTORY_KEY, 'false');
			return;
		}
		rightTab = 'attachments';
		if (!historyOpen) {
			historyOpen = true;
			localStorage.setItem(HISTORY_KEY, 'true');
		}
	}

	/** Opens one file in the shared preview pane. */
	function openAttachment(item: ConversationAttachment) {
		previewAttachment = item;
		// On phones the right panel is a drawer over the column; close it so the
		// preview pane is what the reader sees.
		if (browser && window.innerWidth < 1024) closeSidePanel();
	}

	/**
	 * Closes every attachment surface first, then reveals the turn that carried
	 * the file. The transcript may have been unmounted while a pane held its slot,
	 * so this waits a tick before asking it for a row.
	 */
	async function showAttachmentInChat(item: ConversationAttachment) {
		previewAttachment = null;
		// The panel may stay open on a wide screen; a phone drawer would cover the row.
		if (browser && window.innerWidth < 1024) closeSidePanel();
		await tick();
		await revealMessage(item.message_index);
	}

	/** Loads earlier pages until the turn exists, then centres and flashes it. */
	async function revealMessage(index: number) {
		await tick();
		for (let guard = 0; guard < 10 && index < loadedOffset; guard += 1) {
			const before = loadedOffset;
			await loadOlder(100);
			if (loadedOffset === before) break;
		}
		await tick();
		const node = scroller?.querySelector<HTMLElement>(`[data-chat-turn-index="${index}"]`);
		if (!node) return;
		node.scrollIntoView({ block: 'center', behavior: 'smooth' });
		highlightedMessage = index;
		if (highlightTimer) clearTimeout(highlightTimer);
		highlightTimer = setTimeout(() => (highlightedMessage = null), 1400);
	}

	/** The badge count must be right even while the attachment list is closed. */
	async function refreshAttachmentCount() {
		const currentAgent = agent ?? '';
		const currentId = conversationId;
		if (!currentAgent || !currentId) {
			attachmentCount = 0;
			return;
		}
		try {
			const page = await conversationsApi.attachments(currentAgent, currentId, { limit: 1 });
			if (conversationId !== currentId) return;
			attachmentCount = page.total ?? 0;
		} catch {
			// Keep the last known count; opening the panel retries.
		}
	}

	function onHistoryDeleted(id: string) {
		if (id !== conversationId) return;
		resetChat();
		void goto('/app/chat');
	}

	function onHistoryRenamed(id: string, title: string) {
		if (id !== conversationId) return;
		stopTitlePolling();
		topBarTitle.set(title);
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
	 * A delivery directive is backend syntax. Readers see the path it names, so
	 * `@attach(/a/b.png)` renders as `/a/b.png` without the wrapper.
	 */
	const ATTACH_DIRECTIVE_RE = /@attach\(\s*(?:"([^"]+)"|'([^']+)'|([^)\s]+))\s*\)/gi;
	/** Short form the model sometimes writes: `@/absolute/path`. */
	const BARE_ATTACH_RE = /@((?:~\/?|\/)[^\s"'<>()]+)/g;

	function displayText(text: string): string {
		return text
			.replace(ATTACH_DIRECTIVE_RE, (_match, ...groups) => groups.find((group) => group) ?? '')
			.replace(BARE_ATTACH_RE, '$1');
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
	 * A user turn can carry <userinfo> (channel context) and <attached> (uploaded
	 * file paths). Both are model-only; people see the text they typed.
	 */
	function withoutContext(text: string): string {
		return text
			.replace(/<userinfo>[\s\S]*?<\/userinfo>\s*/g, '')
			.replace(/<attached>[\s\S]*?<\/attached>\s*/g, '')
			.trim();
	}

	type TriggerKind = 'cron' | 'heartbeat' | 'subagent';

	/**
	 * Cron, heartbeat, and sub-agent turns travel inside a model-only block:
	 *
	 *   <trigger kind="cron">…rules… the prompt…</trigger>
	 *
	 * People see a badge and nothing else. Turns stored before the block existed
	 * have no marker and render as plain text. A quoted selections block sits in
	 * front of the question; it is split out here so the bubble stays readable.
	 */
	function splitTrigger(raw: string): {
		trigger: TriggerKind | null;
		text: string;
		quotes: AskQuote[];
	} {
		const cleaned = withoutContext(raw);
		const parsed = parseAskPrompt(cleaned);
		const block = parsed.text.match(
			/^\s*<trigger\b[^>]*\bkind="(cron|heartbeat|subagent)"[^>]*>[\s\S]*?<\/trigger>\s*/i
		);
		if (!block) return { trigger: null, text: parsed.text, quotes: parsed.quotes };

		// The whole block is model-only, so the badge is the message.
		return {
			trigger: block[1].toLowerCase() as TriggerKind,
			text: '',
			quotes: parsed.quotes
		};
	}

	function triggerLabel(trigger: TriggerKind): string {
		return t(`myaaw.chat.trigger_${trigger}`);
	}

	function triggerIcon(trigger: TriggerKind) {
		return trigger === 'cron' ? CalendarClock : trigger === 'heartbeat' ? HeartPulse : Bot;
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
		// Inline image items can be evicted from the model context; the saved upload
		// is the fallback and keeps the photo visible after a reload.
		if (urls.length === 0 && agent) {
			for (const upload of message.uploads ?? []) {
				if (!upload.mime_type?.startsWith('image/')) continue;
				const virtual = uploadVirtualPath(upload);
				if (virtual) urls.push(filesApi.rawUrl(agent, virtual));
			}
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

	/** Sender identity for group/topic bubbles, e.g. "Alice · @alice · #123456789". */
	function senderLabel(item: Item): string {
		const parts = [
			item.senderName?.trim(),
			item.senderUsername ? `@${item.senderUsername}` : '',
			item.senderId ? `#${item.senderId}` : ''
		];
		return parts.filter(Boolean).join(' · ');
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
		// Last step row of the current turn, so the persisted duration can label
		// the "Completed" group after a reload.
		let lastStep = -1;
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
				lastStep = timeline.length - 1;
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
					lastStep = position;
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
					lastStep = timeline.length - 1;
				}
				continue;
			}

			if (message.role === 'user') {
				lastStep = -1;
				const { trigger, text, quotes: storedQuotes } = splitTrigger(messageText(message));
				const images = messageImages(message);
				// A trigger turn has no visible text: the badge is the message, so it still
				// gets a row.
				if (trigger || text || images.length > 0 || (message.uploads?.length ?? 0) > 0) {
					timeline.push({
						kind: 'user',
						text,
						quotes: storedQuotes.length > 0 ? storedQuotes : undefined,
						trigger: trigger ?? undefined,
						images: images.length > 0 ? images : undefined,
						uploads: message.uploads,
						localId: nextId(),
						messageIndex,
						createdAt: message.created_at,
						senderId: message.sender_id,
						senderName: message.sender_name,
						senderUsername: message.sender_username
					});
				}
				continue;
			}

			if (message.role !== 'assistant') continue;

			const text = messageText(message);
			const attachments = message.attachments ?? [];
			if (text || attachments.length > 0) {
				timeline.push({
					kind: 'text',
					text,
					attachments,
					localId: nextId(),
					messageIndex,
					createdAt: message.created_at
				});
			}

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
				lastStep = timeline.length - 1;
				if (id) tools.set(id, timeline.length - 1);
			}

			// The final assistant message carries the turn's duration; pin it on the
			// last step so the group header can show it after a reload.
			if (message.duration_ms && lastStep >= 0) {
				timeline[lastStep] = { ...timeline[lastStep], group_duration_ms: message.duration_ms };
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
		if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
		const withModifier = event.metaKey || event.ctrlKey;
		// In the default mode Enter sends and a modifier is optional. In the other
		// mode Enter breaks the line and only the modifier sends.
		if (preferences.value.sendKey === 'mod-enter' && !withModifier) return;
		event.preventDefault();
		void send();
	}

	/** Turns the page shortcuts into actions; the composer keeps its own keys. */
	function onShortcutKey(event: KeyboardEvent) {
		// The composer already consumed Enter (with or without a modifier).
		if (event.defaultPrevented) return;

		if (matchesShortcut(event, 'escape')) {
			// An open dialog or menu owns Esc, so interrupting waits its turn.
			if (isOverlayOpen()) return;
			if (askAnchor) {
				event.preventDefault();
				askAnchor = null;
				window.getSelection()?.removeAllRanges();
				return;
			}
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
		if (matchesShortcut(event, 'mod+shift+a')) {
			event.preventDefault();
			openAttachments();
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

	/** Grows the inline message editor to fit the whole message, then scrolls. */
	function autosizeEditor(node: HTMLTextAreaElement) {
		const resize = () => {
			node.style.height = 'auto';
			const max = Math.max(160, Math.round(window.innerHeight * 0.45));
			node.style.height = `${Math.min(node.scrollHeight, max)}px`;
			node.style.overflowY = node.scrollHeight > max ? 'auto' : 'hidden';
		};
		// The bound value is applied after the element mounts, so wait a frame.
		requestAnimationFrame(resize);
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

	/** Dictation language has its own setting; it no longer follows the interface. */
	function dictationLanguage() {
		const choice = preferences.value.dictationLang;
		const base = choice === 'app' ? locale.value : choice;
		return base === 'id' ? 'id-ID' : 'en-US';
	}

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
		recognition.lang = dictationLanguage();
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

	function fileExtension(name: string) {
		const dot = name.lastIndexOf('.');
		return dot >= 0 ? name.slice(dot).toLowerCase() : '';
	}
</script>

<svelte:head>
	{#each CHAT_MOODS as mood (mood)}
		<link rel="preload" as="image" type="image/svg+xml" href={moodSrc(mood, markColor)} />
	{/each}
	{#each CHAT_CYCLES as cycle (cycle)}
		<link rel="preload" as="image" href={cycleSrc(cycle, markColor)} />
	{/each}
</svelte:head>

<svelte:window
	onclick={() => {
		openStats = false;
		openUsage = false;
		railSheetOpen = false;
	}}
	onkeydown={onShortcutKey}
/>

<!-- The unfolded turn list: one row per question, each carrying its own tick so
     a row reads as a single line. It is rendered twice because the container
     differs: a hover card beside the rail on a desktop, a floating panel over
     the transcript on a touch screen. -->
{#snippet railCard()}
	<div
		class="relative w-64 rounded-xl border bg-popover p-1.5 shadow-lg frost-popover"
		transition:fade={{ duration: 150 }}
		role="presentation"
		onclick={(event) => event.stopPropagation()}
	>
		<div
			bind:this={cardList}
			onscroll={updateCardScroll}
			class="max-h-71.5 overflow-y-auto overscroll-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
		>
			{#each railTurns as turn (turn.index)}
				<button
					type="button"
					class={cn(
						'flex h-6.5 w-full cursor-pointer items-center gap-3 rounded-md pr-3 pl-2 text-left text-[13px] transition-colors',
						turn.index === activeTurnIndex
							? 'text-primary'
							: turn.index === hoverTurnIndex
								? 'bg-muted/60 text-foreground'
								: 'text-muted-foreground hover:bg-muted/60'
					)}
					data-turn-active={turn.index === activeTurnIndex ? 'true' : undefined}
					onclick={() => pickTurn(turn)}
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
				class="pointer-events-none absolute inset-x-1.5 top-1.5 h-5 rounded-t-lg bg-linear-to-b from-popover to-transparent"
			></div>
		{/if}
		{#if cardScrollDown}
			<div
				class="pointer-events-none absolute inset-x-1.5 bottom-1.5 h-5 rounded-b-lg bg-linear-to-t from-popover to-transparent"
			></div>
		{/if}
	</div>
{/snippet}

<TopBarActions id="chat-actions">
	{#snippet children()}
		{#if !classicMode}
			<TopBarButton label={t('myaaw.chat.new')} icon={MessageSquarePlus} onclick={newChat} />
		{/if}
		<Button
			variant={historyOpen && rightTab === 'attachments' ? 'secondary' : 'outline'}
			size="icon-sm"
			class="relative"
			onclick={openAttachments}
			title={`${t('myaaw.chat.attachments_open')} (${formatShortcut('mod+shift+a')})`}
			aria-label={t('myaaw.chat.attachments_open')}
			aria-keyshortcuts="Control+Shift+A Meta+Shift+A"
			aria-pressed={historyOpen && rightTab === 'attachments'}
		>
			<Paperclip class="h-4 w-4" />
			{#if attachmentCount > 0}
				<span
					class="absolute -top-1.5 -right-1.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[9px] leading-4 font-semibold text-primary-foreground tabular-nums"
				>
					{attachmentCount > 99 ? '99+' : attachmentCount}
				</span>
			{/if}
		</Button>
		{#if !classicMode}
			<Button
				variant={historyOpen && rightTab === 'history' ? 'secondary' : 'outline'}
				size="icon-sm"
				onclick={toggleHistory}
				title={`${historyOpen && rightTab === 'history' ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')} (${formatShortcut('mod+shift+h')})`}
				aria-label={historyOpen && rightTab === 'history'
					? t('myaaw.chat.history_hide')
					: t('myaaw.chat.history_show')}
				aria-pressed={historyOpen && rightTab === 'history'}
				aria-keyshortcuts="Control+Shift+H Meta+Shift+H"
			>
				{#if focusMode}
					{#if historyOpen && rightTab === 'history'}
						<PanelRightClose class="h-4 w-4" />
					{:else}
						<PanelRightOpen class="h-4 w-4" />
					{/if}
				{:else}
					<History class="h-4 w-4" />
				{/if}
			</Button>
		{/if}
	{/snippet}
</TopBarActions>

<div class="relative flex h-full min-h-0">
	<div
		class={[
			'flex min-w-0 flex-1 flex-col transition-[padding] duration-200',
			focusMode && historyOpen ? 'lg:pr-88' : ''
		]}
	>
		{#if agent && previewAttachment}
			<!-- The focus header floats over the column; keep the pane below it there. -->
			<div class={focusMode ? 'min-h-0 flex-1 pt-14 lg:pt-16' : 'min-h-0 flex-1'}>
				<AttachmentPreview
					{agent}
					item={previewAttachment}
					onClose={() => (previewAttachment = null)}
					onShowInChat={showAttachmentInChat}
				/>
			</div>
		{:else if agent && focusMode && focusContextPath}
			<FocusContextEditor {agent} path={focusContextPath} onClose={() => (focusContextPath = '')} />
		{:else if !agent}
			<div
				class="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground"
			>
				{#if focusMode}
					<Bloub mood="confus" size={56} color={markColor} />
				{/if}
				<span>{t('myaaw.agent.none')}</span>
			</div>
		{:else}
			<div class="flex min-h-0 flex-1 flex-col">
				<!-- The rail hangs beside the transcript only, so it never takes width
				     from the composer. -->
				<div class="relative flex min-h-0 flex-1 flex-col">
					{#if focusMode && items.length > 0}
						<!-- Focus mode has no chat title, so the agent floats above the
						     transcript instead: brand mark plus the agent's name. The scrim
						     below fades whatever scrolls under it, using the theme's own
						     background so it works in every light and dark theme. An empty
						     chat shows the mark in its empty state instead, once. -->
						<div
							aria-hidden="true"
							class="pointer-events-none absolute inset-x-0 top-0 z-10 h-40 bg-linear-to-b from-background via-background/70 to-transparent backdrop-blur-[2px] mask-[linear-gradient(to_bottom,black,transparent)]"
						></div>
						<!-- Centred on the browser, not on the column: the 56px rail sits to
						     the left of this box, so half of it is pulled back at md and up.
						     The mark doubles as the panel toggle, since focus mode has no
						     other affordance once the panel is closed. -->
						<button
							type="button"
							class="pointer-events-auto absolute top-3 left-1/2 z-20 flex -translate-x-1/2 cursor-pointer flex-col items-center gap-0 rounded-3xl focus-visible:outline-none md:-ml-7"
							onclick={toggleHistory}
							aria-label={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
							title={historyOpen ? t('myaaw.chat.history_hide') : t('myaaw.chat.history_show')}
						>
							<Bloub
								mood={markMood}
								cycle={markCycle}
								color={markColor}
								size={96}
								eager
								tight
								fade
								class="pointer-events-auto"
								onmouseenter={startHover}
								onmouseleave={stopHover}
							>
								{#snippet fallback()}
									<Logo class="h-16 w-16 text-primary" />
								{/snippet}
							</Bloub>
							<span
								class="mt-1 rounded-full border bg-card/90 px-2.5 py-0.5 text-xs font-semibold shadow-sm backdrop-blur"
							>
								{agent}
							</span>
						</button>
					{/if}
					<div
						bind:this={scroller}
						class="min-h-0 flex-1 overflow-x-hidden overflow-y-auto"
						onscroll={onScroll}
					>
						<div
							bind:this={transcript}
							class={cn(
								'mx-auto relative w-full max-w-4xl space-y-5 px-4 py-6',
								focusMode && items.length > 0 && 'pt-40',
								railTurns.length > 1 && 'pr-8 md:pr-14',
								// Nothing to read: sit in the middle of the column instead of
								// hugging the top edge.
								items.length === 0 && 'flex min-h-full flex-col items-center justify-center'
							)}
							style:padding-bottom={frosted ? `${composerHeight + 24}px` : null}
						>
							{#if quoteRects.length > 0}
								<!-- The yellow wash sits above the text but never touches the DOM, so
								     format wrappers and text nodes stay exactly as rendered. -->
								<div class="pointer-events-none absolute inset-0 z-10" aria-hidden="true">
									{#each quoteRects as rect, index (index)}
										<span
											class="ask-quote-mark"
											style:left={`${rect.left}px`}
											style:top={`${rect.top}px`}
											style:width={`${rect.width}px`}
											style:height={`${rect.height}px`}
										></span>
									{/each}
								</div>
							{/if}
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
								<div class="flex flex-col items-center">
									<Bloub
										mood={hovering ? hoverMood : 'neutre'}
										cycle={hovering ? undefined : 'idle2'}
										color={markColor}
										size={112}
										eager
										tight
										fade
										onmouseenter={startHover}
										onmouseleave={stopHover}
									/>
									{#if focusMode}
										<span
											class="mt-1 rounded-full border bg-card/90 px-2.5 py-0.5 text-xs font-semibold shadow-sm backdrop-blur"
										>
											{agent}
										</span>
									{/if}
									<p class="mt-3 text-center text-sm text-muted-foreground">
										{t('myaaw.chat.empty')}
									</p>
								</div>
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
													{#if preferences.value.showRunMascot}
														<Bloub mood={groupMood(group)} size={18} color={markColor} />
													{/if}
												{:else}
													<Clock class="h-3.5 w-3.5 shrink-0" />
												{/if}
												<span class="font-medium">
													{group.running ? t('myaaw.chat.tool_running') : t('myaaw.chat.completed')}
												</span>
												{#if group.running}
													<span class="shrink-0 text-muted-foreground tabular-nums">
														· {formatDuration(
															now - (activeRun?.groupStartedAt || activeRun?.startedAt || now)
														)}
													</span>
												{:else if group.durationMs !== undefined}
													<span class="shrink-0 text-muted-foreground tabular-nums">
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
														<div
															class="px-1 text-sm"
															data-ask-source="answer"
															data-ask-id={phase.item.localId}
														>
															<Markdown
																content={displayText(phase.item.text ?? '')}
																streaming={streaming && phase.item.localId === latestAnswerKey}
															/>
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
																{#if phaseGroup.running && preferences.value.showRunMascot}
																	<Bloub mood={groupMood(phaseGroup)} size={18} color={markColor} />
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
																			<div data-ask-row={step.localId}>
																				<ChatStep
																					title={t('myaaw.chat.think')}
																					summary={oneLine(step.text ?? '')}
																					icon={Brain}
																					expanded={!!step.expanded}
																					ontoggle={() => toggleItem(step.localId)}
																					running={step.running}
																				>
																					<p
																						class="whitespace-pre-wrap text-muted-foreground"
																						data-ask-source="thinking"
																						data-ask-id={step.localId}
																					>
																						{step.text}
																					</p>
																				</ChatStep>
																			</div>
																		{:else if step.kind === 'tool'}
																			<div data-ask-row={step.localId}>
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
																						data-ask-source="tool"
																						data-ask-label={step.name}
																						data-ask-id={step.localId}
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
																			</div>
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
											class="flex justify-end terminal:justify-start"
											data-chat-turn={item.localId}
											data-chat-turn-index={item.messageIndex}
										>
											<div
												class={cn(
													'group/user flex w-full min-w-0 max-w-[80%] flex-col items-end gap-2 rounded-2xl transition-shadow terminal:max-w-full terminal:items-start',
													highlightedMessage === item.messageIndex &&
														'ring-2 ring-primary/60 ring-offset-2 ring-offset-background'
												)}
											>
												{#if senderLabel(item)}
													<span
														class="text-[10px] font-medium text-muted-foreground"
														title={t('myaaw.chat.sender_label')}
													>
														{senderLabel(item)}
													</span>
												{/if}
												{#if item.images?.length}
													<div class="flex flex-wrap justify-end gap-2 terminal:justify-start">
														{#each item.images as image, imageIndex (imageIndex)}
															<div class="relative inline-block max-w-full">
																<button
																	type="button"
																	class="block cursor-zoom-in rounded-2xl focus-visible:outline-2 focus-visible:outline-offset-2"
																	aria-label={t('myaaw.chat.lightbox_open')}
																	onclick={() => openImageLightbox(item.images ?? [], imageIndex)}
																>
																	<img
																		src={image}
																		alt=""
																		draggable="false"
																		class="max-h-48 rounded-2xl border object-contain"
																	/>
																</button>
																<button
																	type="button"
																	class="absolute right-1.5 bottom-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
																	title={t('myaaw.chat.download_file')}
																	aria-label={t('myaaw.chat.download_file')}
																	onclick={() =>
																		downloadImage(image, imageFileName(image, imageIndex))}
																>
																	<Download class="h-3.5 w-3.5" />
																</button>
															</div>
														{/each}
													</div>
												{/if}
												{#if item.uploads?.length}
													<div class="flex flex-wrap justify-end gap-2 terminal:justify-start">
														{#each item.uploads.filter((upload) => !upload.mime_type?.startsWith('image/')) as upload (upload.path || upload.name)}
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
												{#if item.trigger}
													{@const TriggerIcon = triggerIcon(item.trigger)}
													<div
														class="flex w-full max-w-full flex-col items-end gap-1 terminal:items-start"
													>
														<!-- The block is model-only: the badge is the whole message. -->
														<span
															class="inline-flex items-center gap-1.5 rounded-full border border-(--chat-bubble-ring) bg-secondary px-2.5 py-1 text-[11px] font-medium text-secondary-foreground"
														>
															<TriggerIcon class="h-3.5 w-3.5" />
															{triggerLabel(item.trigger)}
														</span>
														<div class="mt-0.5 flex items-center gap-1 text-muted-foreground">
															<ChatTime at={item.createdAt ?? ''} />
														</div>
													</div>
												{:else if item.text}
													<div
														class="flex w-full max-w-full flex-col items-end terminal:items-start"
													>
														{#if editingMessageId === item.localId}
															<div class="flex w-full max-w-3xl flex-col items-end gap-1">
																<textarea
																	bind:this={draftArea}
																	bind:value={messageDraft}
																	use:autosizeEditor
																	rows="1"
																	class="min-h-11 w-full resize-none rounded-(--chat-bubble-radius) border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-1 focus-visible:ring-ring/50"
																	onkeydown={(event) => {
																		if (event.isComposing) return;
																		if (event.key === 'Escape') {
																			event.preventDefault();
																			cancelEdit();
																		} else if (event.key === 'Enter' && !event.shiftKey) {
																			event.preventDefault();
																			saveEdit(items.indexOf(item), item);
																		}
																	}}
																></textarea>
																<div class="flex items-center gap-1">
																	<Button size="xs" variant="secondary" onclick={cancelEdit}>
																		{t('myaaw.chat.edit_cancel')}
																	</Button>
																	<Button
																		size="xs"
																		disabled={!messageDraft.trim()}
																		onclick={() => saveEdit(items.indexOf(item), item)}
																	>
																		{t('myaaw.chat.edit_save')}
																	</Button>
																</div>
															</div>
														{:else}
															<div
																class="max-w-full whitespace-pre-wrap wrap-break-word rounded-(--chat-bubble-radius) bg-secondary px-4 py-2.5 text-sm text-secondary-foreground ring-1 ring-(--chat-bubble-ring) terminal:bg-transparent terminal:px-0 terminal:py-0 terminal:text-foreground"
															>
																<span
																	class="mr-1.5 hidden select-none font-semibold text-muted-foreground terminal:inline"
																>
																	❯
																</span>
																{item.text}
															</div>
															{#if (item.quotes?.length ?? 0) > 0}
																{@const quoteCount = item.quotes?.length ?? 0}
																<button
																	type="button"
																	class="inline-flex items-center gap-1.5 rounded-full border bg-card px-2.5 py-1 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
																	data-ask-summary
																	onclick={() => openQuotePreview(item.quotes ?? [])}
																	title={t('myaaw.chat.ask_open')}
																>
																	<Quote class="h-3.5 w-3.5" />
																	{quoteCount === 1
																		? t('myaaw.chat.ask_selected_one')
																		: t('myaaw.chat.ask_selected', { count: quoteCount })}
																</button>
															{/if}
															<div
																class={cn(
																	'mt-0.5 flex items-center gap-1 opacity-0 transition-opacity group-hover/user:opacity-100 group-focus-within/user:opacity-100 [@media(hover:none)]:opacity-100',
																	item.localId === latestUserKey && 'opacity-100'
																)}
															>
																<CopyButton text={item.text} />
																{#if !streaming}
																	<button
																		type="button"
																		class="rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground"
																		aria-label={t('myaaw.chat.edit')}
																		title={t('myaaw.chat.edit')}
																		onclick={() => beginEdit(items.indexOf(item), item)}
																	>
																		<Pencil class="h-3.5 w-3.5" />
																	</button>
																{/if}
																<ChatTime at={item.createdAt ?? ''} />
															</div>
														{/if}
													</div>
												{/if}
											</div>
										</div>
									{:else if item.kind === 'text'}
										<div
											data-chat-turn-index={item.messageIndex}
											class={cn(
												'group/message wrap-break-word rounded-xl text-sm transition-shadow terminal:border-l terminal:pl-3',
												highlightedMessage === item.messageIndex &&
													'ring-2 ring-primary/60 ring-offset-2 ring-offset-background'
											)}
										>
											<div data-ask-source="answer" data-ask-id={item.localId}>
												<Markdown
													content={displayText(item.text ?? '')}
													streaming={streaming && item.localId === latestAnswerKey}
												/>
											</div>
											{#if (item.attachments?.length ?? 0) > 0}
												<RunAttachments {agent} attachments={item.attachments ?? []} />
											{/if}
											<div
												class={cn(
													'mt-1 flex items-center gap-1 opacity-0 transition-opacity group-hover/message:opacity-100 group-focus-within/message:opacity-100 [@media(hover:none)]:opacity-100',
													item.localId === latestAnswerKey && 'opacity-100'
												)}
											>
												<CopyButton text={displayText(item.text ?? '')} />
												{#if !(streaming && item.localId === latestAnswerKey)}
													<SpeakButton
														id={item.localId ?? ''}
														text={displayText(item.text ?? '')}
													/>
												{/if}
												{#if !streaming && conversationId}
													<button
														type="button"
														class="rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground"
														aria-label={t('myaaw.chat.regenerate')}
														title={t('myaaw.chat.regenerate')}
														onclick={() => regenerateFrom(items.indexOf(item))}
													>
														<RotateCcw class="h-3.5 w-3.5" />
													</button>
												{/if}
												<ChatTime at={item.createdAt ?? ''} />
											</div>
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
									{#if focusMode && preferences.value.showRunMascot}
										<Bloub mood={bloubMood} size={20} color={markColor} />
									{:else}
										<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
									{/if}
									<span class="font-medium">{t('myaaw.chat.tool_running')}</span>
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
							class="absolute top-1/2 right-1 z-10 w-5 -translate-y-1/2 md:right-5 md:w-6"
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
								<div class="absolute top-1/2 right-0 hidden -translate-y-1/2 md:block">
									{@render railCard()}
								</div>
							{/if}

							<!-- One tick column at a time: the rail steps back under the unfolded list on a
							     desktop, and stays put on touch screens, where the list is tapped open. -->
							{#each railMarks as entry, position (entry.turn.index)}
								<button
									type="button"
									class="absolute right-0 flex h-3 w-full -translate-y-1/2 cursor-pointer items-center justify-end transition-all duration-200"
									style="top: {RAIL_PAD + position * RAIL_GAP}px; opacity: {railOpen
										? 0
										: entry.turn.index === activeTurnIndex
											? 1
											: entry.opacity}"
									aria-label={oneLine(entry.turn.text) || t('myaaw.chat.jump_to_turn')}
									title={oneLine(entry.turn.text)}
									onclick={(event) => {
										// The page closes its floating panels on a click; this one opens a panel.
										event.stopPropagation();
										tapTick(entry.turn);
									}}
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
						</div>
						<!-- Kept outside the rail: the rail is translated, and a transformed
						     ancestor would pin a fixed panel to itself instead of the screen. -->
						{#if railSheetOpen}
							<!-- Touch screens have no hover, so the same list opens over a scrim on a tap
							     and the reader picks a question from there. -->
							<button
								type="button"
								class="fixed inset-0 z-40 bg-black/40"
								aria-label={t('myaaw.chat.turns_close')}
								onclick={() => (railSheetOpen = false)}
							></button>
							<div
								class="fixed top-1/2 right-2 z-50 w-64 max-w-[calc(100vw-1rem)] -translate-y-1/2"
								role="dialog"
								aria-label={t('myaaw.chat.turns_title')}
							>
								{@render railCard()}
							</div>
						{/if}
					{/if}
					<div
						bind:clientHeight={composerHeight}
						class={['shrink-0', frosted && 'relative z-20']}
						style:margin-top={frosted ? `-${composerHeight}px` : null}
					>
						{#if runError || error}
							<div class="px-4 pb-2">
								<div
									class="mx-auto max-w-4xl rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive"
								>
									{runError || error}
								</div>
							</div>
						{/if}

						{#if pendingApproval}
							<div class="px-2 pt-2 sm:px-4">
								<div class="mx-auto w-full max-w-4xl">
									<PendingApprovalCard approval={pendingApproval} onDecide={decide} />
								</div>
							</div>
						{/if}

						<div
							class={[
								'border-t px-2 py-2 sm:px-4 sm:py-3',
								frosted
									? 'bg-linear-to-b from-transparent via-card/40 to-card/40 backdrop-blur-xl'
									: 'bg-background/80 backdrop-blur'
							]}
						>
							<div class="mx-auto w-full max-w-4xl">
								<div
									role="group"
									class={[
										'rounded-2xl border bg-card shadow-sm transition-colors focus-within:border-ring/60 terminal:rounded-none frost-card',
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
									{#if queue.length > 0}
										<div class="border-b px-3 py-2" data-chat-queue>
											<div class="flex items-center justify-between gap-2">
												<button
													type="button"
													class="flex min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:outline-none"
													onclick={() => (queueOpen = !queueOpen)}
													aria-expanded={queueOpen}
													title={t('myaaw.chat.queue_toggle')}
												>
													<ChevronDown
														class={cn(
															'h-3 w-3 shrink-0 transition-transform',
															!queueOpen && '-rotate-90'
														)}
													/>
													<span class="truncate">
														{queue.length === 1
															? t('myaaw.chat.queue_title_one')
															: t('myaaw.chat.queue_title', { count: queue.length })}
													</span>
												</button>
												<button
													type="button"
													class="shrink-0 rounded-md px-1 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
													onclick={clearQueue}
												>
													{t('myaaw.chat.queue_clear')}
												</button>
											</div>
											{#if queueOpen}
												<ul class="mt-1.5 flex flex-col gap-1">
													{#each queue as item, index (item.id)}
														{@const attachments = item.images.length + item.files.length}
														<li
															class="flex items-center gap-2 rounded-lg border bg-muted/30 py-1 pr-1 pl-2.5"
															data-chat-queued={item.id}
														>
															<span class="size-1.5 shrink-0 rounded-full bg-primary/60"></span>
															<span
																class="min-w-0 flex-1 truncate text-xs text-foreground/90"
																title={queuedPreview(item)}
															>
																{queuedPreview(item)}
															</span>
															{#if attachments > 0}
																<span
																	class="inline-flex shrink-0 items-center gap-0.5 text-[10px] text-muted-foreground"
																>
																	<Paperclip class="h-3 w-3" />
																	{attachments}
																</span>
															{/if}
															<Button
																size="xs"
																variant="ghost"
																class="shrink-0 text-[10px]"
																onclick={() => sendQueuedNow(item)}
																title={t('myaaw.chat.queue_send_now')}
															>
																<Send class="h-3 w-3" />
																{t('myaaw.chat.queue_send_now')}
																{#if index === 0 && !canSend}
																	<kbd
																		class="ml-0.5 hidden rounded border border-border/60 bg-background px-1 py-px font-sans text-[9px] font-normal text-muted-foreground sm:inline"
																	>
																		{queueKeyLabel}
																	</kbd>
																{/if}
															</Button>
															<button
																type="button"
																class="grid size-6 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
																onclick={() => editQueued(item)}
																aria-label={t('myaaw.chat.queue_edit')}
																title={t('myaaw.chat.queue_edit')}
															>
																<Pencil class="h-3 w-3" />
															</button>
															<button
																type="button"
																class="grid size-6 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
																onclick={() => removeQueued(item.id)}
																aria-label={t('myaaw.chat.queue_remove')}
																title={t('myaaw.chat.queue_remove')}
															>
																<X class="h-3 w-3" />
															</button>
														</li>
													{/each}
												</ul>
											{/if}
										</div>
									{/if}
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
									{#if quotes.length > 0}
										<div class="px-3 pt-3" data-ask-quotes>
											<AskQuoteList
												{quotes}
												onremove={removeAskQuote}
												onclear={clearAskQuotes}
												onlocate={(quote) => void locateAskQuote(quote)}
											/>
										</div>
									{/if}
									<textarea
										bind:this={composer}
										bind:value={input}
										use:autosize
										onkeydown={onKeydown}
										onpaste={onPaste}
										rows="1"
										placeholder={t('myaaw.chat.placeholder')}
										class="max-h-40 min-h-11 w-full resize-none rounded-t-2xl bg-transparent px-3 py-2.5 text-sm outline-none placeholder:text-muted-foreground terminal:rounded-none sm:min-h-13 sm:px-4 sm:py-3"
									></textarea>
									<div class="flex items-center justify-between px-2 pb-1.5 sm:px-3 sm:pb-2">
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
											{#if listening}
												<span
													class="hidden min-w-0 truncate text-[10px] text-muted-foreground sm:inline"
												>
													{t('myaaw.chat.speech_listening')}
												</span>
											{/if}
										</div>
										<div class="flex shrink-0 items-center gap-1">
											<DropdownMenu.Root
												onOpenChange={(open) => {
													modelMenuOpen = open;
												}}
											>
												<DropdownMenu.Trigger>
													{#snippet child({ props })}
														<button
															{...props}
															type="button"
															class="flex h-8 shrink-0 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none disabled:opacity-60"
															disabled={switchingModel}
															title={activeProvider
																? `${activeProvider} · ${modelDisplayName(activeModel)}`
																: t('myaaw.chat.model_picker')}
															aria-label={t('myaaw.chat.model_picker')}
														>
															{#if switchingModel}
																<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
															{:else}
																<Cpu class="h-3.5 w-3.5 shrink-0" />
															{/if}
															<span class="hidden truncate sm:inline sm:max-w-56">
																{activeModel
																	? modelDisplayName(activeModel)
																	: t('myaaw.chat.model_label')}
															</span>
															<ChevronDown class="h-3 w-3 shrink-0 opacity-60" />
														</button>
													{/snippet}
												</DropdownMenu.Trigger>
												<DropdownMenu.Content align="end" side="top" class="max-h-80 w-72">
													{#if (!providerInfoLoaded || loadingModels) && !modelGroups}
														<div
															class="flex items-center gap-2 px-2 py-2 text-xs text-muted-foreground"
														>
															<Loader2 class="h-3.5 w-3.5 animate-spin" />
															{t('myaaw.chat.model_loading')}
														</div>
													{:else if !modelGroups || modelGroups.length === 0}
														<p class="px-2 py-1 text-xs text-muted-foreground">
															{t('myaaw.chat.model_none')}
														</p>
													{:else}
														{#each modelGroups as group, index (group.name)}
															{#if index > 0}<DropdownMenu.Separator />{/if}
															<DropdownMenu.Group>
																<DropdownMenu.Label
																	class="flex items-center gap-1.5 text-[10px] tracking-wide uppercase"
																>
																	<span class="truncate">{group.name}</span>
																	{#if group.name === activeProvider}
																		<span
																			class="shrink-0 rounded-full bg-primary/10 px-1.5 py-0.5 text-[9px] font-medium text-primary normal-case"
																		>
																			{t('myaaw.chat.model_active')}
																		</span>
																	{/if}
																</DropdownMenu.Label>
																{#each group.models as model (model)}
																	{@const capability = capabilityFor(modelCapabilities, model)}
																	{@const chips = capabilityChips(model)}
																	<DropdownMenu.Item
																		class="items-start gap-2 py-1.5"
																		onclick={() => void switchModel(group.name, model)}
																	>
																		<Check
																			class={cn(
																				'mt-0.5 h-3.5 w-3.5 shrink-0',
																				group.name === activeProvider && model === activeModel
																					? 'opacity-100'
																					: 'opacity-0'
																			)}
																		/>
																		<span class="min-w-0 flex-1">
																			<span class="flex items-center gap-1.5">
																				<span class="truncate font-mono text-xs">
																					{modelDisplayName(model)}
																				</span>
																				{#if modelStatusLabel(capability?.status)}
																					<span
																						class="shrink-0 rounded-full border px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground"
																					>
																						{modelStatusLabel(capability?.status)}
																					</span>
																				{/if}
																				{#if contextWindowLabel(model)}
																					<span
																						class="ml-auto shrink-0 text-[9px] text-muted-foreground"
																					>
																						{t('myaaw.chat.context_window', {
																							count: contextWindowLabel(model)
																						})}
																					</span>
																				{/if}
																			</span>
																			{#if chips.length > 0}
																				<span class="mt-1 flex flex-wrap gap-1">
																					{#each chips as [key, labelKey] (key)}
																						<span
																							class="rounded bg-muted px-1.5 py-0.5 text-[9px] font-normal text-muted-foreground"
																						>
																							{t(labelKey)}
																						</span>
																					{/each}
																				</span>
																			{/if}
																		</span>
																	</DropdownMenu.Item>
																{/each}
																{#if group.models.length === 0}
																	<p class="px-2 py-1 text-[11px] text-muted-foreground">
																		{t('myaaw.chat.model_none')}
																	</p>
																{/if}
																{#if group.warning}
																	<p
																		class="px-2 pb-1 text-[10px] wrap-break-word text-amber-600 dark:text-amber-400"
																	>
																		{group.warning}
																	</p>
																{/if}
															</DropdownMenu.Group>
														{/each}
													{/if}
												</DropdownMenu.Content>
											</DropdownMenu.Root>
											{#if activeThinkingOptions.length > 0}
												<DropdownMenu.Root>
													<DropdownMenu.Trigger>
														{#snippet child({ props })}
															<button
																{...props}
																type="button"
																class="flex h-8 shrink-0 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none disabled:opacity-60"
																disabled={switchingThinking}
																title={t('myaaw.chat.thinking_level')}
																aria-label={t('myaaw.chat.thinking_level')}
															>
																<Brain class="h-3.5 w-3.5 shrink-0" />
																<span class="hidden whitespace-nowrap sm:inline">
																	{activeThinkingValue
																		? thinkingLabel(activeThinkingValue)
																		: t('myaaw.chat.thinking_default_short')}
																</span>
																<ChevronDown class="h-3 w-3 shrink-0 opacity-60" />
															</button>
														{/snippet}
													</DropdownMenu.Trigger>
													<DropdownMenu.Content align="end" side="top" class="w-44">
														<DropdownMenu.Item
															class="items-start gap-2"
															onclick={() => void switchThinkingLevel('')}
														>
															<Check
																class={cn(
																	'mt-0.5 h-3.5 w-3.5 shrink-0',
																	activeThinkingValue ? 'opacity-0' : 'opacity-100'
																)}
															/>
															{t('myaaw.chat.thinking_default')}
														</DropdownMenu.Item>
														<DropdownMenu.Separator />
														{#each activeThinkingOptions as value (value)}
															<DropdownMenu.Item
																class="items-start gap-2"
																onclick={() => void switchThinkingLevel(value)}
															>
																<Check
																	class={cn(
																		'mt-0.5 h-3.5 w-3.5 shrink-0',
																		activeThinkingValue === value ? 'opacity-100' : 'opacity-0'
																	)}
																/>
																{thinkingLabel(value)}
															</DropdownMenu.Item>
														{/each}
													</DropdownMenu.Content>
												</DropdownMenu.Root>
											{/if}
											<DropdownMenu.Root>
												<DropdownMenu.Trigger>
													{#snippet child({ props })}
														<button
															{...props}
															type="button"
															class="flex h-8 shrink-0 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none disabled:opacity-60"
															disabled={switchingApproval}
															title={approvalMode === 'off'
																? t('myaaw.chat.approval_full_access')
																: t('myaaw.chat.approval_manual')}
															aria-label={t('myaaw.chat.approval_picker')}
														>
															{#if switchingApproval}
																<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
															{:else if approvalMode === 'off'}
																<ShieldOff class="h-3.5 w-3.5 shrink-0" />
															{:else}
																<ShieldCheck class="h-3.5 w-3.5 shrink-0" />
															{/if}
															<span class="hidden whitespace-nowrap sm:inline">
																{approvalMode === 'off'
																	? t('myaaw.chat.approval_full_access')
																	: t('myaaw.chat.approval_manual')}
															</span>
															<ChevronDown class="h-3 w-3 shrink-0 opacity-60" />
														</button>
													{/snippet}
												</DropdownMenu.Trigger>
												<DropdownMenu.Content align="end" side="top" class="w-56">
													<DropdownMenu.Item
														class="items-start gap-2"
														onclick={() => void switchApproval('off')}
													>
														<ShieldOff class="mt-0.5 h-4 w-4 shrink-0" />
														<span class="min-w-0 flex-1">
															<span class="block">
																{t('myaaw.chat.approval_full_access')}
															</span>
															<span
																class="block text-[10px] whitespace-normal text-muted-foreground"
															>
																{t('myaaw.chat.approval_full_access_desc')}
															</span>
														</span>
														<Check
															class={cn(
																'mt-0.5 h-3.5 w-3.5 shrink-0',
																approvalMode === 'off' ? 'opacity-100' : 'opacity-0'
															)}
														/>
													</DropdownMenu.Item>
													<DropdownMenu.Item
														class="items-start gap-2"
														onclick={() => void switchApproval('manual')}
													>
														<ShieldCheck class="mt-0.5 h-4 w-4 shrink-0" />
														<span class="min-w-0 flex-1">
															<span class="block">
																{t('myaaw.chat.approval_manual')}
															</span>
															<span
																class="block text-[10px] whitespace-normal text-muted-foreground"
															>
																{t('myaaw.chat.approval_manual_desc')}
															</span>
														</span>
														<Check
															class={cn(
																'mt-0.5 h-3.5 w-3.5 shrink-0',
																approvalMode === 'manual' ? 'opacity-100' : 'opacity-0'
															)}
														/>
													</DropdownMenu.Item>
												</DropdownMenu.Content>
											</DropdownMenu.Root>
											{#if streaming && !canSend}
												<Button
													size="icon"
													variant="destructive"
													class="rounded-full"
													onclick={stop}
													aria-label={t('myaaw.chat.stop')}
													title={t('myaaw.chat.stop')}
												>
													<Square class="h-4 w-4" />
												</Button>
											{:else if streaming && sendWhileStreaming === 'queue'}
												<Button
													size="icon"
													class="rounded-full"
													onclick={send}
													aria-label={t('myaaw.chat.queue_send')}
													title={t('myaaw.chat.queue_send')}
												>
													<ListEnd class="h-4 w-4" />
												</Button>
											{:else if streaming && sendWhileStreaming === 'interrupt'}
												<Button
													size="icon"
													class="rounded-full"
													onclick={send}
													aria-label={t('myaaw.chat.send')}
													title={t('myaaw.chat.send')}
												>
													<Send class="h-4 w-4" />
												</Button>
											{:else if streaming}
												<Button
													size="icon"
													class="rounded-full"
													disabled
													aria-label={t('myaaw.chat.send')}
													title={t('myaaw.chat.send')}
												>
													<Send class="h-4 w-4" />
												</Button>
											{:else}
												<Button
													size="icon"
													class="rounded-full"
													onclick={send}
													disabled={!canSend}
													aria-label={t('myaaw.chat.send')}
													title={t('myaaw.chat.send')}
												>
													<Send class="h-4 w-4" />
												</Button>
											{/if}
										</div>
									</div>
									<div
										class={cn(
											'flex flex-nowrap items-center justify-center gap-x-2 border-t px-3 py-1.5 text-[10px] leading-tight text-muted-foreground',
											// Focus mode keeps the composer bare: no session counters.
											focusMode && 'hidden'
										)}
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
												<span class="whitespace-nowrap">
													{turnLabel(statsTurns)} · {stepsLabel(statsSteps)}
													{#if statsTokPerSec > 0}
														· {t('myaaw.chat.stats_tok_per_sec', { count: statsTokPerSec })}
													{/if}
												</span>
											</button>
											{#if openStats && sessionStats}
												<div
													class="absolute bottom-full left-0 z-30 mb-2 w-64 max-w-[calc(100vw-1.5rem)] rounded-lg border bg-popover p-3 text-xs text-popover-foreground shadow-md frost-popover sm:left-1/2 sm:-translate-x-1/2"
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

										{#if statsTokenText || statsCacheText || contextKnown}
											<div class="relative min-w-0">
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
													{#if contextKnown}
														<span
															class="inline-flex items-center gap-1"
															title={t('myaaw.chat.context_used', {
																percent: contextPercent
															})}
														>
															<svg
																viewBox="0 0 36 36"
																class="h-3.5 w-3.5 shrink-0 -rotate-90"
																aria-hidden="true"
															>
																<circle
																	cx="18"
																	cy="18"
																	r="15"
																	fill="none"
																	class="stroke-muted"
																	stroke-width="4"
																/>
																<circle
																	cx="18"
																	cy="18"
																	r="15"
																	fill="none"
																	class={contextStrokeClass(contextPercent)}
																	stroke-width="4"
																	stroke-linecap="round"
																	stroke-dasharray={contextDash}
																/>
															</svg>
															<span class="tabular-nums">{contextPercent}%</span>
														</span>
														<span aria-hidden="true">·</span>
													{/if}
													<Database class="h-3 w-3 shrink-0" />
													{#if statsTokenText}
														<span class="whitespace-nowrap">{statsTokenText}</span>
													{/if}
													{#if statsCacheText}
														<span class="hidden whitespace-nowrap sm:inline"
															>· {statsCacheText}</span
														>
													{/if}
												</button>
												{#if openUsage && (sessionStats || onlineContext)}
													<div
														class="absolute right-0 bottom-full z-30 mb-2 w-64 max-w-[calc(100vw-1.5rem)] rounded-lg border bg-popover p-3 text-xs text-popover-foreground shadow-md frost-popover sm:right-auto sm:left-1/2 sm:-translate-x-1/2"
														onclick={(event) => event.stopPropagation()}
														role="presentation"
													>
														{#if onlineContext}
															<div class="space-y-1.5">
																<div class="flex items-center justify-between gap-3 font-medium">
																	<span>
																		{contextKnown
																			? t('myaaw.chat.context_used', { percent: contextPercent })
																			: t('myaaw.chat.context_unknown')}
																	</span>
																	<span class="tabular-nums">{contextTokenText}</span>
																</div>
																{#if contextPartsTotal > 0}
																	<div class="flex h-1.5 overflow-hidden rounded-full bg-muted">
																		<div
																			class="bg-primary/40"
																			style={`width: ${(contextSystem / contextPartsTotal) * 100}%`}
																		></div>
																		<div
																			class="bg-primary/70"
																			style={`width: ${(contextTool / contextPartsTotal) * 100}%`}
																		></div>
																		<div
																			class="bg-primary"
																			style={`width: ${(contextMessage / contextPartsTotal) * 100}%`}
																		></div>
																	</div>
																{/if}
																<dl class="space-y-1.5">
																	<div class="flex justify-between gap-3">
																		<dt class="text-muted-foreground">
																			{t('myaaw.chat.context_system')}
																		</dt>
																		<dd class="tabular-nums">~{formatTokens(contextSystem)}</dd>
																	</div>
																	<div class="flex justify-between gap-3">
																		<dt class="text-muted-foreground">
																			{t('myaaw.chat.context_tools')}
																		</dt>
																		<dd class="tabular-nums">~{formatTokens(contextTool)}</dd>
																	</div>
																	<div class="flex justify-between gap-3">
																		<dt class="text-muted-foreground">
																			{t('myaaw.chat.context_messages')}
																		</dt>
																		<dd class="tabular-nums">~{formatTokens(contextMessage)}</dd>
																	</div>
																</dl>
															</div>
														{/if}
														{#if sessionStats}
															{#if onlineContext}<div class="my-2 h-px bg-border"></div>{/if}
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
														{/if}
													</div>
												{/if}
											</div>
										{/if}
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if agent && focusMode}
		<!-- Focus mode keeps the panel mounted so opening and closing slides it in
		     and out, like the left drawer in advanced mode. -->
		{#if historyOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 bg-black/40 lg:hidden"
				aria-label={t('myaaw.chat.history_hide')}
				onclick={closeSidePanel}
			></button>
		{/if}
		<aside
			class={[
				'fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l bg-card transition-transform duration-200 lg:w-88 lg:max-w-none frost-drawer',
				historyOpen ? 'translate-x-0' : 'translate-x-full'
			]}
			inert={!historyOpen}
		>
			{#if rightTab === 'attachments'}
				<!-- The focus header floats over the panel on a desktop and its actions
				     would cover the title, so keep the panel below them there. A phone
				     drawer sits above the header and needs no gap. -->
				<div class="min-h-0 flex-1 lg:pt-16">
					<AttachmentPanel
						{agent}
						{conversationId}
						refreshKey={attachmentsRefresh}
						onOpen={openAttachment}
						onShowInChat={showAttachmentInChat}
						onClose={closeSidePanel}
					/>
				</div>
			{:else}
				<FocusAgentPanel
					{agent}
					activeId={conversationId}
					bind:tab={focusPanelTab}
					onOpen={openConversation}
					onClose={toggleHistory}
					onOpenContext={(path) => {
						focusContextPath = path;
						if (browser && window.innerWidth < 1024) toggleHistory();
					}}
				/>
			{/if}
		</aside>
	{:else if agent && !classicMode}
		<!-- Drawer on small screens, a static third column from lg up. Kept mounted
		     on desktop so hiding it animates the width instead of snapping the
		     transcript and the composer. -->
		{#if historyOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 bg-black/40 lg:hidden"
				aria-label={t('myaaw.chat.history_hide')}
				onclick={closeSidePanel}
			></button>
		{/if}
		<aside
			inert={!historyOpen}
			class={[
				'fixed inset-y-0 right-0 z-50 w-full flex-col overflow-hidden border-l bg-card lg:static lg:z-auto lg:h-full lg:max-w-none frost-drawer lg:transition-[width] lg:duration-200',
				historyOpen ? 'flex lg:w-72' : 'hidden lg:flex lg:w-0 lg:border-l-0'
			]}
		>
			<div class="flex h-full w-full flex-col lg:w-72 lg:min-w-72">
				{#if rightTab === 'attachments'}
					<AttachmentPanel
						{agent}
						{conversationId}
						refreshKey={attachmentsRefresh}
						onOpen={openAttachment}
						onShowInChat={showAttachmentInChat}
						onClose={closeSidePanel}
					/>
				{:else}
					<ConversationList
						activeId={conversationId}
						onOpen={openConversation}
						onClose={closeSidePanel}
						onNewInFolder={(folderId) =>
							void goto(`/app/chat?new=1&folder=${encodeURIComponent(folderId)}`)}
						onDeleted={onHistoryDeleted}
						onRenamed={onHistoryRenamed}
						bind:searchInput={searchBox}
					/>
				{/if}
			</div>
		</aside>
	{/if}

	{#if agent && classicMode}
		<!-- Classic borrows the advanced layout: a drawer on phones and a static
		     third column from lg up. History already lives in its left sidebar. -->
		{#if classicAttachmentsOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 bg-black/40 lg:hidden"
				aria-label={t('myaaw.chat.attachments_close_list')}
				onclick={() => (classicAttachmentsOpen = false)}
			></button>
		{/if}
		<aside
			inert={!classicAttachmentsOpen}
			class={[
				'fixed inset-y-0 right-0 z-50 w-full flex-col overflow-hidden border-l bg-card lg:static lg:z-auto lg:h-full lg:max-w-none frost-drawer lg:transition-[width] lg:duration-200',
				classicAttachmentsOpen ? 'flex lg:w-72' : 'hidden lg:flex lg:w-0 lg:border-l-0'
			]}
		>
			<div class="flex h-full w-full flex-col lg:w-72 lg:min-w-72">
				<AttachmentPanel
					{agent}
					{conversationId}
					refreshKey={attachmentsRefresh}
					onOpen={openAttachment}
					onShowInChat={showAttachmentInChat}
					onClose={() => (classicAttachmentsOpen = false)}
				/>
			</div>
		</aside>
	{/if}

	<ImageLightbox bind:open={lightboxOpen} bind:index={lightboxIndex} items={lightboxItems} />

	{#if askAnchor}
		<!-- The one action a text selection unlocks. It floats above the reply so the
		     reader can quote without hunting for a toolbar. -->
		<button
			type="button"
			data-ask-anchor
			class="fixed z-50 inline-flex items-center gap-1.5 rounded-full border bg-popover px-3 py-1.5 text-xs font-medium text-popover-foreground shadow-md transition-colors hover:bg-muted"
			style:top={`${askAnchor.top}px`}
			style:left={`${askAnchor.left}px`}
			title={t('myaaw.chat.ask_hint')}
			aria-label={t('myaaw.chat.ask_hint')}
			onmousedown={(event) => event.preventDefault()}
			onclick={addAskQuote}
			in:fade={{ duration: 90 }}
		>
			<Quote class="h-3.5 w-3.5" />
			{t('myaaw.chat.ask')}
		</button>
	{/if}

	<Dialog.Root
		open={quotePreview !== null}
		onOpenChange={(open) => {
			if (!open) quotePreview = null;
		}}
	>
		<Dialog.Content class="sm:max-w-xl" data-ask-preview>
			<Dialog.Header>
				<Dialog.Title>{t('myaaw.chat.ask_preview_title')}</Dialog.Title>
				<Dialog.Description>{t('myaaw.chat.ask_preview_desc')}</Dialog.Description>
			</Dialog.Header>
			<div class="max-h-[60vh] space-y-2 overflow-y-auto pr-1">
				{#each quotePreview ?? [] as quote (quote.id)}
					{@const Icon = askQuoteIcon(quote)}
					<div class="rounded-xl border bg-muted/30 px-3 py-2">
						<span
							class="mb-1.5 flex items-center gap-1.5 text-[10px] font-medium text-muted-foreground"
						>
							<Icon class="h-3 w-3 shrink-0" />
							{askQuoteLabel(quote)}
						</span>
						<p class="max-h-52 overflow-y-auto text-xs whitespace-pre-wrap text-foreground/85">
							{quote.text}
						</p>
					</div>
				{/each}
			</div>
		</Dialog.Content>
	</Dialog.Root>
</div>
