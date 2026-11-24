import { untrack } from 'svelte';
import { groups as groupsApi } from '$lib/apis/groups';
import { getErrorMessage } from '$lib/utils/api';
import { stripInteractionTags } from '$lib/utils/interactions';
import type { SSEEvent } from '$lib/utils/sse';
import type {
	CreateGroupRequest,
	Group,
	GroupDetail,
	GroupDetailParams,
	GroupInteractionResponse,
	GroupMessage,
	GroupRunState,
	GroupSummary,
	InteractionRequest,
	SendGroupMessageRequest,
	SendGroupMessageResponse,
	StopGroupResponse,
	ContinueGroupResponse,
	UpdateGroupRequest
} from '$lib/types';

let list = $state<GroupSummary[]>([]);
let loading = $state(false);
let loaded = $state(false);
let error = $state('');
let createDialogOpen = $state(false);
let openGroupId = $state('');
let unreadCounts = $state<Record<string, number>>({});
const announcedMessageIds = new Set<string>();
const detailCache = new Map<string, GroupDetail>();

function cacheTailDetail(id: string, detail: GroupDetail) {
	detailCache.set(id, {
		...detail,
		unread_count: 0,
		first_unread_message_id: ''
	});
}

function toSummary(group: Group, existing?: GroupSummary, runState?: GroupRunState): GroupSummary {
	return {
		id: group.id,
		name: group.name,
		description: group.description,
		members: group.members ?? [],
		member_count: (group.members ?? []).length,
		discussion_mode: group.discussion_mode,
		max_turns: group.max_turns,
		running: runState ? runState.running : (existing?.running ?? false),
		current_agent: runState ? runState.current_agent : (existing?.current_agent ?? ''),
		last_message: existing?.last_message ?? null,
		created_at: group.created_at,
		updated_at: group.updated_at
	};
}

function sortByUpdatedDesc(items: GroupSummary[]): GroupSummary[] {
	return [...items].sort((a, b) => {
		const aTime = Date.parse(a.last_message?.created_at || a.updated_at || '') || 0;
		const bTime = Date.parse(b.last_message?.created_at || b.updated_at || '') || 0;
		return bTime - aTime;
	});
}

function upsertSummary(summary: GroupSummary) {
	const index = list.findIndex((entry) => entry.id === summary.id);
	if (index >= 0) {
		list = sortByUpdatedDesc(list.map((entry, pos) => (pos === index ? summary : entry)));
	} else {
		list = sortByUpdatedDesc([summary, ...list]);
	}
}

