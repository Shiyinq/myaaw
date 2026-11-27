import type { ConversationSummary } from '$lib/types';

export interface ConversationDayGroup {
	key: string;
	label: string;
	items: ConversationSummary[];
}

/**
 * Formats a calendar day heading: Today, Yesterday, "Nov 25" in the current
 * year, or "Nov 25, 2025" whenever the conversation falls in an earlier year.
 */
export function formatConversationDayLabel(
	date: Date,
	locale: string | undefined,
	todayLabel: string,
	yesterdayLabel: string
): string {
	if (Number.isNaN(date.getTime())) return '';
	const now = new Date();
	const startOfDay = (value: Date) =>
		new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime();
	const diff = Math.round((startOfDay(now) - startOfDay(date)) / 86_400_000);
	if (diff === 0) return todayLabel;
	if (diff === 1) return yesterdayLabel;
	const sameYear = date.getFullYear() === now.getFullYear();
	return date.toLocaleDateString(
		locale || undefined,
		sameYear
			? { month: 'short', day: 'numeric' }
			: { month: 'short', day: 'numeric', year: 'numeric' }
	);
}

/**
 * Groups an already-sorted list of conversations (newest first) into contiguous
 * local-date sections (Today, Yesterday, per day, including year when different).
 */
export function groupConversationsByDay(
	items: ConversationSummary[],
	locale: string | undefined,
	todayLabel: string,
	yesterdayLabel: string
): ConversationDayGroup[] {
	const groups: ConversationDayGroup[] = [];
	for (const item of items) {
		const date = new Date(item.updated_at);
		const key = Number.isNaN(date.getTime())
			? 'unknown'
			: `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
		const last = groups[groups.length - 1];
		if (last && last.key === key) {
			last.items.push(item);
		} else {
			groups.push({
				key,
				label: formatConversationDayLabel(date, locale, todayLabel, yesterdayLabel),
				items: [item]
			});
		}
	}
	return groups;
}

/** Compact item timestamp matching ConversationList ("Nov 27, 14:51" or with year for prior years). */
export function formatConversationTimestamp(raw: string, locale: string | undefined): string {
	const date = new Date(raw);
	if (Number.isNaN(date.getTime())) return '';
	const now = new Date();
	const sameYear = date.getFullYear() === now.getFullYear();
	return date.toLocaleString(
		locale || undefined,
		sameYear
			? { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
			: { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' }
	);
}
