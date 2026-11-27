import { agentSelection, toastStore } from '$lib/stores';
import { agentsStore } from '$lib/stores/agents.svelte';
import { askConfirm } from '$lib/stores/confirm.svelte';
import { serverStore } from '$lib/stores/server.svelte';

export type FleetBulkKind = 'start' | 'restart' | 'stop';

type TranslateFn = (key: string, params?: Record<string, string | number>) => string;

/** Polls until every named agent reports the expected running state, then gives up. */
async function waitForStates(names: string[], running: boolean) {
	const pending = new Set(names);
	for (let attempt = 0; attempt < 20 && pending.size > 0; attempt += 1) {
		await new Promise((resolve) => setTimeout(resolve, 300));
		try {
			const list = await agentsStore.reload();
			for (const agent of list) {
				if (pending.has(agent.name) && agent.running === running) pending.delete(agent.name);
			}
		} catch {
			// Transient failures are expected while daemons boot; keep polling.
		}
	}
}

class FleetBulkStore {
	// Which fleet-wide action is running, if any. One at a time, so daemons never
	// race for the same port.
	busy = $state<'' | FleetBulkKind>('');

	/**
	 * Starts what is stopped, or restarts/stops what is running, for the whole fleet.
	 * Every badge flips before the first request goes out, so labels answer the click
	 * immediately instead of waiting for every daemon to finish booting.
	 */
	async run(kind: FleetBulkKind, t: TranslateFn) {
		if (this.busy) return;
		const targets = agentSelection.agents.filter((agent) =>
			kind === 'start' ? !agent.running : agent.running
		);
		if (targets.length === 0) return;
		if (
			kind === 'stop' &&
			!(await askConfirm({
				message: t('myaaw.home.stop_all_confirm', { count: targets.length }),
				variant: 'destructive'
			}))
		) {
			return;
		}

		this.busy = kind;
		const running = kind !== 'stop';
		for (const agent of targets) agentSelection.setRunning(agent.name, running);

		const failed: string[] = [];
		for (const agent of targets) {
			try {
				if (kind === 'start') await serverStore.start(agent.name);
				else if (kind === 'restart') await serverStore.restart(agent.name);
				else await serverStore.stop(agent.name);
			} catch {
				failed.push(agent.name);
			}
		}

		await waitForStates(
			targets.filter((agent) => !failed.includes(agent.name)).map((agent) => agent.name),
			running
		);
		if (failed.length > 0) await agentSelection.load(true);

		this.busy = '';
		if (failed.length > 0) {
			toastStore.error(t('myaaw.server.bulk_failed', { names: failed.join(', ') }));
		}
		const done = targets.length - failed.length;
		if (done > 0) {
			const key =
				kind === 'start' ? 'bulk_started' : kind === 'restart' ? 'bulk_restarted' : 'bulk_stopped';
			toastStore.success(t(`myaaw.server.${key}`, { count: done }));
		}
	}
}

export const fleetBulk = new FleetBulkStore();
