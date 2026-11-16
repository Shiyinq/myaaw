import { matchesShortcut } from './keys';

export interface SearchNavItem {
	id: string;
	activate: () => void;
}

export interface SearchNavOptions {
	items: SearchNavItem[];
	cursor: string;
	setCursor: (id: string) => void;
	input: HTMLInputElement | null;
	list: HTMLElement | null;
	query: string;
	clear: () => void;
}

/** True while the filter is really on screen; a collapsed sidebar is not. */
function inputOnScreen(input: HTMLInputElement | null): boolean {
	if (!input) return false;
	const rect = input.getBoundingClientRect();
	return rect.width > 0 && rect.right > 0 && rect.left < window.innerWidth;
}

/**
 * Keyboard behaviour shared by the workspace sidebars.
 *
 * `Cmd/Ctrl+Shift+S` focuses the filter, the arrow keys walk the visible rows,
 * `Enter` opens the highlighted row (or the first match when nothing is
 * highlighted yet), and `Escape` clears the filter. Rows opt in with
 * `data-nav-id`; the caller renders the cursor ring from the same id.
 *
 * Returns true when the event was consumed.
 */
export function handleSearchNav(event: KeyboardEvent, options: SearchNavOptions): boolean {
	if (event.defaultPrevented || event.isComposing) return false;

	if (matchesShortcut(event, 'mod+shift+s')) {
		event.preventDefault();
		options.input?.focus();
		options.input?.select();
		return true;
	}

	if (event.altKey || event.metaKey || event.ctrlKey) return false;

	const target = event.target as HTMLElement | null;
	const inBox = target === options.input;
	// Opening a row leaves focus on the page, so the page itself counts too.
	const onPage = !target || target === document.body || target === document.documentElement;
	const inList = !!target && !!options.list?.contains(target);
	if (!inBox && !inList && !onPage) return false;
	// A page-level key must not steal the arrows when the sidebar is off screen.
	if (!inBox && !inputOnScreen(options.input)) return false;

	const items = options.items;
	const index = items.findIndex((item) => item.id === options.cursor);

	if (event.key === 'ArrowDown') {
		event.preventDefault();
		if (items.length === 0) return true;
		const next = index < 0 ? 0 : Math.min(items.length - 1, index + 1);
		options.setCursor(items[next].id);
		return true;
	}

	if (event.key === 'ArrowUp') {
		event.preventDefault();
		if (index <= 0) {
			options.setCursor('');
			options.input?.focus();
			return true;
		}
		options.setCursor(items[index - 1].id);
		return true;
	}

	if (event.key === 'Enter') {
		const entry =
			items.find((item) => item.id === options.cursor) ??
			(options.query.trim() ? items[0] : undefined);
		if (!entry) return false;
		event.preventDefault();
		entry.activate();
		return true;
	}

	if (event.key === 'Escape' && inBox && options.query) {
		event.preventDefault();
		options.clear();
		options.setCursor('');
		options.input?.focus();
		return true;
	}

	return false;
}

export interface SearchNavSource {
	items: () => SearchNavItem[];
	query: () => string;
	clear: () => void;
}

/**
 * The per-page half of the sidebar keyboard navigation. Bind the filter ref and
 * the scroll container, render `cursor` as the row ring, and call `onKeydown`
 * from `<svelte:window>`.
 */
export function createSearchNav(source: SearchNavSource) {
	let input = $state<HTMLInputElement | null>(null);
	let list = $state<HTMLElement | null>(null);
	let cursor = $state('');

	$effect(() => {
		const items = source.items();
		if (cursor && !items.some((item) => item.id === cursor)) cursor = '';
	});

	$effect(() => {
		if (!cursor || !list) return;
		const row = [...list.querySelectorAll<HTMLElement>('[data-nav-id]')].find(
			(element) => element.dataset.navId === cursor
		);
		row?.scrollIntoView({ block: 'nearest' });
	});

	function onKeydown(event: KeyboardEvent) {
		handleSearchNav(event, {
			items: source.items(),
			cursor,
			setCursor: (id) => (cursor = id),
			input,
			list,
			query: source.query(),
			clear: source.clear
		});
	}

	return {
		get input() {
			return input;
		},
		set input(value: HTMLInputElement | null) {
			input = value;
		},
		get list() {
			return list;
		},
		set list(value: HTMLElement | null) {
			list = value;
		},
		get cursor() {
			return cursor;
		},
		onKeydown
	};
}
