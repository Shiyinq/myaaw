let title = $state('');
let description = $state('');
let desktopHidden = $state(false);

/**
 * The title the shell shows instead of the static page label. The chat page
 * owns it while a conversation is open, and the agent form uses it when it is
 * editing an existing agent; both clear it on the way out.
 */
export const topBarTitle = {
	get value() {
		return title;
	},
	/** Optional second line; empty keeps the page's own description. */
	get description() {
		return description;
	},
	/** Hides the floating desktop title when a full-height preview owns the top edge. */
	get desktopHidden() {
		return desktopHidden;
	},
	set(next: string, subtitle = '') {
		title = next;
		description = subtitle;
	},
	setDesktopHidden(hidden: boolean) {
		desktopHidden = hidden;
	},
	clear() {
		title = '';
		description = '';
	}
};
