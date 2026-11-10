export interface CreateProviderRequest {
	name: string;
	type: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
	thinking_level?: string;
	auth_mode?: 'oauth';
	credential_id?: string;
}

export interface UpdateProviderRequest {
	type?: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
	thinking_level?: string;
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

export interface ModelCapabilityFlags {
	thinking?: boolean | null;
	vision?: boolean | null;
	function_calling?: boolean | null;
	parallel_function_calling?: boolean | null;
	structured_outputs?: boolean | null;
	json_mode?: boolean | null;
	streaming?: boolean | null;
	prompt_caching?: boolean | null;
	batch_api?: boolean | null;
}

export interface ModelThinkingCapability {
	can_disable?: boolean | null;
	effort_values?: string[] | null;
}

export interface ModelCapabilityEntry {
	provider?: string;
	status?: string;
	capabilities?: ModelCapabilityFlags;
	thinking?: ModelThinkingCapability;
	input_modalities?: string[];
	output_modalities?: string[];
	context_window_tokens?: number | null;
	builtin_tools?: Record<string, boolean | null>;
}

export interface ModelCapabilitiesView {
	version: number;
	models: Record<string, ModelCapabilityEntry>;
	warning?: string;
}
