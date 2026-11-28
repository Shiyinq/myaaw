<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { tick, untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import {
		AlertTriangle,
		ArrowDown,
		AtSign,
		Brain,
		ChevronDown,
		Clock,
		Download,
		FileText,
		Loader2,
		Mic,
		PanelRight,
		Paperclip,
		Play,
		Quote,
		Reply,
		Send,
		Square,
		Users,
		Wrench,
		X
	} from 'lucide-svelte';
	import AskQuoteList from '$lib/components/app/AskQuoteList.svelte';
	import AttachmentPreview from '$lib/components/app/AttachmentPreview.svelte';
	import LightBox from '$lib/components/app/LightBox.svelte';
	import PendingApprovalCard from '$lib/components/app/PendingApprovalCard.svelte';
	import RunAttachments from '$lib/components/app/RunAttachments.svelte';
	import ExtraMessages from '$lib/components/extra/ExtraMessages.svelte';
	import InteractionBadge from '$lib/components/extra/InteractionBadge.svelte';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import ChatStep from '$lib/components/common/ChatStep.svelte';
	import ChatTime from '$lib/components/common/ChatTime.svelte';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import GroupInspectorPanel from '$lib/components/groups/GroupInspectorPanel.svelte';
	import GroupReplyBar from '$lib/components/groups/GroupReplyBar.svelte';
	import {
		AttachWorkingDirDialog,
		ChatRightActivityBar,
		ProjectComposerControls,
		ProjectSidebarPanel,
		ProjectTabBar,
		ProjectTerminalGroup,
		ProjectWorkspacePane
	} from '$lib/components/workspace';
	import {
		createProjectWorkspace,
		type ChatRightPanelKind
	} from '$lib/stores/projectWorkspace.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import {
		highlightMentions,
		mentionPillClass,
		splitMentionSegments,
		type MentionTarget
	} from '$lib/components/groups/mentions';
	import { Button } from '$lib/components/ui/button';
	import GroupMessagesSkeleton from './GroupMessagesSkeleton.svelte';
	import { colorOf } from '$lib/bloub';
	import {
		agentSelection,
		chatStore,
		groupsStore,
		preferences,
		toastStore,
		topBarTitle
	} from '$lib/stores';
	import { approvalQueue, type PendingApproval } from '$lib/stores/approvalQueue.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { cn } from '$lib/utils';
	import { getErrorMessage } from '$lib/utils/api';
	import { MAX_ASK_QUOTES, MAX_ASK_QUOTE_CHARS } from '$lib/utils/ask';
	import {
		downloadGroupAttachment,
		groupAttachmentUrl,
		stripAttachDirectives,
		toRunAttachments
	} from '$lib/utils/attachments';
	import {
		mergeExtraMessages,
		parseInteractionText,
		patchExtraBlockState
	} from '$lib/utils/interactions';
	import { formatShortcut, isOverlayOpen, isTypingTarget, matchesShortcut } from '$lib/utils/keys';
	import type { SSEEvent } from '$lib/utils/sse';
	import type {
		ApprovalDecision,
		AskQuote,
		AskQuoteSource,
		ChatFile,
		ExtraMessage,
		Group,
		GroupAttachment,
		GroupChunkEvent,
		GroupExtraBlockUpdatedEvent,
		GroupExtraMessagesEvent,
		GroupPendingApproval,
		GroupErrorEvent,
		GroupMessage,
		GroupMessageEvent,
		GroupRunEndEvent,
		GroupRunState,
		GroupStatusEvent,
		GroupThinkingEvent,
		GroupToolEndEvent,
		GroupToolStartEvent,
		GroupTurnEndEvent,
		GroupTurnStartEvent,
		InteractionRequest,
		InteractionState,
		LightboxImage,
		ProjectDiffTarget,
		ProjectMultiDiffTarget,
		ReplyPreview,
		TimelineItem
	} from '$lib/types';

	const { t, locale } = useTranslation();

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

	const MAX_IMAGES = 6;
	const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
	const MAX_IMAGE_EDGE = 1600;
	const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

	const MAX_FILES = 8;
	const MAX_FILE_BYTES = 10 * 1024 * 1024;
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

	interface LiveStep extends TimelineItem {
		localId: string;
		running?: boolean;
		expanded?: boolean;
	}

	interface ActiveTurn {
		agent: string;
		agentColor: string;
		turn: number;
		maxTurns: number;
		requestId: string;
		text: string;
		timeline: LiveStep[];
		extraMessages?: ExtraMessage[];
	}

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

	interface MentionCandidate {
		handle: string;
		label: string;
		isAll?: boolean;
		agentColor?: string;
		running?: boolean;
	}

	const EMPTY_RUN_STATE: GroupRunState = {
		running: false,
		paused: false,
		pause_reason: '',
		current_agent: '',
		turn_count: 0,
		max_turns: 8,
		queue: []
	};

	const PAGE_SIZE = 50;
	const INSPECTOR_KEY = 'myaaw_group_inspector_open';

	const initialGroupId = page.params.id ?? '';
	const initialCached = initialGroupId ? groupsStore.peekDetail(initialGroupId) : undefined;
	const initialGroup =
		initialCached?.group ?? (initialGroupId ? groupsStore.peekGroup(initialGroupId) : null);

	let groupId = $derived(page.params.id ?? '');
	let group = $state<Group | null>(initialGroup);
	let messages = $state<GroupMessage[]>(initialCached?.messages ?? []);
	let hasMore = $state(Boolean(initialCached?.has_more));
	let totalMessages = $state(
		Math.max(
			initialCached?.total_messages ?? initialCached?.messages.length ?? 0,
			initialCached?.messages.length ?? 0
		)
	);
	let loadingOlder = $state(false);
	let prepending = false;
	let switchingGroup = false;
	let jumpingToMessage = false;
	let jumpTimer: ReturnType<typeof setTimeout> | undefined;
	let runState = $state<GroupRunState>(initialCached?.run_state ?? { ...EMPTY_RUN_STATE });
	let activeTurn = $state<ActiveTurn | null>(null);
	let loading = $state(!initialCached);
	let loadError = $state('');

	let input = $state('');
	let sending = $state(false);
	let stopping = $state(false);
	let continuing = $state(false);

	// Pending image & document attachments for the next message.
	let pending = $state<PendingImage[]>([]);
	let pendingFiles = $state<PendingFile[]>([]);
	let dragOver = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);

	// Voice dictation state.
	let listening = $state(false);
	let speech: RecognitionLike | null = null;
	let speechHead = '';
	let speechSettled = '';

	// WhatsApp-style full-message reply target.
	let replyTo = $state<GroupMessage | null>(null);
	// Text-selection quotes (same UX as 1-on-1 chat, coexisting cleanly with Reply).
	let pendingQuotes = $state<AskQuote[]>([]);
	let askCounter = 0;
	let askAnchor = $state<AskAnchor | null>(null);
	let quoteRects = $state<{ left: number; top: number; width: number; height: number }[]>([]);
	let quoteMarkTimer: ReturnType<typeof setTimeout> | undefined;

	// Highlighted target message after clicking a reply_preview card.
	let highlightedMessageId = $state('');
	let highlightTimer: ReturnType<typeof setTimeout> | undefined;

	// Collapsible step state for persisted agent messages and the live turn.
	let expandedSteps = $state<Record<string, boolean>>({});
	let expandedStepGroups = $state<Record<string, boolean>>({});
	let liveStepsOverride = $state<boolean | null>(null);

	// Shared project workspace controller (reused identically from 1-on-1 DM chat).
	const project = createProjectWorkspace();
	const terminalGroupHolder: { current?: ProjectTerminalGroup } = {};

	// Right panel state: unified between Group Inspector ('info' | 'attachments') and Project ('project').
	let activeRightPanel = $state<ChatRightPanelKind | null>(
		browser && window.innerWidth >= 1024 && localStorage.getItem(INSPECTOR_KEY) !== 'false'
			? 'info'
			: null
	);
	let inspectorTab = $state<'members' | 'attachments' | 'settings'>('members');
	const inspectorOpen = $derived(activeRightPanel !== null);
	let attachmentsRefreshKey = $state(0);
	let previewAttachment = $state<GroupAttachment | null>(null);
	let lightboxOpen = $state(false);
	let lightboxIndex = $state(0);
	let lightboxItems = $state<LightboxImage[]>([]);

	/** Effective agent host context backing shared /fs, /git, and /terminal routes for this group. */
	const workspaceAgent = $derived.by(() => {
		const known = new Set(agentSelection.agents.map((a) => a.name));
		const runningMember = group?.members.find((m) => m.running && known.has(m.name));
		if (runningMember) return runningMember.name;
		const firstMember = group?.members.find((m) => known.has(m.name));
		if (firstMember) return firstMember.name;
		return (
			agentSelection.agents.find((a) => a.default)?.name || agentSelection.agents[0]?.name || ''
		);
	});
	const projectContextId = $derived(group ? `group:${group.id}` : '');
	const projectScopeKeyStr = $derived(
		group && project.workingDir ? `group:${group.id}:${project.workingDir}` : ''
	);

	// Sync shared project workspace controller whenever the active group changes.
	$effect(() => {
		const current = group;
		if (!current) {
			untrack(() => project.reset(false));
			return;
		}
		const wd = current.working_dir ?? '';
		const exists = current.working_dir_exists ?? true;
		const git = Boolean(current.has_git);
		const branch = current.git_branch ?? '';
		const scopeAgent = `group:${current.id}`;
		const ctxId = `group:${current.id}`;
		untrack(() => {
			project.setDirectory(wd, exists, git, scopeAgent, ctxId, { gitBranch: branch });
			if (!wd && activeRightPanel === 'project') {
				activeRightPanel =
					browser && window.innerWidth >= 1024 && localStorage.getItem(INSPECTOR_KEY) !== 'false'
						? 'info'
						: null;
			}
		});
	});

	function openImageAttachmentsLightbox(images: GroupAttachment[], index: number) {
		if (images.length === 0) return;
		lightboxItems = images.map((att) => ({
			src: groupAttachmentUrl(att),
			name: att.file_name,
			downloadSrc: groupAttachmentUrl(att, { download: true })
		}));
		lightboxIndex = index;
		lightboxOpen = true;
	}

	function openAttachmentPreview(item: GroupAttachment) {
		previewAttachment = item;
	}

	function showAttachmentInChat(item: GroupAttachment) {
		previewAttachment = null;
		project.selectChat();
		if (browser && window.innerWidth < 1024 && activeRightPanel !== null) {
			activeRightPanel = null;
		}
		void jumpToMessage(item.message_id);
	}

	function closeRightPanel() {
		activeRightPanel = null;
		project.panelOpen = false;
		if (browser && window.innerWidth >= 1024) {
			localStorage.setItem(INSPECTOR_KEY, 'false');
		}
	}

	function openProjectPanel() {
		if (!project.workingDir) {
			project.dialogOpen = true;
			return;
		}
		if (activeRightPanel === 'project') {
			closeRightPanel();
			return;
		}
		activeRightPanel = 'project';
		project.panelOpen = true;
	}

	function toggleAttachmentsPanel() {
		if (activeRightPanel === 'attachments') {
			closeRightPanel();
			return;
		}
		inspectorTab = 'attachments';
		activeRightPanel = 'attachments';
		project.panelOpen = false;
	}

	async function handleSelectWorkingDir(dir: string) {
		if (!group) return;
		try {
			const updated = await groupsStore.update(group.id, { working_dir: dir });
			group = updated;
			project.setDirectory(
				updated.working_dir ?? dir,
				updated.working_dir_exists ?? true,
				Boolean(updated.has_git),
				`group:${updated.id}`,
				`group:${updated.id}`,
				{ openPanelIfFresh: true, gitBranch: updated.git_branch ?? '' }
			);
			activeRightPanel = 'project';
			toastStore.success(t('myaaw.workspace.attached'));
		} catch (err) {
			toastStore.error(getErrorMessage(err));
			throw err;
		}
	}

	function openProjectFile(virtualPath: string) {
		previewAttachment = null;
		const mobile = browser && window.innerWidth < 1024;
		project.openFile(virtualPath, mobile);
		if (mobile) activeRightPanel = null;
	}

	function openProjectDiff(req: ProjectDiffTarget) {
		previewAttachment = null;
		const mobile = browser && window.innerWidth < 1024;
		project.openDiff(req, mobile);
		if (mobile) activeRightPanel = null;
	}

	function openProjectMultiDiff(req: ProjectMultiDiffTarget) {
		previewAttachment = null;
		const mobile = browser && window.innerWidth < 1024;
		project.openMultiDiff(req, mobile);
		if (mobile) activeRightPanel = null;
	}

	function toggleProjectTerminal() {
		previewAttachment = null;
		const mobile = browser && window.innerWidth < 1024;
		project.toggleTerminal();
		if (mobile) activeRightPanel = null;
	}

	function closeProjectTerminal() {
		terminalGroupHolder.current?.terminateAll();
		project.closeTerminal();
	}

	function handleComposerFolderClick() {
		if (!project.workingDir) {
			project.dialogOpen = true;
			return;
		}
		openProjectPanel();
	}

	// Scroll & composer refs.
	let scroller = $state<HTMLDivElement | null>(null);
	let transcript = $state<HTMLDivElement | null>(null);
	let composer = $state<HTMLTextAreaElement | null>(null);
	let stickToBottom = $state(true);
	let unreadBelowCount = $state(0);

	// @mention autocomplete state.
	let mentionOpen = $state(false);
	let mentionQuery = $state('');
	let mentionStart = $state(-1);
	let mentionIndex = $state(0);

	let stepCounter = 0;
	let loadToken = 0;

	const memberColorMap = $derived.by(() => {
		const map = new Map<string, string>();
		for (const agent of agentSelection.agents) {
			if (agent.agent_color) map.set(agent.name, agent.agent_color);
		}
		for (const member of group?.members ?? []) {
			if (!map.has(member.name) && member.agent_color) {
				map.set(member.name, member.agent_color);
			}
		}
		return map;
	});

	const groupApprovals = $derived(approvalQueue.listForGroup(groupId));
	const activeGroupApproval = $derived(groupApprovals[0] ?? null);

	async function decideGroupApproval(entry: PendingApproval, decision: ApprovalDecision) {
		try {
			await chatStore.decide(entry.agent, entry.approvalId, decision);
			approvalQueue.resolved({ approval_id: entry.approvalId });
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		}
	}

	const mentionCandidates = $derived.by<MentionCandidate[]>(() => {
		if (!group) return [];
		const q = mentionQuery.toLowerCase();
		const items: MentionCandidate[] = [
			{
				handle: 'all',
				label: t('myaaw.groups.mention_all_desc'),
				isAll: true
			},
			...group.members.map((member) => {
				const live = agentSelection.agents.find((a) => a.name === member.name);
				return {
					handle: member.name,
					label: (live ? live.running : member.running)
						? t('myaaw.groups.online')
						: t('myaaw.bots.stopped'),
					agentColor: live?.agent_color || member.agent_color,
					running: live ? live.running : member.running
				};
			})
		];
		return items.filter((item) => item.handle.toLowerCase().includes(q));
	});

	const mentionTargets = $derived.by<MentionTarget[]>(() =>
		(group?.members ?? []).map((member) => ({
			name: member.name,
			color: memberColorMap.get(member.name) || member.agent_color
		}))
	);

	const canSend = $derived(
		(input.trim().length > 0 || pending.length > 0 || pendingFiles.length > 0) &&
			!sending &&
			Boolean(group)
	);

	function colorForAgent(name: string, fallback?: string): string {
		return memberColorMap.get(name) || fallback || 'encre';
	}

	function toggleInspector() {
		if (activeRightPanel === 'info') {
			closeRightPanel();
			return;
		}
		if (inspectorTab === 'attachments') {
			inspectorTab = 'members';
		}
		activeRightPanel = 'info';
		project.panelOpen = false;
		if (browser && window.innerWidth >= 1024) {
			localStorage.setItem(INSPECTOR_KEY, 'true');
		}
	}

	// Keep the activity bar indicator synchronized when the user switches tabs inside GroupInspectorPanel.
	$effect(() => {
		if (activeRightPanel === 'info' && inspectorTab === 'attachments') {
			activeRightPanel = 'attachments';
		} else if (activeRightPanel === 'attachments' && inspectorTab !== 'attachments') {
			activeRightPanel = 'info';
		}
	});

	// Support `?inspector=1` / `?inspector=settings` deep links from sidebar actions.
	$effect(() => {
		const param = page.url.searchParams.get('inspector');
		if (!param) return;
		activeRightPanel = 'info';
		inspectorTab = param === 'members' ? 'members' : 'settings';
	});

	$effect(() => () => topBarTitle.clear());

	// Hide the floating desktop group title while AttachmentPreview or ProjectTabBar
	// owns the top of the column (matching regular 1-on-1 chat).
	$effect(() => {
		topBarTitle.setDesktopHidden(Boolean(previewAttachment || project.hasTabBar));
		return () => topBarTitle.setDesktopHidden(false);
	});

	// Load group detail whenever route param changes.
	$effect(() => {
		const id = groupId;
		if (!id) return;
		const token = ++loadToken;
		untrack(() => {
			const cached = groupsStore.peekDetail(id);
			const fallbackGroup = cached?.group ?? groupsStore.peekGroup(id);
			loadError = '';
			loadingOlder = false;
			prepending = false;
			switchingGroup = true;
			jumpingToMessage = false;
			stickToBottom = true;
			unreadBelowCount = 0;
			sseStatusReceived = false;
			liveIdsDuringLoad = new Set<string>();
			unreadBannerId = '';
			unreadBannerCount = 0;
			previewAttachment = null;
			activeTurn = null;
			replyTo = null;
			pendingQuotes = [];
			pending = [];
			pendingFiles = [];
			stopSpeech();
			askAnchor = null;
			mentionOpen = false;
			expandedSteps = {};
			expandedStepGroups = {};
			liveStepsOverride = null;

			if (cached) {
				group = cached.group;
				messages = cached.messages;
				hasMore = Boolean(cached.has_more);
				totalMessages = Math.max(
					cached.total_messages ?? cached.messages.length,
					cached.messages.length
				);
				runState = cached.run_state ?? { ...EMPTY_RUN_STATE };
				loading = false;
				void tick().then(() => {
					if (token !== loadToken) return;
					scrollToBottom(false);
					requestAnimationFrame(() => {
						if (token === loadToken) {
							if (stickToBottom) scrollToBottom(false);
							switchingGroup = false;
						}
					});
				});
			} else {
				group = fallbackGroup;
				messages = [];
				hasMore = false;
				totalMessages = 0;
				runState = { ...EMPTY_RUN_STATE };
				loading = true;
			}
			void loadGroup(id, token, Boolean(cached));
		});
	});

	let sseStatusReceived = false;
	let liveIdsDuringLoad = new Set<string>();
	/** First unread message ID and count captured when opening this group. */
	let unreadBannerId = $state('');
	let unreadBannerCount = $state(0);

	function sameMessages(a: GroupMessage[], b: GroupMessage[]): boolean {
		if (a.length !== b.length) return false;
		for (let i = 0; i < a.length; i += 1) {
			if (
				a[i].id !== b[i].id ||
				a[i].content !== b[i].content ||
				(a[i].extra_messages?.length ?? 0) !== (b[i].extra_messages?.length ?? 0) ||
				(a[i].attachments?.length ?? 0) !== (b[i].attachments?.length ?? 0)
			) {
				return false;
			}
		}
		return true;
	}

	async function loadGroup(id: string, token: number, hadCache: boolean) {
		try {
			const detail = await groupsStore.get(id, { limit: PAGE_SIZE, offset: -1 });
			if (token !== loadToken) return;
			group = detail.group;
			const incoming = detail.messages ?? [];
			const seen = new Set(incoming.map((m) => m.id));
			const liveAfter = messages.filter((m) => liveIdsDuringLoad.has(m.id) && !seen.has(m.id));
			const nextMessages = [...incoming, ...liveAfter];
			const messagesChanged = !hadCache || !sameMessages(messages, nextMessages);
			if (messagesChanged) {
				messages = nextMessages;
			}
			hasMore = Boolean(detail.has_more);
			totalMessages = Math.max(detail.total_messages ?? incoming.length, messages.length);
			if (!sseStatusReceived) {
				runState = detail.run_state ?? { ...EMPTY_RUN_STATE };
				approvalQueue.syncGroup(id, detail.run_state?.pending_approvals ?? []);
			}

			const initialUnread = detail.unread_count ?? 0;
			const firstUnreadId = detail.first_unread_message_id ?? '';
			if (initialUnread > 0 && firstUnreadId) {
				unreadBannerId = firstUnreadId;
				unreadBannerCount = initialUnread;
			}

			// Flip `loading` off BEFORE awaiting `tick()` so the real transcript
			// mounts (replacing the skeleton) and `scroller` has its real height.
			loading = false;

			if (initialUnread > 0 && firstUnreadId) {
				// Persist read watermark now that the user opened the room.
				groupsStore.markRead(id);
				stickToBottom = false;
				// If more than one page of unread messages arrived while away, load
				// earlier pages until the top-most unread message is in the transcript.
				for (
					let guard = 0;
					guard < 10 &&
					hasMore &&
					!messages.some((m) => m.id === firstUnreadId) &&
					token === loadToken;
					guard++
				) {
					const added = await loadOlder(PAGE_SIZE);
					if (added === 0) break;
				}
				if (token !== loadToken) return;
				await tick();
				if (token !== loadToken) return;
				const scrollToUnread = () => {
					if (token !== loadToken || !transcript) return;
					const divider = transcript.querySelector<HTMLElement>('[data-group-unread-divider]');
					const target =
						divider ??
						transcript.querySelector<HTMLElement>(
							`[data-group-message-id="${CSS.escape(firstUnreadId)}"]`
						);
					if (target) {
						target.scrollIntoView({ behavior: 'instant', block: 'start' });
					}
					stickToBottom = nearBottom();
					unreadBelowCount = stickToBottom ? 0 : initialUnread;
					switchingGroup = false;
				};
				scrollToUnread();
				requestAnimationFrame(scrollToUnread);
			} else if (messagesChanged || !hadCache) {
				stickToBottom = true;
				unreadBelowCount = 0;
				await tick();
				if (token !== loadToken) return;
				scrollToBottom(false);
				requestAnimationFrame(() => {
					if (token === loadToken) {
						if (stickToBottom) scrollToBottom(false);
						switchingGroup = false;
					}
				});
			} else {
				switchingGroup = false;
			}
		} catch (err) {
			if (token !== loadToken) return;
			switchingGroup = false;
			if (!hadCache) {
				loadError = getErrorMessage(err);
				topBarTitle.setChat(`group:${id}`, '');
				group = null;
			}
			loading = false;
		}
	}

	/**
	 * Fetches the latest page and appends any messages that landed while a
	 * reload or SSE reconnect was in flight.
	 */
	async function refreshLatestMessages(id: string) {
		const token = loadToken;
		try {
			const detail = await groupsStore.get(id, { limit: 20, offset: -1 });
			if (token !== loadToken || id !== groupId) return;
			const incoming = detail.messages ?? [];
			if (incoming.length === 0) return;
			const existing = new Set(messages.map((m) => m.id));
			const missing = incoming.filter((m) => !existing.has(m.id));
			if (missing.length === 0) return;
			messages = [...messages, ...missing];
			totalMessages = Math.max(detail.total_messages ?? messages.length, messages.length);
			maybeAutoScroll();
		} catch {
			// Ignore transient refresh failures.
		}
	}

	/**
	 * Fetches the page of older messages strictly before `messages[0].id` using
	 * cursor pagination (`before_id`), preserving the user's scroll position so
	 * prepending never jumps the viewport.
	 */
	async function loadOlder(pageSize = PAGE_SIZE): Promise<number> {
		const id = groupId;
		const oldestId = messages[0]?.id;
		if (!id || !oldestId || !hasMore || loadingOlder || loading) return 0;
		const token = loadToken;
		loadingOlder = true;
		prepending = true;
		const prevScrollHeight = scroller?.scrollHeight ?? 0;
		const prevScrollTop = scroller?.scrollTop ?? 0;
		try {
			const detail = await groupsStore.get(id, {
				limit: pageSize,
				before_id: oldestId
			});
			if (token !== loadToken || id !== groupId) return 0;
			const older = detail.messages ?? [];
			if (detail.total_messages !== undefined) {
				totalMessages = detail.total_messages;
			}
			hasMore = Boolean(detail.has_more) && older.length > 0;
			if (older.length === 0) return 0;
			const known = new Set(messages.map((m) => m.id));
			const uniqueOlder = older.filter((m) => !known.has(m.id));
			if (uniqueOlder.length === 0) return 0;
			messages = [...uniqueOlder, ...messages];
			await tick();
			if (token === loadToken && scroller) {
				scroller.scrollTop = prevScrollTop + (scroller.scrollHeight - prevScrollHeight);
			}
			return uniqueOlder.length;
		} catch (err) {
			if (token === loadToken) {
				toastStore.error(getErrorMessage(err));
			}
			return 0;
		} finally {
			if (token === loadToken) {
				loadingOlder = false;
				prepending = false;
			}
		}
	}

	// Track which group is currently on screen so unread badges and browser
	// notifications only fire when the reader is away or the tab is hidden.
	$effect(() => {
		const id = groupId;
		if (!browser || !id) return;
		groupsStore.setOpen(id);
		return () => groupsStore.setOpen('');
	});

	// Subscribe to live SSE events for this group.
	$effect(() => {
		const id = groupId;
		if (!browser || !id) return;
		return groupsStore.connectStream(id, (event) => handleSSEEvent(id, event));
	});

	function handleSSEEvent(id: string, event: SSEEvent) {
		if (id !== groupId) return;
		const data = event.data as Record<string, unknown> | null;
		if (!data || typeof data !== 'object') return;

		switch (event.event) {
			case 'status': {
				const payload = data as unknown as GroupStatusEvent;
				if (!payload.run_state) return;
				const wasRunning = runState.running;
				sseStatusReceived = true;
				runState = payload.run_state;
				approvalQueue.syncGroup(id, payload.run_state.pending_approvals ?? []);
				groupsStore.syncSummaryFromRunState(id, payload.run_state);
				if (!payload.run_state.running) {
					activeTurn = null;
					if (wasRunning) {
						void refreshLatestMessages(id);
					}
				}
				break;
			}
			case 'turn_start': {
				const payload = data as unknown as GroupTurnStartEvent;
				liveStepsOverride = null;
				activeTurn = {
					agent: payload.agent,
					agentColor: payload.agent_color || colorForAgent(payload.agent),
					turn: payload.turn,
					maxTurns: payload.max_turns,
					requestId: payload.request_id,
					text: '',
					timeline: []
				};
				runState = {
					...runState,
					running: true,
					paused: false,
					pause_reason: '',
					current_agent: payload.agent,
					turn_count: payload.turn,
					max_turns: payload.max_turns
				};
				groupsStore.syncSummaryFromRunState(id, runState);
				maybeAutoScroll();
				break;
			}
			case 'thinking': {
				const payload = data as unknown as GroupThinkingEvent;
				const current = ensureActiveTurn(payload.agent);
				const last = current.timeline[current.timeline.length - 1];
				if (last && last.kind === 'thought' && last.running) {
					current.timeline = [
						...current.timeline.slice(0, -1),
						{ ...last, text: (last.text ?? '') + (payload.text ?? '') }
					];
				} else {
					current.timeline = [
						...current.timeline,
						{
							localId: `live-${++stepCounter}`,
							kind: 'thought',
							text: payload.text ?? '',
							running: true,
							expanded: true
						}
					];
				}
				activeTurn = { ...current };
				maybeAutoScroll();
				break;
			}
			case 'chunk': {
				const payload = data as unknown as GroupChunkEvent;
				const current = ensureActiveTurn(payload.agent);
				current.timeline = current.timeline.map((step) =>
					step.kind === 'thought' && step.running
						? { ...step, running: false, expanded: false }
						: step
				);
				current.text += payload.text ?? '';
				activeTurn = { ...current };
				maybeAutoScroll();
				break;
			}
			case 'tool_start': {
				const payload = data as unknown as GroupToolStartEvent;
				const current = ensureActiveTurn(payload.agent);
				current.timeline = current.timeline.map((step) =>
					step.kind === 'thought' && step.running
						? { ...step, running: false, expanded: false }
						: step
				);
				current.timeline = [
					...current.timeline,
					{
						localId: `live-${++stepCounter}`,
						kind: 'tool',
						id: payload.id,
						name: payload.name,
						arguments: payload.arguments,
						running: true,
						expanded: false
					}
				];
				activeTurn = { ...current };
				maybeAutoScroll();
				break;
			}
			case 'tool_end': {
				const payload = data as unknown as GroupToolEndEvent;
				const current = ensureActiveTurn(payload.agent);
				const idx = current.timeline.findIndex(
					(step) => step.kind === 'tool' && step.id === payload.id
				);
				if (idx >= 0) {
					current.timeline = current.timeline.map((step, pos) =>
						pos === idx
							? {
									...step,
									name: payload.name || step.name,
									result: payload.result,
									is_error: payload.is_error,
									running: false
								}
							: step
					);
				} else {
					current.timeline = [
						...current.timeline,
						{
							localId: `live-${++stepCounter}`,
							kind: 'tool',
							id: payload.id,
							name: payload.name,
							result: payload.result,
							is_error: payload.is_error,
							running: false,
							expanded: false
						}
					];
				}
				activeTurn = { ...current };
				maybeAutoScroll();
				break;
			}
			case 'extra_messages': {
				const payload = data as unknown as GroupExtraMessagesEvent;
				if (!payload?.agent || !Array.isArray(payload.extra_messages)) break;
				const current = ensureActiveTurn(payload.agent);
				current.extraMessages = mergeExtraMessages(current.extraMessages, payload.extra_messages);
				activeTurn = { ...current };
				maybeAutoScroll();
				break;
			}
			case 'extra_block_updated': {
				const payload = data as unknown as GroupExtraBlockUpdatedEvent;
				if (!payload?.block_id || !payload?.state) break;
				patchGroupExtraBlock(payload.block_id, payload.state, payload.message_id);
				break;
			}
			case 'message': {
				const payload = data as unknown as GroupMessageEvent;
				const msg = payload.message;
				if (!msg?.id) return;
				liveIdsDuringLoad.add(msg.id);
				if (!messages.some((existing) => existing.id === msg.id)) {
					messages = [...messages, msg];
					totalMessages += 1;
					if (!stickToBottom && msg.sender_type !== 'user') {
						unreadBelowCount += 1;
					}
				}
				if (msg.attachments && msg.attachments.length > 0) {
					attachmentsRefreshKey += 1;
				}
				if (msg.sender_type === 'agent' && activeTurn?.agent === msg.sender_name) {
					activeTurn = null;
				}
				groupsStore.syncSummaryFromMessage(id, msg);
				if (msg.sender_type === 'agent' && document.visibilityState !== 'hidden') {
					groupsStore.markRead(id, msg.id);
				}
				maybeAutoScroll();
				break;
			}
			case 'turn_end': {
				const payload = data as unknown as GroupTurnEndEvent;
				if (activeTurn?.agent === payload.agent) {
					activeTurn = null;
				}
				project.bumpRefresh();
				break;
			}
			case 'approval_required': {
				const payload = data as unknown as GroupPendingApproval;
				if (!payload?.approval_id || !payload?.agent) return;
				approvalQueue.required(payload.agent, {
					approval_id: payload.approval_id,
					agent: payload.agent,
					agent_color: payload.agent_color,
					conversation_id: payload.conversation_id,
					group_id: payload.group_id || id,
					scope: `group:${payload.group_id || id}`,
					run_id: payload.run_id,
					tool_name: payload.tool_name,
					action: payload.action,
					args_summary: payload.args_summary,
					args: payload.args,
					subagent: payload.subagent
				});
				break;
			}
			case 'approval_resolved': {
				const payload = data as unknown as { approval_id?: string };
				if (payload?.approval_id) {
					approvalQueue.resolved({ approval_id: payload.approval_id });
				}
				break;
			}
			case 'run_end': {
				const payload = data as unknown as GroupRunEndEvent;
				activeTurn = null;
				if (payload.run_state) {
					runState = payload.run_state;
					approvalQueue.syncGroup(id, payload.run_state.pending_approvals ?? []);
					groupsStore.syncSummaryFromRunState(id, payload.run_state);
				} else {
					approvalQueue.syncGroup(id, []);
					runState = {
						...runState,
						running: false,
						paused: Boolean(payload.paused),
						pause_reason: payload.reason === 'max_turns' ? 'max_turns' : '',
						current_agent: '',
						turn_count: payload.turn_count ?? runState.turn_count,
						max_turns: payload.max_turns ?? runState.max_turns
					};
					groupsStore.syncSummaryFromRunState(id, runState);
				}
				if (payload.reason === 'no_online_members') {
					toastStore.warning(t('myaaw.groups.no_online_members'));
				}
				project.bumpRefresh();
				break;
			}
			case 'error': {
				const payload = data as unknown as GroupErrorEvent;
				if (payload.error) {
					toastStore.error(payload.agent ? `${payload.agent}: ${payload.error}` : payload.error);
				}
				break;
			}
		}
	}

	function ensureActiveTurn(agentName: string): ActiveTurn {
		if (activeTurn && activeTurn.agent === agentName) return activeTurn;
		return {
			agent: agentName,
			agentColor: colorForAgent(agentName),
			turn: runState.turn_count || 1,
			maxTurns: runState.max_turns || group?.max_turns || 8,
			requestId: '',
			text: '',
			timeline: []
		};
	}

	function patchGroupExtraBlock(blockId: string, state: InteractionState, messageId?: string) {
		if (!blockId) return;
		messages = messages.map((msg) => {
			if (messageId && msg.id !== messageId) return msg;
			if (!msg.extra_messages?.length) return msg;
			const next = patchExtraBlockState(msg.extra_messages, blockId, state);
			return next === msg.extra_messages ? msg : { ...msg, extra_messages: next };
		});
		if (groupId) groupsStore.syncCachedMessages(groupId, messages, totalMessages);
	}

	async function handleExtraInteract(req: InteractionRequest) {
		const id = groupId;
		if (!id) return;
		try {
			const res = await groupsStore.respondInteraction(id, req);
			patchGroupExtraBlock(res.block_id || req.block_id, res.state, req.message_id);
			if (res.message?.id && !messages.some((existing) => existing.id === res.message.id)) {
				messages = [...messages, res.message];
				totalMessages += 1;
				maybeAutoScroll();
			}
			if (res.run_state) {
				runState = res.run_state;
			}
		} catch (err) {
			const apiErr = err as { status?: number; state?: InteractionState } | undefined;
			if (apiErr?.status === 409) {
				if (apiErr.state) {
					patchGroupExtraBlock(req.block_id, apiErr.state, req.message_id);
				}
				toastStore.info(t('myaaw.extra.already_submitted'));
				return;
			}
			toastStore.error(getErrorMessage(err));
		}
	}

	function nearBottom(slack = 96): boolean {
		if (!scroller) return true;
		return scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= slack;
	}

	function scrollToBottom(smooth = true) {
		stickToBottom = true;
		unreadBelowCount = 0;
		if (!scroller) return;
		if (!smooth) {
			scroller.scrollTop = scroller.scrollHeight;
			return;
		}
		scroller.scrollTo({
			top: scroller.scrollHeight,
			behavior: 'smooth'
		});
	}

	// Images, code blocks, and Mermaid diagrams report their real height after
	// mount; keep following the bottom while the reader is already at the end
	// (same ResizeObserver behavior as regular 1-on-1 chat).
	$effect(() => {
		const node = transcript;
		if (!node || typeof ResizeObserver === 'undefined') return;
		const observer = new ResizeObserver(() => {
			if (!prepending && preferences.value.autoScroll && stickToBottom) scrollToBottom(false);
		});
		observer.observe(node);
		return () => observer.disconnect();
	});

	function maybeAutoScroll() {
		if (prepending || !preferences.value.autoScroll || !stickToBottom) return;
		void tick().then(() => {
			if (!prepending && scroller && stickToBottom) {
				scroller.scrollTop = scroller.scrollHeight;
			}
		});
	}

	function onScroll() {
		if (switchingGroup) return;
		stickToBottom = nearBottom();
		if (stickToBottom) unreadBelowCount = 0;
		askAnchor = null;
		// Near the top of the transcript, pull in the previous page seamlessly.
		if (
			scroller &&
			scroller.scrollTop < 120 &&
			hasMore &&
			!loadingOlder &&
			!loading &&
			!jumpingToMessage
		) {
			void loadOlder();
		}
	}

	function resizeComposer() {
		if (!composer) return;
		composer.style.height = 'auto';
		composer.style.height = `${Math.min(composer.scrollHeight, 200)}px`;
	}

	async function focusComposer() {
		await tick();
		composer?.focus();
	}

	// ---------------------------------------------------- @mention autocomplete

	function updateMentionMenu() {
		const el = composer;
		if (!el) {
			mentionOpen = false;
			return;
		}
		const cursor = el.selectionStart ?? input.length;
		const before = input.slice(0, cursor);
		const match = /(^|[\s(])@([A-Za-z0-9_-]*)$/.exec(before);
		if (!match) {
			mentionOpen = false;
			mentionStart = -1;
			return;
		}
		mentionStart = cursor - match[2].length - 1;
		mentionQuery = match[2];
		mentionOpen = true;
		if (mentionIndex >= mentionCandidates.length) mentionIndex = 0;
	}

	function insertMention(candidate: MentionCandidate) {
		const el = composer;
		if (!el || mentionStart < 0) return;
		const cursor = el.selectionStart ?? input.length;
		const prefix = input.slice(0, mentionStart);
		const suffix = input.slice(cursor);
		const inserted = `@${candidate.handle} `;
		input = prefix + inserted + suffix;
		mentionOpen = false;
		mentionStart = -1;
		mentionIndex = 0;
		const nextPos = prefix.length + inserted.length;
		void tick().then(() => {
			resizeComposer();
			el.focus();
			el.setSelectionRange(nextPos, nextPos);
		});
	}

	function handleComposerInput() {
		resizeComposer();
		updateMentionMenu();
	}

	function handleComposerKeydown(event: KeyboardEvent) {
		if (mentionOpen && mentionCandidates.length > 0) {
			if (event.key === 'ArrowDown') {
				event.preventDefault();
				mentionIndex = (mentionIndex + 1) % mentionCandidates.length;
				return;
			}
			if (event.key === 'ArrowUp') {
				event.preventDefault();
				mentionIndex = (mentionIndex - 1 + mentionCandidates.length) % mentionCandidates.length;
				return;
			}
			if (event.key === 'Enter' || event.key === 'Tab') {
				event.preventDefault();
				const picked = mentionCandidates[mentionIndex];
				if (picked) insertMention(picked);
				return;
			}
			if (event.key === 'Escape') {
				event.preventDefault();
				mentionOpen = false;
				return;
			}
		}

		if (event.key === 'Escape') {
			if (listening) {
				event.preventDefault();
				stopSpeech();
				return;
			}
			if (replyTo) {
				event.preventDefault();
				replyTo = null;
				return;
			}
			if (runState.running) {
				event.preventDefault();
				void handleStop();
				return;
			}
		}

		if (event.key === 'Enter' && !event.isComposing) {
			const modEnter = preferences.value.sendKey === 'mod-enter';
			const shouldSend = modEnter
				? event.metaKey || event.ctrlKey
				: !event.shiftKey && !event.altKey && !event.metaKey && !event.ctrlKey;
			if (shouldSend) {
				event.preventDefault();
				void handleSend();
			}
		}
	}

	// -------------------------------------------------- Attachments & Dictation

	function fileExtension(name: string): string {
		const dot = name.lastIndexOf('.');
		return dot >= 0 ? name.slice(dot).toLowerCase() : '';
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function readAsDataUrl(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(String(reader.result));
			reader.onerror = () => reject(reader.error ?? new Error('read failed'));
			reader.readAsDataURL(file);
		});
	}

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

	function base64Bytes(value: string): number {
		return Math.ceil((value.length * 3) / 4);
	}

	function uploadBytes(images: PendingImage[], files: PendingFile[]): number {
		const imageBytes = images.reduce((sum, item) => sum + base64Bytes(item.base64), 0);
		const fileBytes = files.reduce((sum, item) => sum + item.size, 0);
		return imageBytes + fileBytes;
	}

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
			if (!FILE_EXTENSIONS.has(fileExtension(file.name))) {
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

	function dictationLanguage(): string {
		const choice = preferences.value.dictationLang;
		const base = choice === 'app' ? locale.value : choice;
		return base === 'id' ? 'id-ID' : 'en-US';
	}

	function speechConstructor(): (new () => RecognitionLike) | null {
		if (!browser) return null;
		const scope = window as unknown as {
			SpeechRecognition?: new () => RecognitionLike;
			webkitSpeechRecognition?: new () => RecognitionLike;
		};
		return scope.SpeechRecognition ?? scope.webkitSpeechRecognition ?? null;
	}

	async function setSpeechText(value: string) {
		input = value;
		await tick();
		resizeComposer();
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

	$effect(() => () => {
		listening = false;
		try {
			speech?.abort();
		} catch {
			// Already stopped.
		}
		speech = null;
	});

	// Global shortcuts matching regular 1-on-1 chat:
	// - /: focus the message composer
	// - Mod+Shift+H: toggle right Group Info inspector panel
	// - Mod+Shift+F: attach file/image
	// - Mod+Shift+Space: toggle voice dictation
	$effect(() => {
		if (!browser) return;
		const onKey = (event: KeyboardEvent) => {
			if (event.defaultPrevented || isOverlayOpen()) return;
			if (matchesShortcut(event, '/') && !isTypingTarget(event.target)) {
				event.preventDefault();
				if (composer) {
					composer.focus();
					const end = composer.value.length;
					composer.setSelectionRange(end, end);
				}
				return;
			}
			if (matchesShortcut(event, 'mod+shift+h')) {
				event.preventDefault();
				toggleInspector();
				return;
			}
			if (matchesShortcut(event, 'mod+shift+space')) {
				event.preventDefault();
				toggleSpeech();
				return;
			}
			if (matchesShortcut(event, 'mod+shift+f')) {
				event.preventDefault();
				if (!(pending.length >= MAX_IMAGES && pendingFiles.length >= MAX_FILES)) {
					fileInput?.click();
				}
			}
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	// -------------------------------------------------- Send / Stop / Continue

	async function handleSend() {
		if (!canSend || !group) return;
		stopSpeech();
		const text = input.trim();
		const replyTarget = replyTo;
		const attachedQuotes = [...pendingQuotes];
		const attachedImages = [...pending];
		const attachedFiles = [...pendingFiles];

		const imagesPayload = attachedImages.map((item) => item.base64);
		const filesPayload: ChatFile[] = attachedFiles.map((item) => ({
			name: item.name,
			mime_type: item.mimeType || undefined,
			data_base64: item.base64
		}));

		sending = true;
		unreadBannerId = '';
		unreadBannerCount = 0;
		input = '';
		replyTo = null;
		pendingQuotes = [];
		pending = [];
		pendingFiles = [];
		if (fileInput) fileInput.value = '';
		mentionOpen = false;
		await tick();
		resizeComposer();

		try {
			const res = await groupsStore.sendMessage(group.id, {
				text,
				reply_to_id: replyTarget?.id || undefined,
				quotes: attachedQuotes.length > 0 ? attachedQuotes : undefined,
				images: imagesPayload.length > 0 ? imagesPayload : undefined,
				files: filesPayload.length > 0 ? filesPayload : undefined
			});
			if (res.message?.id && !messages.some((m) => m.id === res.message.id)) {
				messages = [...messages, res.message];
				totalMessages += 1;
			}
			if (res.run_state) {
				// SSE `turn_start`/`status` may have already advanced past the initial
				// turn_count=0 snapshot returned by POST /messages.
				const sseAhead =
					runState.running &&
					res.run_state.running &&
					(runState.turn_count > res.run_state.turn_count ||
						(Boolean(runState.current_agent) && !res.run_state.current_agent));
				if (!sseAhead) {
					runState = res.run_state;
				}
			}
			await tick();
			scrollToBottom(true);
		} catch (err) {
			// Restore draft on error so nothing is lost.
			input = text;
			replyTo = replyTarget;
			pendingQuotes = attachedQuotes;
			pending = attachedImages;
			pendingFiles = attachedFiles;
			toastStore.error(getErrorMessage(err));
			await tick();
			resizeComposer();
		} finally {
			sending = false;
		}
	}

	async function handleStop() {
		if (!group || stopping) return;
		stopping = true;
		try {
			const res = await groupsStore.stop(group.id);
			runState = res.run_state;
			activeTurn = null;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			stopping = false;
		}
	}

	async function handleContinue() {
		if (!group || continuing) return;
		continuing = true;
		try {
			const res = await groupsStore.continueRun(group.id, 5);
			runState = res.run_state;
		} catch (err) {
			toastStore.error(getErrorMessage(err));
		} finally {
			continuing = false;
		}
	}

	// -------------------------------------------------- WhatsApp-style Reply

	function startReply(msg: GroupMessage) {
		replyTo = msg;
		void focusComposer();
	}

	async function jumpToMessage(targetId: string) {
		if (!browser || !transcript || !targetId) return;
		const token = loadToken;
		let node = transcript.querySelector<HTMLElement>(
			`[data-group-message-id="${CSS.escape(targetId)}"]`
		);
		if (!node && !messages.some((m) => m.id === targetId) && hasMore) {
			jumpingToMessage = true;
			clearTimeout(jumpTimer);
			try {
				for (
					let guard = 0;
					guard < 20 && hasMore && !messages.some((m) => m.id === targetId) && token === loadToken;
					guard += 1
				) {
					const added = await loadOlder(100);
					if (added === 0) break;
				}
			} finally {
				jumpTimer = setTimeout(() => (jumpingToMessage = false), 1200);
			}
			if (token !== loadToken || !transcript) return;
			await tick();
			node = transcript.querySelector<HTMLElement>(
				`[data-group-message-id="${CSS.escape(targetId)}"]`
			);
		}
		if (!node) return;
		node.scrollIntoView({ behavior: 'smooth', block: 'center' });
		highlightedMessageId = targetId;
		clearTimeout(highlightTimer);
		highlightTimer = setTimeout(() => {
			if (highlightedMessageId === targetId) highlightedMessageId = '';
		}, 1800);
	}

	$effect(() => () => {
		clearTimeout(highlightTimer);
		clearTimeout(jumpTimer);
	});

	// -------------------------------------------------- Text-selection Quotes

	function askSourceHost(node: Node | null): HTMLElement | null {
		if (!node) return null;
		const el = node.nodeType === Node.ELEMENT_NODE ? (node as HTMLElement) : node.parentElement;
		return el?.closest<HTMLElement>('[data-ask-source]') ?? null;
	}

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
		if (!browser || !transcript) return;
		const selection = window.getSelection();
		if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
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
		const raw = range.toString();
		const startOffset = textOffsetIn(startHost, range.startContainer, range.startOffset);
		const endOffset = textOffsetIn(startHost, range.endContainer, range.endOffset);
		const leading = raw.length - raw.trimStart().length;
		const trailing = raw.length - raw.trimEnd().length;
		const exactRange = startOffset >= 0 && endOffset - trailing > startOffset + leading;

		const width = 104;
		const height = 34;
		const margin = 8;
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
		if (pendingQuotes.length >= MAX_ASK_QUOTES) {
			toastStore.error(t('myaaw.chat.ask_limit', { count: MAX_ASK_QUOTES }));
			return;
		}
		const trimmed = anchor.text.length > MAX_ASK_QUOTE_CHARS;
		const text = trimmed ? anchor.text.slice(0, MAX_ASK_QUOTE_CHARS).trimEnd() : anchor.text;
		askAnchor = null;
		window.getSelection()?.removeAllRanges();
		if (pendingQuotes.some((q) => q.text === text && q.source === anchor.source)) {
			return;
		}
		askCounter += 1;
		pendingQuotes = [
			...pendingQuotes,
			{
				id: `gquote-${askCounter}`,
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
		pendingQuotes = pendingQuotes.filter((q) => q.id !== id);
	}

	function clearAskQuotes() {
		pendingQuotes = [];
	}

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

	function clearQuoteFlash() {
		clearTimeout(quoteMarkTimer);
		quoteRects = [];
	}

	async function locateAskQuote(quote: AskQuote) {
		if (!browser || !transcript) return;
		clearQuoteFlash();
		const sourceId = quote.sourceId;
		let host = sourceId
			? transcript.querySelector<HTMLElement>(`[data-ask-id="${CSS.escape(sourceId)}"]`)
			: null;

		let range =
			host && quote.startOffset !== undefined && quote.endOffset !== undefined
				? rangeFromOffsets(host, quote.startOffset, quote.endOffset)
				: null;

		if (!range) {
			const hosts = Array.from(transcript.querySelectorAll<HTMLElement>('[data-ask-source]'));
			for (const candidate of hosts) {
				const idx = (candidate.textContent ?? '').indexOf(quote.text);
				if (idx < 0) continue;
				const found = rangeFromOffsets(candidate, idx, idx + quote.text.length);
				if (found) {
					host = candidate;
					range = found;
					break;
				}
			}
		}

		if (!host || !range) {
			if (sourceId) void jumpToMessage(sourceId);
			return;
		}

		const base = transcript.getBoundingClientRect();
		const rects = Array.from(range.getClientRects())
			.filter((r) => r.width > 0 && r.height > 0)
			.map((r) => ({
				left: r.left - base.left,
				top: r.top - base.top,
				width: r.width,
				height: r.height
			}));
		if (rects.length > 0) {
			quoteRects = rects;
			quoteMarkTimer = setTimeout(clearQuoteFlash, 3000);
		}
		const targetEl = range.startContainer.parentElement ?? host;
		targetEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
	}

	$effect(() => () => clearQuoteFlash());

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

	// -------------------------------------------------- Step helpers

	function stepsOf<T extends TimelineItem>(timeline?: T[]): T[] {
		if (!timeline?.length) return [];
		const hideThinking = Boolean(group?.hide_thinking);
		const hideTools = Boolean(group?.hide_tools);
		return timeline.filter((item) => {
			if (item.kind === 'thought') return !hideThinking;
			if (item.kind === 'tool') return !hideTools;
			return false;
		});
	}

	function stepSummaryLabel(steps: TimelineItem[]): string {
		const thoughts = steps.filter((s) => s.kind === 'thought').length;
		const tools = steps.filter((s) => s.kind === 'tool').length;
		const parts: string[] = [];
		if (thoughts > 0) parts.push(t('myaaw.groups.steps_thoughts', { count: thoughts }));
		if (tools > 0) parts.push(t('myaaw.groups.steps_tools', { count: tools }));
		return parts.join(' · ') || t('myaaw.groups.steps_count', { count: steps.length });
	}

	function replySenderLabel(preview: ReplyPreview): string {
		return preview.sender_type === 'user'
			? t('myaaw.groups.you')
			: preview.sender_name || t('myaaw.groups.system');
	}

	function oneLine(text: string): string {
		return text.replace(/\s+/g, ' ').trim();
	}

	function toolInvocation(step: TimelineItem): string {
		if (step.arguments) {
			try {
				const parsed = JSON.parse(step.arguments) as Record<string, unknown>;
				if (typeof parsed.command === 'string') return parsed.command;
				if (typeof parsed.code === 'string') return parsed.code;
				if (step.name === 'filesystem' && typeof parsed.tool_name === 'string') {
					const target = ['path', 'old_path', 'new_path', 'pattern']
						.map((key) => parsed[key])
						.find((value): value is string => typeof value === 'string' && value.length > 0);
					return target ? `${parsed.tool_name} ${target}` : parsed.tool_name;
				}
				return JSON.stringify(parsed, null, 2);
			} catch {
				return step.arguments;
			}
		}
		return step.name ?? '';
	}

	function formatToolSummary(step: TimelineItem): string {
		const raw = toolInvocation(step);
		if (!raw || raw === step.name) return '';
		const clean = oneLine(raw);
		return clean.length > 72 ? `${clean.slice(0, 72)}…` : clean;
	}

	function isLiveStepsOpen(turn: ActiveTurn): boolean {
		if (liveStepsOverride !== null) return liveStepsOverride;
		return turn.timeline.some((step) => step.running) || !turn.text;
	}

	function toggleLiveStep(localId: string) {
		if (!activeTurn) return;
		activeTurn = {
			...activeTurn,
			timeline: activeTurn.timeline.map((step) =>
				step.localId === localId ? { ...step, expanded: !step.expanded } : step
			)
		};
	}

	function insertStarterMention(name: string) {
		const token = `@${name} `;
		if (!input.startsWith(token)) {
			input = input ? `${token}${input}` : token;
		}
		void focusComposer();
	}
</script>

{#snippet mentionText(text: string)}
	{#each splitMentionSegments(text, mentionTargets) as segment, idx (idx)}
		{#if segment.mention}
			<span
				data-mention-pill={segment.mention.isAll ? 'all' : segment.mention.handle}
				class={mentionPillClass(segment.mention.color, segment.mention.isAll)}>{segment.text}</span
			>
		{:else}{segment.text}{/if}
	{/each}
{/snippet}

{#snippet replyCard(preview: ReplyPreview)}
	{@const isAgent = preview.sender_type === 'agent'}
	{@const accent = isAgent ? groupAccent(colorForAgent(preview.sender_name)) : null}
	<button
		type="button"
		onclick={() => void jumpToMessage(preview.id)}
		class={[
			'mb-2 flex w-full flex-col rounded-lg border border-l-[3px] bg-background/70 px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-background',
			accent ? accent.border : 'border-l-primary'
		]}
	>
		<span class={['truncate text-[11px] font-semibold', accent ? accent.text : 'text-primary']}>
			{replySenderLabel(preview)}
		</span>
		<span class="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground">
			{@render mentionText(preview.excerpt)}
		</span>
	</button>
{/snippet}

<TopBarActions id="group-chat">
	{#if group}
		<Button
			variant={inspectorOpen ? 'secondary' : 'ghost'}
			size="icon-sm"
			class={[
				'rounded-full lg:hidden',
				!inspectorOpen && 'text-muted-foreground hover:bg-muted/80 hover:text-foreground'
			]}
			onclick={toggleInspector}
			title={`${inspectorOpen ? t('myaaw.groups.hide_inspector') : t('myaaw.groups.show_inspector')} (${formatShortcut('mod+shift+h')})`}
			aria-label={inspectorOpen
				? t('myaaw.groups.hide_inspector')
				: t('myaaw.groups.show_inspector')}
			aria-pressed={inspectorOpen}
			aria-keyshortcuts="Control+Shift+H Meta+Shift+H"
		>
			<PanelRight class="h-4 w-4" />
		</Button>
	{/if}
</TopBarActions>

<div class="relative flex h-full min-h-0 w-full overflow-hidden">
	<!-- Main chat & workspace column -->
	<div
		class={[
			'relative flex min-h-0 min-w-0 flex-1 flex-col',
			!previewAttachment && !project.hasTabBar && 'lg:pt-14'
		]}
	>
		{#if previewAttachment}
			{@const activeAtt = previewAttachment}
			<AttachmentPreview
				agent={activeAtt.agent ?? ''}
				item={{
					path: activeAtt.path ?? '',
					file_name: activeAtt.file_name,
					mime_type: activeAtt.mime_type,
					kind: activeAtt.kind as 'image' | 'video' | 'audio' | 'pdf' | 'file' | 'project',
					project_type: activeAtt.project_type,
					size: activeAtt.size,
					origin: activeAtt.sender_type === 'agent' ? 'agent' : 'user',
					message_id: activeAtt.message_id,
					message_index: 0,
					created_at: activeAtt.created_at
				}}
				srcOverride={activeAtt.agent && activeAtt.path ? undefined : groupAttachmentUrl(activeAtt)}
				downloadSrcOverride={activeAtt.agent && activeAtt.path
					? undefined
					: groupAttachmentUrl(activeAtt, { download: true })}
				onDownload={() => downloadGroupAttachment(activeAtt)}
				originLabel={activeAtt.sender_type === 'agent' && activeAtt.sender_name
					? `@${activeAtt.sender_name}`
					: undefined}
				onClose={() => (previewAttachment = null)}
				onShowInChat={() => showAttachmentInChat(activeAtt)}
			/>
		{:else if !loading && (loadError || !group)}
			<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
				<AlertTriangle class="h-8 w-8 text-muted-foreground" />
				<p class="text-sm font-medium">{loadError || t('myaaw.groups.not_found')}</p>
				<Button variant="outline" size="sm" onclick={() => void goto('/app/chat')}>
					{t('myaaw.groups.back_to_chat')}
				</Button>
			</div>
		{:else}
			{#if project.hasTabBar}
				<ProjectTabBar
					tabs={project.tabs}
					terminalOpen={project.terminalOpen}
					activeTabId={project.activeTabId}
					onSelectChat={() => project.selectChat()}
					onSelectTerminal={() => project.selectTerminal()}
					onCloseTerminal={closeProjectTerminal}
					onSelectTab={(id) => project.selectTab(id)}
					onCloseTab={(id) => project.closeTab(id)}
				/>
			{/if}

			{#if project.showingFileTab && workspaceAgent}
				<ProjectWorkspacePane
					agent={workspaceAgent}
					conversationId={projectContextId}
					activeTab={project.activeTab}
					refreshKey={project.refreshKey}
					onCloseTab={(id) => project.closeTab(id)}
					onOpenFile={openProjectFile}
					onSaved={() => project.bumpRefresh()}
				/>
			{/if}

			{#if project.workingDir && project.terminalOpen && workspaceAgent}
				<div class={project.showingTerminal ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
					<ProjectTerminalGroup
						bind:this={
							() => terminalGroupHolder.current,
							(instance) => {
								terminalGroupHolder.current = instance;
							}
						}
						agent={workspaceAgent}
						cwd={project.workingDir}
						conversationId={projectContextId}
						scopeKey={projectScopeKeyStr}
						onCloseAll={() => project.closeTerminal()}
					/>
				</div>
			{/if}

			<div class={project.showingChat ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
				<!-- Scrollable transcript (exact max-w-4xl & px-4 py-6 matching regular 1-on-1 chat) -->
				<div
					bind:this={scroller}
					onscroll={onScroll}
					class="relative min-h-0 flex-1 overflow-x-hidden overflow-y-auto"
				>
					<div
						bind:this={transcript}
						data-total-messages={totalMessages}
						data-has-more={hasMore}
						class={cn(
							'relative mx-auto flex w-full max-w-4xl flex-col gap-5 px-4 py-6',
							!loading && messages.length === 0 && !activeTurn && 'min-h-full justify-center'
						)}
					>
						{#if loading}
							<GroupMessagesSkeleton label={t('myaaw.chat.loading')} />
						{:else}
							<!-- Temporary yellow highlight overlay for located quotes -->
							{#each quoteRects as rect, index (index)}
								<span
									aria-hidden="true"
									class="pointer-events-none absolute z-10 rounded-xs bg-amber-300/45 dark:bg-amber-400/30"
									style:left={`${rect.left}px`}
									style:top={`${rect.top}px`}
									style:width={`${rect.width}px`}
									style:height={`${rect.height}px`}
								></span>
							{/each}

							{#if loadingOlder}
								<p
									class="flex items-center justify-center gap-2 text-xs text-muted-foreground"
									aria-live="polite"
								>
									<Loader2 class="h-3.5 w-3.5 animate-spin" />
									{t('myaaw.chat.loading_older')}
								</p>
							{/if}

							{#if messages.length === 0 && !activeTurn && group}
								<!-- Empty state -->
								<div class="my-auto flex flex-col items-center justify-center py-12 text-center">
									<div class="flex items-center -space-x-2.5">
										{#each group.members.slice(0, 5) as member (member.name)}
											<span
												class="grid h-12 w-12 place-items-center rounded-full bg-muted/80 ring-2 ring-background"
											>
												<Bloub
													mood="attentif"
													size={36}
													tight
													color={colorOf(colorForAgent(member.name, member.agent_color))}
												/>
											</span>
										{/each}
									</div>
									<h2 class="mt-4 text-base font-semibold">{group.name}</h2>
									{#if group.description}
										<p class="mt-1 max-w-md text-xs text-muted-foreground">{group.description}</p>
									{:else}
										<p class="mt-1 max-w-md text-xs text-muted-foreground">
											{t('myaaw.groups.empty_hint')}
										</p>
									{/if}

									<div class="mt-4 flex flex-wrap items-center justify-center gap-1.5">
										<button
											type="button"
											onclick={() => insertStarterMention('all')}
											class="inline-flex items-center gap-1 rounded-full border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
										>
											<AtSign class="h-3 w-3" />
											all
										</button>
										{#each group.members as member (member.name)}
											<button
												type="button"
												onclick={() => insertStarterMention(member.name)}
												class="inline-flex items-center gap-1.5 rounded-full border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
											>
												<Bloub
													mood="attentif"
													size={16}
													tight
													color={colorOf(colorForAgent(member.name, member.agent_color))}
												/>
												@{member.name}
											</button>
										{/each}
									</div>
								</div>
							{/if}

							{#each messages as msg (msg.id)}
								{@const highlighted = highlightedMessageId === msg.id}
								{#if unreadBannerId === msg.id && unreadBannerCount > 0}
									<div
										data-group-unread-divider
										class="my-2 flex scroll-mt-4 items-center gap-3 select-none"
									>
										<div class="h-px flex-1 bg-primary/30"></div>
										<span
											class="rounded-full bg-primary/10 px-2.5 py-0.5 text-[11px] font-medium text-primary"
										>
											{t('myaaw.groups.unread_divider', { count: unreadBannerCount })}
										</span>
										<div class="h-px flex-1 bg-primary/30"></div>
									</div>
								{/if}
								{#if msg.sender_type === 'system'}
									<div
										data-group-message-id={msg.id}
										class="mx-auto my-1 max-w-md rounded-full bg-muted/70 px-3 py-1 text-center text-[11px] text-muted-foreground"
									>
										{msg.content}
									</div>
								{:else if msg.sender_type === 'user'}
									{@const parsedInteraction = parseInteractionText(msg.content)}
									{@const userDisplayText = parsedInteraction.kind
										? parsedInteraction.text
										: msg.content}
									{@const imageAttachments = (msg.attachments ?? []).filter(
										(att) => att.kind === 'image'
									)}
									{@const fileAttachments = (msg.attachments ?? []).filter(
										(att) => att.kind !== 'image'
									)}
									<div
										data-group-message-id={msg.id}
										class={[
											'group ml-auto flex max-w-[85%] flex-col items-end gap-1.5 rounded-2xl transition-shadow duration-300',
											highlighted
												? 'ring-2 ring-primary/60 ring-offset-2 ring-offset-background'
												: ''
										]}
									>
										{#if parsedInteraction.kind}
											<InteractionBadge kind={parsedInteraction.kind} />
										{/if}
										{#if imageAttachments.length > 0}
											<div class="flex flex-wrap justify-end gap-2">
												{#each imageAttachments as att, imgIdx (att.id)}
													{@const imgUrl = groupAttachmentUrl(att)}
													<div class="relative inline-block max-w-full">
														<button
															type="button"
															class="block cursor-zoom-in rounded-2xl focus-visible:outline-2 focus-visible:outline-offset-2"
															aria-label={t('myaaw.chat.lightbox_open')}
															onclick={() => openImageAttachmentsLightbox(imageAttachments, imgIdx)}
														>
															<img
																src={imgUrl}
																alt={att.file_name}
																draggable="false"
																class="max-h-48 rounded-2xl border object-contain"
															/>
														</button>
														<button
															type="button"
															class="absolute right-1.5 bottom-1.5 grid size-7 place-items-center rounded-full bg-black/60 text-white shadow-sm transition-colors hover:bg-black/80"
															title={t('myaaw.chat.download_file')}
															aria-label={t('myaaw.chat.download_file')}
															onclick={() => downloadGroupAttachment(att)}
														>
															<Download class="h-3.5 w-3.5" />
														</button>
													</div>
												{/each}
											</div>
										{/if}
										{#if fileAttachments.length > 0}
											<div class="flex flex-wrap justify-end gap-2">
												{#each fileAttachments as att (att.id)}
													<button
														type="button"
														onclick={() => openAttachmentPreview(att)}
														title={att.file_name}
														class="group/file flex max-w-[16rem] items-center gap-2 rounded-lg border bg-muted/40 px-2.5 py-1.5 text-left transition-colors hover:border-foreground/30 hover:bg-muted"
													>
														<FileText class="h-4 w-4 shrink-0 text-muted-foreground" />
														<span class="min-w-0 flex-1 truncate text-xs">{att.file_name}</span>
														{#if att.size}
															<span class="shrink-0 text-[10px] text-muted-foreground">
																{formatBytes(att.size)}
															</span>
														{/if}
														<Download
															class="h-3.5 w-3.5 shrink-0 text-muted-foreground group-hover/file:text-foreground"
														/>
													</button>
												{/each}
											</div>
										{/if}
										{#if userDisplayText.trim().length > 0 || msg.reply_preview || (msg.quotes && msg.quotes.length > 0)}
											<div
												class="w-fit max-w-full rounded-2xl rounded-tr-sm bg-primary/10 px-3.5 py-2.5 text-sm text-foreground ring-1 ring-primary/15"
											>
												{#if msg.reply_preview}
													{@render replyCard(msg.reply_preview)}
												{/if}
												{#if msg.quotes && msg.quotes.length > 0}
													<div class="mb-2">
														<AskQuoteList
															quotes={msg.quotes}
															onlocate={(quote) => void locateAskQuote(quote)}
														/>
													</div>
												{/if}
												{#if userDisplayText.trim().length > 0}
													<p class="whitespace-pre-wrap wrap-break-word">
														{@render mentionText(userDisplayText)}
													</p>
												{/if}
											</div>
										{/if}

										<div
											class="mt-1 flex items-center gap-1.5 px-1 text-[11px] text-muted-foreground"
										>
											<span
												class="inline-flex items-center gap-1 opacity-100 transition-opacity sm:pointer-events-none sm:opacity-0 sm:group-focus-within:pointer-events-auto sm:group-focus-within:opacity-100 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100"
											>
												<button
													type="button"
													onclick={() => startReply(msg)}
													title={t('myaaw.groups.reply')}
													aria-label={t('myaaw.groups.reply')}
													class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
												>
													<Reply class="h-3 w-3" />
													<span class="hidden sm:inline">{t('myaaw.groups.reply')}</span>
												</button>
												<CopyButton text={userDisplayText} />
											</span>
											<ChatTime at={msg.created_at} />
										</div>
									</div>
								{:else}
									{@const agentColor = colorForAgent(msg.sender_name, msg.agent_color)}
									{@const accent = groupAccent(agentColor)}
									{@const steps = stepsOf(msg.timeline)}
									{@const groupStepsOpen = Boolean(expandedStepGroups[msg.id])}
									{@const displayAgentText = stripAttachDirectives(msg.content)}
									{@const agentRunAttachments = toRunAttachments(msg.attachments)}
									<div
										data-group-message-id={msg.id}
										class={[
											'group flex items-start gap-2.5 rounded-2xl p-1 -m-1 transition-all duration-300',
											highlighted
												? 'bg-primary/5 ring-2 ring-primary/60 ring-offset-2 ring-offset-background'
												: ''
										]}
									>
										<div
											class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-xl bg-muted/70 ring-1 ring-border/50"
										>
											<Bloub mood="attentif" size={26} tight color={colorOf(agentColor)} />
										</div>

										<div class="min-w-0 flex-1">
											<div class="mb-1 flex items-center gap-2">
												<span class={['text-xs font-semibold', accent.text]}>
													{msg.sender_name}
												</span>
												<ChatTime at={msg.created_at} />
											</div>

											{#if steps.length > 0}
												<div class="mb-1.5 space-y-0.5">
													<button
														type="button"
														onclick={() =>
															(expandedStepGroups = {
																...expandedStepGroups,
																[msg.id]: !groupStepsOpen
															})}
														aria-expanded={groupStepsOpen}
														class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
													>
														<Clock class="h-3.5 w-3.5 shrink-0" />
														<span class="font-medium">{stepSummaryLabel(steps)}</span>
														<ChevronDown
															class={cn(
																'h-3.5 w-3.5 shrink-0 transition-transform',
																groupStepsOpen && 'rotate-180'
															)}
														/>
													</button>

													{#if groupStepsOpen}
														<div class="space-y-0.5 pl-2">
															{#each steps as step, sIndex (`${msg.id}-step-${sIndex}`)}
																{@const stepKey = `${msg.id}-step-${sIndex}`}
																{@const stepExpanded = Boolean(expandedSteps[stepKey])}
																{#if step.kind === 'thought'}
																	<div data-ask-row={stepKey}>
																		<ChatStep
																			title={t('myaaw.chat.thinking')}
																			summary={oneLine(step.text ?? '')}
																			icon={Brain}
																			expanded={stepExpanded}
																			ontoggle={() =>
																				(expandedSteps = {
																					...expandedSteps,
																					[stepKey]: !stepExpanded
																				})}
																			copyText={step.text ?? ''}
																		>
																			<p
																				data-ask-source="thinking"
																				data-ask-id={stepKey}
																				class="whitespace-pre-wrap text-muted-foreground"
																			>
																				{step.text}
																			</p>
																		</ChatStep>
																	</div>
																{:else if step.kind === 'tool'}
																	<div data-ask-row={stepKey}>
																		<ChatStep
																			title={step.name || t('myaaw.chat.tool')}
																			summary={formatToolSummary(step)}
																			icon={Wrench}
																			error={Boolean(step.is_error)}
																			expanded={stepExpanded}
																			ontoggle={() =>
																				(expandedSteps = {
																					...expandedSteps,
																					[stepKey]: !stepExpanded
																				})}
																			copyText={toolInvocation(step)}
																		>
																			<div
																				data-ask-source="tool"
																				data-ask-label={step.name || ''}
																				data-ask-id={stepKey}
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
																				{/if}
																			</div>
																		</ChatStep>
																	</div>
																{/if}
															{/each}
														</div>
													{/if}
												</div>
											{/if}

											<div
												class="w-fit max-w-full rounded-2xl rounded-tl-sm border bg-card px-3.5 py-2.5 text-sm shadow-2xs"
											>
												{#if msg.reply_preview}
													{@render replyCard(msg.reply_preview)}
												{/if}
												<div
													data-ask-source="answer"
													data-ask-id={msg.id}
													use:highlightMentions={{
														mentions: mentionTargets,
														key: displayAgentText
													}}
												>
													<Markdown content={displayAgentText} />
												</div>
											</div>

											{#if agentRunAttachments.length > 0}
												<div class="mt-2.5">
													<RunAttachments
														agent={msg.sender_name}
														attachments={agentRunAttachments}
													/>
												</div>
											{/if}

											{#if (msg.extra_messages?.length ?? 0) > 0}
												<ExtraMessages
													agent={msg.sender_name}
													extraMessages={msg.extra_messages ?? []}
													messageId={msg.id}
													onInteract={handleExtraInteract}
												/>
											{/if}

											<div
												class="mt-1 flex items-center gap-1.5 px-1 opacity-100 transition-opacity sm:pointer-events-none sm:opacity-0 sm:group-focus-within:pointer-events-auto sm:group-focus-within:opacity-100 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100"
											>
												<button
													type="button"
													onclick={() => startReply(msg)}
													title={t('myaaw.groups.reply')}
													aria-label={t('myaaw.groups.reply')}
													class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
												>
													<Reply class="h-3 w-3" />
													<span>{t('myaaw.groups.reply')}</span>
												</button>
												<CopyButton text={msg.content} label />
											</div>
										</div>
									</div>
								{/if}
							{/each}

							<!-- Live streaming turn bubble -->
							{#if activeTurn}
								{@const liveAccent = groupAccent(activeTurn.agentColor)}
								{@const visibleLiveTimeline = stepsOf(activeTurn.timeline)}
								<div class="flex items-start gap-2.5" in:fade={{ duration: 120 }}>
									<div
										class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-xl bg-muted/70 ring-1 ring-primary/40"
									>
										<Bloub mood="attentif" size={26} tight color={colorOf(activeTurn.agentColor)} />
									</div>

									<div class="min-w-0 flex-1">
										<div class="mb-1 flex items-center gap-2">
											<span class={['text-xs font-semibold', liveAccent.text]}>
												{activeTurn.agent}
											</span>
											<span
												class="inline-flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary"
											>
												<Loader2 class="h-2.5 w-2.5 animate-spin" />
												{t('myaaw.groups.turn_badge', {
													turn: activeTurn.turn,
													max: activeTurn.maxTurns
												})}
											</span>
										</div>

										{#if visibleLiveTimeline.length > 0}
											{@const liveStepsRunning = visibleLiveTimeline.some((s) => s.running)}
											{@const liveOpen = isLiveStepsOpen(activeTurn)}
											<div class="mb-1.5 space-y-0.5">
												<button
													type="button"
													onclick={() => (liveStepsOverride = !liveOpen)}
													aria-expanded={liveOpen}
													class="inline-flex items-center gap-2 rounded-md px-1 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/60"
												>
													{#if liveStepsRunning}
														<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
													{:else}
														<Clock class="h-3.5 w-3.5 shrink-0" />
													{/if}
													<span class="font-medium">
														{liveStepsRunning
															? t('myaaw.chat.tool_running')
															: stepSummaryLabel(visibleLiveTimeline)}
													</span>
													<ChevronDown
														class={cn(
															'h-3.5 w-3.5 shrink-0 transition-transform',
															liveOpen && 'rotate-180'
														)}
													/>
												</button>

												{#if liveOpen}
													<div class="space-y-0.5 pl-2">
														{#each visibleLiveTimeline as step (step.localId)}
															{#if step.kind === 'thought'}
																<ChatStep
																	title={t('myaaw.chat.thinking')}
																	summary={oneLine(step.text ?? '')}
																	icon={Brain}
																	running={Boolean(step.running)}
																	expanded={Boolean(step.expanded)}
																	ontoggle={() => toggleLiveStep(step.localId)}
																>
																	<p class="whitespace-pre-wrap text-muted-foreground">
																		{step.text}
																	</p>
																</ChatStep>
															{:else if step.kind === 'tool'}
																<ChatStep
																	title={step.name || t('myaaw.chat.tool')}
																	summary={formatToolSummary(step)}
																	icon={Wrench}
																	running={Boolean(step.running)}
																	error={Boolean(step.is_error)}
																	expanded={Boolean(step.expanded)}
																	ontoggle={() => toggleLiveStep(step.localId)}
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
															{/if}
														{/each}
													</div>
												{/if}
											</div>
										{/if}

										<div
											use:highlightMentions={{ mentions: mentionTargets, key: activeTurn.text }}
											class="w-fit max-w-full rounded-2xl rounded-tl-sm border border-primary/25 bg-card px-3.5 py-2.5 text-sm"
										>
											{#if activeTurn.text}
												<Markdown content={activeTurn.text} streaming />
											{:else}
												<div class="flex items-center gap-1.5 py-1 text-xs text-muted-foreground">
													<span
														class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:-0.2s]"
													></span>
													<span
														class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:-0.1s]"
													></span>
													<span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary"></span>
													<span class="ml-1"
														>{t('myaaw.groups.agent_thinking', { name: activeTurn.agent })}</span
													>
												</div>
											{/if}
										</div>
										{#if (activeTurn.extraMessages?.length ?? 0) > 0}
											<ExtraMessages
												agent={activeTurn.agent}
												extraMessages={activeTurn.extraMessages ?? []}
												disabled
											/>
										{/if}
									</div>
								</div>
							{/if}
						{/if}
					</div>
				</div>

				<!-- Composer footer (no border-t divider, exact max-w-4xl & padding matching regular 1-on-1 chat) -->
				<div class="shrink-0 px-2 py-2 sm:px-4 sm:py-3">
					<div class="relative mx-auto w-full max-w-4xl space-y-2">
						{#if !stickToBottom && messages.length > 0}
							<button
								type="button"
								transition:fade={{ duration: 150 }}
								onclick={() => scrollToBottom(true)}
								title={t('myaaw.groups.scroll_bottom')}
								aria-label={t('myaaw.groups.scroll_bottom')}
								class="absolute right-1 -top-12 z-20 inline-flex h-9 items-center justify-center gap-1.5 rounded-full border border-border/80 bg-card/95 px-2.5 text-muted-foreground shadow-md backdrop-blur-sm transition-colors hover:bg-muted hover:text-foreground"
							>
								{#if unreadBelowCount > 0}
									<span
										class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1.5 text-[11px] leading-none font-semibold text-primary-foreground tabular-nums"
									>
										{unreadBelowCount > 99 ? '99+' : unreadBelowCount}
									</span>
								{/if}
								<ArrowDown class="h-4 w-4" />
							</button>
						{/if}
						<!-- Live run status / pause banner -->
						{#if runState.running}
							<div
								class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-primary/25 bg-primary/8 px-3 py-2 text-xs"
							>
								<div class="flex min-w-0 items-center gap-2">
									<span class="h-2 w-2 shrink-0 animate-pulse rounded-full bg-primary"></span>
									<span class="truncate font-medium text-foreground">
										{#if runState.current_agent}
											{t('myaaw.groups.status_thinking', {
												name: runState.current_agent,
												turn: runState.turn_count,
												max: runState.max_turns
											})}
										{:else}
											{t('myaaw.groups.status_running', {
												turn: runState.turn_count,
												max: runState.max_turns
											})}
										{/if}
									</span>
									{#if runState.queue && runState.queue.length > 0}
										<span class="hidden truncate text-[11px] text-muted-foreground sm:inline">
											· {t('myaaw.groups.next_queue', {
												agents: runState.queue.map((a) => `@${a}`).join(', ')
											})}
										</span>
									{/if}
								</div>
								<Button
									size="sm"
									variant="outline"
									class="h-7 gap-1.5 border-destructive/40 px-2.5 text-xs text-destructive hover:bg-destructive/10 hover:text-destructive"
									disabled={stopping}
									onclick={() => void handleStop()}
								>
									{#if stopping}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Square class="h-3 w-3" />
									{/if}
									{t('myaaw.groups.stop')}
								</Button>
							</div>
						{:else if runState.paused && runState.pause_reason === 'max_turns'}
							<div
								class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-500/35 bg-amber-500/10 px-3 py-2 text-xs"
							>
								<div class="min-w-0">
									<p class="font-medium text-amber-800 dark:text-amber-300">
										{t('myaaw.groups.paused_max_turns', {
											turn: runState.turn_count,
											max: runState.max_turns
										})}
									</p>
									{#if runState.queue && runState.queue.length > 0}
										<p class="truncate text-[11px] text-muted-foreground">
											{t('myaaw.groups.next_queue', {
												agents: runState.queue.map((a) => `@${a}`).join(', ')
											})}
										</p>
									{/if}
								</div>
								<Button
									size="sm"
									class="h-7 gap-1.5 px-2.5 text-xs"
									disabled={continuing}
									onclick={() => void handleContinue()}
								>
									{#if continuing}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Play class="h-3 w-3" />
									{/if}
									{t('myaaw.groups.continue_turns', { count: 5 })}
								</Button>
							</div>
						{/if}

						<!-- WhatsApp-style Reply bar -->
						{#if replyTo}
							<GroupReplyBar
								message={replyTo}
								agentColor={replyTo.sender_type === 'agent'
									? colorForAgent(replyTo.sender_name, replyTo.agent_color)
									: undefined}
								onclear={() => (replyTo = null)}
							/>
						{/if}

						<!-- @mention autocomplete popup -->
						{#if mentionOpen && mentionCandidates.length > 0}
							<div
								role="listbox"
								aria-label={t('myaaw.groups.mention_list')}
								class="absolute bottom-full left-0 z-30 mb-2 w-64 overflow-hidden rounded-xl border bg-popover p-1 shadow-lg"
							>
								{#each mentionCandidates as candidate, idx (candidate.handle)}
									<button
										type="button"
										role="option"
										aria-selected={idx === mentionIndex}
										onmousedown={(e) => {
											e.preventDefault();
											insertMention(candidate);
										}}
										onmouseenter={() => (mentionIndex = idx)}
										class={[
											'flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors',
											idx === mentionIndex
												? 'bg-accent text-accent-foreground'
												: 'hover:bg-muted/70'
										]}
									>
										{#if candidate.isAll}
											<span
												class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-primary/15 text-primary"
											>
												<Users class="h-3.5 w-3.5" />
											</span>
										{:else}
											<span class="relative shrink-0">
												<Bloub
													mood="attentif"
													size={22}
													tight
													color={colorOf(candidate.agentColor)}
												/>
												<span
													aria-hidden="true"
													class={[
														'absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 ring-popover',
														candidate.running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
													]}
												></span>
											</span>
										{/if}
										<span class="min-w-0 flex-1">
											<span class="block truncate font-semibold">@{candidate.handle}</span>
											<span class="block truncate text-[10px] text-muted-foreground">
												{candidate.label}
											</span>
										</span>
									</button>
								{/each}
							</div>
						{/if}

						{#if activeGroupApproval}
							<div class="mb-2">
								<PendingApprovalCard
									approval={{
										approval_id: activeGroupApproval.approvalId,
										tool_name: activeGroupApproval.toolName,
										action: activeGroupApproval.action,
										args_summary: activeGroupApproval.argsSummary,
										args: activeGroupApproval.args,
										subagent: activeGroupApproval.subAgent
									}}
									agentName={activeGroupApproval.agent}
									agentColor={memberColorMap.get(activeGroupApproval.agent) ||
										activeGroupApproval.agentColor}
									queuePosition={1}
									queueTotal={groupApprovals.length}
									onDecide={(decision) => void decideGroupApproval(activeGroupApproval, decision)}
								/>
							</div>
						{/if}

						<!-- Rounded floating composer card (matching regular 1-on-1 chat) -->
						<div
							role="region"
							aria-label={t('myaaw.groups.composer_placeholder')}
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

							{#if pendingQuotes.length > 0}
								<div class="px-3 pt-3" data-ask-quotes>
									<AskQuoteList
										quotes={pendingQuotes}
										onremove={removeAskQuote}
										onclear={clearAskQuotes}
										onlocate={(quote) => void locateAskQuote(quote)}
									/>
								</div>
							{/if}

							<textarea
								bind:this={composer}
								bind:value={input}
								oninput={handleComposerInput}
								onkeydown={handleComposerKeydown}
								onpaste={onPaste}
								onclick={updateMentionMenu}
								rows={1}
								placeholder={t('myaaw.groups.composer_placeholder')}
								class="max-h-40 min-h-11 w-full resize-none rounded-t-2xl bg-transparent px-3 py-2.5 text-sm outline-none placeholder:text-muted-foreground terminal:rounded-none sm:min-h-13 sm:px-4 sm:py-3"
							></textarea>

							<div
								class="@container flex flex-wrap items-center justify-between gap-1 px-2 pb-1.5 sm:px-3 sm:pb-2"
							>
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
									{#if workspaceAgent}
										<ProjectComposerControls
											workingDir={project.workingDir}
											workingDirName={project.workingDirName}
											hasGit={project.hasGit}
											gitBranch={project.gitBranch}
											terminalOpen={project.showingTerminal}
											onFolderClick={handleComposerFolderClick}
											onToggleTerminal={toggleProjectTerminal}
										/>
									{/if}
								</div>

								<Button
									type="button"
									size="icon-sm"
									class="shrink-0 rounded-full"
									disabled={!canSend}
									onclick={() => void handleSend()}
									title={t('myaaw.chat.send')}
									aria-label={t('myaaw.chat.send')}
								>
									{#if sending}
										<Loader2 class="h-4 w-4 animate-spin" />
									{:else}
										<Send class="h-4 w-4" />
									{/if}
								</Button>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- Right inspector / project sidebar panel + unified activity bar -->
	{#if group}
		{#if inspectorOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 bg-black/40 lg:hidden"
				aria-label={t('myaaw.focus.panel_close')}
				onclick={closeRightPanel}
				transition:fade={{ duration: 150 }}
			></button>
		{/if}
		<aside
			inert={!inspectorOpen}
			class={[
				'fixed inset-y-0 right-0 z-50 flex w-80 max-w-[85vw] shrink-0 flex-col overflow-hidden border-l bg-card shadow-xl frost-drawer transition-transform duration-200 ease-out lg:static lg:z-auto lg:h-full lg:max-w-none lg:translate-x-0 lg:shadow-none lg:transition-[width] lg:duration-200',
				inspectorOpen
					? activeRightPanel === 'project'
						? 'translate-x-0 lg:w-[17rem]'
						: 'translate-x-0 lg:w-80'
					: 'pointer-events-none translate-x-full lg:pointer-events-auto lg:w-0 lg:border-l-0'
			]}
		>
			<div
				class={[
					'flex h-full w-full flex-col',
					activeRightPanel === 'project' ? 'lg:w-[17rem] lg:min-w-[17rem]' : 'lg:w-80 lg:min-w-80'
				]}
			>
				{#if project.workingDir && workspaceAgent}
					<div class={activeRightPanel === 'project' ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
						<ProjectSidebarPanel
							agent={workspaceAgent}
							conversationId={projectContextId}
							workingDir={project.workingDir}
							workingDirExists={project.workingDirExists}
							hasGit={project.hasGit}
							refreshKey={project.refreshKey}
							terminalOpen={project.showingTerminal}
							activeFilePath={project.activeFilePath}
							subTab={project.sidebarTab}
							scopeKey={projectScopeKeyStr}
							showChatsTab={false}
							onSubTabChange={(tab) => (project.sidebarTab = tab)}
							onOpenFile={openProjectFile}
							onOpenDiff={openProjectDiff}
							onOpenMultiDiff={openProjectMultiDiff}
							onToggleTerminal={toggleProjectTerminal}
							onFileRenamed={(fromPath, toPath) => project.handleFileRenamed(fromPath, toPath)}
							onFileDeleted={(deletedPath) => project.handleFileDeleted(deletedPath)}
							onGitBranchChange={(branch) => {
								project.gitBranch = branch;
							}}
							onClose={closeRightPanel}
						/>
					</div>
				{/if}

				{#if activeRightPanel === 'info' || activeRightPanel === 'attachments'}
					<GroupInspectorPanel
						{group}
						bind:tab={inspectorTab}
						pendingApprovals={groupApprovals}
						{attachmentsRefreshKey}
						onDecideApproval={(entry, decision) => void decideGroupApproval(entry, decision)}
						onOpenAttachment={openAttachmentPreview}
						onShowAttachmentInChat={showAttachmentInChat}
						onClose={closeRightPanel}
						onUpdated={(updated) => (group = updated)}
						onCleared={() => {
							messages = [];
							hasMore = false;
							totalMessages = 0;
							loadingOlder = false;
							activeTurn = null;
							replyTo = null;
							pendingQuotes = [];
							attachmentsRefreshKey += 1;
							runState = { ...EMPTY_RUN_STATE, max_turns: group?.max_turns ?? 8 };
						}}
					/>
				{/if}
			</div>
		</aside>

		<ChatRightActivityBar
			activePanel={activeRightPanel}
			showInfoAndNewChat={true}
			showNewChat={false}
			infoKind="group"
			infoLabel={t('myaaw.groups.show_inspector')}
			showProject={Boolean(workspaceAgent)}
			hasProject={Boolean(project.workingDir)}
			hasGit={project.hasGit}
			workingDir={project.workingDir}
			onToggleInfo={toggleInspector}
			onToggleProject={openProjectPanel}
			onToggleAttachments={toggleAttachmentsPanel}
		/>
	{/if}

	{#if group && workspaceAgent && !project.workingDir}
		<AttachWorkingDirDialog
			bind:open={project.dialogOpen}
			agent={workspaceAgent}
			onSelect={handleSelectWorkingDir}
		/>
	{/if}

	<LightBox bind:open={lightboxOpen} bind:index={lightboxIndex} items={lightboxItems} />

	<!-- Floating text-selection Quote button -->
	{#if askAnchor}
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
</div>
