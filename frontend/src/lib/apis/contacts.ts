import { agentPath, client } from './client';
import type { ContactView, ContactsResponse, CreateContactRequest, OKResponse } from '$lib/types';

export const contacts = {
	list: (agent: string) => client<ContactsResponse>(agentPath(agent, '/contacts')),
	create: (agent: string, payload: CreateContactRequest) =>
		client<ContactView>(agentPath(agent, '/contacts'), { method: 'POST', body: payload }),
	remove: (agent: string, name: string) =>
		client<OKResponse>(agentPath(agent, `/contacts/${encodeURIComponent(name)}`), {
			method: 'DELETE'
		})
};