export const groupsStore = {
	get groups() {
		return list;
	},
	get loading() {
		return loading;
	},
	get loaded() {
		return loaded;
	},
	get error() {
		return error;
	},
	get createDialogOpen() {
		return createDialogOpen;
	},
	openCreateDialog() {
		createDialogOpen = true;
	},
	closeCreateDialog() {
		createDialogOpen = false;
	},
	setCreateDialogOpen(open: boolean) {
		createDialogOpen = open;
	},
	reset() {
		list = [];
		loaded = false;
		loading = false;
		error = '';
		openGroupId = '';
		unreadCounts = {};
		announcedMessageIds.clear();
		detailCache.clear();
	},
	peekDetail(id: string): GroupDetail | undefined {
		return id ? detailCache.get(id) : undefined;
	},
	peekGroup(id: string): Group | null {
		if (!id) return null;
		const cached = detailCache.get(id)?.group;
		if (cached) return cached;
		const summary = list.find((entry) => entry.id === id);
		if (!summary) return null;
		return {
			id: summary.id,
			name: summary.name,
			description: summary.description,
			members: summary.members,
			discussion_mode: summary.discussion_mode,
			max_turns: summary.max_turns,
			created_at: summary.created_at,
			updated_at: summary.updated_at
		};
	},
	syncCachedMessages(id: string, messages: GroupMessage[], total?: number) {
		const cached = detailCache.get(id);
		if (!cached) return;
		detailCache.set(id, {
			...cached,
			messages,
			total_messages:
				total !== undefined
					? Math.max(total, messages.length)
					: Math.max(cached.total_messages ?? messages.length, messages.length)
		});
	},
	async load(force = false) {
		if (untrack(() => loading)) return;
		if (!force && untrack(() => loaded)) return;
		loading = true;
		error = '';
		try {
			const res = await groupsApi.list();
			const fetched = sortByUpdatedDesc(res.groups ?? []);
			list = fetched;
			const nextUnreads: Record<string, number> = {};
			for (const grp of fetched) {
				if (grp.id !== openGroupId && (grp.unread_count ?? 0) > 0) {
					nextUnreads[grp.id] = grp.unread_count ?? 0;
				}
			}
			unreadCounts = nextUnreads;
			loaded = true;
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
		}
	},
	async create(payload: CreateGroupRequest): Promise<Group> {
		const created = await groupsApi.create(payload);
		upsertSummary(toSummary(created));
		loaded = true;
		return created;
	},
	async get(id: string, params?: GroupDetailParams): Promise<GroupDetail> {
		const detail = await groupsApi.get(id, params);
		const existing = list.find((entry) => entry.id === id);
		const summary = toSummary(detail.group, existing, detail.run_state);
		// Only overwrite `last_message` when fetching the tail or full transcript —
		// backward pagination (`before_id` or a positive slice) returns older messages.
		const isTailOrFull =
			!params?.before_id &&
			(params?.limit === undefined ||
				params.limit === 0 ||
				params.offset === undefined ||
				params.offset < 0);
		if (isTailOrFull) {
			const lastMsg =
				detail.messages.length > 0 ? detail.messages[detail.messages.length - 1] : null;
			summary.last_message = lastMsg
				? {
						id: lastMsg.id,
						sender_type: lastMsg.sender_type,
						sender_name: lastMsg.sender_name,
						content: stripInteractionTags(lastMsg.content),
						created_at: lastMsg.created_at
					}
				: null;
			cacheTailDetail(id, detail);
		}
		upsertSummary(summary);
		return detail;
	},
	async update(id: string, payload: UpdateGroupRequest): Promise<Group> {
		const updated = await groupsApi.update(id, payload);
		const existing = list.find((entry) => entry.id === id);
		upsertSummary(toSummary(updated, existing));
		const cached = detailCache.get(id);
		if (cached) {
			detailCache.set(id, { ...cached, group: updated });
		}
		return updated;
	},
	async remove(id: string): Promise<void> {
		await groupsApi.remove(id);
		list = list.filter((entry) => entry.id !== id);
		detailCache.delete(id);
	},
	async clearMessages(id: string): Promise<void> {
		await groupsApi.clearMessages(id);
		list = list.map((entry) =>
			entry.id === id
				? {
						...entry,
						running: false,
						current_agent: '',
						last_message: null,
						updated_at: new Date().toISOString()
					}
				: entry
		);
		const cached = detailCache.get(id);
		if (cached) {
			detailCache.set(id, {
				...cached,
				messages: [],
				has_more: false,
				total_messages: 0,
				run_state: {
					...cached.run_state,
					running: false,
					paused: false,
					pause_reason: '',
					current_agent: ''
				}
			});
		}
	},
	async sendMessage(
		id: string,
		payload: SendGroupMessageRequest
	): Promise<SendGroupMessageResponse> {
		const res = await groupsApi.sendMessage(id, payload);
		groupsStore.syncSummaryFromMessage(id, res.message);
		const existing = list.find((entry) => entry.id === id);
		// SSE `turn_start`/`status` often arrives before the 202 HTTP response;
		// keep the live `current_agent` instead of regressing to the initial empty one.
		const nextRunState =
			existing?.running &&
			existing.current_agent &&
			res.run_state.running &&
			!res.run_state.current_agent
				? { ...res.run_state, current_agent: existing.current_agent }
				: res.run_state;
		groupsStore.syncSummaryFromRunState(id, nextRunState);
		return res;
	},
	async respondInteraction(
		id: string,
		payload: InteractionRequest
	): Promise<GroupInteractionResponse> {
		const res = await groupsApi.respondInteraction(id, payload);
		if (res.message?.id) {
			groupsStore.syncSummaryFromMessage(id, res.message);
		}
		if (res.run_state) {
			const existing = list.find((entry) => entry.id === id);
			const nextRunState =
				existing?.running &&
				existing.current_agent &&
				res.run_state.running &&
				!res.run_state.current_agent
					? { ...res.run_state, current_agent: existing.current_agent }
					: res.run_state;
			groupsStore.syncSummaryFromRunState(id, nextRunState);
		}
		return res;
	},
	async stop(id: string): Promise<StopGroupResponse> {
		const res = await groupsApi.stop(id);
		groupsStore.syncSummaryFromRunState(id, res.run_state);
		return res;
	},
	continueRun: async (id: string, extraTurns?: number): Promise<ContinueGroupResponse> => {
		const res = await groupsApi.continueRun(
			id,
			extraTurns !== undefined ? { extra_turns: extraTurns } : {}
		);
		groupsStore.syncSummaryFromRunState(id, res.run_state);
		return res;
	},
	fetchAttachments(id: string, params?: { limit?: number; offset?: number; kind?: string }) {
		return groupsApi.attachments(id, params);
	},
	uploadUrl(
		groupId: string,
		messageId: string,
		storedName: string,
		options?: { download?: boolean }
	) {
		return groupsApi.uploadUrl(groupId, messageId, storedName, options);
	},
	downloadUpload(groupId: string, messageId: string, storedName: string, fileName?: string) {
		return groupsApi.downloadUpload(groupId, messageId, storedName, fileName);
	},
	syncSummaryFromRunState(id: string, runState: GroupRunState) {
		list = list.map((entry) =>
			entry.id === id
				? {
						...entry,
						running: runState.running,
						current_agent: runState.current_agent,
						pending_approval_count: runState.pending_approvals?.length ?? 0
					}
				: entry
		);
		const cached = detailCache.get(id);
		if (cached) {
			detailCache.set(id, { ...cached, run_state: runState });
		}
	},
	syncSummaryFromMessage(id: string, msg: GroupMessage) {
		const cached = detailCache.get(id);
		if (cached && !cached.messages.some((existingMsg) => existingMsg.id === msg.id)) {
			const nextMessages = [...cached.messages, msg];
			detailCache.set(id, {
				...cached,
				messages: nextMessages,
				total_messages:
					Math.max(cached.total_messages ?? cached.messages.length, cached.messages.length) + 1
			});
		}
		const existing = list.find((entry) => entry.id === id);
		if (!existing) return;
		upsertSummary({
			...existing,
			last_message: {
				id: msg.id,
				sender_type: msg.sender_type,
				sender_name: msg.sender_name,
				content: stripInteractionTags(msg.content),
				created_at: msg.created_at
			},
			updated_at: msg.created_at || new Date().toISOString()
		});
	},
	setOpen(id: string) {
		openGroupId = id;
		if (id && unreadCounts[id]) {
			const next = { ...unreadCounts };
			delete next[id];
			unreadCounts = next;
		}
	},
	isOpen(id: string): boolean {
		return Boolean(id) && openGroupId === id;
	},
	markUnread(id: string) {
		if (!id) return;
		unreadCounts = { ...unreadCounts, [id]: (unreadCounts[id] ?? 0) + 1 };
	},
	setUnreadCount(id: string, count: number) {
		if (!id) return;
		if (count <= 0) {
			if (!unreadCounts[id]) return;
			const next = { ...unreadCounts };
			delete next[id];
			unreadCounts = next;
			return;
		}
		unreadCounts = { ...unreadCounts, [id]: count };
	},
	markRead(id: string, messageId?: string) {
		if (!id) return;
		groupsStore.setUnreadCount(id, 0);
		void groupsApi.markRead(id, messageId).catch(() => {});
	},
	unreadCount(id: string): number {
		return id ? (unreadCounts[id] ?? 0) : 0;
	},
	nameOf(id: string): string {
		return list.find((entry) => entry.id === id)?.name ?? '';
	},
	/** Returns true once per message id so reconnects never double-alert. */
	claimMessageAnnouncement(messageId: string): boolean {
		if (!messageId || announcedMessageIds.has(messageId)) return false;
		announcedMessageIds.add(messageId);
		if (announcedMessageIds.size > 500) {
			const oldest = announcedMessageIds.values().next().value;
			if (oldest) announcedMessageIds.delete(oldest);
		}
		return true;
	},
	/**
	 * Opens a live SSE connection to `GET /groups/:id/stream`. Reconnects
	 * gracefully if the server closes the stream until the returned cleanup
	 * function is invoked.
	 */
	connectStream(
		id: string,
		onEvent: (event: SSEEvent) => void,
		onError?: (err: unknown) => void
	): () => void {
		const controller = new AbortController();
		const abortOnUnload = () => controller.abort();
		if (typeof window !== 'undefined') {
			window.addEventListener('beforeunload', abortOnUnload);
			window.addEventListener('pagehide', abortOnUnload);
		}

		void (async () => {
			let delayMs = 1500;
			while (!controller.signal.aborted) {
				try {
					await groupsApi.stream(id, onEvent, controller.signal);
					delayMs = 1500;
				} catch (err) {
					if (controller.signal.aborted) return;
					const status = (err as { status?: number } | undefined)?.status;
					// Do not spin on auth or missing-group errors.
					if (status === 401 || status === 403 || status === 404) {
						onError?.(err);
						return;
					}
					onError?.(err);
					delayMs = Math.min(delayMs * 2, 10000);
				}
				if (controller.signal.aborted) return;
				await new Promise((resolve) => setTimeout(resolve, delayMs));
			}
		})();

		return () => {
			if (typeof window !== 'undefined') {
				window.removeEventListener('beforeunload', abortOnUnload);
				window.removeEventListener('pagehide', abortOnUnload);
			}
			controller.abort();
		};
	}
};
