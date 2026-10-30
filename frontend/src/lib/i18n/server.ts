import type { Cookies } from '@sveltejs/kit';
import { building } from '$app/environment';

export type Locale = 'id' | 'en';

/**
 * Centrally detects the appropriate locale based on URL, Cookies, and Device settings.
 */
export function detectLocale(request: Request, cookies: Cookies, url: URL): Locale {
	const STORAGE_KEY = 'app_locale';

	// 1. URL Priority (?lang=)
	if (!building) {
		const urlLocale = url.searchParams.get('lang');
		if (urlLocale && (urlLocale === 'id' || urlLocale === 'en')) {
			return urlLocale as Locale;
		}
	}

	// 2. Cookie Priority
	const cookieLocale = cookies.get(STORAGE_KEY);
	if (cookieLocale && (cookieLocale === 'id' || cookieLocale === 'en')) {
		return cookieLocale as Locale;
	}

	// 3. Device/Browser Language (Accept-Language header)
	const acceptLanguage = request.headers.get('accept-language');
	if (acceptLanguage) {
		const preferredLocales = acceptLanguage
			.split(',')
			.map((l) => l.split(';')[0].trim().split('-')[0]);
		const found = preferredLocales.find((l) => l === 'id' || l === 'en');
		if (found) return found as Locale;
	}

	// 4. Default Fallback
	return 'en';
}
