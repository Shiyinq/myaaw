let title = $state('');
let description = $state('');

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
	set(next: string, subtitle = '') {
		title = next;
		description = subtitle;
	},
	clear() {
		title = '';
		description = '';
	}
};
