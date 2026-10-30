import { agentPath, client } from './client';
import type {
	CreateProviderRequest,
	ModelListView,
	ModelLookupRequest,
	OKResponse,
	ProvidersView,
	UpdateProviderRequest
} from '$lib/types';

export const providers = {
	list: (agent: string) => client<ProvidersView>(agentPath(agent, '/providers')),
	create: (agent: string, payload: CreateProviderRequest) =>
		client<OKResponse>(agentPath(agent, '/providers'), { method: 'POST', body: payload }),
	update: (agent: string, name: string, payload: UpdateProviderRequest) =>
		client<OKResponse>(agentPath(agent, `/providers/${encodeURIComponent(name)}`), {
			method: 'PUT',
			body: payload
		}),
	remove: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, `/providers/${encodeURIComponent(name)}`), {
			method: 'DELETE'
		}),
	setDefault: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, '/providers/default'), {
			method: 'POST',
			body: { name }
		}),
	modelsForCredentials: (payload: ModelLookupRequest) =>
		client<ModelListView>('/providers/models', { method: 'POST', body: payload }),
	models: (agent: string, name: string) =>
		client<ModelListView>(agentPath(agent, `/providers/${encodeURIComponent(name)}/models`))
};
