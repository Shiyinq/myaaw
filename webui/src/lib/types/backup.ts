export interface BackupArchive {
	name: string;
	agent: string;
	size: number;
	created_at: string;
}

export interface BackupList {
	/** Directory the archives live in, resolved on the server. */
	dir: string;
	archives: BackupArchive[];
}

export interface BackupRequest {
	/** Empty backs up every initialized agent. */
	agents?: string[];
	include_workspace?: boolean;
	include_logs?: boolean;
	include_venv?: boolean;
	include_caches?: boolean;
	no_secrets?: boolean;
}

export interface BackupEntry {
	agent: string;
	path: string;
	files: number;
	bytes: number;
}

export interface BackupCreated {
	dir: string;
	archives: BackupEntry[];
}

export interface BackupRestoreRequest {
	agent?: string;
	force?: boolean;
	start?: boolean;
	restore_packages?: boolean;
}

export interface BackupRestoreResult {
	agent: string;
	files: number;
	pre_restore_backup?: string;
	port_warning?: string;
	note?: string;
}
