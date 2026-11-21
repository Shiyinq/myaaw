import { browser } from '$app/environment';
import { settings as settingsApi } from '$lib/apis/settings';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { isAuthenticated } from '$lib/stores/authStatus.svelte';
import { logger } from '$lib/utils/logger';
import type { ControlPlaneSettings, ControlPlaneSettingsPatch } from '$lib/types';

const SYNC_DEBOUNCE_MS = 200;

let applyingRemote = 0;
let pendingPatch: ControlPlaneSettingsPatch | null = null;
let flushTimer: ReturnType<typeof setTimeout> | null = null;
let flushing = false;

/**
 * Runs a local hydration callback without echoing those changes back to the
 * control plane via PATCH /settings.
 */
export function withRemoteSettingsSuppression(fn: () => void): void {
	applyingRemote += 1;
	try {
		fn();
	} finally {
		applyingRemote -= 1;
	}
}

/**
 * Queues a partial settings update to be persisted in ~/.myaaw/controlplane/controlplane.db.
 * Rapid changes (such as dragging range sliders) are coalesced into a single PATCH,
 * and requests are serialized so earlier patches never overwrite newer ones.
 */
export function queueSettingsPatch(patch: ControlPlaneSettingsPatch): void {
	if (!browser || applyingRemote > 0) return;
	if (!isAuthenticated.value || !accessToken.value) return;
	if (Object.keys(patch).length === 0) return;

	pendingPatch = { ...pendingPatch, ...patch };
	if (flushTimer !== null) clearTimeout(flushTimer);
	flushTimer = setTimeout(() => {
		flushTimer = null;
		void flushPendingSettings();
	}, SYNC_DEBOUNCE_MS);
}

async function flushPendingSettings(): Promise<void> {
	if (flushing || !pendingPatch) return;
	if (!isAuthenticated.value || !accessToken.value) {
		pendingPatch = null;
		return;
	}

	const payload = pendingPatch;
	pendingPatch = null;
	flushing = true;
	try {
		await settingsApi.patch(payload);
	} catch (err) {
		logger.warn('Failed to sync settings to control plane', err, { context: 'settingsSync' });
	} finally {
		flushing = false;
		if (pendingPatch) {
			void flushPendingSettings();
		}
	}
}

/** Discards any unsent patch (used when logging out). */
export function cancelPendingSettingsSync(): void {
	if (flushTimer !== null) {
		clearTimeout(flushTimer);
		flushTimer = null;
	}
	pendingPatch = null;
}

/** Fetches the persisted settings object from the control plane. */
export function fetchRemoteSettings(): Promise<ControlPlaneSettings> {
	return settingsApi.get();
}

// Flush any debounced patch with keepalive when the tab is reloaded or closed
// so a quick change right before reload is never dropped.
if (browser) {
	window.addEventListener('pagehide', () => {
		if (!pendingPatch || !isAuthenticated.value || !accessToken.value) return;
		if (flushTimer !== null) {
			clearTimeout(flushTimer);
			flushTimer = null;
		}
		const payload = pendingPatch;
		pendingPatch = null;
		void settingsApi.patch(payload, { keepalive: true }).catch(() => {});
	});
}
