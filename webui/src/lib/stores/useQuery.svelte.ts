import { untrack } from 'svelte';
import type { QueryStore } from './query.svelte';

/**
 * Wires a query store to a component's lifecycle.
 *
 * Call it once, at the top level of `<script>`. The effect re-runs when the
 * reactive args change (typically the selected agent); `untrack` keeps the
 * store's own state reads from re-triggering the effect, so a background refresh
 * never loops.
 *
 * The store serves its cached value immediately, so a remount shows the previous
 * data and refreshes it in the background instead of flashing a skeleton.
 */
export function useQuery<TArgs, TData>(
	store: QueryStore<TArgs, TData>,
	args: () => TArgs | null | undefined
): void {
	$effect(() => {
		const value = args();
		if (value === null || value === undefined) return;
		untrack(() => {
			void store.ensure(value);
		});
	});
}
