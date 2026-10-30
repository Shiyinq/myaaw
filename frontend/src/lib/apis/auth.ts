import { client } from './client';
import type {
	ChangePasswordRequest,
	ChangeUsernameRequest,
	LoginRequest,
	LoginResponse,
	MeResponse,
	OKResponse
} from '$lib/types';

export const auth = {
	login: (payload: LoginRequest) =>
		client<LoginResponse>('/auth/login', { method: 'POST', body: payload }),
	me: () => client<MeResponse>('/auth/me'),
	logout: () => client<OKResponse>('/auth/logout', { method: 'POST' }),
	changePassword: (payload: ChangePasswordRequest) =>
		client<OKResponse>('/auth/change-password', { method: 'POST', body: payload }),
	changeUsername: (payload: ChangeUsernameRequest) =>
		client<MeResponse>('/auth/username', { method: 'PUT', body: payload })
};
