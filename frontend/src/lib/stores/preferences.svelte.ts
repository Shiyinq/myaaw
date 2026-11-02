import { browser } from '$app/environment';

export type Density = 'comfortable' | 'compact';
export type SendKey = 'enter' | 'mod-enter';
export type StepDisplay = 'summary' | 'expanded';
/** Audio language: follow the interface, or force one of the shipped locales. */
export type AudioLanguage = 'app' | 'id' | 'en';
/** Read-aloud may also guess the language from the answer itself. */
export type SpeechLanguage = AudioLanguage | 'auto';

/**
 * Per-browser preferences that are not a theme: how dense the lists are, how
 * large the type is, and how the chat composer behaves. Themes live in
 * `lib/theme.ts`; this file only holds the knobs that are not colours.
 */
export interface Preferences {
	density: Density;
	textScale: number;
	reduceMotion: boolean;
	/** Chat-first shell: the sidebar becomes an agent picker plus the chat list. */
	simpleMode: boolean;
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
	notifySound: string;
	notifyVolume: number;
}

const STORAGE_KEY = 'myaaw_prefs';

const DEFAULTS: Preferences = {
	density: 'comfortable',
	textScale: 100,
	reduceMotion: false,
	simpleMode: false,
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
	notifySound: 'chime',
	notifyVolume: 0.7
};

function audioLanguage(value: unknown): AudioLanguage {
	return value === 'id' || value === 'en' ? value : 'app';
}

function speechLanguage(value: unknown): SpeechLanguage {
	return value === 'app' || value === 'id' || value === 'en' ? value : 'auto';
}

/** Reads the stored preferences, dropping unknown or malformed values. */
function load(): Preferences {
	if (!browser) return { ...DEFAULTS };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { ...DEFAULTS };
		const parsed = JSON.parse(raw) as Partial<Preferences>;
		return {
			density: parsed.density === 'compact' ? 'compact' : 'comfortable',
			textScale: Math.min(125, Math.max(100, Math.round(Number(parsed.textScale) || 100))),
			reduceMotion: parsed.reduceMotion === true,
			simpleMode: parsed.simpleMode === true,
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
			notifySound:
				typeof parsed.notifySound === 'string' && parsed.notifySound ? parsed.notifySound : 'chime',
			notifyVolume: Math.min(
				1,
				Math.max(0, Math.round((Number(parsed.notifyVolume) || 0.7) * 100) / 100)
			)
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
