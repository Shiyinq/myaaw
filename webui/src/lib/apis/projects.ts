import { agentPath, client } from './client';
import type { ProjectStatus } from '$lib/types';

/** Project preview status and build control for one virtual project path. */
export const projects = {
	status: (agent: string, path: string) =>
		client<ProjectStatus>(agentPath(agent, `/project/status?path=${encodeURIComponent(path)}`)),
	/** Starts a sandboxed build; the response carries the building state. */
	build: (agent: string, path: string) =>
		client<ProjectStatus>(agentPath(agent, `/project/build?path=${encodeURIComponent(path)}`), {
			method: 'POST'
		})
};
