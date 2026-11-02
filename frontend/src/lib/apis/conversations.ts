import { agentPath, client } from './client';
import type {
	ChatFolder,
	ChatFolderDelete,
	ChatFolderList,
	ConversationActivity,
	ConversationDetail,
	ConversationPage,
	OKResponse
} from '$lib/types';

export interface ConversationFilters {
	search?: string;
	limit?: number;
	offset?: number;
	/** Only pinned conversations. */
	pinned?: boolean;
	/** Only Recents: not pinned and not in a folder. */
	unfiled?: boolean;
	/** Only conversations in this folder. */
	folder?: string;
}

export const conversations = {
	list: (agent: string, params: ConversationFilters = {}) => {
		const query = new URLSearchParams();
		if (params.search) query.set('search', params.search);
		if (params.limit !== undefined) query.set('limit', String(params.limit));
		if (params.offset !== undefined) query.set('offset', String(params.offset));
		if (params.pinned) query.set('pinned', '1');
		if (params.unfiled) query.set('unfiled', '1');
		if (params.folder) query.set('folder', params.folder);
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<ConversationPage>(agentPath(agent, `/conversations${suffix}`));
	},
	get: (agent: string, id: string, params: { limit?: number; offset?: number } = {}) => {
		const query = new URLSearchParams();
		if (params.limit !== undefined) query.set('limit', String(params.limit));
		if (params.offset !== undefined) query.set('offset', String(params.offset));
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<ConversationDetail>(
			agentPath(agent, `/conversations/${encodeURIComponent(id)}${suffix}`)
		);
	},
	rename: (agent: string, id: string, title: string) =>
		client<{ ok: boolean; id: string; title: string }>(
			agentPath(agent, `/conversations/${encodeURIComponent(id)}`),
			{ method: 'PATCH', body: { title } }
		),
	remove: (agent: string, id: string) =>
		client<OKResponse>(agentPath(agent, `/conversations/${encodeURIComponent(id)}`), {
			method: 'DELETE'
		}),
	/** Pinned chats sort first and never change the transcript order. */
	setPinned: (agent: string, id: string, pinned: boolean) =>
		client<OKResponse>(agentPath(agent, `/conversations/${encodeURIComponent(id)}/pin`), {
			method: 'PUT',
			body: { pinned }
		}),
	/** `null` moves the chat out of its folder, into Recents. */
	setFolder: (agent: string, id: string, folderId: string | null) =>
		client<OKResponse>(agentPath(agent, `/conversations/${encodeURIComponent(id)}/folder`), {
			method: 'PUT',
			body: { folder_id: folderId }
		}),
	activity: (agent: string, params: { from: number; to: number }) =>
		client<ConversationActivity>(
			agentPath(agent, `/conversations/activity?from=${params.from}&to=${params.to}`)
		)
};

export const chatFolders = {
	list: (agent: string) => client<ChatFolderList>(agentPath(agent, '/chat-folders')),
	create: (agent: string, name: string) =>
		client<ChatFolder>(agentPath(agent, '/chat-folders'), {
			method: 'POST',
			body: { name }
		}),
	rename: (agent: string, id: string, name: string) =>
		client<ChatFolder>(agentPath(agent, `/chat-folders/${encodeURIComponent(id)}`), {
			method: 'PATCH',
			body: { name }
		}),
	remove: (agent: string, id: string) =>
		client<ChatFolderDelete>(agentPath(agent, `/chat-folders/${encodeURIComponent(id)}`), {
			method: 'DELETE'
		})
};
