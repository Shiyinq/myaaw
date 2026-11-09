import { browser } from '$app/environment';

const STORAGE_KEY = 'myaaw_sidebar_collapsed';

let collapsed = $state(browser ? localStorage.getItem(STORAGE_KEY) === 'true' : false);

export const sidebar = {
	get collapsed() {
		return collapsed;
	},
	set(value: boolean) {
		collapsed = value;
		if (browser) localStorage.setItem(STORAGE_KEY, String(value));
	},
	toggle() {
		sidebar.set(!collapsed);
	}
};
