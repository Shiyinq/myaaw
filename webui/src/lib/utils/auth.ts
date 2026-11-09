export function isExpired(expiresAt?: string | null): boolean {
	if (!expiresAt) return false;
	const timestamp = Date.parse(expiresAt);
	if (Number.isNaN(timestamp)) return false;
	// Small buffer so a request is not sent with a token that expires mid-flight.
	return timestamp - 10_000 <= Date.now();
}
