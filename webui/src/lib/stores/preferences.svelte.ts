import { browser } from '$app/environment';

export type Density = 'comfortable' | 'compact';
export type SendKey = 'enter' | 'mod-enter';
export type StepDisplay = 'summary' | 'expanded';
/** Audio language: follow the interface, or force one of the shipped locales. */
export type AudioLanguage = 'app' | 'id' | 'en';
/** Read-aloud may also guess the language from the answer itself. */
export type SpeechLanguage = AudioLanguage | 'auto';

/**
 * Which shell the app renders: the full navigation meant for people who want
 * every page, the chat-first layout of the first LLM apps, or the minimal focus
 * layout meant for a single agent.
 */
export type ShellMode = 'advanced' | 'classic' | 'focus';

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
	/** Show the agent mascot while an answer is working. */
	showRunMascot: boolean;
}

const STORAGE_KEY = 'myaaw_prefs';

const DEFAULTS: Preferences = {
	density: 'comfortable',
	textScale: 100,
	reduceMotion: false,
	shellMode: 'advanced',
	focusAgent: '',
	sendKey: 'enter',
	autoScroll: true,
	steps: 'summary',
	keepDrafts: true,
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
	if (value === 'advanced' || value === 'classic' || value === 'focus') return value;
	// This shell was called "standard" before "advanced", and the classic shell
	// was called "simple" before that.
	if (value === 'standard') return 'advanced';
	if (value === 'simple' || parsed.simpleMode === true) return 'classic';
	return 'advanced';
}

/** Reads the stored preferences, dropping unknown or malformed values. */
function load(): Preferences {
	if (!browser) return { ...DEFAULTS };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { ...DEFAULTS };
		const parsed = JSON.parse(raw) as StoredPreferences;
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
				Math.max(0, Math.round((Number(parsed.notifyVolume) || 0.7) * 100) / 100)
			),
			showHostBadge: parsed.showHostBadge !== false,
			showRunMascot: parsed.showRunMascot !== false
		};
	} catch {
		return { ...DEFAULTS };
	}
}

let current = $state<Preferences>(load());

export const preferences = {
	get value() {
		return current;
	},
	/** Merges a patch in, persists it, and lets the applying effect pick it up. */
	update(patch: Partial<Preferences>) {
		current = { ...current, ...patch };
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
