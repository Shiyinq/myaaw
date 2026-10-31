let title = $state('');

/**
 * The open conversation's title, shown by the shell instead of the static page
 * label. The chat page owns it: it sets the title when a conversation loads and
 * clears it when the chat is new.
 */
export const topBarTitle = {
	get value() {
		return title;
	},
	set(next: string) {
		title = next;
	},
	clear() {
		title = '';
	}
};
