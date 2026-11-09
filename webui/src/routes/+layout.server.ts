import type { LayoutServerLoad } from './$types';
import { detectLocale } from '$lib/i18n/server';
import { isBinaryInstalled, webuiConfigApiUrl } from '$lib/server/setup';

export const load: LayoutServerLoad = async (event) => {
	const locale = detectLocale(event.request, event.cookies, event.url);

	return {
		locale,
		apiUrl: webuiConfigApiUrl(),
		binaryInstalled: isBinaryInstalled()
	};
};
