import { auth as authApi } from '$lib/apis/auth';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { isAuthenticated } from '$lib/stores/authStatus.svelte';
import { getErrorMessage } from '$lib/utils/api';
import { logger } from '$lib/utils/logger';
import type {
	ChangePasswordRequest,
	ChangeUsernameRequest,
	LoginRequest,
	MeResponse
} from '$lib/types';

let isLoading = $state(false);
let error = $state('');
let profile = $state<MeResponse | null>(null);

export const authStore = {
	get isLoading() {
		return isLoading;
	},
	get error() {
		return error;
	},
	get profile() {
		return profile;
	},

	async login(credentials: LoginRequest) {
		isLoading = true;
		error = '';
		try {
			const response = await authApi.login(credentials);
			accessToken.set(response.token, response.expires_at);
			isAuthenticated.set(true);
			await authStore.loadProfile();
			return response;
		} catch (err) {
			error = getErrorMessage(err);
			logger.error('Login failed', err, { context: 'authStore' });
			throw err;
		} finally {
			isLoading = false;
		}
	},

	async logout() {
		try {
			await authApi.logout();
		} catch (err) {
			logger.warn('Logout request failed', err, { context: 'authStore' });
		} finally {
			accessToken.clear();
			isAuthenticated.set(false);
			profile = null;
		}
	},

	async loadProfile() {
		try {
			profile = await authApi.me();
			return profile;
		} catch (err) {
			logger.error('Failed to load profile', err, { context: 'authStore' });
			throw err;
		}
	},

	async changePassword(payload: ChangePasswordRequest) {
		await authApi.changePassword(payload);
		accessToken.clear();
		isAuthenticated.set(false);
		profile = null;
	},

	async changeUsername(payload: ChangeUsernameRequest) {
		profile = await authApi.changeUsername(payload);
		return profile;
	},

	clearError() {
		error = '';
	}
};
