import type { SecretState } from './config';

export interface ContactView {
	name: string;
	url: string;
	description?: string;
	token: SecretState;
}

export interface ContactsResponse {
	contacts: ContactView[];
}

export interface CreateContactRequest {
	name: string;
	url: string;
	token?: string;
	description?: string;
}
