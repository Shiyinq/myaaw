export interface ContextFile {
	name: string;
	path: string;
	folder: string;
	size: number;
	mod_time?: string;
}

export interface ContextFilesResponse {
	files: ContextFile[];
}

export interface ContextContent extends ContextFile {
	content: string;
}
