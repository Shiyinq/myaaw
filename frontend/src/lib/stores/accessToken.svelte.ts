import { browser } from '$app/environment';
import { ACCESS_TOKEN_KEY, TOKEN_EXPIRES_KEY } from '$lib/constants';
import { isExpired } from '$lib/utils/auth';

let token = $state(browser ? (localStorage.getItem(ACCESS_TOKEN_KEY) ?? '') : '');
let expiresAt = $state(browser ? (localStorage.getItem(TOKEN_EXPIRES_KEY) ?? '') : '');

export const accessToken = {
	get value() {
		return token;
	},
	get expiresAt() {
		return expiresAt;
	},
	get isExpired() {
		return isExpired(expiresAt);
	},
	set: (nextToken: string, nextExpiresAt = '') => {
		token = nextToken;
		expiresAt = nextExpiresAt;
		if (browser) {
			if (nextToken) {
				localStorage.setItem(ACCESS_TOKEN_KEY, nextToken);
				localStorage.setItem(TOKEN_EXPIRES_KEY, nextExpiresAt);
			} else {
				localStorage.removeItem(ACCESS_TOKEN_KEY);
				localStorage.removeItem(TOKEN_EXPIRES_KEY);
			}
		}
	},
	clear: () => {
		accessToken.set('', '');
	}
};
