import { error, json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import {
	checkSetupToken,
	consumeSetupToken,
	failInstallProgress,
	finishInstallProgress,
	installBinary,
	isBinaryInstalled,
	startControlPlane,
	startControlPlaneProgress
} from '$lib/server/setup';

export async function POST({ request, url }) {
	// The dashboard is loopback-only, but a malicious page could still POST here.
	// Browsers send Origin on cross-origin POSTs, so reject any mismatch.
	const origin = request.headers.get('origin');
	if (origin && origin !== url.origin) {
		error(403, 'Cross-origin install is not allowed');
	}

	// Installing twice is a no-op at best and a re-download at worst.
	if (isBinaryInstalled()) {
		error(409, 'Myaaw is already installed');
	}

	const token = request.headers.get('x-setup-token');
	if (!checkSetupToken(token)) {
		error(403, 'Invalid or expired setup token');
	}

	const apiUrl = env.MYAAW_API_BASE_URL || 'http://localhost:9000';
	try {
		const binary = await installBinary();
		startControlPlaneProgress();
		consumeSetupToken(token);
		await startControlPlane(apiUrl);
		finishInstallProgress();
		return json({ ok: true, binary });
	} catch (cause) {
		const message = cause instanceof Error ? cause.message : 'Install failed';
		failInstallProgress(message);
		error(500, message);
	}
}
