import { browser } from '$app/environment';
import { queueSettingsPatch } from '$lib/stores/settingsSync.svelte';
import type { ControlPlaneSettings, ControlPlaneSettingsPatch } from '$lib/types';

export type Density = 'comfortable' | 'compact';
export type SendKey = 'enter' | 'mod-enter';
export type StepDisplay = 'summary' | 'expanded';
/** Audio language: follow the interface, or force one of the shipped locales. */
export type AudioLanguage = 'app' | 'id' | 'en';
/** Read-aloud may also guess the language from the answer itself. */
export type SpeechLanguage = AudioLanguage | 'auto';

/**
 * Which shell the app renders: the full navigation meant for people who want
 * every page, the chat-first layout of the first LLM apps, the minimal focus
 * layout meant for a single agent, or the bots layout listing every agent as a
 * direct chat contact.
 */
export type ShellMode = 'studio' | 'classic' | 'focus' | 'bots';

/**
 * Per-browser preferences that are not a theme: how dense the lists are, how
 * large the type is, and how the chat composer behaves. Themes live in
 * `lib/theme.ts`; this file only holds the knobs that are not colours.
 */
export interface Preferences {
	density: Density;
	textScale: number;
	reduceMotion: boolean;
	/** Classic shell: the sidebar becomes an agent picker plus the chat list. */
	shellMode: ShellMode;
	/** Agent used by focus mode; empty falls back to the default agent. */
	focusAgent: string;
	sendKey: SendKey;
	autoScroll: boolean;
	steps: StepDisplay;
	keepDrafts: boolean;
	/** While a run streams, hold typed messages in a queue above the composer. */
	queueMessages: boolean;
	/** While a run streams, sending interrupts the run and sends immediately. */
	interruptOnSend: boolean;
	speakResponses: boolean;
	speechRate: number;
	speechLang: SpeechLanguage;
	speechVoice: string;
	dictationLang: AudioLanguage;
	notifyEnabled: boolean;
	notifyBrowser: boolean;
	notifySound: string;
	notifyVolume: number;
	/** Show which control-plane host the header is talking to. */
	showHostBadge: boolean;
	showConnectionBadge: boolean;
	/** Show the agent mascot while an answer is working. */
	showRunMascot: boolean;
}

const STORAGE_KEY = 'myaaw_prefs';

const DEFAULTS: Preferences = {
	density: 'comfortable',
	textScale: 100,
	reduceMotion: false,
	shellMode: 'studio',
	focusAgent: '',
	sendKey: 'enter',
	autoScroll: true,
	steps: 'summary',
	keepDrafts: true,
	queueMessages: false,
	interruptOnSend: true,
	speakResponses: false,
	speechRate: 1,
	speechLang: 'auto',
	speechVoice: '',
	dictationLang: 'app',
	notifyEnabled: true,
	notifyBrowser: false,
	notifySound: 'chime',
	notifyVolume: 0.7,
	showHostBadge: true,
	showConnectionBadge: false,
	showRunMascot: true
};

function audioLanguage(value: unknown): AudioLanguage {
	return value === 'id' || value === 'en' ? value : 'app';
}

function speechLanguage(value: unknown): SpeechLanguage {
	return value === 'app' || value === 'id' || value === 'en' ? value : 'auto';
}

/**
 * What the browser stored. Shell mode is loose on purpose: older builds wrote
 * `standard` for this shell and `simple` for the classic one, and the oldest
 * builds stored a boolean instead.
 */
interface StoredPreferences extends Partial<Omit<Preferences, 'shellMode'>> {
	shellMode?: string;
	simpleMode?: unknown;
}

/** Reads the shell mode, migrating the names of the shells that came before. */
function shellMode(parsed: StoredPreferences): ShellMode {
	const value = parsed.shellMode;
	if (value === 'studio' || value === 'classic' || value === 'focus' || value === 'bots') {
		return value;
	}
	// This shell was called "advanced" and "standard" before "studio", and the
	// classic shell was called "simple" before that.
	if (value === 'advanced' || value === 'standard') return 'studio';
	if (value === 'simple' || parsed.simpleMode === true) return 'classic';
	return 'studio';
}

/** Normalizes a partial/stored preferences object against defaults. */
function normalize(parsed: StoredPreferences): Preferences {
	// The two send-while-streaming modes are mutually exclusive; the queue
	// wins when a hand-edited store turns both on at once.
	const queueMessages = parsed.queueMessages === true;
	const interruptOnSend = parsed.interruptOnSend !== false && !queueMessages;
	return {
		density: parsed.density === 'compact' ? 'compact' : 'comfortable',
		textScale: Math.min(125, Math.max(100, Math.round(Number(parsed.textScale) || 100))),
		reduceMotion: parsed.reduceMotion === true,
		shellMode: shellMode(parsed),
		focusAgent: typeof parsed.focusAgent === 'string' ? parsed.focusAgent : '',
		sendKey: parsed.sendKey === 'mod-enter' ? 'mod-enter' : 'enter',
		autoScroll: parsed.autoScroll !== false,
		steps: parsed.steps === 'expanded' ? 'expanded' : 'summary',
		keepDrafts: parsed.keepDrafts !== false,
		queueMessages,
		interruptOnSend,
		speakResponses: parsed.speakResponses === true,
		speechRate: Math.min(
			1.5,
			Math.max(0.75, Math.round((Number(parsed.speechRate) || 1) * 100) / 100)
		),
		speechLang: speechLanguage(parsed.speechLang),
		speechVoice: typeof parsed.speechVoice === 'string' ? parsed.speechVoice : '',
		dictationLang: audioLanguage(parsed.dictationLang),
		notifyEnabled: parsed.notifyEnabled !== false,
		notifyBrowser: parsed.notifyBrowser === true,
		notifySound:
			typeof parsed.notifySound === 'string' && parsed.notifySound ? parsed.notifySound : 'chime',
		notifyVolume: Math.min(
			1,
			Math.max(
				0,
				Math.round(
					(parsed.notifyVolume === undefined || !Number.isFinite(Number(parsed.notifyVolume))
						? 0.7
						: Number(parsed.notifyVolume)) * 100
				) / 100
			)
		),
		showHostBadge: parsed.showHostBadge !== false,
		showConnectionBadge: parsed.showConnectionBadge === true,
		showRunMascot: parsed.showRunMascot !== false
	};
}

