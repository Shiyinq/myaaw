import type { LucideIcon } from '$lib/types/icons';
import {
	Accessibility,
	BookOpen,
	CloudFog,
	Contrast,
	Film,
	Grid3x3,
	Layers,
	Monitor,
	Moon,
	MoonStar,
	PawPrint,
	PenLine,
	Sparkles,
	Sun,
	SunMedium,
	Terminal,
	Type
} from 'lucide-svelte';
import { resetMode, setMode, setTheme } from 'mode-watcher';

export type ThemeMode = 'light' | 'dark' | 'system';

/**
 * A named theme is a `data-theme` value layered on top of a fixed mode. The
 * matching `:root[data-theme='…']` block in `app.css` restyles tokens (and, for
 * a few, the page background or the typeface), so adding a theme is usually a
 * registry entry here plus one CSS block — no component markup changes.
 * Translucent themes (glass, mist) share the named `frost-*` surface classes,
 * and only `terminal` needs a variant, because it re-reads the transcript.
 */
export type ThemeId =
	| 'siamese'
	| 'glass'
	| 'carbon'
	| 'terminal'
	| 'paper'
	| 'blueprint'
	| 'graphite'
	| 'slate'
	| 'typewriter'
	| 'kontras'
	| 'kontras-light'
	| 'noir'
	| 'redup'
	| 'mist';

/**
 * Three colours that stand in for a theme in the Settings preview: its page, a
 * sheet above it, and the accent. Kept here next to the theme it describes so a
 * new entry cannot be added without one.
 */
export interface ThemeSwatch {
	bg: string;
	card: string;
	accent: string;
}

/**
 * One selectable appearance, offered in Settings and the command palette. The
 * plain Light, Dark and System entries are modes with no custom theme; the rest
 * set `data-theme` and switch to the mode they are designed for.
 */
export interface AppearanceOption {
	id: string;
	labelKey: string;
	mode: ThemeMode;
	theme: '' | ThemeId;
	aliases: string[];
	icon: LucideIcon;
	swatch: ThemeSwatch;
}

