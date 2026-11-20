import { client } from './client';
import type { ControlPlaneSettings, ControlPlaneSettingsPatch } from '$lib/types';

export const settings = {
	get: () => client<ControlPlaneSettings>('/settings'),
	patch: (payload: ControlPlaneSettingsPatch, opts?: { keepalive?: boolean }) =>
		client<ControlPlaneSettings>('/settings', {
			method: 'PATCH',
			body: payload,
			keepalive: opts?.keepalive
		})
};
