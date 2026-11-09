export interface ApiError {
	status?: number;
	detail?:
		| string
		| { loc: (string | number)[]; msg: string; type: string }[]
		| Record<string, unknown>;
	error?: string;
	message?: string;
}

export interface GenericResponse {
	message?: string;
	ok?: boolean;
}

export interface OKResponse {
	ok: boolean;
}

export interface Toast {
	id: string;
	message: string;
	type: 'success' | 'error' | 'info' | 'warning';
	duration?: number;
}
