import { json } from '@sveltejs/kit';
import { currentInstallProgress } from '$lib/server/setup';

// Polled by the setup page while POST /setup/install is in flight. The hook in
// hooks.server.ts hides every /setup route in public hosting mode.
export function GET() {
	return json(currentInstallProgress());
}
