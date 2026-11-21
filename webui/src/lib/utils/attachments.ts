import { filesStore } from '$lib/stores/files.svelte';
import { groupsStore } from '$lib/stores/groups.svelte';
import type { Attachment, GroupAttachment } from '$lib/types';

/**
 * A delivery directive is backend syntax. Readers see the path it names, so
 * `@attach(/a/b.png)` renders as `/a/b.png` without the wrapper.
 */
const ATTACH_DIRECTIVE_RE = /@attach\(\s*(?:"([^"]+)"|'([^']+)'|([^)\s]+))\s*\)/gi;
/** Short form the model sometimes writes: `@/absolute/path`. */
const BARE_ATTACH_RE = /@((?:~\/?|\/)[^\s"'<>()]+)/g;

/** Strips `@attach(...)` and bare `@/path` delivery directives for display. */
export function stripAttachDirectives(text: string): string {
	return text
		.replace(ATTACH_DIRECTIVE_RE, (_match, ...groups) => groups.find((group) => group) ?? '')
		.replace(BARE_ATTACH_RE, '$1');
}

/** Resolves the fetch/preview URL for either a user group upload or an agent `@attach` file. */
export function groupAttachmentUrl(
	item: GroupAttachment,
	options?: { download?: boolean }
): string {
	if (item.agent && item.path) {
		return filesStore.rawUrl(item.agent, item.path, options);
	}
	if (item.stored_name) {
		return groupsStore.uploadUrl(item.group_id, item.message_id, item.stored_name, options);
	}
	return '';
}

/** Triggers a download for either a user group upload or an agent `@attach` file. */
export function downloadGroupAttachment(item: GroupAttachment): void {
	if (item.agent && item.path) {
		filesStore.download(item.agent, item.path);
		return;
	}
	if (item.stored_name) {
		void groupsStore.downloadUpload(
			item.group_id,
			item.message_id,
			item.stored_name,
			item.file_name
		);
	}
}

/** Converts agent-delivered `GroupAttachment` entries into `RunAttachments` items. */
export function toRunAttachments(items?: GroupAttachment[]): Attachment[] {
	if (!items || items.length === 0) return [];
	return items
		.filter((item) => Boolean(item.path))
		.map((item) => ({
			file_name: item.file_name,
			mime_type: item.mime_type,
			path: item.path,
			kind: item.kind as Attachment['kind'],
			project_type: item.project_type,
			size: item.size
		}));
}