export const APPEARANCE: AppearanceOption[] = [
	{
		id: 'siamese',
		labelKey: 'myaaw.settings.theme_siamese',
		mode: 'light',
		theme: 'siamese',
		aliases: ['siamese', 'siam', 'kucing siam', 'seal point', 'blue eyes', 'mata biru'],
		icon: PawPrint,
		swatch: {
			bg: 'oklch(0.96 0.013 82)',
			card: 'oklch(0.985 0.008 82)',
			accent: 'oklch(0.34 0.03 55)'
		}
	},
	{
		id: 'light',
		labelKey: 'myaaw.settings.theme_light',
		mode: 'light',
		theme: '',
		aliases: ['light', 'terang'],
		icon: Sun,
		swatch: { bg: 'oklch(1 0 0)', card: 'oklch(0.96 0 0)', accent: 'oklch(0.205 0 0)' }
	},
	{
		id: 'paper',
		labelKey: 'myaaw.settings.theme_paper',
		mode: 'light',
		theme: 'paper',
		aliases: ['paper', 'kertas', 'read', 'baca', 'sepia'],
		icon: BookOpen,
		swatch: {
			bg: 'oklch(0.975 0.008 85)',
			card: 'oklch(0.995 0.004 85)',
			accent: 'oklch(0.24 0.014 70)'
		}
	},
	{
		id: 'slate',
		labelKey: 'myaaw.settings.theme_slate',
		mode: 'light',
		theme: 'slate',
		aliases: ['slate', 'cool', 'dingin', 'grey', 'abu'],
		icon: Layers,
		swatch: {
			bg: 'oklch(0.965 0.006 250)',
			card: 'oklch(0.995 0.002 250)',
			accent: 'oklch(0.22 0.012 250)'
		}
	},
	{
		id: 'graphite',
		labelKey: 'myaaw.settings.theme_graphite',
		mode: 'light',
		theme: 'graphite',
		aliases: ['graphite', 'grafit', 'print', 'printout', 'cetak'],
		icon: PenLine,
		swatch: { bg: 'oklch(0.965 0 0)', card: 'oklch(1 0 0)', accent: 'oklch(0.17 0 0)' }
	},
	{
		id: 'typewriter',
		labelKey: 'myaaw.settings.theme_typewriter',
		mode: 'light',
		theme: 'typewriter',
		aliases: ['typewriter', 'mesin tik', 'mono', 'light terminal'],
		icon: Type,
		swatch: { bg: 'oklch(0.985 0 0)', card: 'oklch(1 0 0)', accent: 'oklch(0.16 0 0)' }
	},
	{
		id: 'kontras-light',
		labelKey: 'myaaw.settings.theme_kontras_light',
		mode: 'light',
		theme: 'kontras-light',
		aliases: ['contrast light', 'kontras terang', 'accessibility', 'aksesibilitas'],
		icon: SunMedium,
		swatch: { bg: 'oklch(1 0 0)', card: 'oklch(0.93 0 0)', accent: 'oklch(0 0 0)' }
	},
	{
		id: 'mist',
		labelKey: 'myaaw.settings.theme_mist',
		mode: 'light',
		theme: 'mist',
		aliases: ['mist', 'kabut', 'frosted light', 'glass light'],
		icon: CloudFog,
		swatch: {
			bg: 'oklch(0.94 0.006 250)',
			card: 'oklch(1 0 0)',
			accent: 'oklch(0.2 0.01 250)'
		}
	},
	{
		id: 'dark',
		labelKey: 'myaaw.settings.theme_dark',
		mode: 'dark',
		theme: '',
		aliases: ['dark', 'gelap'],
		icon: Moon,
		swatch: { bg: 'oklch(0.145 0 0)', card: 'oklch(0.21 0 0)', accent: 'oklch(0.985 0 0)' }
	},
	{
		id: 'carbon',
		labelKey: 'myaaw.settings.theme_carbon',
		mode: 'dark',
		theme: 'carbon',
		aliases: ['carbon', 'karbon', 'black', 'hitam', 'amoled', 'oled'],
		icon: Contrast,
		swatch: { bg: 'oklch(0 0 0)', card: 'oklch(0.09 0 0)', accent: 'oklch(0.985 0 0)' }
	},
	{
		id: 'noir',
		labelKey: 'myaaw.settings.theme_noir',
		mode: 'dark',
		theme: 'noir',
		aliases: ['noir', 'grain', 'film', 'texture', 'tekstur'],
		icon: Film,
		swatch: { bg: 'oklch(0.14 0 0)', card: 'oklch(0.18 0 0)', accent: 'oklch(0.97 0 0)' }
	},
	{
		id: 'redup',
		labelKey: 'myaaw.settings.theme_redup',
		mode: 'dark',
		theme: 'redup',
		aliases: ['dim', 'redup', 'night', 'malam', 'low contrast'],
		icon: MoonStar,
		swatch: { bg: 'oklch(0.19 0 0)', card: 'oklch(0.23 0 0)', accent: 'oklch(0.88 0 0)' }
	},
	{
		id: 'kontras',
		labelKey: 'myaaw.settings.theme_kontras',
		mode: 'dark',
		theme: 'kontras',
		aliases: ['contrast', 'kontras', 'accessibility', 'aksesibilitas', 'high contrast'],
		icon: Accessibility,
		swatch: { bg: 'oklch(0 0 0)', card: 'oklch(0.14 0 0)', accent: 'oklch(1 0 0)' }
	},
	{
		id: 'blueprint',
		labelKey: 'myaaw.settings.theme_blueprint',
		mode: 'dark',
		theme: 'blueprint',
		aliases: ['blueprint', 'cetak biru', 'grid', 'grid paper', 'blue'],
		icon: Grid3x3,
		swatch: {
			bg: 'oklch(0.16 0.008 255)',
			card: 'oklch(0.19 0.01 255)',
			accent: 'oklch(0.985 0 0)'
		}
	},
	{
		id: 'terminal',
		labelKey: 'myaaw.settings.theme_terminal',
		mode: 'dark',
		theme: 'terminal',
		aliases: ['terminal', 'mono', 'monospace', 'console'],
		icon: Terminal,
		swatch: { bg: 'oklch(0.145 0 0)', card: 'oklch(0.17 0 0)', accent: 'oklch(0.985 0 0)' }
	},
	{
		id: 'glass',
		labelKey: 'myaaw.settings.theme_glass',
		mode: 'dark',
		theme: 'glass',
		aliases: ['glass', 'kaca', 'frosted', 'blur'],
		icon: Sparkles,
		swatch: { bg: 'oklch(0.135 0 0)', card: 'oklch(0.19 0 0)', accent: 'oklch(0.985 0 0)' }
	},
	{
		id: 'system',
		labelKey: 'myaaw.settings.theme_system',
		mode: 'system',
		theme: '',
		aliases: ['system', 'sistem', 'auto'],
		icon: Monitor,
		swatch: {
			bg: 'linear-gradient(135deg, oklch(0.98 0 0) 50%, oklch(0.16 0 0) 50%)',
			card: 'oklch(0.55 0 0)',
			accent: 'oklch(0.5 0 0)'
		}
	}
];

/** The plain modes, shown apart from the named themes in Settings. */
export const MODES = APPEARANCE.filter((option) => option.theme === '');

/** The named themes. */
export const THEMES = APPEARANCE.filter((option) => option.theme !== '');

/**
 * The mode a named theme is designed for, or `undefined` for the plain modes.
 * Used to keep `data-theme` and the `dark` class from disagreeing.
 */
export function modeForTheme(theme: string | undefined): ThemeMode | undefined {
	return APPEARANCE.find((option) => option.theme !== '' && option.theme === theme)?.mode;
}

/** Whether an appearance option is the one currently in effect. */
export function isAppearanceActive(
	option: AppearanceOption,
	mode: ThemeMode,
	theme: string | undefined
): boolean {
	if (option.theme) return theme === option.theme;
	return theme === '' && mode === option.mode;
}

/**
 * Applies an appearance option. Plain modes clear `data-theme` first so a
 * surface that was translucent (glass) or restyled never lingers; named themes
 * set the theme and then the mode it is designed for.
 */
export function applyAppearance(option: AppearanceOption): void {
	setTheme(option.theme);
	if (option.mode === 'system') resetMode();
	else setMode(option.mode);
}

/** Whether the current custom theme paints frosted sheets (glass or mist). */
export function isFrosted(theme: string | undefined): boolean {
	return theme === 'glass' || theme === 'mist';
}
