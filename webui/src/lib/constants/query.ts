/**
 * Cache freshness windows for `createQueryStore`.
 *
 * The numbers live in one place so they can be tuned without hunting through
 * every store, while each resource keeps its own window (a daemon status goes
 * stale faster than the embedded capability catalog).
 *
 * A store that omits `staleTime` falls back to `DEFAULT_QUERY_STALE_TIME`.
 */

/** Fallback for list resources with no specific window. */
export const DEFAULT_QUERY_STALE_TIME = 30_000;

/** Background refetch is on unless a store opts out. */
export const DEFAULT_QUERY_SWR = true;

/** The embedded capability catalog changes only with the binary. */
export const CAPABILITIES_STALE_TIME = 5 * 60_000;

/** Agent config is form-like; revalidate soon after an edit. */
export const CONFIG_STALE_TIME = 15_000;

/** Aggregated observability numbers; one page view is enough. */
export const OBSERVABILITY_STALE_TIME = 20_000;

/** The agent daemon can start/bind a port, so poll it relatively often. */
export const SERVER_STATUS_STALE_TIME = 10_000;

/** A project build changes state while the drawer polls it. */
export const PROJECT_STATUS_STALE_TIME = 10_000;
