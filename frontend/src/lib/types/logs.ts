export interface LogFile {
	name: string;
	size: number;
	mod_time?: string;
}

export interface LogsResponse {
	files: LogFile[];
}
