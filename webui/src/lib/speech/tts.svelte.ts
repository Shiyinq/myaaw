import { browser } from '$app/environment';
import { locale, t } from '$lib/i18n';
import { preferences } from '$lib/stores/preferences.svelte';
import { toastStore } from '$lib/stores';
import { chunkForSpeech, detectSpeechLang, speakableText } from './text';

/**
 * Read-aloud built on the browser's own synthesiser: no library and no audio
 * leaving this tab for anything except a cloud voice the user picked.
 *
 * An answer is spoken chunk by chunk because Chrome cuts an utterance after
 * roughly fifteen seconds. Every playback gets a generation number, so a stale
 * `onend` from a cancelled utterance can never start the next answer.
 */

type Phase = 'idle' | 'starting' | 'speaking';

function engine(): SpeechSynthesis | null {
	if (!browser || !('speechSynthesis' in window)) return null;
	return window.speechSynthesis;
}

function supported(): boolean {
	return browser && 'speechSynthesis' in window && 'SpeechSynthesisUtterance' in window;
}

let voices = $state<SpeechSynthesisVoice[]>([]);
let phase = $state<Phase>('idle');
let activeId = $state('');
let generation = 0;
let chunks: string[] = [];
let index = 0;
let voicesBound = false;
// Languages already reported as missing a voice, so the notice appears once.
const reportedMissing = new Set<string>();
// Languages where the user picked a local system voice that Chrome is known
// to mispronounce, so the hint appears once instead of on every answer.
const reportedLocal = new Set<string>();
// Chrome can collect an utterance that nothing references, and then `onend`
// never fires; holding the current one keeps the queue moving.
let current: SpeechSynthesisUtterance | null = null;

/** Loads the voice list; it arrives asynchronously on most browsers. */
function refreshVoices() {
	const speech = engine();
	if (!speech) return;
	voices = speech.getVoices();
	if (voicesBound) return;
	voicesBound = true;
	speech.addEventListener('voiceschanged', () => {
		const next = engine();
		if (next) voices = next.getVoices();
	});
}

if (browser) refreshVoices();

function reset() {
	generation += 1;
	phase = 'idle';
	activeId = '';
	chunks = [];
	index = 0;
	if (current) {
		current.onend = null;
		current.onerror = null;
		current = null;
	}
}

function stop() {
	if (phase === 'idle' && !activeId) return;
	try {
		engine()?.cancel();
	} catch {
		// Nothing was queued.
	}
	reset();
}

/** The interface locale and the audio language are deliberately separate. */
function languageFor(text: string): string {
	const choice = preferences.value.speechLang;
	const base =
		choice === 'app'
			? locale.value
			: choice === 'auto'
				? (detectSpeechLang(text) ?? locale.value)
				: choice;
	return base === 'id' ? 'id-ID' : 'en-US';
}

function uiLanguage(): string {
	return typeof navigator !== 'undefined' ? navigator.language : 'en';
}

function remoteVoiceFor(lang: string): SpeechSynthesisVoice | undefined {
	const primary = lang.slice(0, 2).toLowerCase();
	const list = voices.length > 0 ? voices : (engine()?.getVoices() ?? []);
	return list.find((voice) => voice.lang.toLowerCase().startsWith(primary) && !voice.localService);
}

function voiceFor(lang: string): SpeechSynthesisVoice | null {
	if (voices.length === 0) refreshVoices();
	const list = voices.length > 0 ? voices : (engine()?.getVoices() ?? []);
	const saved = preferences.value.speechVoice;
	const savedVoice = saved ? list.find((voice) => voice.voiceURI === saved) : undefined;
	const primary = lang.slice(0, 2).toLowerCase();

	// An explicit choice always wins when it can speak the requested language.
	if (savedVoice && savedVoice.lang.toLowerCase().startsWith(primary)) return savedVoice;

	const candidates = list.filter((voice) => voice.lang.toLowerCase().startsWith(primary));
	if (candidates.length === 0) return savedVoice ?? null;

	// Chrome on macOS tends to read a foreign-language *system* voice with its
	// default voice (Damayanti → English), while its own cloud voices are
	// reliable. Prefer the cloud voice for a language other than the interface's,
	// so auto-speak picks one that actually speaks the language.
	const uiPrimary = uiLanguage().slice(0, 2).toLowerCase();
	if (primary !== uiPrimary) {
		candidates.sort((a, b) => Number(a.localService) - Number(b.localService));
	}

	return candidates[0];
}

