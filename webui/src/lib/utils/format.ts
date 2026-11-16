import { formatDate } from '$lib/i18n';

/** Bytes as a short human string, matching the file browsers. */
export function formatSize(size: number): string {
	if (size < 1024) return `${size} B`;
	if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
	return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
}

/** A compact date for list metadata: "3 Nov". */
export function formatShortDate(value: string | undefined): string {
	if (!value) return '';
	return formatDate(value, { day: 'numeric', month: 'short' });
}
