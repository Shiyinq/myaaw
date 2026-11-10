import type { Snippet } from 'svelte';

/**
 * One page can own the right side of the top bar. The slot receives a snippet,
 * so a page can render any control there (a button, a search box, or a group)
 * while keeping its own state and handlers.
 */
export interface TopBarSlot {
	id: string;
	content: Snippet;
}

let slot = $state<TopBarSlot | null>(null);

export const topBarSlot = {
	get current() {
		return slot;
	},
	set(next: TopBarSlot) {
		slot = next;
	},
	clear(id?: string) {
		if (!id || slot?.id === id) {
			slot = null;
		}
	}
};
