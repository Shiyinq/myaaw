import { getApiBase } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type {
	ContinueGroupRequest,
	ContinueGroupResponse,
	CreateGroupRequest,
	Group,
	GroupAttachmentsPage,
	GroupDetail,
	GroupDetailParams,
	GroupSummary,
	SendGroupMessageRequest,
	SendGroupMessageResponse,
	StopGroupResponse,
	UpdateGroupRequest
} from '$lib/types';

function groupPath(id: string, suffix = ''): string {
	return `/groups/${encodeURIComponent(id)}${suffix}`;
}

async function saveBlobUrl(url: string, name: string) {
	try {
		const response = await fetch(url);
		if (!response.ok) throw new Error(String(response.status));
		const objectUrl = URL.createObjectURL(await response.blob());
		const anchor = document.createElement('a');
		anchor.href = objectUrl;
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
		setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
	} catch {
		const anchor = document.createElement('a');
		anchor.href = url;
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
	}
}

export const groups = {
	list: () => client<{ groups: GroupSummary[] }>('/groups'),
	create: (payload: CreateGroupRequest) =>
		client<Group>('/groups', { method: 'POST', body: payload }),
	get: (id: string, params?: GroupDetailParams) => {
		const query = new URLSearchParams();
		if (params?.limit !== undefined) query.set('limit', String(params.limit));
		if (params?.offset !== undefined) query.set('offset', String(params.offset));
		if (params?.before_id) query.set('before_id', params.before_id);
		const qs = query.toString();
		return client<GroupDetail>(groupPath(id, qs ? `?${qs}` : ''));
	},
	update: (id: string, payload: UpdateGroupRequest) =>
		client<Group>(groupPath(id), { method: 'PATCH', body: payload }),
	remove: (id: string) => client<{ deleted: boolean }>(groupPath(id), { method: 'DELETE' }),
	clearMessages: (id: string) =>
		client<{ cleared: boolean }>(groupPath(id, '/messages'), { method: 'DELETE' }),
	sendMessage: (id: string, payload: SendGroupMessageRequest) =>
		client<SendGroupMessageResponse>(groupPath(id, '/messages'), {
			method: 'POST',
			body: payload
		}),
	stop: (id: string) => client<StopGroupResponse>(groupPath(id, '/stop'), { method: 'POST' }),
	continueRun: (id: string, payload: ContinueGroupRequest = {}) =>
		client<ContinueGroupResponse>(groupPath(id, '/continue'), {
			method: 'POST',
			body: payload
		}),
	attachments: (id: string, params?: { limit?: number; offset?: number; kind?: string }) => {
		const query = new URLSearchParams();
		if (params?.limit !== undefined) query.set('limit', String(params.limit));
		if (params?.offset !== undefined) query.set('offset', String(params.offset));
		if (params?.kind) query.set('kind', params.kind);
		const qs = query.toString();
		return client<GroupAttachmentsPage>(groupPath(id, `/attachments${qs ? `?${qs}` : ''}`));
	},
	uploadUrl: (
		groupId: string,
		messageId: string,
		storedName: string,
		options?: { download?: boolean }
	) => {
		const query = new URLSearchParams();
		const token = accessToken.value;
		if (token) query.set('token', token);
		if (options?.download) query.set('download', '1');
		const qs = query.toString();
		return `${getApiBase()}/groups/${encodeURIComponent(groupId)}/uploads/${encodeURIComponent(messageId)}/${encodeURIComponent(storedName)}${qs ? `?${qs}` : ''}`;
	},
	downloadUpload: (groupId: string, messageId: string, storedName: string, fileName?: string) => {
		const url = groups.uploadUrl(groupId, messageId, storedName, { download: true });
		return saveBlobUrl(url, fileName || storedName);
	},
	stream: (id: string, onEvent: (event: SSEEvent) => void, signal?: AbortSignal) =>
		streamSSE(groupPath(id, '/stream'), {
			method: 'GET',
			signal,
			onEvent,
			retry: {
				attempts: 6,
				baseDelayMs: 1000,
				maxDelayMs: 8000
			}
		})
};
