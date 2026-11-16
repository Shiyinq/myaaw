import { contacts as contactsApi } from '$lib/apis/contacts';
import type {
	ContactView,
	ContactsResponse,
	CreateContactRequest,
	UpdateContactRequest
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const query = createQueryStore<{ agent: string }, ContactsResponse>({
	key: (args) => `contacts:${args.agent}`,
	fetcher: (args) => contactsApi.list(args.agent)
});

export const contactsStore = {
	query,
	get data() {
		return query.data;
	},
	get contacts(): ContactView[] {
		return query.data?.contacts ?? [];
	},
	get loading() {
		return query.loading;
	},
	get refreshing() {
		return query.refreshing;
	},
	get error() {
		return query.error;
	},
	get status() {
		return query.status;
	},
	ensure(agent: string) {
		return query.ensure({ agent });
	},
	refresh() {
		return query.refresh();
	},
	async create(agent: string, payload: CreateContactRequest) {
		const created = await contactsApi.create(agent, payload);
		await query.refresh({ agent });
		return created;
	},
	async update(agent: string, contact: string, payload: UpdateContactRequest) {
		const updated = await contactsApi.update(agent, contact, payload);
		await query.refresh({ agent });
		return updated;
	},
	async remove(agent: string, name: string) {
		await contactsApi.remove(agent, name);
		await query.refresh({ agent });
	}
};
