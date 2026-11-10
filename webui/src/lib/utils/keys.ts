/**
 * Keyboard shortcuts shared by the shell and the pages.
 *
 * A combo is written as `mod+shift+<key>`, where `mod` is Cmd on macOS and Ctrl
 * elsewhere. Keys are matched by name (`enter`, `space`, `escape`, `backslash`,
 * `arrowup`, `arrowdown`), by single letter, or as `?` for the help key.
 */

export type Platform = 'mac' | 'other';

export const platform: Platform =
	typeof navigator !== 'undefined' &&
	/mac|iphone|ipad|ipod/i.test(navigator.platform || navigator.userAgent)
		? 'mac'
		: 'other';

export const isMac = platform === 'mac';

const MOD_LABELS = { mod: 'Ctrl', shift: 'Shift', alt: 'Alt' } as Record<string, string>;
const MAC_SYMBOLS: Record<string, string> = {
	mod: '⌘',
	shift: '⇧',
	alt: '⌥',
	ctrl: '⌃'
};
const KEY_LABELS: Record<string, string> = {
	enter: 'Enter',
	escape: 'Esc',
	space: 'Space',
	backslash: '\\',
	arrowup: '↑',
	arrowdown: '↓',
	'?': '?'
};

/** True while the keystroke belongs to a text field, where most shortcuts must not fire. */
export function isTypingTarget(target: EventTarget | null): boolean {
	const element = target as HTMLElement | null;
	if (!element) return false;
	const tag = element.tagName;
	return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || element.isContentEditable;
}

/** True while a dialog, dropdown or popover is on screen, so Esc closes it first. */
export function isOverlayOpen(): boolean {
	if (typeof document === 'undefined') return false;
	return (
		document.querySelector('[role="dialog"][data-state="open"]') !== null ||
		document.querySelector('[data-slot="dropdown-menu-content"][data-state="open"]') !== null ||
		document.querySelector('[data-slot="popover-content"][data-state="open"]') !== null
	);
}

function keyMatches(event: KeyboardEvent, token: string): boolean {
	switch (token) {
		case 'enter':
			return event.key === 'Enter';
		case 'space':
			return event.key === ' ' || event.code === 'Space';
		case 'escape':
			return event.key === 'Escape';
		case 'backslash':
			// Lettered keys go through `code`: Option+letter on macOS changes `key`.
			return event.code === 'Backslash' || event.key === '\\';
		case 'arrowup':
			return event.key === 'ArrowUp';
		case 'arrowdown':
			return event.key === 'ArrowDown';
		case '?':
			return event.key === '?';
		case '/':
			return event.key === '/';
		default:
			if (/^[a-z]$/.test(token)) {
				return event.code === `Key${token.toUpperCase()}` || event.key.toLowerCase() === token;
			}
			return event.key.toLowerCase() === token;
	}
}

/** Whether the event is exactly the combo, modifiers included. */
export function matchesShortcut(event: KeyboardEvent, combo: string): boolean {
	if (event.repeat) return false;
	const tokens = combo.toLowerCase().split('+');
	const key = tokens.pop() ?? '';
	const wantsShift = tokens.includes('shift');
	const wantsAlt = tokens.includes('alt');
	const wantsMod = tokens.includes('mod');
	// `?` and `/` already need Shift on most layouts, so Shift is not compared.
	if (key !== '?' && key !== '/' && event.shiftKey !== wantsShift) return false;
	if (event.altKey !== wantsAlt) return false;
	if (wantsMod) {
		const primary = isMac ? event.metaKey : event.ctrlKey;
		const secondary = isMac ? event.ctrlKey : event.metaKey;
		if (!primary || secondary) return false;
	} else if (event.metaKey || event.ctrlKey) {
		return false;
	}
	return keyMatches(event, key);
}

/** Human label for a combo; `target` previews how the other platform shows it. */
export function formatShortcut(combo: string, target: Platform = platform): string {
	const tokens = combo.toLowerCase().split('+');
	const key = tokens.pop() ?? '';
	const mac = target === 'mac';
	const parts = tokens.map((token) =>
		mac ? (MAC_SYMBOLS[token] ?? token) : (MOD_LABELS[token] ?? token)
	);
	parts.push(KEY_LABELS[key] ?? key.toUpperCase());
	return parts.join(mac ? '' : '+');
}

/** The same combo as the other platform writes it, when that differs. */
export function otherPlatformShortcut(combo: string): string | null {
	const other = formatShortcut(combo, isMac ? 'other' : 'mac');
	return other === formatShortcut(combo) ? null : other;
}
