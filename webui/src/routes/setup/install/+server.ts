import { error, json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { installBinary, startControlPlane } from '$lib/server/setup';

export async function POST({ request, url }) {
	// The dashboard is loopback-only, but a malicious page could still POST here.
	// Browsers send Origin on cross-origin POSTs, so reject any mismatch.
	const origin = request.headers.get('origin');
	if (origin && origin !== url.origin) {
		error(403, 'Cross-origin install is not allowed');
	}

	const apiUrl = env.MYAAW_API_BASE_URL || 'http://localhost:9000';
	try {
		const binary = await installBinary();
		await startControlPlane(apiUrl);
		return json({ ok: true, binary });
	} catch (cause) {
		error(500, cause instanceof Error ? cause.message : 'Install failed');
	}
}
