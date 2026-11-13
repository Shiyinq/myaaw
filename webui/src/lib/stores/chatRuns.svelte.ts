import { browser } from '$app/environment';
import { chat as chatApi } from '$lib/apis/chat';
import { composeAskPrompt } from '$lib/utils/ask';
import type {
	ApprovalPrompt,
	AskQuote,
	Attachment,
	ChatFile,
	ChatRunInfo,
	ChatStreamEvent,
	ContextUsage,
	TimelineItem,
	Upload,
	Usage
} from '$lib/types';

/**
 * Chat runs live here, not in the page component. A run keeps its stream, its
 * timeline, and its pending approval while the user opens another conversation
 * or another menu, so coming back shows the answer still streaming instead of a
 * restart or a stalled transcript. Only Stop (button or Esc) or closing the tab
 * ends a run.
 */

export interface ChatItem extends TimelineItem {
	localId?: string;
	/** Position of the stored message this row was rebuilt from, if any. */
	messageIndex?: number;
	/** UTC timestamp shown next to user and answer rows. */
	createdAt?: string;
	/** Sender identity for user rows that came from a shared chat (group/topic). */
	senderId?: string;
	senderName?: string;
	senderUsername?: string;
	/**
	 * Set when the turn was started by the system rather than typed: a cron job, a
	 * heartbeat tick, or a finished sub-agent. The stored text carries a
	 * model-only <trigger> block; the row shows a badge and hides the block.
	 */
	trigger?: 'cron' | 'heartbeat' | 'subagent';
	images?: string[];
	uploads?: Upload[];
	/** Excerpts quoted from earlier answers, thinking, or tool output. */
	quotes?: AskQuote[];
	/** Files delivered with this answer; rehydrated from the stored transcript. */
	attachments?: Attachment[];
	running?: boolean;
	expanded?: boolean;
	group_duration_ms?: number;
}

const STEP_KINDS = new Set(['thought', 'tool', 'retry', 'compaction']);
const LAST_CONVERSATION_KEY = 'myaaw_last_conversation';

export interface StartRunOptions {
	agent: string;
	/** Conversation being continued; empty for a brand new chat. */
	conversationId: string;
	text: string;
	/** Excerpts quoted from earlier replies; folded into the prompt before sending. */
	quotes?: AskQuote[];
	images: string[];
	imagesDisplay: string[];
	files: ChatFile[];
	uploads: Upload[];
	/** Rows already on screen, so continuing a chat keeps its earlier turns. */
	initialItems?: ChatItem[];
	/** Rewinds the transcript instead of appending: 0 = regenerate the last turn. */
	rewindUserOffset?: number;
}

/** The page attaches these while a run is on screen. */
export interface RunView {
	onConversation?: (conversationId: string) => void;
	onDone?: (run: ChatRun) => void;
	onError?: (message: string) => void;
}

function rememberConversation(agent: string, id: string) {
	if (!browser || !agent) return;
	let map: Record<string, string> = {};
	try {
		const parsed: unknown = JSON.parse(localStorage.getItem(LAST_CONVERSATION_KEY) ?? '{}');
		if (parsed && typeof parsed === 'object') map = parsed as Record<string, string>;
	} catch {
		map = {};
	}
	if (id) map[agent] = id;
	else delete map[agent];
	localStorage.setItem(LAST_CONVERSATION_KEY, JSON.stringify(map));
}

export class ChatRun {
	readonly agent: string;
	readonly requestId: string;
	conversationId = $state('');
	items = $state<ChatItem[]>([]);
	streaming = $state(true);
	usage = $state<Usage | null>(null);
	context = $state<ContextUsage | null>(null);
	error = $state('');
	pendingApproval = $state<ApprovalPrompt | null>(null);
	/** Step group that is still collecting rows, so its header keeps spinning. */
	openGroup = $state('');
	/** When this run started, used by the live duration in the process header. */
	startedAt = $state(Date.now());
	/** When the open step group started; the completed duration uses the same point. */
	groupStartedAt = $state(0);
	finishedAt = $state(0);
	/**
	 * Why the run ended. `done` means the answer landed, so it is worth announcing
	 * to a reader who left; `error` and `stopped` are the reader's own doing or a
	 * failure and stay quiet.
	 */
	finishReason = $state<'done' | 'error' | 'stopped' | null>(null);

	/** Set by the mounted page; cleared when it unmounts. */
	view: RunView | null = null;
	/** Set by the registry so a lost run can be forgotten. */
	discard?: () => void;
	/** True when the run was reattached after a reload; it must not speak by itself. */
	readonly resumed: boolean;

