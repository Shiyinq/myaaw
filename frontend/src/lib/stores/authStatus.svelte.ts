import { browser } from '$app/environment';
import { AUTH_STORAGE_KEY } from '$lib/constants';

let authenticated = $state(browser ? localStorage.getItem(AUTH_STORAGE_KEY) === 'true' : false);

export const isAuthenticated = {
	get value() {
		return authenticated;
	},
	set: (value: boolean) => {
		authenticated = value;
		if (browser) {
			if (value) {
				localStorage.setItem(AUTH_STORAGE_KEY, 'true');
			} else {
				localStorage.removeItem(AUTH_STORAGE_KEY);
			}
		}
	}
};
