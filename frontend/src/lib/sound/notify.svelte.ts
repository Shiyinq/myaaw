import { browser } from '$app/environment';

/**
 * Short "answer finished" tones synthesised with the Web Audio API, so there is
 * no audio asset to ship and no dependency to add. Each sound is a small set of
 * oscillators: a note and its timing, wave shape and gain.
 */

interface Note {
	freq: number;
	/** Seconds after the sound starts. */
	at: number;
	duration: number;
	type: OscillatorType;
	gain: number;
}

export interface NotificationSound {
	id: string;
	labelKey: string;
	notes: Note[];
}

const SOUNDS: NotificationSound[] = [
	{
		id: 'chime',
		labelKey: 'myaaw.settings.notify_sound_chime',
		notes: [
			{ freq: 659.25, at: 0, duration: 0.5, type: 'sine', gain: 0.5 },
			{ freq: 880, at: 0.12, duration: 0.7, type: 'sine', gain: 0.5 }
		]
	},
	{
		id: 'bell',
		labelKey: 'myaaw.settings.notify_sound_bell',
		notes: [
			{ freq: 1046.5, at: 0, duration: 0.8, type: 'sine', gain: 0.45 },
			{ freq: 2093, at: 0, duration: 0.5, type: 'sine', gain: 0.12 }
		]
	},
	{
		id: 'blip',
		labelKey: 'myaaw.settings.notify_sound_blip',
		notes: [
			{ freq: 660, at: 0, duration: 0.09, type: 'square', gain: 0.14 },
			{ freq: 990, at: 0.08, duration: 0.12, type: 'square', gain: 0.14 }
		]
	},
	{
		id: 'soft',
		labelKey: 'myaaw.settings.notify_sound_soft',
		notes: [
			{ freq: 523.25, at: 0, duration: 0.35, type: 'sine', gain: 0.34 },
			{ freq: 392, at: 0.18, duration: 0.5, type: 'sine', gain: 0.34 }
		]
	},
	{
		id: 'ping',
		labelKey: 'myaaw.settings.notify_sound_ping',
		notes: [{ freq: 880, at: 0, duration: 0.45, type: 'sine', gain: 0.5 }]
	},
	{
		id: 'triple',
		labelKey: 'myaaw.settings.notify_sound_triple',
		notes: [
			{ freq: 523.25, at: 0, duration: 0.5, type: 'sine', gain: 0.42 },
			{ freq: 659.25, at: 0.12, duration: 0.5, type: 'sine', gain: 0.42 },
			{ freq: 783.99, at: 0.24, duration: 0.6, type: 'sine', gain: 0.42 }
		]
	},
	{
		id: 'doorbell',
		labelKey: 'myaaw.settings.notify_sound_doorbell',
		notes: [
			{ freq: 440, at: 0, duration: 0.55, type: 'sine', gain: 0.45 },
			{ freq: 330, at: 0.24, duration: 0.7, type: 'sine', gain: 0.45 }
		]
	},
	{
		id: 'marimba',
		labelKey: 'myaaw.settings.notify_sound_marimba',
		notes: [
			{ freq: 740, at: 0, duration: 0.18, type: 'sine', gain: 0.4 },
			{ freq: 987.77, at: 0.14, duration: 0.18, type: 'sine', gain: 0.4 }
		]
	}
];

/** Every sound available in Settings. */
export const notificationSounds = SOUNDS;

let context: AudioContext | null = null;

function ensureContext(): AudioContext | null {
	if (!browser) return null;
	const Constructor =
		window.AudioContext ??
		(window as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
	if (!Constructor) return null;
	if (!context) context = new Constructor();
	if (context.state === 'suspended') void context.resume();
	return context;
}

/**
 * Plays one of the notification sounds; unknown ids fall back to the chime.
 * `volume` is 0–1. A limiter sits before the output so the louder notes stay
 * clean even when two overlap, instead of clipping.
 */
export function playNotification(id: string, volume = 1) {
	const sound = SOUNDS.find((candidate) => candidate.id === id) ?? SOUNDS[0];
	const level = Math.max(0, Math.min(1, volume));
	if (level < 0.001) return;

	const audio = ensureContext();
	if (!audio) return;

	const master = audio.createGain();
	master.gain.value = level;

	const limiter = audio.createDynamicsCompressor();
	limiter.threshold.value = -3;
	limiter.knee.value = 0;
	limiter.ratio.value = 20;
	limiter.attack.value = 0.001;
	limiter.release.value = 0.1;

	master.connect(limiter);
	limiter.connect(audio.destination);

	const start = audio.currentTime + 0.01;
	for (const note of sound.notes) {
		const oscillator = audio.createOscillator();
		const gain = audio.createGain();
		const at = start + note.at;

		oscillator.type = note.type;
		oscillator.frequency.value = note.freq;

		gain.gain.setValueAtTime(0.0001, at);
		gain.gain.exponentialRampToValueAtTime(note.gain, at + 0.01);
		gain.gain.exponentialRampToValueAtTime(0.0001, at + note.duration);

		oscillator.connect(gain);
		gain.connect(master);
		oscillator.start(at);
		oscillator.stop(at + note.duration + 0.05);
	}
}