	private controller: AbortController | null = null;
	private counter = 0;
	private groupStart = 0;
	private groupLastStep = '';
	private adopted = false;
	private doneNotified = false;

	constructor(agent: string, requestId: string, resumed = false) {
		this.agent = agent;
		this.requestId = requestId;
		this.resumed = resumed;
	}

	// ---------------------------------------------------------------- timeline

	private stepRow(item: ChatItem) {
		return STEP_KINDS.has(item.kind);
	}

	/** Records how long the open step group took on its last step. */
	private closeGroup() {
		if (!this.groupStart || !this.groupLastStep) return;
		const duration = Date.now() - this.groupStart;
		const last = this.groupLastStep;
		this.groupStart = 0;
		this.groupStartedAt = 0;
		this.groupLastStep = '';
		this.openGroup = '';
		this.items = this.items.map((item) =>
			item.localId === last ? { ...item, group_duration_ms: duration } : item
		);
	}

	push(item: ChatItem) {
		const localId = item.localId ?? `item-${++this.counter}`;
		if (this.stepRow(item)) {
			if (!this.groupStart) {
				this.groupStart = Date.now();
				this.groupStartedAt = this.groupStart;
				this.openGroup = localId;
			}
			this.groupLastStep = localId;
		} else if (item.kind !== 'text') {
			// Text belongs to the run: the model often narrates between tools.
			this.closeGroup();
		}
		this.items = [...this.items, { ...item, localId }];
	}

	/** A thinking block closes as soon as the model stops thinking. */
	private finishThoughts() {
		this.items = this.items.map((item) =>
			item.kind === 'thought' && item.running ? { ...item, running: false, expanded: false } : item
		);
	}

	private mergeLast(kind: string, text: string) {
		const last = this.items[this.items.length - 1];
		if (last && last.kind === kind && (kind !== 'thought' || last.running)) {
			this.items = [...this.items.slice(0, -1), { ...last, text: (last.text ?? '') + text }];
		} else if (kind === 'thought') {
			this.push({ kind, text, running: true, expanded: true });
		} else {
			this.push({ kind, text });
		}
	}

	private setToolResult(id: string, result: string, isError?: boolean) {
		const index = this.items.findIndex((item) => item.kind === 'tool' && item.id === id);
		if (index < 0) {
			this.push({ kind: 'tool', id, result, is_error: isError, running: false, expanded: false });
			return;
		}
		this.items = this.items.map((item, position) =>
			position === index
				? { ...item, result, is_error: isError, running: false, expanded: false }
				: item
		);
	}

	/** Folds an interrupted or finished run so no step keeps spinning. */
	private endRun() {
		this.finishThoughts();
		this.items = this.items.map((item) => (item.running ? { ...item, running: false } : item));
		this.closeGroup();
		this.openGroup = '';
	}

	toggleItem(localId?: string) {
		if (!localId) return;
		this.items = this.items.map((item) =>
			item.localId === localId ? { ...item, expanded: !item.expanded } : item
		);
	}

	// ------------------------------------------------------------------ control

	start(options: StartRunOptions) {
		this.conversationId = options.conversationId;
		this.startedAt = Date.now();
		this.finishedAt = 0;
		const earlier = options.initialItems ?? [];
		this.items = [...earlier];
		// Continue the id sequence of the rows we adopted, so a stale local id
		// cannot toggle or merge the wrong row.
		this.counter = earlier.reduce((max, item) => {
			const match = /^item-(\d+)$/.exec(item.localId ?? '');
			return match ? Math.max(max, Number(match[1])) : max;
		}, 0);
		// An edit or a regeneration already carries its user turn in `initialItems`;
		// only a normal send appends a new question.
		if (options.rewindUserOffset === undefined) {
			this.push({
				kind: 'user',
				text: options.text,
				quotes: options.quotes?.length ? options.quotes : undefined,
				images: options.imagesDisplay.length > 0 ? options.imagesDisplay : undefined,
				uploads: options.uploads.length > 0 ? options.uploads : undefined,
				createdAt: new Date().toISOString()
			});
		}
		this.controller = new AbortController();

		void this.stream(options).finally(() => {
			this.endRun();
			this.streaming = false;
			this.finishedAt = Date.now();
			this.controller = null;
		});
	}

	/**
	 * Runs the page's completion callback once. A restored run can finish before
	 * the page attaches its view, so the page calls this after attaching.
	 */
	notifyDone() {
		if (this.doneNotified || !this.view) return;
		this.doneNotified = true;
		this.view.onDone?.(this);
	}

