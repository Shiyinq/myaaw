import { addToast } from '$lib/stores/toast.svelte';
import { logger } from '$lib/utils/logger';

/**
 * Copies text to clipboard and shows a success toast.
 * @param text - The string to copy
 * @param label - Descriptive name of what was copied (e.g., "API Key")
 */
export async function copyToClipboard(text: string, label = 'Content') {
	try {
		await navigator.clipboard.writeText(text);
		addToast(`${label} copied to clipboard.`, 'success');
		return true;
	} catch (err) {
		logger.error('Failed to copy to clipboard', err, { context: 'clipboard' });
		addToast('Failed to copy content.', 'error');
		return false;
	}
}
