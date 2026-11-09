import type { LucideIcon } from '$lib/types/icons';

/** One destination of the advanced shell's sidebar. */
export interface NavItem {
	href: string;
	label: string;
	description: string;
	icon: LucideIcon;
}

/** A titled group of destinations, rendered as a labelled section. */
export interface NavGroup {
	label: string;
	items: NavItem[];
}

/** True when `pathname` is the page itself or one of its sub-pages. */
export function isActiveHref(pathname: string, href: string): boolean {
	// The fleet overview is the index of /app, so only that exact path belongs to it.
	if (href === '/app') return pathname === '/app';
	return pathname === href || pathname.startsWith(href + '/');
}
