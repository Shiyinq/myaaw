export interface FileEntry {
	name: string;
	path: string;
	is_dir: boolean;
	size: number;
	mod_time?: string;
}

export interface FileListResponse {
	path: string;
	entries: FileEntry[];
}

export interface FileContent {
	path: string;
	content: string;
	size: number;
	mod_time?: string;
}

export interface SearchMatch {
	path: string;
	line: number;
	text: string;
}

export interface SearchResponse {
	matches: SearchMatch[];
}
