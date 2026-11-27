export interface OpenCreateAgentOptions {
	/** Optional runtime preset to pre-select (e.g. 'codex', 'antigravity', 'claude_code'). */
	runtime?: string;
	/** Existing agent name when reconfiguring instead of creating a new agent. */
	agent?: string;
	/** Called after the agent is created or updated successfully. */
	onSuccess?: (targetName: string, created: boolean) => void;
}

let open = $state(false);
let runtimePreset = $state('');
let agentName = $state('');
let onSuccessCallback: ((targetName: string, created: boolean) => void) | null = null;

export const createAgentDialog = {
	get open(): boolean {
		return open;
	},

	get runtimePreset(): string {
		return runtimePreset;
	},

	get agentName(): string {
		return agentName;
	},

	openCreate(options: Omit<OpenCreateAgentOptions, 'agent'> = {}): void {
		agentName = '';
		runtimePreset = (options.runtime ?? '').trim();
		onSuccessCallback = options.onSuccess ?? null;
		open = true;
	},

	openReconfigure(
		agent: string,
		options: Omit<OpenCreateAgentOptions, 'agent' | 'runtime'> = {}
	): void {
		agentName = agent.trim();
		runtimePreset = '';
		onSuccessCallback = options.onSuccess ?? null;
		open = true;
	},

	setOpen(next: boolean): void {
		open = next;
		if (!next) {
			onSuccessCallback = null;
		}
	},

	close(): void {
		open = false;
		onSuccessCallback = null;
	},

	notifySuccess(targetName: string, created: boolean): void {
		const cb = onSuccessCallback;
		open = false;
		onSuccessCallback = null;
		cb?.(targetName, created);
	}
};
