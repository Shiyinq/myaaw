import { browser } from '$app/environment';
import { locale, type Locale } from '$lib/i18n';
import { applyRemoteAppearance } from '$lib/theme';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { botsSidebarStore } from '$lib/stores/botsSidebar.svelte';
import { preferences } from '$lib/stores/preferences.svelte';
import {
	cancelPendingSettingsSync,
	fetchRemoteSettings,
	withRemoteSettingsSuppression
} from '$lib/stores/settingsSync.svelte';
import { logger } from '$lib/utils/logger';

let loaded = $state(false);
let inFlight: Promise<void> | null = null;
let activeRequest = 0;

export const settingsStore = {
	get loaded() {
		return loaded;
	},

	/**
	 * Fetches dashboard settings from ~/.myaaw/controlplane/controlplane.db once per
	 * login/session and hydrates local stores (preferences, theme, mode, locale).
	 */
	async loadOnce(force = false): Promise<void> {
		if (!browser || !accessToken.value) return;
		if (!force && loaded) return;
		if (inFlight && !force) return inFlight;

		const reqId = ++activeRequest;
		const task = (async () => {
			try {
				const remote = await fetchRemoteSettings();
				if (activeRequest !== reqId) return;
				withRemoteSettingsSuppression(() => {
					preferences.hydrate(remote);
					botsSidebarStore.hydrate(remote.bots_sidebar);
					if (remote.locale === 'id' || remote.locale === 'en') {
						locale.value = remote.locale as Locale;
					}
					applyRemoteAppearance(remote.theme_mode, remote.theme);
				});
				loaded = true;
			} catch (err) {
				logger.warn('Failed to load settings from control plane', err, {
					context: 'settingsStore'
				});
			} finally {
				if (activeRequest === reqId) inFlight = null;
			}
		})();

		inFlight = task;
		return task;
	},

	/** Resets sync state on logout so the next login loads fresh settings. */
	reset(): void {
		activeRequest += 1;
		loaded = false;
		inFlight = null;
		cancelPendingSettingsSync();
	}
};
