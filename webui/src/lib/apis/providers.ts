import { agentPath, client } from './client';
import type {
	ChatGPTConnectSession,
	ChatGPTConnectStatus,
	CreateProviderRequest,
	ModelListView,
	ModelLookupRequest,
	OKResponse,
	ProvidersView,
	UpdateProviderRequest
} from '$lib/types';

export const providers = {
	connectChatGPT: (agent: string, credentialId?: string) =>
		client<ChatGPTConnectSession>(agentPath(agent, '/providers/chatgpt/connect'), {
			method: 'POST',
			body: credentialId ? { credential_id: credentialId } : {}
		}),
	chatGPTStatus: (agent: string, session: string, signal?: AbortSignal) =>
		client<ChatGPTConnectStatus>(
			agentPath(agent, `/providers/chatgpt/connect/${encodeURIComponent(session)}`),
			{ signal }
		),
	cancelChatGPT: (agent: string, session: string) =>
		client<OKResponse>(
			agentPath(agent, `/providers/chatgpt/connect/${encodeURIComponent(session)}`),
			{ method: 'DELETE', keepalive: true }
		),
	disconnect: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, `/providers/${encodeURIComponent(name)}/disconnect`), {
			method: 'POST'
		}),
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
