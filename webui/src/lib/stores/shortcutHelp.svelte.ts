let open = $state(false);

/** One dialog instance in the shell, opened from the `?` key or any button. */
export const shortcutHelp = {
	get open() {
		return open;
	},
	set(value: boolean) {
		open = value;
	},
	show() {
		open = true;
	},
	hide() {
		open = false;
	}
};