	/**
	 * Seeds a resumed run with the transcript the page already loaded. A reloaded
	 * tab starts the run with only the replayed events, so without this the live
	 * answer would replace the whole conversation until the run ends.
	 */
	adopt(earlier: ChatItem[]): boolean {
		if (this.adopted || earlier.length === 0) return false;
		this.adopted = true;
		this.items = [...earlier, ...this.items];
		return true;
	}

	/**
	 * Attaches to a run that started before this page existed (a reload, a closed
	 * tab, a logout). The server replays its buffer, so the timeline is rebuilt
	 * from the beginning and then keeps streaming.
	 */
	async resume() {
		this.streaming = true;
		this.controller = new AbortController();
		try {
			await chatApi.attach(
				this.agent,
				this.requestId,
				(event) => this.handleEvent(event as ChatEvent),
				this.controller.signal
			);
		} catch (err) {
			const status = (err as { status?: number } | undefined)?.status;
			if (status === 404) {
				// The server restarted or the run aged out; the transcript is history now.
				this.discard?.();
				return;
			}
			this.error = (err as Error)?.message || 'Stream error';
			this.view?.onError?.(this.error);
		} finally {
			this.endRun();
			this.streaming = false;
			this.finishedAt = Date.now();
			this.controller = null;
		}
	}

	private async stream(options: StartRunOptions) {
		try {
			await chatApi.stream(
				this.agent,
				{
					text: composeAskPrompt(options.text, options.quotes),
					request_id: this.requestId,
					conversation_id: options.conversationId || undefined,
					images: options.images.length > 0 ? options.images : undefined,
					files: options.files.length > 0 ? options.files : undefined,
					rewind_user_offset: options.rewindUserOffset
				},
				(event) => this.handleEvent(event),
				this.controller?.signal
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				this.error = (err as Error)?.message || 'Stream error';
				this.view?.onError?.(this.error);
			}
		}
	}

	private handleEvent(event: ChatEvent) {
		const data = event.data as Record<string, any>;
		switch (event.event as ChatStreamEvent) {
			case 'thought':
				this.mergeLast('thought', data?.text ?? '');
				break;
			case 'conversation':
				if (data?.conversation_id) this.bindConversation(data.conversation_id);
				break;
			case 'chunk':
				this.finishThoughts();
				this.mergeLast('text', data?.text ?? '');
				break;
			case 'tool':
				this.finishThoughts();
				this.push({
					kind: 'tool',
					id: data?.id,
					name: data?.name,
					arguments: data?.arguments,
					running: true,
					expanded: true
				});
				break;
			case 'tool_result':
				this.setToolResult(data?.id, data?.result ?? '', data?.is_error);
				break;
			case 'retry':
				this.push({ kind: 'retry', attempt: data?.attempt });
				break;
			case 'compaction':
				this.push({
					kind: 'compaction',
					tokens_before: data?.tokens_before,
					tokens_after: data?.tokens_after,
					dropped_messages: data?.dropped_messages
				});
				break;
			case 'context':
				// The newest request against the context window. A later event for the
				// same request replaces the estimate with the exact provider count.
				this.context = {
					tokens: data?.tokens ?? 0,
					window: data?.window ?? 0,
					system_tokens: data?.system_tokens ?? 0,
					tool_tokens: data?.tool_tokens ?? 0,
					message_tokens: data?.message_tokens ?? 0
				};
				break;
			case 'approval_required': {
				// Only the card above the composer renders it; a transcript row would
				// duplicate the same prompt in two places.
				this.pendingApproval = {
					approval_id: data?.approval_id,
					tool_name: data?.tool_name,
					action: data?.action,
					args_summary: data?.args_summary,
					args: data?.args,
					subagent: data?.subagent
				};
				break;
			}
			case 'approval_resolved':
				this.resolveApproval(data?.approval_id ?? '');
				break;
			case 'done': {
				this.endRun();
				this.finishReason = 'done';
				this.streaming = false;
				const next = data?.conversation_id;
				if (typeof next === 'string' && next) this.bindConversation(next);
				const doneAttachments = data?.attachments ?? [];
				if (data?.text || doneAttachments.length > 0) {
					// The final text is authoritative: it has the delivery directives
					// removed, while the streamed deltas still contain them. Attachments
					// ride on the same answer so history renders them too.
					let lastText = -1;
					for (let i = this.items.length - 1; i >= 0; i -= 1) {
						if (this.items[i].kind === 'text') {
							lastText = i;
							break;
						}
					}
					if (lastText >= 0) {
						this.items[lastText] = {
							...this.items[lastText],
							...(data?.text ? { text: data.text } : {}),
							attachments: doneAttachments
						};
					} else {
						this.push({ kind: 'text', text: data?.text ?? '', attachments: doneAttachments });
					}
				}
				// Live rows have no stored timestamp yet; stamp the answer when it lands.
				const stampedAt = new Date().toISOString();
				this.items = this.items.map((item) =>
					item.kind === 'text' && !item.createdAt ? { ...item, createdAt: stampedAt } : item
				);
				this.usage = data?.usage ?? null;
				this.notifyDone();
				break;
			}
			case 'error': {
				this.endRun();
				this.finishReason = 'error';
				this.streaming = false;
				this.error = data?.error ?? 'Stream error';
				this.push({ kind: 'error', text: this.error });
				this.view?.onError?.(this.error);
				break;
			}
		}
	}

