export interface LoginRequest {
	username: string;
	password: string;
}

export interface LoginResponse {
	token: string;
	expires_at: string;
	user_id: number;
	must_change_password?: boolean;
}

export interface MeResponse {
	username: string;
	user_id: number;
	must_change_password?: boolean;
}

export interface ChangePasswordRequest {
	current_password: string;
	new_password: string;
}

export interface ChangeUsernameRequest {
	password: string;
	new_username: string;
}
