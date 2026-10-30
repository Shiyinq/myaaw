import { t as translate, locale, locales } from './index';
import type { Locale } from './index';

/**
 * Custom hook for using translations in Svelte components.
 * Provides a reactive way to access translations and locale utilities.
 */
export function useTranslation() {
	const changeLocale = (newLocale: Locale): void => {
		locale.value = newLocale;
	};

	const isLocale = (checkLocale: Locale): boolean => {
		return locale.value === checkLocale;
	};

	return {
		t: translate,
		locale,
		get localeInfo() {
			return locales.find((l) => l.code === locale.value) || locales[0];
		},
		changeLocale,
		availableLocales: locales,
		isLocale
	};
}

export { type Locale, type LocaleInfo } from './index';
export { locale, locales } from './index';
