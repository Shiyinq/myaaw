import { agentPath, client } from './client';
import type { OKResponse, SkillDetail, SkillFileContent, SkillsResponse } from '$lib/types';

export const skills = {
	list: (agent: string) => client<SkillsResponse>(agentPath(agent, '/skills')),
	get: (agent: string, skill: string) =>
		client<SkillDetail>(agentPath(agent, `/skills/${encodeURIComponent(skill)}`)),
	file: (agent: string, skill: string, path: string) =>
		client<SkillFileContent>(
			agentPath(agent, `/skills/${encodeURIComponent(skill)}/file?path=${encodeURIComponent(path)}`)
		),
	toggle: (agent: string, skill: string, enabled: boolean) =>
		client<{ ok: boolean; name: string; enabled: boolean }>(
			agentPath(agent, `/skills/${encodeURIComponent(skill)}`),
			{ method: 'PATCH', body: { enabled } }
		),
	remove: (agent: string, skill: string) =>
		client<OKResponse>(agentPath(agent, `/skills/${encodeURIComponent(skill)}`), {
			method: 'DELETE'
		}),
	removeConfig: (agent: string, skill: string) =>
		client<OKResponse>(agentPath(agent, `/skills/${encodeURIComponent(skill)}/config`), {
			method: 'DELETE'
		})
};
