import id from './locales/id.json';
import en from './locales/en.json';

// Types
export type Locale = 'id' | 'en';

export interface LocaleInfo {
	code: Locale;
	name: string;
	nativeName: string;
	flag: string;
}

export type TranslationValue = string | Record<string, unknown>;

export interface Translations {
	[key: string]: TranslationValue | Translations;
}

// Available locales configuration
export const locales: LocaleInfo[] = [
	{ code: 'id', name: 'Indonesian', nativeName: 'Bahasa Indonesia', flag: '🇮🇩' },
	{ code: 'en', name: 'English', nativeName: 'English', flag: '🇺🇸' }
];

// Translation dictionaries
const translations: Record<Locale, Translations> = {
	id: id as Translations,
	en: en as Translations
};

const DEFAULT_LOCALE: Locale = 'en';
const STORAGE_KEY = 'app_locale'; // General name as requested

const isBrowser = typeof window !== 'undefined';

function getInitialLocale(): Locale {
	if (isBrowser) {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored && (stored === 'id' || stored === 'en')) {
			return stored as Locale;
		}
	}
	return DEFAULT_LOCALE;
}

// Global reactive state for locale
let _locale = $state<Locale>(getInitialLocale());

export const locale = {
	get value() {
		return _locale;
	},
	set value(newLocale: Locale) {
		if (locales.some((l) => l.code === newLocale)) {
			_locale = newLocale;
			if (isBrowser) {
				localStorage.setItem(STORAGE_KEY, newLocale);
				// Set cookie for SSR support (expires in 1 year)
				document.cookie = `${STORAGE_KEY}=${newLocale}; path=/; max-age=31536000; SameSite=Lax`;
				document.documentElement.lang = newLocale;
			}
		}
	},
	// Compatibility setter
	set: (v: Locale) => {
		locale.value = v;
	}
};

function getNestedValue(obj: Translations, path: string): string {
	const keys = path.split('.');
	let current: any = obj;

	for (const key of keys) {
		if (current && typeof current === 'object' && key in current) {
			current = current[key];
		} else {
			return path;
		}
	}

	return typeof current === 'string' ? current : path;
}

/**
 * Translation function - reactive via Svelte 5 Runes
 */
export function t(key: string, params?: Record<string, string | number>): string {
	const translation = getNestedValue(translations[_locale], key);

	if (params) {
		return Object.entries(params).reduce((str, [paramKey, value]) => {
			return str.replace(new RegExp(`{${paramKey}}`, 'g'), String(value));
		}, translation);
	}

	return translation;
}

export function formatTime(
	date: Date | string | number,
	options: Intl.DateTimeFormatOptions = {}
): string {
	const d = new Date(date);
	if (isNaN(d.getTime())) return String(date);
	const localeMap = { id: 'id-ID', en: 'en-US' };
	return d.toLocaleTimeString(localeMap[_locale], options);
}

export function formatDate(
	date: Date | string | number,
	options: Intl.DateTimeFormatOptions = {}
): string {
	const d = new Date(date);
	if (isNaN(d.getTime())) return String(date);
	const localeMap = { id: 'id-ID', en: 'en-US' };
	return d.toLocaleDateString(localeMap[_locale], options);
}

export const i18n = {
	t,
	formatDate,
	formatTime
};
