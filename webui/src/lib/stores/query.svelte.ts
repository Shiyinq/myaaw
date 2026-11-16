import { DEFAULT_QUERY_STALE_TIME, DEFAULT_QUERY_SWR } from '$lib/constants/query';
import { getErrorMessage } from '$lib/utils/api';

/**
 * A tiny stale-while-revalidate query store.
 *
 * Every UI read goes through one of these instead of calling `lib/apis/*` from a
 * component. The store owns the cached value, serves it immediately on a second
 * mount, and refreshes it in the background without ever blanking the screen:
 *
 * - `loading` is true only for the very first load (no cached value yet);
 * - `refreshing` is true while a stale value is being replaced in the background;
 * - `data` keeps the previous value until the new one arrives.
 *
 * The cache lives in the module, so navigating away and back reuses it.
 */

export type QueryStatus = 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';

export interface QueryState<T> {
	data: T | undefined;
	error: string;
	status: QueryStatus;
	updatedAt: number;
}

export interface QueryOptions<TArgs, TData> {
	/** Stable cache key; prefix it with the agent so agents never mix. */
	key: (args: TArgs) => string;
	fetcher: (args: TArgs) => Promise<TData>;
	/** Serve cached data and refetch in the background. Default: true. */
	swr?: boolean;
	/** How long a cached value stays fresh, in ms. Default: 30_000. */
	staleTime?: number;
}

export interface QueryStore<TArgs, TData> {
	readonly data: TData | undefined;
	readonly error: string;
	readonly status: QueryStatus;
	/** First load only: no cached value yet. */
	readonly loading: boolean;
	/** Background refetch while a cached value is still on screen. */
	readonly refreshing: boolean;
	readonly updatedAt: number;
	readonly args: TArgs | null;
	/** Cached value for a key without making it the active one. */
	peek(args: TArgs): TData | undefined;
	/** Serve the cache immediately and refetch in the background when stale. */
	ensure(args: TArgs): Promise<void>;
	/** Force a background refetch for `args` (or the active args). */
	refresh(args?: TArgs): Promise<void>;
	/** Mark one entry (or every entry) stale without clearing its data. */
	invalidate(args?: TArgs): void;
	/** Write a fresh value for `args`, e.g. from a mutation response. */
	write(args: TArgs, data: TData): void;
	/** Patch the active entry in place. */
	setData(updater: (previous: TData | undefined) => TData | undefined): void;
	/** Drop the cache for one entry (or everything). */
	reset(args?: TArgs): void;
}

const EMPTY: QueryState<unknown> = Object.freeze({
	data: undefined,
	error: '',
	status: 'idle',
	updatedAt: 0
});

export function createQueryStore<TArgs, TData>(
	options: QueryOptions<TArgs, TData>
): QueryStore<TArgs, TData> {
	const swr = options.swr ?? DEFAULT_QUERY_SWR;
	const staleTime = options.staleTime ?? DEFAULT_QUERY_STALE_TIME;
	const inflight = new Map<string, Promise<void>>();

	let cache = $state<Record<string, QueryState<TData>>>({});
	let activeKey = $state('');
	let activeArgs: TArgs | null = null;

	function read(key: string): QueryState<TData> {
		return (cache[key] ?? EMPTY) as QueryState<TData>;
	}

	function patch(key: string, next: Partial<QueryState<TData>>) {
		const previous = read(key);
		cache = { ...cache, [key]: { ...previous, ...next } };
	}

	async function load(key: string, args: TArgs): Promise<void> {
		const running = inflight.get(key);
		if (running) return running;

		const previous = read(key);
		const hasData = previous.data !== undefined;
		// Keep the old value on screen; only the status changes.
		patch(key, { error: '', status: hasData ? 'refreshing' : 'loading' });

		const job = (async () => {
			try {
				const data = await options.fetcher(args);
				patch(key, { data, error: '', status: 'ready', updatedAt: Date.now() });
			} catch (error) {
				patch(key, {
					error: getErrorMessage(error),
					status: hasData ? 'ready' : 'error'
				});
			} finally {
				inflight.delete(key);
			}
		})();

		inflight.set(key, job);
		return job;
	}

	return {
		get data() {
			return read(activeKey).data;
		},
		get error() {
			return read(activeKey).error;
		},
		get status() {
			return read(activeKey).status;
		},
		get loading() {
			return read(activeKey).status === 'loading';
		},
		get refreshing() {
			return read(activeKey).status === 'refreshing';
		},
		get updatedAt() {
			return read(activeKey).updatedAt;
		},
		get args() {
			return activeArgs;
		},
		peek(args: TArgs) {
			return read(options.key(args)).data;
		},
		async ensure(args: TArgs) {
			const key = options.key(args);
			activeKey = key;
			activeArgs = args;
			const current = read(key);
			if (current.data !== undefined) {
				const fresh = Date.now() - current.updatedAt < staleTime;
				if (fresh || !swr) return;
			}
			await load(key, args);
		},
		async refresh(args?: TArgs) {
			const target = args ?? activeArgs;
			if (target === null) return;
			const key = options.key(target);
			activeKey = key;
			activeArgs = target;
			await load(key, target);
		},
		invalidate(args?: TArgs) {
			if (args !== undefined) {
				patch(options.key(args), { updatedAt: 0 });
				return;
			}
			const next: Record<string, QueryState<TData>> = {};
			for (const [key, entry] of Object.entries(cache)) next[key] = { ...entry, updatedAt: 0 };
			cache = next;
		},
		write(args: TArgs, data: TData) {
			const key = options.key(args);
			if (key === activeKey) activeArgs = args;
			patch(key, { data, error: '', status: 'ready', updatedAt: Date.now() });
		},
		setData(updater: (previous: TData | undefined) => TData | undefined) {
			if (!activeKey) return;
			patch(activeKey, {
				data: updater(read(activeKey).data),
				error: '',
				status: 'ready',
				updatedAt: Date.now()
			});
		},
		reset(args?: TArgs) {
			if (args !== undefined) {
				const key = options.key(args);
				const next = { ...cache };
				delete next[key];
				cache = next;
				if (activeKey === key) activeKey = '';
				return;
			}
			cache = {};
			activeKey = '';
			activeArgs = null;
		}
	};
}
