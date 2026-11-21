import { untrack } from 'svelte';
import { groups as groupsApi } from '$lib/apis/groups';
import { getErrorMessage } from '$lib/utils/api';
import type { SSEEvent } from '$lib/utils/sse';
import type {
	CreateGroupRequest,
	Group,
	GroupDetail,
	GroupMessage,
	GroupRunState,
	GroupSummary,
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
	},
	async load(force = false) {
		if (untrack(() => loading)) return;
		if (!force && untrack(() => loaded)) return;
		loading = true;
		error = '';
		try {
			const res = await groupsApi.list();
			list = sortByUpdatedDesc(res.groups ?? []);
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
	async get(id: string): Promise<GroupDetail> {
		const detail = await groupsApi.get(id);
		const existing = list.find((entry) => entry.id === id);
		const lastMsg = detail.messages.length > 0 ? detail.messages[detail.messages.length - 1] : null;
		const summary = toSummary(detail.group, existing, detail.run_state);
		summary.last_message = lastMsg
			? {
					id: lastMsg.id,
					sender_type: lastMsg.sender_type,
					sender_name: lastMsg.sender_name,
					content: lastMsg.content,
					created_at: lastMsg.created_at
				}
			: null;
		upsertSummary(summary);
		return detail;
	},
	async update(id: string, payload: UpdateGroupRequest): Promise<Group> {
		const updated = await groupsApi.update(id, payload);
		const existing = list.find((entry) => entry.id === id);
		upsertSummary(toSummary(updated, existing));
		return updated;
	},
	async remove(id: string): Promise<void> {
		await groupsApi.remove(id);
		list = list.filter((entry) => entry.id !== id);
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
	},
	async sendMessage(
		id: string,
		payload: SendGroupMessageRequest
	): Promise<SendGroupMessageResponse> {
		const res = await groupsApi.sendMessage(id, payload);
		groupsStore.syncSummaryFromMessage(id, res.message);
		groupsStore.syncSummaryFromRunState(id, res.run_state);
		return res;
	},
	async stop(id: string): Promise<StopGroupResponse> {
		const res = await groupsApi.stop(id);
		groupsStore.syncSummaryFromRunState(id, res.run_state);
		return res;
	},
	async continueRun(id: string, extraTurns?: number): Promise<ContinueGroupResponse> {
		const res = await groupsApi.continueRun(
			id,
			extraTurns !== undefined ? { extra_turns: extraTurns } : {}
		);
		groupsStore.syncSummaryFromRunState(id, res.run_state);
		return res;
	},
	syncSummaryFromRunState(id: string, runState: GroupRunState) {
		list = list.map((entry) =>
			entry.id === id
				? {
						...entry,
						running: runState.running,
						current_agent: runState.current_agent
					}
				: entry
		);
	},
	syncSummaryFromMessage(id: string, msg: GroupMessage) {
		const existing = list.find((entry) => entry.id === id);
		if (!existing) return;
		upsertSummary({
			...existing,
			last_message: {
				id: msg.id,
				sender_type: msg.sender_type,
				sender_name: msg.sender_name,
				content: msg.content,
				created_at: msg.created_at
			},
			updated_at: msg.created_at || new Date().toISOString()
		});
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

		return () => controller.abort();
	}
};
