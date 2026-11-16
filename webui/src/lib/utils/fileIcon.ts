import { File, FileCode2, FileText, Image as ImageIcon, Music, Table2, Video } from 'lucide-svelte';
import { detectPreview } from './filePreview';
import type { LucideIcon } from '$lib/types/icons';

/** A lucide glyph for a file path, chosen from the preview kind. */
export function fileIcon(path: string): LucideIcon {
	switch (detectPreview(path || 'file.md').kind) {
		case 'markdown':
			return FileText;
		case 'code':
		case 'html':
			return FileCode2;
		case 'csv':
			return Table2;
		case 'image':
			return ImageIcon;
		case 'video':
			return Video;
		case 'audio':
			return Music;
		case 'binary':
			return File;
		default:
			return FileText;
	}
}
