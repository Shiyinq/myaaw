import type { Locale } from '$lib/i18n';

const DAY_MS = 24 * 60 * 60 * 1000;

function localeTag(locale: Locale): string {
	return locale === 'id' ? 'id-ID' : 'en-US';
}

function startOfDay(date: Date): number {
	return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

/**
 * A transcript timestamp that gets coarser as it ages, like a chat app:
 * today shows the clock, yesterday says so, the past week adds the weekday,
 * the rest of the year shows the date, and older years add the year.
 *
 * Stored times are UTC; `Date` converts them to the reader's timezone.
 */
export function formatChatTimestamp(
	value: string | number | Date | null | undefined,
	locale: Locale,
	yesterdayLabel: string,
	now = new Date()
): string {
	if (value === null || value === undefined || value === '') return '';
	const date = value instanceof Date ? value : new Date(value);
	if (Number.isNaN(date.getTime())) return '';

	const tag = localeTag(locale);
	const time = new Intl.DateTimeFormat(tag, {
		hour: '2-digit',
		minute: '2-digit'
	}).format(date);

	const days = Math.round((startOfDay(now) - startOfDay(date)) / DAY_MS);
	if (days <= 0) return time;
	if (days === 1) return `${yesterdayLabel} · ${time}`;

	const dayMonth = new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'short' }).format(date);
	if (days < 7) {
		const weekday = new Intl.DateTimeFormat(tag, { weekday: 'short' }).format(date);
		return `${weekday}, ${dayMonth} · ${time}`;
	}
	if (date.getFullYear() === now.getFullYear()) return `${dayMonth} · ${time}`;

	const full = new Intl.DateTimeFormat(tag, {
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	}).format(date);
	return `${full} · ${time}`;
}
