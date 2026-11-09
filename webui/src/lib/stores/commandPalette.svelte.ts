let open = $state(false);

/** One palette instance in the shell, opened by Mod+K or by any button. */
export const commandPalette = {
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
	},
	toggle() {
		open = !open;
	}
};