	private bindConversation(id: string) {
		if (!id || this.conversationId === id) return;
		this.conversationId = id;
		rememberConversation(this.agent, id);
		this.view?.onConversation?.(id);
	}

	/** Marks an approval resolved, whether decided here or on the server. */
	resolveApproval(id: string) {
		if (!id) return;
		if (this.pendingApproval?.approval_id === id) this.pendingApproval = null;
	}

	/**
	 * Stops the run on the agent server first: aborting the fetch only closes
	 * this tab's copy of the stream.
	 */
	async stop() {
		if (!this.streaming) return;
		// Close this tab's stream first. The server request is best-effort, and a
		// slow control plane must not make the Stop button feel dead.
		this.controller?.abort();
		this.controller = null;
		this.endRun();
		this.finishReason = 'stopped';
		this.streaming = false;
		this.finishedAt = Date.now();
		try {
			await chatApi.stop(this.agent, this.requestId);
		} catch {
			// The run may already be gone; the abort above still detached this tab.
		}
	}
}

type ChatEvent = { event: string; data: unknown };

const runs = $state<ChatRun[]>([]);
// Agents whose in-flight runs were already recovered after a reload.
const restored = new Set<string>();

export const chatRuns = {
	/** Runs for every conversation, newest last. */
	all(): ChatRun[] {
		return runs;
	},
	/**
	 * Picks up the runs the agent server still holds. Call it when the chat page
	 * mounts: after a reload, a closed tab, or a fresh login the browser has no
	 * streams, but the answers are still running and their buffers are replayable.
	 */
	async restore(agentName: string) {
		if (!agentName || restored.has(agentName)) return;
		restored.add(agentName);

		let infos: ChatRunInfo[] = [];
		try {
			infos = (await chatApi.runs(agentName)).runs ?? [];
		} catch {
			// Allow a later attempt instead of giving up for the whole session.
			restored.delete(agentName);
			return;
		}

		for (const info of infos) {
			if (!info.streaming || !info.request_id) continue;
			if (runs.some((run) => run.requestId === info.request_id)) continue;

			const run = new ChatRun(agentName, info.request_id, true);
			run.conversationId = info.conversation_id ?? '';
			const startedAt = Date.parse(info.started_at ?? '');
			if (Number.isFinite(startedAt)) run.startedAt = startedAt;
			run.discard = () => {
				const index = runs.indexOf(run);
				if (index >= 0) runs.splice(index, 1);
			};
			runs.push(run);
			void run.resume();
		}
	},
	/**
	 * The run of the open conversation. A run stays unbound (empty id) only
	 * between sending in a new chat and the server announcing its id.
	 */
	forConversation(agent: string, conversationId: string): ChatRun | undefined {
		const matches = runs.filter(
			(run) => run.agent === agent && run.conversationId === conversationId
		);
		// A live run wins over the transcript of the previous one.
		return matches.find((run) => run.streaming) ?? matches[matches.length - 1];
	},
	start(options: StartRunOptions): ChatRun {
		// A finished run of the same conversation would shadow the new one.
		for (let index = runs.length - 1; index >= 0; index--) {
			const run = runs[index];
			if (run.agent === options.agent && run.conversationId === options.conversationId) {
				runs.splice(index, 1);
			}
		}

		const run = new ChatRun(options.agent, crypto.randomUUID());
		runs.push(run);
		run.start(options);
		return run;
	},
	/**
	 * Drops a finished run once its transcript has been loaded from the server,
	 * so the stored answer takes over and memory does not grow forever.
	 */
	settle(agent: string, conversationId: string) {
		for (let index = runs.length - 1; index >= 0; index--) {
			const run = runs[index];
			if (run.agent === agent && run.conversationId === conversationId && !run.streaming) {
				runs.splice(index, 1);
			}
		}
	},
	/** Removes every finished run of an agent (switching agent or starting over). */
	settleAgent(agent: string) {
		for (let index = runs.length - 1; index >= 0; index--) {
			const run = runs[index];
			if (run.agent === agent && !run.streaming) runs.splice(index, 1);
		}
	}
};
