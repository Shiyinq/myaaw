import { OBSERVABILITY_STALE_TIME } from '$lib/constants/query';
import { observability as observabilityApi, type RunFilters } from '$lib/apis/observability';
import type {
	ObservabilitySummary,
	RunDetail,
	RunList,
	SessionDetail,
	SessionList,
	SessionStats
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const summary = createQueryStore<{ agent: string; days: number }, ObservabilitySummary>({
	key: (args) => `obs:summary:${args.agent}:${args.days}`,
	fetcher: (args) => observabilityApi.summary(args.agent, args.days),
	staleTime: OBSERVABILITY_STALE_TIME
});

const runs = createQueryStore<{ agent: string; filters: RunFilters }, RunList>({
	key: (args) => `obs:runs:${args.agent}:${JSON.stringify(args.filters)}`,
	fetcher: (args) => observabilityApi.runs(args.agent, args.filters)
});

const run = createQueryStore<{ agent: string; id: string }, RunDetail>({
	key: (args) => `obs:run:${args.agent}:${args.id}`,
	fetcher: (args) => observabilityApi.run(args.agent, args.id)
});

const sessions = createQueryStore<{ agent: string; limit: number }, SessionList>({
	key: (args) => `obs:sessions:${args.agent}:${args.limit}`,
	fetcher: (args) => observabilityApi.sessions(args.agent, args.limit)
});

const session = createQueryStore<{ agent: string; id: string; spans: boolean }, SessionDetail>({
	key: (args) => `obs:session:${args.agent}:${args.id}:${args.spans ? 'spans' : 'plain'}`,
	fetcher: (args) => observabilityApi.session(args.agent, args.id, args.spans)
});

const sessionStats = createQueryStore<{ agent: string; id: string }, SessionStats>({
	key: (args) => `obs:sessionStats:${args.agent}:${args.id}`,
	fetcher: (args) => observabilityApi.sessionStats(args.agent, args.id)
});

export const observabilityStore = {
	summary,
	runs,
	run,
	sessions,
	session,
	sessionStats,
	get data() {
		return summary.data;
	},
	get loading() {
		return summary.loading;
	},
	get refreshing() {
		return summary.refreshing;
	},
	get error() {
		return summary.error;
	},
	get status() {
		return summary.status;
	},
	ensureSummary(agent: string, days = 7) {
		return summary.ensure({ agent, days });
	},
	ensureRuns(agent: string, filters: RunFilters = {}) {
		return runs.ensure({ agent, filters });
	},
	ensureRun(agent: string, id: string) {
		return run.ensure({ agent, id });
	},
	ensureSessions(agent: string, limit = 50) {
		return sessions.ensure({ agent, limit });
	},
	ensureSession(agent: string, id: string, spans = true) {
		return session.ensure({ agent, id, spans });
	},
	ensureSessionStats(agent: string, id: string) {
		return sessionStats.ensure({ agent, id });
	},
	fetchSessionStats(agent: string, id: string) {
		return observabilityApi.sessionStats(agent, id);
	},
	/** One-shot fetch for the two highlight runs, which must coexist. */
	fetchRun(agent: string, id: string) {
		return observabilityApi.run(agent, id);
	},
	async purge(agent: string) {
		await observabilityApi.purge(agent);
		summary.invalidate();
		runs.invalidate();
		sessions.invalidate();
		run.reset();
		session.reset();
		sessionStats.reset();
		await summary.refresh({ agent, days: 7 });
	}
};
