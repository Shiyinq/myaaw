export interface ServerStatus {
	running: boolean;
	pid?: number;
	port?: number;
	base_url?: string;
}
