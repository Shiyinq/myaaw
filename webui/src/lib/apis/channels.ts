import { agentPath, client } from './client';
import type { ChannelInput, ChannelsView, OKResponse } from '$lib/types';

export const channels = {
	list: (agent: string) => client<ChannelsView>(agentPath(agent, '/channels')),
	upsert: (agent: string, channel: 'telegram' | 'discord', payload: ChannelInput) =>
		client<OKResponse>(agentPath(agent, `/channels/${channel}`), {
			method: 'PUT',
			body: payload
		}),
	disable: (agent: string, channel: 'telegram' | 'discord') =>
		client<OKResponse>(agentPath(agent, `/channels/${channel}`), { method: 'DELETE' })
};
