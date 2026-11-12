import { agentPath, client } from './client';
import type {
	ChatFolder,
	ChatFolderDelete,
	ChatFolderList,
	ConversationActivity,
	ConversationAttachmentPage,
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
	/** Only sessions owned by one platform (telegram, discord, api, cli). */
	channel?: string;
}

export interface AttachmentFilters {
	limit?: number;
	offset?: number;
	/** Comma-separated rendering kinds: image, video, audio, pdf, file. */
	kind?: string;
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
		if (params.channel) query.set('channel', params.channel);
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
	/**
	 * Stores the newest turn the reader has acknowledged. Shared by every device,
	 * so reading a chat on one clears the unread marker on the others.
	 */
	markRead: (agent: string, id: string, at?: string) =>
		client<OKResponse>(agentPath(agent, `/conversations/${encodeURIComponent(id)}/read`), {
			method: 'POST',
			body: at ? { at } : {}
		}),
	activity: (agent: string, params: { from: number; to: number }) =>
		client<ConversationActivity>(
			agentPath(agent, `/conversations/activity?from=${params.from}&to=${params.to}`)
		),
	/**
	 * The files of one conversation, oldest first and deduplicated by path. The
	 * list is paged like the conversation list, so a long chat never ships its
	 * whole transcript to the sidebar.
	 */
	attachments: (agent: string, id: string, params: AttachmentFilters = {}) => {
		const query = new URLSearchParams();
		if (params.limit !== undefined) query.set('limit', String(params.limit));
		if (params.offset !== undefined) query.set('offset', String(params.offset));
		if (params.kind) query.set('kind', params.kind);
		const suffix = query.toString() ? `?${query.toString()}` : '';
		return client<ConversationAttachmentPage>(
			agentPath(agent, `/conversations/${encodeURIComponent(id)}/attachments${suffix}`)
		);
	}
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
