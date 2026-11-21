import { client } from './client';
import { streamSSE, type SSEEvent } from '$lib/utils/sse';
import type {
	ContinueGroupRequest,
	ContinueGroupResponse,
	CreateGroupRequest,
	Group,
	GroupDetail,
	GroupSummary,
	SendGroupMessageRequest,
	SendGroupMessageResponse,
	StopGroupResponse,
	UpdateGroupRequest
} from '$lib/types';

function groupPath(id: string, suffix = ''): string {
	return `/groups/${encodeURIComponent(id)}${suffix}`;
}

export const groups = {
	list: () => client<{ groups: GroupSummary[] }>('/groups'),
	create: (payload: CreateGroupRequest) =>
		client<Group>('/groups', { method: 'POST', body: payload }),
	get: (id: string) => client<GroupDetail>(groupPath(id)),
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
