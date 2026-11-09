export interface CreateProviderRequest {
	name: string;
	type: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
	auth_mode?: 'oauth';
	credential_id?: string;
}

export interface UpdateProviderRequest {
	type?: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
	auth_mode?: 'oauth';
	credential_id?: string;
}

export interface ModelLookupRequest {
	type: string;
	base_url?: string;
	api_key?: string;
	credential_id?: string;
}

export interface ChatGPTConnectSession {
	session_id: string;
	authorization_url: string;
	expires_at: string;
}

export interface ChatGPTConnectStatus {
	status: 'pending' | 'connected' | 'failed' | 'expired';
	credential_id?: string;
	email?: string;
	error?: string;
}

export interface ProviderConnection {
	configured: boolean;
	status: string;
	account_label?: string;
	expires_at?: string;
}

export interface ModelListView {
	models: string[];
	source: 'live' | 'default' | string;
	warning?: string;
}
