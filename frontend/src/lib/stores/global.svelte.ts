/**
 * Global application state using Svelte 5 runes.
 */

let initialDataLoaded = $state(false);

export const isInitialDataLoaded = {
	get value() {
		return initialDataLoaded;
	},
	set value(v: boolean) {
		initialDataLoaded = v;
	}
};
