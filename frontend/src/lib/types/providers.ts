

export interface CreateProviderRequest {
	name: string;
	type: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
}

export interface UpdateProviderRequest {
	type?: string;
	base_url?: string;
	api_key?: string;
	default_model?: string;
}

export interface ModelLookupRequest {
	type: string;
	base_url?: string;
	api_key?: string;
}

export interface ModelListView {
	models: string[];
	source: 'live' | 'default' | string;
	warning?: string;
}