/** Reads the stored preferences, dropping unknown or malformed values. */
function load(): Preferences {
	if (!browser) return { ...DEFAULTS };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { ...DEFAULTS };
		return normalize(JSON.parse(raw) as StoredPreferences);
	} catch {
		return { ...DEFAULTS };
	}
}

function toSettingsPatch(patch: Partial<Preferences>): ControlPlaneSettingsPatch {
	const out: ControlPlaneSettingsPatch = {};
	if (patch.density !== undefined) out.density = patch.density;
	if (patch.textScale !== undefined) out.text_scale = patch.textScale;
	if (patch.reduceMotion !== undefined) out.reduce_motion = patch.reduceMotion;
	if (patch.shellMode !== undefined) out.shell_mode = patch.shellMode;
	if (patch.focusAgent !== undefined) out.focus_agent = patch.focusAgent;
	if (patch.sendKey !== undefined) out.send_key = patch.sendKey;
	if (patch.autoScroll !== undefined) out.auto_scroll = patch.autoScroll;
	if (patch.steps !== undefined) out.steps = patch.steps;
	if (patch.keepDrafts !== undefined) out.keep_drafts = patch.keepDrafts;
	if (patch.queueMessages !== undefined) out.queue_messages = patch.queueMessages;
	if (patch.interruptOnSend !== undefined) out.interrupt_on_send = patch.interruptOnSend;
	if (patch.speakResponses !== undefined) out.speak_responses = patch.speakResponses;
	if (patch.speechRate !== undefined) out.speech_rate = patch.speechRate;
	if (patch.speechLang !== undefined) out.speech_lang = patch.speechLang;
	if (patch.speechVoice !== undefined) out.speech_voice = patch.speechVoice;
	if (patch.dictationLang !== undefined) out.dictation_lang = patch.dictationLang;
	if (patch.notifyEnabled !== undefined) out.notify_enabled = patch.notifyEnabled;
	if (patch.notifyBrowser !== undefined) out.notify_browser = patch.notifyBrowser;
	if (patch.notifySound !== undefined) out.notify_sound = patch.notifySound;
	if (patch.notifyVolume !== undefined) out.notify_volume = patch.notifyVolume;
	if (patch.showConnectionBadge !== undefined)
		out.show_connection_badge = patch.showConnectionBadge;
	if (patch.showHostBadge !== undefined) out.show_host_badge = patch.showHostBadge;
	if (patch.showRunMascot !== undefined) out.show_run_mascot = patch.showRunMascot;
	return out;
}

function fromControlPlaneSettings(remote: ControlPlaneSettings): Preferences {
	return normalize({
		density: remote.density as Density,
		textScale: remote.text_scale,
		reduceMotion: remote.reduce_motion,
		shellMode: remote.shell_mode,
		focusAgent: remote.focus_agent,
		sendKey: remote.send_key as SendKey,
		autoScroll: remote.auto_scroll,
		steps: remote.steps as StepDisplay,
		keepDrafts: remote.keep_drafts,
		queueMessages: remote.queue_messages,
		interruptOnSend: remote.interrupt_on_send,
		speakResponses: remote.speak_responses,
		speechRate: remote.speech_rate,
		speechLang: remote.speech_lang as SpeechLanguage,
		speechVoice: remote.speech_voice,
		dictationLang: remote.dictation_lang as AudioLanguage,
		notifyEnabled: remote.notify_enabled,
		notifyBrowser: remote.notify_browser,
		notifySound: remote.notify_sound,
		notifyVolume: remote.notify_volume,
		showHostBadge: remote.show_host_badge,
		showConnectionBadge: remote.show_connection_badge,
		showRunMascot: remote.show_run_mascot
	});
}

let current = $state<Preferences>(load());
let connectionBadgePreview = $state(0);

export const preferences = {
	get value() {
		return current;
	},
	get connectionBadgePreview() {
		return connectionBadgePreview;
	},
	/** Merges a patch in, persists it locally, and syncs it to the control plane. */
	update(patch: Partial<Preferences>) {
		if (
			patch.showConnectionBadge !== undefined &&
			patch.showConnectionBadge !== current.showConnectionBadge
		) {
			connectionBadgePreview++;
		}
		current = { ...current, ...patch };
		if (browser) localStorage.setItem(STORAGE_KEY, JSON.stringify(current));
		queueSettingsPatch(toSettingsPatch(patch));
	},
	/** Applies preferences fetched from controlplane.db without echoing back to the server. */
	hydrate(remote: ControlPlaneSettings) {
		current = fromControlPlaneSettings(remote);
		if (browser) localStorage.setItem(STORAGE_KEY, JSON.stringify(current));
	}
};

/**
 * Pushes the preferences that live on `<html>` into the DOM. Font size is a
 * percentage of the browser's own default, so a larger system font still works.
 */
export function applyPreferences(p: Preferences) {
	if (!browser) return;
	const root = document.documentElement;
	root.style.fontSize = p.textScale === 100 ? '' : `${p.textScale}%`;
	root.dataset.density = p.density;
	root.dataset.reduceMotion = p.reduceMotion ? 'true' : '';
}
