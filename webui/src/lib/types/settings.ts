export type BotsSidebarItemKind = 'agent' | 'group';

export interface BotsSidebarItemRef {
	kind: BotsSidebarItemKind;
	id: string;
}

export interface BotsSidebarSection {
	id: string;
	name: string;
	items: BotsSidebarItemRef[];
}

export interface BotsSidebarConfig {
	section_order: string[];
	sections: BotsSidebarSection[];
}

/**
 * Dashboard settings persisted in the `settings` table of ~/.myaaw/controlplane/controlplane.db
 * so preferences stay in sync across browsers.
 */
export interface ControlPlaneSettings {
	locale: string;
	theme_mode: string;
	theme: string;
	density: string;
	text_scale: number;
	reduce_motion: boolean;
	shell_mode: string;
	focus_agent: string;
	send_key: string;
	auto_scroll: boolean;
	steps: string;
	keep_drafts: boolean;
	queue_messages: boolean;
	interrupt_on_send: boolean;
	speak_responses: boolean;
	speech_rate: number;
	speech_lang: string;
	speech_voice: string;
	dictation_lang: string;
	notify_enabled: boolean;
	notify_browser: boolean;
	notify_sound: string;
	notify_volume: number;
	show_host_badge: boolean;
	show_connection_badge: boolean;
	show_run_mascot: boolean;
	bots_sidebar?: BotsSidebarConfig;
}

export type ControlPlaneSettingsPatch = Partial<ControlPlaneSettings>;
