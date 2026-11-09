import { browser } from '$app/environment';
import { showApprovalNotification } from '$lib/notifications/browser.svelte';
import { playNotification } from '$lib/sound/notify.svelte';
import { chatRuns } from '$lib/stores/chatRuns.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import { unreadChats } from '$lib/stores/unreadChats.svelte';

/**
 * Tool calls waiting for a decision, across every conversation of an agent.
 *
 * Two sources feed it: the open chat's run stream (`ChatRun.pendingApproval`,
 * instant) and the agent's `approval.required` / `approval.resolved` events on
 * `/api/events`. The second is what makes a sub-agent approval visible: its run
 * outlives the parent chat stream, so the dashboard would otherwise never see it.
 */
export interface PendingApproval {
	approvalId: string;
	agent: string;
	conversationId: string;
	toolName: string;
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
	conversation_id?: string;
	run_id?: string;
	tool_name?: string;
	args_summary?: string;
	args?: string;
	subagent?: string;
	decision?: string;
	resolved?: boolean;
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
	if (!approvalId || !agent || !conversationId || events[approvalId]) return;

	const entry: PendingApproval = {
		approvalId,
		agent,
		conversationId,
		toolName: data.tool_name || 'tool',
		argsSummary: data.args_summary,
		args: data.args,
		subAgent: data.subagent,
		runId: data.run_id
	};
	events = { ...events, [approvalId]: entry };
	alertIfAway(entry);
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
	/** Every pending approval of one agent, deduplicated by approval id. */
	list(agent: string): PendingApproval[] {
		if (!agent) return [];
		const merged = new Map<string, PendingApproval>();
		for (const entry of fromRuns(agent)) merged.set(entry.approvalId, entry);
		for (const entry of Object.values(events)) {
			if (entry.agent === agent) merged.set(entry.approvalId, entry);
		}
		return [...merged.values()];
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
	setTitle(agent: string, id: string, title: string) {
		if (!agent || !id || !title) return;
		titles = { ...titles, [titleKey(agent, id)]: title };
	},
	titleOf(agent: string, id: string): string {
		return titles[titleKey(agent, id)] ?? '';
	}
};
