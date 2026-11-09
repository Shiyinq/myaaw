export interface TerminalStatus {
	enabled: boolean;
}

export interface TerminalInfo {
	id: string;
	shell: string;
	cwd: string;
	cols: number;
	rows: number;
}
