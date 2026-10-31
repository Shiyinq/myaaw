import { API_BASE } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { client } from './client';
import type {
	BackupCreated,
	BackupList,
	BackupRequest,
	BackupRestoreRequest,
	BackupRestoreResult,
	OKResponse
} from '$lib/types';

/** One archive, addressed by its file name inside the backup directory. */
function archivePath(name: string) {
	return `/backups/${encodeURIComponent(name)}`;
}

export const backups = {
	list: () => client<BackupList>('/backups'),
	create: (body: BackupRequest = {}) => client<BackupCreated>('/backups', { method: 'POST', body }),
	remove: (name: string) => client<OKResponse>(archivePath(name), { method: 'DELETE' }),
	restore: (name: string, body: BackupRestoreRequest = {}) =>
		client<BackupRestoreResult>(`${archivePath(name)}/restore`, { method: 'POST', body }),
	/**
	 * Same-origin URL for the archive. <a download> cannot send an Authorization
	 * header, so the session token travels as a query parameter.
	 */
	downloadUrl: (name: string) => {
		const query = new URLSearchParams();
		if (accessToken.value) query.set('token', accessToken.value);
		const suffix = query.toString();
		return `${API_BASE}${archivePath(name)}${suffix ? `?${suffix}` : ''}`;
	},
	download: (name: string) => {
		const anchor = document.createElement('a');
		anchor.href = backups.downloadUrl(name);
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
	}
};
