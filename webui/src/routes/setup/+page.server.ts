import { isBinaryInstalled, issueSetupToken } from '$lib/server/setup';

export function load() {
	return { token: isBinaryInstalled() ? null : issueSetupToken() };
}
