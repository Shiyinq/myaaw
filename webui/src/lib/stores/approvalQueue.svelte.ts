import { browser } from '$app/environment';
import { showApprovalNotification } from '$lib/notifications/browser.svelte';
import { playNotification } from '$lib/sound/notify.svelte';
import { chatRuns } from '$lib/stores/chatRuns.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import { unreadChats } from '$lib/stores/unreadChats.svelte';
import type { GroupPendingApproval } from '$lib/types';

/**
 * Tool calls waiting for a decision, across every conversation of an agent and
 * every multi-agent group.
 *
 * Two sources feed it: the open chat's run stream (`ChatRun.pendingApproval`,
 * instant) and the agent's `approval.required` / `approval.resolved` events on
 * `/api/events` (plus group SSE events and initial `run_state.pending_approvals`).
 * Group-scoped approvals (`scope: "group:<id>"`) are kept strictly separate from
 * 1-on-1 agent DM lists so they appear only in their owning group.
 */
export interface PendingApproval {
	approvalId: string;
	agent: string;
	conversationId: string;
	scope?: string;
	groupId?: string;
	agentColor?: string;
	toolName: string;
	action?: string;
	argsSummary?: string;
	/** Full raw argument JSON; present on the event and on the chat stream. */
	args?: string;
	subAgent?: string;
	runId?: string;
}

/** Payload of an `approval.required` / `approval.resolved` event. */
export interface ApprovalEvent {
	approval_id?: string;
	agent?: string;
	agent_color?: string;
	conversation_id?: string;
	scope?: string;
	group_id?: string;
	run_id?: string;
	tool_name?: string;
	action?: string;
	args_summary?: string;
	args?: string;
	subagent?: string;
	decision?: string;
	resolved?: boolean;
}

function groupIdFromScope(scope?: string): string | undefined {
	if (!scope || !scope.startsWith('group:')) return undefined;
	const id = scope.slice('group:'.length).trim();
	return id || undefined;
}

let events = $state<Record<string, PendingApproval>>({});
/** Titles learned from the chat page, so a chat that is not in the list yet still has a name. */
let titles = $state<Record<string, string>>({});
/** Approvals that already alerted this tab, so leaving and returning cannot ring twice. */
const alerted = new Set<string>();

function titleKey(agent: string, id: string) {
	return `${agent}:${id}`;
}

/**
 * Announces a pending approval once. The tone belongs to the event itself, the way
 * a finished turn rings even while its chat is open; the browser notification is
 * only for a prompt the reader cannot see (another chat, another page, a hidden
 * tab). Deduplicated per approval so no later re-check can ring twice.
 */
function alertIfAway(entry: PendingApproval, onScreen?: boolean) {
	if (!browser || alerted.has(entry.approvalId)) return;
	const visible = onScreen ?? unreadChats.isOpen(entry.agent, entry.conversationId);
	const hidden = document.visibilityState === 'hidden';
	alerted.add(entry.approvalId);

	if (preferences.value.notifyEnabled) {
		playNotification(preferences.value.notifySound, preferences.value.notifyVolume);
	}
	if (preferences.value.notifyBrowser && (!visible || hidden)) {
		showApprovalNotification({
			agent: entry.agent,
			conversationId: entry.conversationId,
			toolName: entry.toolName,
			subAgent: entry.subAgent
		});
	}
}

/** Adds a pending approval from a live event. Returns silently when unknown already. */
function required(connectionAgent: string, data: ApprovalEvent) {
	const approvalId = data?.approval_id;
	const agent = data?.agent || connectionAgent;
	const conversationId = data?.conversation_id || '';
	const scope = data?.scope || (data?.group_id ? `group:${data.group_id}` : undefined);
	const groupId = data?.group_id || groupIdFromScope(scope);
	if (!approvalId || !agent || (!conversationId && !groupId)) return;

	const existing = events[approvalId];
	const entry: PendingApproval = {
		approvalId,
		agent,
		conversationId: conversationId || existing?.conversationId || '',
		scope: scope || existing?.scope,
		groupId: groupId || existing?.groupId,
		agentColor: data.agent_color || existing?.agentColor,
		toolName: data.tool_name || existing?.toolName || 'tool',
		action: data.action || existing?.action,
		argsSummary: data.args_summary ?? existing?.argsSummary,
		args: data.args ?? existing?.args,
		subAgent: data.subagent ?? existing?.subAgent,
		runId: data.run_id ?? existing?.runId
	};
	events = { ...events, [approvalId]: entry };
	if (!existing) {
		alertIfAway(entry, groupId ? false : undefined);
	}
}