/** Speaks an answer, or stops it when the same row is already playing. */
function speak(id: string, markdown: string) {
	const speech = engine();
	if (!speech) {
		toastStore.error(t('myaaw.chat.speak_unsupported'));
		return;
	}
	if (activeId === id && phase !== 'idle') {
		stop();
		return;
	}

	const text = speakableText(markdown);
	const parts = chunkForSpeech(text);
	if (parts.length === 0) return;

	// Chrome ignores a new utterance queued in the same task as cancel().
	try {
		speech.cancel();
	} catch {
		// Nothing was queued.
	}

	generation += 1;
	const run = generation;
	chunks = parts;
	index = 0;
	phase = 'starting';
	activeId = id;

	const lang = languageFor(text);
	const voice = voiceFor(lang);
	const rate = preferences.value.speechRate;

	// No installed voice for this language: the utterance still carries the tag,
	// but the browser reads it with the default (usually English) voice. The
	// check waits a moment because some browsers fill the voice list late.
	if (!voice) {
		setTimeout(() => {
			if (voiceFor(lang) || reportedMissing.has(lang)) return;
			reportedMissing.add(lang);
			const language = lang.startsWith('id')
				? t('myaaw.settings.speech_language_id')
				: t('myaaw.settings.speech_language_en');
			toastStore.add(t('myaaw.chat.speak_no_voice', { language }), 'info', 6000);
		}, 800);
	}

	// A user may have picked a local macOS voice for a foreign language; Chrome
	// often reads those with the default voice. Point at the cloud voice once.
	if (
		voice &&
		voice.localService &&
		lang.slice(0, 2).toLowerCase() !== uiLanguage().slice(0, 2).toLowerCase() &&
		remoteVoiceFor(lang) &&
		!reportedLocal.has(lang)
	) {
		reportedLocal.add(lang);
		toastStore.add(t('myaaw.chat.speak_local_voice'), 'info', 7000);
	}

	const speakNext = () => {
		if (run !== generation) return;
		const chunk = chunks[index];
		if (!chunk) {
			reset();
			return;
		}

		const utterance = new SpeechSynthesisUtterance(chunk);
		utterance.lang = lang;
		if (voice) utterance.voice = voice;
		utterance.rate = rate;
		utterance.onstart = () => {
			if (run === generation) phase = 'speaking';
		};
		utterance.onend = () => {
			if (run !== generation) return;
			index += 1;
			speakNext();
		};
		utterance.onerror = (event) => {
			if (run !== generation) return;
			if (event.error === 'interrupted' || event.error === 'canceled') {
				reset();
				return;
			}
			toastStore.error(t('myaaw.chat.speak_error'));
			stop();
		};

		current = utterance;
		setTimeout(() => {
			if (run !== generation) return;
			try {
				speech.speak(utterance);
			} catch {
				stop();
			}
		}, 0);
	};

	speakNext();

	// Safari starts audio only inside a user gesture and fails silently, which
	// leaves the row looking stuck. Offer the tap that makes it work instead.
	setTimeout(() => {
		if (run !== generation || phase !== 'starting') return;
		if (speech.speaking || speech.pending) return;
		reset();
		toastStore.add(t('myaaw.chat.speak_blocked'), 'info', 5000);
	}, 1500);
}

export const tts = {
	get supported() {
		return supported();
	},
	get voices() {
		return voices;
	},
	get phase() {
		return phase;
	},
	get activeId() {
		return activeId;
	},
	speak,
	toggle: speak,
	stop,
	refreshVoices
};
