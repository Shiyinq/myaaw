export type ExtraBlockType =
	| 'media'
	| 'carousel'
	| 'buttons'
	| 'quick_replies'
	| 'form'
	| 'checklist'
	| 'callout'
	| 'summary'
	| 'link_preview';

export type InteractionKind = 'button' | 'quick_reply' | 'form' | 'checklist';

export interface InteractionState {
	status: 'pending' | 'submitted' | string;
	answered_at?: string;
	actor?: string;
	summary?: string;
	payload?: Record<string, unknown>;
}

export interface ExtraMediaItem {
	url?: string;
	path?: string;
	kind?: 'image' | 'video' | 'audio' | 'pdf' | 'file' | string;
	title?: string;
	caption?: string;
	file_name?: string;
	mime_type?: string;
	size?: number;
}

export interface ExtraMediaBlockData {
	layout?: 'grid' | 'carousel' | 'list' | string;
	caption?: string;
	items: ExtraMediaItem[];
}

export interface ExtraButton {
	id: string;
	label: string;
	type: 'action' | 'link' | string;
	value?: string;
	url?: string;
	style?: 'primary' | 'secondary' | 'danger' | string;
	confirm?: string;
}

export interface ExtraButtonsBlockData {
	prompt?: string;
	layout?: 'row' | 'stack' | string;
	buttons: ExtraButton[];
}

export interface ExtraQuickReplyOption {
	id: string;
	label: string;
	value?: string;
}

export interface ExtraQuickRepliesBlockData {
	prompt?: string;
	options: ExtraQuickReplyOption[];
}

export interface ExtraCarouselCard {
	id: string;
	title: string;
	subtitle?: string;
	description?: string;
	image_url?: string;
	image_path?: string;
	badges?: string[];
	buttons?: ExtraButton[];
}

export interface ExtraCarouselBlockData {
	cards: ExtraCarouselCard[];
}

export interface ExtraFormOption {
	label: string;
	value: string;
}

export type ExtraFormFieldType =
	| 'text'
	| 'textarea'
	| 'number'
	| 'select'
	| 'multiselect'
	| 'checkbox'
	| 'switch'
	| 'date'
	| 'time'
	| 'radio';

export interface ExtraFormField {
	name: string;
	label: string;
	type: ExtraFormFieldType | string;
	required?: boolean;
	placeholder?: string;
	default?: unknown;
	help?: string;
	min?: number;
	max?: number;
	options?: ExtraFormOption[];
}

export interface ExtraFormBlockData {
	id: string;
	title: string;
	description?: string;
	submit_label?: string;
	fields: ExtraFormField[];
}

export interface ExtraChecklistItem {
	id: string;
	label: string;
	description?: string;
	checked?: boolean;
}

export interface ExtraChecklistBlockData {
	title?: string;
	description?: string;
	submit_label?: string;
	items: ExtraChecklistItem[];
}

export interface ExtraCalloutBlockData {
	variant?: 'info' | 'success' | 'warning' | 'error' | string;
	title?: string;
	text: string;
}

export interface ExtraSummaryItem {
	label: string;
	value: string;
	tone?: 'default' | 'positive' | 'warning' | 'danger' | string;
}

export interface ExtraSummaryBlockData {
	title?: string;
	subtitle?: string;
	items: ExtraSummaryItem[];
}

export interface ExtraLinkPreviewBlockData {
	url: string;
	title: string;
	description?: string;
	domain?: string;
	image_url?: string;
}

export interface ExtraBlock {
	id: string;
	type: ExtraBlockType | string;
	interactive?: boolean;
	single_use?: boolean;
	data: unknown;
	state?: InteractionState;
}

export interface ExtraMessage {
	id: string;
	blocks: ExtraBlock[];
	created_at?: string;
}

export interface InteractionRequest {
	message_id?: string;
	block_id: string;
	kind?: InteractionKind;
	action_id?: string;
	card_id?: string;
	label?: string;
	value?: string;
	values?: Record<string, unknown>;
	display_text?: string;
}

export interface InteractionResponse {
	ok: boolean;
	block_id: string;
	state: InteractionState;
	interaction_text: string;
}
