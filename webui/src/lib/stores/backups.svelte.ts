import { backups as backupsApi } from '$lib/apis/backup';
import type {
	BackupList,
	BackupRequest,
	BackupRestoreRequest,
	BackupRestoreResult
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<Record<string, never>, BackupList>({
	key: () => 'backups',
	fetcher: () => backupsApi.list()
});

export const backupsStore = {
	query,
	get data() {
		return query.data;
	},
	get loading() {
		return query.loading;
	},
	get refreshing() {
		return query.refreshing;
	},
	get error() {
		return query.error;
	},
	get status() {
		return query.status;
	},
	ensure() {
		return query.ensure({});
	},
	refresh() {
		return query.refresh();
	},
	async create(body: BackupRequest = {}) {
		const result = await backupsApi.create(body);
		await query.refresh();
		return result;
	},
	async remove(name: string) {
		await backupsApi.remove(name);
		await query.refresh();
	},
	async restore(name: string, body: BackupRestoreRequest = {}): Promise<BackupRestoreResult> {
		const result = await backupsApi.restore(name, body);
		await query.refresh();
		return result;
	},
	download(name: string) {
		backupsApi.download(name);
	},
	downloadUrl(name: string) {
		return backupsApi.downloadUrl(name);
	}
};
