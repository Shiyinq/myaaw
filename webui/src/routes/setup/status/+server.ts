import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { setupStatus } from '$lib/server/setup';

export async function GET() {
	const apiUrl = env.MYAAW_API_BASE_URL || 'http://localhost:9000';
	return json(await setupStatus(apiUrl));
}
