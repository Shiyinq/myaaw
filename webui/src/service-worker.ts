/// <reference lib="webworker" />
import { precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { NetworkOnly } from 'workbox-strategies';

declare let self: ServiceWorkerGlobalScope;

// 1. Pre-cache all static assets injected by Vite
precacheAndRoute(self.__WB_MANIFEST);

// 2. Strategy to catch navigation requests (HTML) that fail due to offline status
const fallbackRoute = new NavigationRoute(async (options) => {
	try {
		// Attempt to fetch the page from the network
		const networkOnly = new NetworkOnly();
		return await networkOnly.handle(options);
	} catch (_error) {
		// If it fails (due to being offline), return SvelteKit's offline page from cache
		// We use caches.match to search all caches, and ignoreSearch to ignore Vite's revision queries
		let offlineResponse = await caches.match('/offline', { ignoreSearch: true });
		if (!offlineResponse) {
			offlineResponse = await caches.match('/offline/index.html', { ignoreSearch: true });
		}

		if (offlineResponse) return offlineResponse;

		// Emergency fallback if the cache is not found
		return new Response('Connection Lost. You are currently offline.', {
			status: 503,
			headers: { 'Content-Type': 'text/html' }
		});
	}
});
registerRoute(fallbackRoute);

// 3. Listen for the SKIP_WAITING command from the Update Prompt UI
self.addEventListener('message', (event) => {
	if (event.data && event.data.type === 'SKIP_WAITING') {
		self.skipWaiting();
	}
});

self.addEventListener('activate', (event: ExtendableEvent) => {
	event.waitUntil(self.clients.claim());
});
