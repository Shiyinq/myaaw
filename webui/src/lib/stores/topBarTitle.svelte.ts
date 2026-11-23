let title = $state('');
let description = $state('');
let desktopHidden = $state(false);
let chatKey = $state('');

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
	chatTitle(key: string): string | undefined {
		return chatKey === key ? title : undefined;
	},
	setChat(key: string, next: string) {
		this.set(next);
		chatKey = key;
	},
	set(next: string, subtitle = '') {
		chatKey = '';
		title = next;
		description = subtitle;
	},
	setDesktopHidden(hidden: boolean) {
		desktopHidden = hidden;
	},
	clear() {
		this.set('');
	}
};
