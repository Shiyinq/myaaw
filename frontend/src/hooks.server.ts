import type { Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { detectLocale } from '$lib/i18n/server';

// Same-origin proxy: the browser talks to /api on this app, and the server
// forwards it to the control plane. That keeps tokens out of CORS preflights
// and lets SSE work with a plain fetch.
const CONTROL_PLANE = (env.MYAAW_API_BASE_URL || 'http://localhost:9000').replace(/\/+$/, '');

export const handle: Handle = async ({ event, resolve }) => {
	if (event.url.pathname === '/health') {
		return new Response('OK', { status: 200 });
	}

	const locale = detectLocale(event.request, event.cookies, event.url);

	if (event.url.pathname.startsWith('/api')) {
		const target = event.url.pathname.replace('/api', CONTROL_PLANE);
		const requestHeaders = new Headers(event.request.headers);
		requestHeaders.set('host', new URL(CONTROL_PLANE).host);

		try {
			const response = await fetch(target + event.url.search, {
				method: event.request.method,
				headers: requestHeaders,
				body:
					event.request.method !== 'GET' && event.request.method !== 'HEAD'
						? await event.request.arrayBuffer()
						: undefined,
				// @ts-expect-error - duplex is needed for streaming request bodies
				duplex: 'half'
			});

			return response;
		} catch (error) {
			if (process.env.NODE_ENV !== 'production') {
				console.error('[hooks.server] Proxy error:', error);
			}
			return new Response('Control plane unreachable', { status: 502 });
		}
	}

	return resolve(event, {
		transformPageChunk: ({ html }) => html.replace('%lang%', locale)
	});
};