/** Replaces the server-known pending approvals for one group (e.g. on initial load/refresh). */
function syncGroup(groupId: string, pending: GroupPendingApproval[]) {
	if (!groupId) return;
	const keepIds = new Set(pending.map((item) => item.approval_id));
	const next: Record<string, PendingApproval> = {};
	for (const [id, entry] of Object.entries(events)) {
		// Keep all non-group entries, entries from other groups, and still-live entries
		// (plus any sub-agent entries that arrived via /api/events for this group).
		if (entry.groupId === groupId && !keepIds.has(id) && !entry.subAgent) {
			alerted.delete(id);
			continue;
		}
		next[id] = entry;
	}
	for (const item of pending) {
		if (!item.approval_id || !item.agent) continue;
		const prev = next[item.approval_id];
		next[item.approval_id] = {
			approvalId: item.approval_id,
			agent: item.agent,
			conversationId: item.conversation_id || prev?.conversationId || '',
			scope: `group:${groupId}`,
			groupId,
			agentColor: item.agent_color || prev?.agentColor,
			toolName: item.tool_name || prev?.toolName || 'tool',
			action: item.action || prev?.action,
			argsSummary: item.args_summary ?? prev?.argsSummary,
			args: item.args ?? prev?.args,
			subAgent: item.subagent ?? prev?.subAgent,
			runId: item.run_id ?? prev?.runId
		};
	}
	events = next;
}

/** Removes a pending approval after a decision (or a timeout). */
function resolved(data: ApprovalEvent) {
	const approvalId = data?.approval_id;
	if (!approvalId) return;
	alerted.delete(approvalId);
	if (!events[approvalId]) return;
	const next = { ...events };
	delete next[approvalId];
	events = next;
}

/** Approvals still pending on the runs this tab is streaming. */
function fromRuns(agent: string): PendingApproval[] {
	const list: PendingApproval[] = [];
	for (const run of chatRuns.all()) {
		const prompt = run.pendingApproval;
		if (!prompt?.approval_id || run.agent !== agent || !run.conversationId) continue;
		list.push({
			approvalId: prompt.approval_id,
			agent: run.agent,
			conversationId: run.conversationId,
			toolName: prompt.tool_name || 'tool',
			argsSummary: prompt.args_summary,
			args: prompt.args,
			subAgent: prompt.subagent,
			runId: run.requestId
		});
	}
	return list;
}

export const approvalQueue = {
	/** Every non-group pending approval of one agent, deduplicated by approval id. */
	list(agent: string): PendingApproval[] {
		if (!agent) return [];
		const merged = new Map<string, PendingApproval>();
		for (const entry of fromRuns(agent)) merged.set(entry.approvalId, entry);
		for (const entry of Object.values(events)) {
			if (entry.agent === agent && !entry.groupId) merged.set(entry.approvalId, entry);
		}
		return [...merged.values()];
	},
	/** Every pending approval belonging to one multi-agent group, in FIFO order. */
	listForGroup(groupId: string): PendingApproval[] {
		if (!groupId) return [];
		const result: PendingApproval[] = [];
		for (const entry of Object.values(events)) {
			if (entry.groupId === groupId) result.push(entry);
		}
		return result;
	},
	/** Count of pending approvals for one group (for sidebar badges). */
	groupCount(groupId: string): number {
		if (!groupId) return 0;
		let count = 0;
		for (const entry of Object.values(events)) {
			if (entry.groupId === groupId) count += 1;
		}
		return count;
	},
	has(agent: string, conversationId: string): boolean {
		return approvalQueue.list(agent).some((entry) => entry.conversationId === conversationId);
	},
	/**
	 * Re-evaluates every parked approval. The layout calls this whenever the
	 * selected agent, the parked list, or the open chat changes, so an approval
	 * that was on screen and is no longer still rings once.
	 */
	syncAlerts(agent: string) {
		for (const entry of approvalQueue.list(agent)) {
			alertIfAway(entry, unreadChats.isOpen(entry.agent, entry.conversationId));
		}
	},
	/** The pending approval of one conversation, for the in-chat card. */
	pendingFor(agent: string, conversationId: string): PendingApproval | undefined {
		if (!agent || !conversationId) return undefined;
		return approvalQueue.list(agent).find((entry) => entry.conversationId === conversationId);
	},
	required,
	resolved,
	syncGroup,
	setTitle(agent: string, id: string, title: string) {
		if (!agent || !id || !title) return;
		titles = { ...titles, [titleKey(agent, id)]: title };
	},
	titleOf(agent: string, id: string): string {
		return titles[titleKey(agent, id)] ?? '';
	}
};
